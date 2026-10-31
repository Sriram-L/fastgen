# fastgen: Go + Charm stress-testing CLI

## 1. What it does

```
fastgen --sol a.cpp --gen gen.cpp --brute brute.cpp -n 1000
```

For each test: run the generator, feed its output to both your solution and the correct (brute) solution, compare the outputs, and show live progress in a TUI. On the first mismatch (or a crash or timeout), it shows the failing input and a diff.

## 2. Stack

- **Bubble Tea**: TUI framework (model/update/view)
- **Bubbles**: spinner, progress bar, table, viewport
- **Lip Gloss**: styling and layout (colored verdicts, panes, borders)
- **Cobra** or plain `flag`: CLI args (`flag` is enough for v1)
- **sergi/go-diff** (or a hand-rolled line diff): for the failing case view

## 3. Project layout

```
fastgen/
├── main.go                 # parse flags, start tea.Program
├── internal/
│   ├── compile/compile.go  # g++ invocation, caching
│   ├── runner/
│   │   ├── exec.go         # run a binary w/ stdin, timeout, capture stdout/stderr/time
│   │   ├── stress.go       # the test loop, emits events
│   │   └── compare.go      # token-based / exact output comparison
│   ├── tui/
│   │   ├── model.go        # state + Update
│   │   ├── view.go         # View + Lip Gloss styles
│   │   ├── styles.go
│   │   └── messages.go     # tea.Msg types
│   └── store/save.go       # write failing testcase to disk
└── go.mod
```

Keeping the runner independent of the TUI makes it testable and lets you add a `--plain` mode (no TUI, for CI or piping) almost for free.

## 4. Core pipeline

### a) Compile stage

- Compile all three files in parallel (goroutines) into a temp dir: `g++ -O2 -std=c++17 file.cpp -o bin`.
- Make flags configurable (`--cxxflags`), and optionally add `-fsanitize=address,undefined` for the solution so crashes are caught.
- Cache by file content hash so unchanged files are not recompiled.
- On compile error, show stderr in a red-bordered viewport and stop.

### b) Test loop (per test `i`, seed = `start + i`)

1. Run the generator with the seed as `argv[1]` (a common testlib-style convention) and capture stdout as the input.
2. Run the brute and the solution with that input on stdin, each with a `context.WithTimeout`.
3. Record for each run: stdout, stderr, exit code, duration.
4. Assign a verdict:
   - `AC`: outputs match
   - `WA`: outputs differ
   - `RE`: non-zero exit or signal
   - `TLE`: timed out (kill the whole process group, using `Setpgid`)
   - `GEN_FAIL`: the generator crashed (a distinct error, not a user-solution failure)
5. Compare outputs with whitespace-insensitive token comparison by default, with an optional `--exact` flag.
6. Stop on the first failure by default, with `--keep-going` to collect all failures.

### c) Runner → TUI communication

- The runner runs in a goroutine and sends events (`TestDone{i, verdict, time}`, `Finished`, `Failed{...}`) through a channel.
- A `tea.Cmd` waits on that channel and turns each event into a `tea.Msg`. Alternatively, hold the `*tea.Program` and call `p.Send(...)`.
- Optional: a worker pool (`--jobs N`) for parallel tests.

## 5. TUI design

**States:** `Compiling → Running → Passed | Failed`

### Running screen

```
╭─ fastgen ─────────────────────────────────╮
│ ⠋ Running   427 / 1000                    │
│ ████████████░░░░░░░░░░░░  42%             │
│                                           │
│ AC 427   WA 0   RE 0   TLE 0              │
│ Avg: sol 12ms | brute 85ms | Max: 140ms   │
│                                           │
│ Current test                              │
│  #427  seed 427   AC   11ms / 80ms        │
╰───────────────────────────────────────────╯
```

### In-place rendering (no scrolling output)

Each finished testcase **overwrites** the previous one in the same spot instead of appending a new line. The screen never grows; only the "Current test" block, the progress bar, and the counters change.

- **How it works:** Bubble Tea already redraws the whole `View()` string in place on every update (it diffs against the previous frame and rewrites changed lines). So the model stores only the *latest* result (`lastResult`) rather than a list, and `View()` renders just that. There is no history slice to append to.
- **Fixed-height layout:** every line in the running view has a constant width and line count (pad with `lipgloss.Width/Height` or `strings.Repeat`). Otherwise a shorter line leaves stale characters behind, or the frame height jumps and flickers.
- **Throttle redraws:** fast tests can finish thousands of times per second. Don't render per event. Have the model absorb events into its counters and `lastResult`, and redraw on a ~30 FPS `tea.Tick` (or let Bubble Tea's built-in framerate cap handle it). The counters stay exact, and only the display is sampled.
- **Use the alt screen** (`tea.WithAltScreen()`) so the in-place view doesn't leave artifacts in the terminal scrollback, and the final summary prints cleanly on exit.
- **`--plain` mode is the exception:** with no TUI, print one line per test, or overwrite a single line using `\r` plus the ANSI clear-line sequence (`\x1b[2K`) when stdout is a TTY. If it's piped (CI), fall back to normal newline output.
- **Failures are never overwritten:** the first non-AC result stops the loop (or is pinned, with `--keep-going`) and switches to the failure screen, so the evidence isn't wiped by the next test.

### Failure screen

Three tabs or panes (switch with `tab`/`←→`), each a scrollable `viewport`:

- **Input**: generated testcase
- **Expected vs Got**: side-by-side with Lip Gloss `JoinHorizontal`, with the first differing line highlighted
- **Stderr / Details**: crash info, exit code, timing

**Keys:** `q` quit, `tab` switch pane, `↑/↓` scroll, `s` save the testcase, `r` rerun.

## 6. Extras worth including

- `--save-dir`: write `fail_<seed>.in`, `.expected`, and `.got` to disk.
- `--timeout`, `-n`, `--seed`, `--checker` (a custom checker binary for problems with multiple valid answers).
- Clean up temp files and kill child processes on `Ctrl+C`.
- Check that `g++` exists on the PATH up front and give a clear error if not.

## 7. Milestones

1. **Runner only (no TUI):** compile, run, compare, print verdicts to stdout. Confirm it catches a deliberately buggy solution.
2. **Bubble Tea skeleton:** spinner, progress bar, and counters driven by runner events.
3. **Failure view:** viewport panes, the diff, and save-to-disk.
4. **Polish:** Lip Gloss theming, in-place current-test display (tick-throttled), timing stats, the `--plain` mode.
5. **Stretch goals:**
   - Test case minimization (shrink the failing input)
   - Memory limits (`rlimit`)
   - Custom checker support
   - Config file (`.fastgen.toml`)
   - Windows support (process-group handling differs)

## 8. Pitfalls to watch

- **Timeouts must kill the process tree**, or hung binaries pile up.
- **Large outputs:** cap captured stdout, since an infinite-print bug can eat memory.
- **Generator determinism:** the same seed must produce the same input, or failures won't reproduce.
- **Don't block `Update()`:** all execution stays in goroutines, with only messages flowing into the model.
- **Stale characters and flicker:** with in-place updates, keep line widths and the frame height constant, and throttle redraws to a tick rather than one per test.
