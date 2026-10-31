package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Verdict is the outcome of one test case.
type Verdict string

const (
	AC      Verdict = "AC"
	WA      Verdict = "WA"
	RE      Verdict = "RE"
	TLE     Verdict = "TLE"
	GENFAIL Verdict = "GEN_FAIL"
)

// Result captures everything needed to display or reproduce one test.
type Result struct {
	Index int
	Seed  int64

	Verdict Verdict
	Note    string

	Input    []byte
	Expected []byte
	Got      []byte

	SolutionExit  int
	BruteExit     int
	GeneratorExit int

	SolutionTime  time.Duration
	BruteTime     time.Duration
	GeneratorTime time.Duration

	SolutionStderr  []byte
	BruteStderr     []byte
	GeneratorStderr []byte

	Truncated      bool
	InputTruncated bool
	Canceled       bool
}

// Failed reports whether the test did not pass.
func (r Result) Failed() bool { return r.Verdict != AC }

// Config drives a stress run.
type Config struct {
	Generator string
	Solution  string
	Brute     string
	Checker   string

	StartSeed int64
	Tests     int
	Timeout   time.Duration
	Exact     bool
	KeepGoing bool
	Jobs      int
	MaxOutput int
}

// Event is emitted by Run.
type Event interface{ eventMarker() }

// TestStarted reports that a test case began, so the UI can clear the
// previous testcase.
type TestStarted struct {
	Index int
	Seed  int64
}

func (TestStarted) eventMarker() {}

// TestDone reports a completed test case.
type TestDone struct{ Result Result }

func (TestDone) eventMarker() {}

// Finished reports the end of the run.
type Finished struct {
	Total   int
	Done    int
	Counts  map[Verdict]int
	Wall    time.Duration
	Aborted bool
}

func (Finished) eventMarker() {}

// Fatal reports a runner-level failure.
type Fatal struct{ Err error }

func (Fatal) eventMarker() {}

