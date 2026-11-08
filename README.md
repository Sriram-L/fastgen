# fastgen

`fastgen` is a Go command-line tool for stress-testing competitive-programming
solutions. It generates random test cases, runs them against both your solution
and a known-correct "brute force" implementation, and reports the first input
that makes them disagree.

## How it works

For each test case `fastgen`:

1. Runs a test generator, passing the seed as `argv[1]`, and captures its
   stdout as the test input.
2. Feeds that input to your solution and to the brute-force solution on stdin.
3. Compares the two outputs (whitespace-insensitive by default, or
   byte-exact with `--exact`).
4. Assigns a verdict: `AC`, `WA`, `RE`, `TLE`, or `GEN_FAIL`.

The first failure stops the run by default and shows the offending input with a
diff, which can be saved to disk for reproduction.

## Features

- Live terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  and [Lip Gloss](https://github.com/charmbracelet/lipgloss), plus a `--plain`
  mode for CI and piping.
- Configurable compiler and flags, with presets for C++, C, Rust, and Go.
- Per-process timeouts that kill the whole process group.
- Parallel test execution via `--jobs`.
- Optional sanitizer builds for catching undefined behavior in C/C++.
- Output size limits to guard against runaway programs.
- Custom checker support for problems with multiple valid answers.
- Save failing test cases (`.in`, `.expected`, `.got`) to a directory.

## Usage

```sh
fastgen --sol a.cpp --gen gen.cpp --brute brute.cpp -n 1000
```

Common options:

| Flag | Description |
| --- | --- |
| `--sol` | Solution source file (required) |
| `--gen` | Test generator source file (required) |
| `--brute` | Brute-force / correct solution source file (required) |
| `-n` | Number of test cases (default 1000) |
| `--seed` | First seed passed to the generator (default 1) |
| `--timeout` | Per-process time limit (default 2s) |
| `--jobs` | Number of test cases to run in parallel |
| `--exact` | Compare output byte-for-byte |
| `--keep-going` | Collect all failures instead of stopping at the first |
| `--sanitize` | Build the solution with sanitizer flags |
| `--save-dir` | Directory for saving failing cases |
| `--plain` | Disable the TUI and print one line per test |
| `--spec` | Path to a YAML config file |

## Configuration

By default `fastgen` loads `config.yaml` from the directory containing the
binary. Command-line flags override values from the config.

```yaml
compile:
  language: c++
  flags: ["-std=c++17", "-O2", "-Wall"]
  sanitize_flags: ["-fsanitize=address,undefined", "-fno-omit-frame-pointer", "-g"]
  command: ["{compiler}", "{flags}", "{source}", "-o", "{output}"]

defaults:
  tests: 1000
  seed: 1
  timeout: 2s
  jobs: 4
  max_output: 4194304
  exact: false
  keep_going: false
  sanitize: false
  save_dir: ""
```

## Build

```sh
go build -o fastgen .
```

## Test

```sh
go test ./...
```
