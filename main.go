package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fastgen/internal/compile"
	"fastgen/internal/runner"
	"fastgen/internal/store"
	"fastgen/internal/tui"
)

type options struct {
	solution  string
	generator string
	brute     string
	checker   string

	tests     int
	seed      int64
	timeout   time.Duration
	exact     bool
	keepGoing bool
	jobs      int
	maxOutput int
	saveDir   string
	plain     bool

	cxx      string
	cxxflags string
	sanitize bool
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fastgen:", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if opts.plain {
		os.Exit(runPlain(ctx, opts))
	}

	compileCfg := compileConfig(opts)
	m := tui.New(ctx, tui.Options{
		Solution:  opts.solution,
		Generator: opts.generator,
		Brute:     opts.brute,
		Checker:   opts.checker,
		Tests:     opts.tests,
		Seed:      opts.seed,
		Timeout:   opts.timeout,
		Exact:     opts.exact,
		KeepGoing: opts.keepGoing,
		Jobs:      opts.jobs,
		MaxOutput: opts.maxOutput,
		SaveDir:   opts.saveDir,
		Compile:   compileCfg,
	})

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "fastgen:", err)
		os.Exit(1)
	}
}

func parseFlags() (options, error) {
	var opts options
	defaults := compile.DefaultConfig()

	flag.StringVar(&opts.solution, "sol", "", "solution source file (required)")
	flag.StringVar(&opts.generator, "gen", "", "test generator source file (required)")
	flag.StringVar(&opts.brute, "brute", "", "brute force / correct solution source file (required)")
	flag.StringVar(&opts.checker, "checker", "", "custom checker binary: checker <input> <got> <expected>, exit 0 = OK")
	flag.IntVar(&opts.tests, "n", 1000, "number of test cases")
	flag.Int64Var(&opts.seed, "seed", 1, "first seed passed to the generator as argv[1]")
	flag.DurationVar(&opts.timeout, "timeout", 2*time.Second, "per-process time limit")
	flag.BoolVar(&opts.exact, "exact", false, "compare output byte-for-byte instead of by whitespace tokens")
	flag.BoolVar(&opts.keepGoing, "keep-going", false, "collect all failures instead of stopping at the first")
	flag.IntVar(&opts.jobs, "jobs", 1, "number of test cases to run in parallel")
	flag.IntVar(&opts.maxOutput, "max-output", 4<<20, "maximum captured bytes per stream")
	flag.StringVar(&opts.saveDir, "save-dir", "", "save failing cases to this directory")
	flag.BoolVar(&opts.plain, "plain", false, "no TUI; print one line per test (good for CI)")

	flag.StringVar(&opts.cxx, "cxx", defaults.CXX, "C++ compiler")
	flag.StringVar(&opts.cxxflags, "cxxflags", strings.Join(defaults.Flags, " "), "compiler flags (space separated)")
	flag.BoolVar(&opts.sanitize, "sanitize", false, "build the solution with -fsanitize=address,undefined")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `fastgen - stress-test a C++ solution against a brute force

Usage:
  fastgen --sol a.cpp --gen gen.cpp --brute brute.cpp -n 1000

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), `
The generator receives the seed as its first argument. Solutions read the
input on stdin and are compared to the brute force output.

Examples:
  fastgen --sol sol.cpp --gen gen.cpp --brute brute.cpp -n 5000 --jobs 4
  fastgen --sol sol.cpp --gen gen.cpp --brute brute.cpp --plain --save-dir fails
  fastgen --sol sol.cpp --gen gen.cpp --brute brute.cpp --keep-going --exact
`)
	}

	flag.Parse()

	if flag.NArg() > 0 {
		return opts, fmt.Errorf("unexpected arguments: %s", strings.Join(flag.Args(), " "))
	}
	if opts.solution == "" || opts.generator == "" || opts.brute == "" {
		flag.Usage()
		return opts, errors.New("--sol, --gen, and --brute are required")
	}
	if opts.tests < 1 {
		return opts, errors.New("-n must be at least 1")
	}
	if opts.timeout <= 0 {
		return opts, errors.New("--timeout must be positive")
	}
	if opts.jobs < 1 {
		return opts, errors.New("--jobs must be at least 1")
	}
	if opts.maxOutput < 1024 {
		opts.maxOutput = 1024
	}
	return opts, nil
}

func compileConfig(opts options) compile.Config {
	cfg := compile.DefaultConfig()
	cfg.CXX = opts.cxx
	cfg.Flags = strings.Fields(opts.cxxflags)
	cfg.Sanitize = opts.sanitize
	return cfg
}

func runnerConfig(opts options, bins compile.Binaries) runner.Config {
	return runner.Config{
		Generator: bins.Generator,
		Solution:  bins.Solution,
		Brute:     bins.Brute,
		Checker:   opts.checker,
		StartSeed: opts.seed,
		Tests:     opts.tests,
		Timeout:   opts.timeout,
		Exact:     opts.exact,
		KeepGoing: opts.keepGoing,
		Jobs:      opts.jobs,
		MaxOutput: opts.maxOutput,
	}
}

func runPlain(ctx context.Context, opts options) int {
	bins, err := compile.Build(ctx, compileConfig(opts), compile.Targets{
		Solution:  opts.solution,
		Generator: opts.generator,
		Brute:     opts.brute,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "fastgen:", err)
		return 1
	}

	events := make(chan runner.Event, 1024)
	go runner.Run(ctx, runnerConfig(opts, bins), events)

	tty := isTTY(os.Stdout)
	firstShown := false
	failures := 0
	var fin runner.Finished

	for ev := range events {
		switch e := ev.(type) {
		case runner.TestDone:
			r := e.Result
			line := plainLine(r, opts.tests)
			if tty {
				fmt.Print("\r\x1b[2K" + line)
			} else {
				fmt.Println(line)
			}
			if r.Failed() {
				failures++
				if tty {
					fmt.Print("\r\x1b[2K")
				}
				var saved []string
				var saveErr error
				if opts.saveDir != "" {
					saved, saveErr = store.Save(opts.saveDir, r)
				}
				printFailure(r, !firstShown, saved, saveErr)
				firstShown = true
			}
		case runner.Finished:
			fin = e
		case runner.Fatal:
			fmt.Fprintln(os.Stderr, "fastgen:", e.Err)
			return 1
		}
	}

	if tty {
		fmt.Println()
	}
	printSummary(fin)
	if ctx.Err() != nil {
		return 1
	}
	if failures > 0 {
		return 1
	}
	return 0
}

func plainLine(r runner.Result, total int) string {
	line := fmt.Sprintf("[%d/%d] #%d seed=%d %-8s sol=%s brute=%s",
		r.Index+1, total, r.Index, r.Seed, r.Verdict,
		shortDur(r.SolutionTime), shortDur(r.BruteTime))
	if r.Note != "" {
		line += "  (" + r.Note + ")"
	}
	return line
}

func printFailure(r runner.Result, full bool, saved []string, saveErr error) {
	fmt.Printf("FAILED %s  test #%d  seed %d\n", r.Verdict, r.Index, r.Seed)
	if r.Note != "" {
		fmt.Printf("  %s\n", r.Note)
	}
	fmt.Printf("  exits: solution %d, brute %d, generator %d\n", r.SolutionExit, r.BruteExit, r.GeneratorExit)
	if full {
		if len(r.Input) > 0 {
			fmt.Println("--- input ---")
			fmt.Println(limitString(string(r.Input), 100_000))
		}
		fmt.Println("--- diff (expected -> got) ---")
		fmt.Println(runner.DiffSummary(string(r.Expected), string(r.Got), 200))
		if len(r.SolutionStderr) > 0 {
			fmt.Println("--- solution stderr ---")
			fmt.Println(limitString(string(r.SolutionStderr), 20_000))
		}
		if len(r.GeneratorStderr) > 0 {
			fmt.Println("--- generator stderr ---")
			fmt.Println(limitString(string(r.GeneratorStderr), 20_000))
		}
	}
	for _, p := range saved {
		fmt.Println("  saved:", p)
	}
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "  save failed:", saveErr)
	}
	fmt.Println()
}

func printSummary(f runner.Finished) {
	if f.Total == 0 && f.Done == 0 {
		fmt.Println("no tests ran")
		return
	}
	fmt.Printf("done: %d/%d in %s  |  AC %d  WA %d  RE %d  TLE %d  GEN %d\n",
		f.Done, f.Total, shortDur(f.Wall),
		f.Counts[runner.AC], f.Counts[runner.WA], f.Counts[runner.RE],
		f.Counts[runner.TLE], f.Counts[runner.GENFAIL])
	if f.Aborted {
		fmt.Println("run aborted")
	}
}

func shortDur(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func limitString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... truncated ..."
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