// Run executes every test, sending events on ch and closing ch when finished.
func Run(ctx context.Context, cfg Config, events chan<- Event) {
	defer close(events)

	start := time.Now()
	if cfg.Tests < 0 {
		cfg.Tests = 0
	}
	if cfg.Jobs < 1 {
		cfg.Jobs = 1
	}
	if cfg.Jobs > cfg.Tests && cfg.Tests > 0 {
		cfg.Jobs = cfg.Tests
	}

	counts := map[Verdict]int{}
	done := 0
	var failedAbort atomic.Bool

	if cfg.Tests == 0 {
		events <- Finished{Wall: time.Since(start)}
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var failOnce sync.Once
	testCh := make(chan int)
	results := make(chan Result, cfg.Jobs)

	var wg sync.WaitGroup
	for w := 0; w < cfg.Jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range testCh {
				if runCtx.Err() != nil {
					return
				}
				seed := cfg.StartSeed + int64(i)
				select {
				case events <- TestStarted{Index: i, Seed: seed}:
				case <-runCtx.Done():
					return
				}
				r := runOne(runCtx, cfg, i, seed)
				if r.Canceled {
					return
				}
				if r.Failed() && !cfg.KeepGoing {
					first := false
					failOnce.Do(func() { first = true })
					if first {
						failedAbort.Store(true)
						select {
						case results <- r:
						case <-ctx.Done():
						}
					}
					cancel()
					return
				}
				select {
				case results <- r:
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(testCh)
		for i := 0; i < cfg.Tests; i++ {
			select {
			case testCh <- i:
			case <-runCtx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.Canceled {
			continue
		}
		done++
		counts[r.Verdict]++
		select {
		case events <- TestDone{Result: r}:
		case <-ctx.Done():
		}
	}

	fin := Finished{
		Total:   cfg.Tests,
		Done:    done,
		Counts:  counts,
		Wall:    time.Since(start),
		Aborted: failedAbort.Load() || ctx.Err() != nil,
	}
	select {
	case events <- fin:
	case <-ctx.Done():
	}
}

// Repeat re-runs the solution and brute force on an existing input.
func Repeat(ctx context.Context, cfg Config, previous Result) Result {
	r := Result{Index: previous.Index, Seed: previous.Seed, Input: previous.Input}
	return evaluate(ctx, cfg, r)
}

func runOne(ctx context.Context, cfg Config, index int, seed int64) Result {
	r := Result{Index: index, Seed: seed}

	gen := Exec(ctx, cfg.Generator, []string{strconv.FormatInt(seed, 10)}, nil, cfg.Timeout, cfg.MaxOutput)
	r.GeneratorExit = gen.ExitCode
	r.GeneratorTime = gen.Duration
	r.GeneratorStderr = gen.Stderr

	if gen.Canceled {
		r.Canceled = true
		return r
	}
	switch {
	case gen.TimedOut:
		r.Verdict = GENFAIL
		r.Note = "generator timed out"
		return r
	case gen.Err != nil:
		r.Verdict = GENFAIL
		r.Note = "generator failed to start: " + gen.Err.Error()
		return r
	case gen.ExitCode != 0:
		r.Verdict = GENFAIL
		r.Note = fmt.Sprintf("generator exited with code %d", gen.ExitCode)
		return r
	}

	r.Input = gen.Stdout
	return evaluate(ctx, cfg, r)
}

func evaluate(ctx context.Context, cfg Config, r Result) Result {
	var sol, brute ExecResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sol = Exec(ctx, cfg.Solution, nil, r.Input, cfg.Timeout, cfg.MaxOutput)
	}()
	go func() {
		defer wg.Done()
		brute = Exec(ctx, cfg.Brute, nil, r.Input, cfg.Timeout, cfg.MaxOutput)
	}()
	wg.Wait()

	r.SolutionExit = sol.ExitCode
	r.BruteExit = brute.ExitCode
	r.SolutionTime = sol.Duration
	r.BruteTime = brute.Duration
	r.SolutionStderr = sol.Stderr
	r.BruteStderr = brute.Stderr
	r.Got = sol.Stdout
	r.Expected = brute.Stdout
	r.Truncated = sol.Truncated || brute.Truncated

	if sol.Canceled || brute.Canceled {
		r.Canceled = true
		return r
	}

	var notes []string
	switch {
	case sol.TimedOut && brute.TimedOut:
		r.Verdict = TLE
		notes = append(notes, "solution and brute force timed out")
	case sol.TimedOut:
		r.Verdict = TLE
		notes = append(notes, "solution timed out")
	case brute.TimedOut:
		r.Verdict = TLE
		notes = append(notes, "brute force timed out")
	case sol.Err != nil:
		r.Verdict = RE
		notes = append(notes, "solution failed to start: "+sol.Err.Error())
	case brute.Err != nil:
		r.Verdict = RE
		notes = append(notes, "brute force failed to start: "+brute.Err.Error())
	case sol.ExitCode != 0:
		r.Verdict = RE
		notes = append(notes, fmt.Sprintf("solution exited with code %d", sol.ExitCode))
	case brute.ExitCode != 0:
		r.Verdict = RE
		notes = append(notes, fmt.Sprintf("brute force exited with code %d", brute.ExitCode))
	default:
		if cfg.Checker != "" {
			ok, note, canceled := runChecker(ctx, cfg, r.Input, r.Got, r.Expected)
			if canceled {
				r.Canceled = true
				return r
			}
			if ok {
				r.Verdict = AC
			} else {
				r.Verdict = WA
			}
			if note != "" {
				notes = append(notes, note)
			}
		} else if Match(r.Expected, r.Got, cfg.Exact) {
			r.Verdict = AC
		} else {
			r.Verdict = WA
			notes = append(notes, "outputs differ")
		}
	}

	if r.Truncated {
		notes = append(notes, "output was truncated")
	}
	r.Note = strings.Join(notes, "; ")

	if r.Verdict == AC {
		// Keep a small input preview for the live TUI; drop the rest.
		const inputPreviewLimit = 4 << 10
		if len(r.Input) > inputPreviewLimit {
			r.Input = append([]byte(nil), r.Input[:inputPreviewLimit]...)
			r.InputTruncated = true
		}
		r.Expected = nil
		r.Got = nil
		r.SolutionStderr = nil
		r.BruteStderr = nil
		r.GeneratorStderr = nil
	}
	return r
}

func runChecker(ctx context.Context, cfg Config, input, got, expected []byte) (ok bool, note string, canceled bool) {
	dir, err := os.MkdirTemp("", "fastgen-check-*")
	if err != nil {
		return false, "checker: " + err.Error(), false
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "input")
	gotPath := filepath.Join(dir, "got")
	expPath := filepath.Join(dir, "expected")
	if err := os.WriteFile(inPath, input, 0o644); err != nil {
		return false, "checker: " + err.Error(), false
	}
	if err := os.WriteFile(gotPath, got, 0o644); err != nil {
		return false, "checker: " + err.Error(), false
	}
	if err := os.WriteFile(expPath, expected, 0o644); err != nil {
		return false, "checker: " + err.Error(), false
	}

	res := Exec(ctx, cfg.Checker, []string{inPath, gotPath, expPath}, nil, cfg.Timeout, 64<<10)
	if res.Canceled {
		return false, "", true
	}
	if res.TimedOut {
		return false, "checker timed out", false
	}
	if res.Err != nil {
		return false, "checker failed to start: " + res.Err.Error(), false
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(res.Stdout))
		}
		if msg != "" {
			return false, "checker: " + msg, false
		}
		return false, fmt.Sprintf("checker rejected output (exit %d)", res.ExitCode), false
	}
	return true, "", false
}
