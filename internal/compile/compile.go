package compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Config describes how sources are compiled.
type Config struct {
	CXX      string
	Flags    []string
	Sanitize bool
	CacheDir string
}

// Targets are the three source files fastgen needs.
type Targets struct {
	Solution  string
	Generator string
	Brute     string
}

// Binaries holds the compiled binaries.
type Binaries struct {
	Solution  string
	Generator string
	Brute     string
}

// CompileError is returned when a translation unit fails to compile.
type CompileError struct {
	File   string
	Args   []string
	Output string
	Err    error
}

func (e *CompileError) Error() string {
	msg := strings.TrimSpace(e.Output)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("failed to compile %s:\n%s", e.File, msg)
}

func (e *CompileError) Unwrap() error { return e.Err }

// DefaultConfig returns the same compiler setup as the user's ~/.zshrc co()
// function (Homebrew g++-16, C++17, -O2 -Wall) with a persistent
// content-addressed cache. It falls back to the g++ on PATH when the
// Homebrew compiler is not installed.
func DefaultConfig() Config {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		cache = os.TempDir()
	}

	cxx := "g++"
	for _, candidate := range []string{"/opt/homebrew/bin/g++-16"} {
		if _, err := os.Stat(candidate); err == nil {
			cxx = candidate
			break
		}
	}

	return Config{
		CXX:      cxx,
		Flags:    []string{"-std=c++17", "-O2", "-Wall"},
		CacheDir: filepath.Join(cache, "fastgen"),
	}
}

var sanitizeFlags = []string{"-fsanitize=address,undefined", "-fno-omit-frame-pointer", "-g"}

// Build compiles all targets in parallel, reusing cached binaries for
// unchanged sources.
func Build(ctx context.Context, cfg Config, targets Targets) (Binaries, error) {
	var out Binaries

	if cfg.CXX == "" {
		cfg.CXX = "g++"
	}
	if _, err := exec.LookPath(cfg.CXX); err != nil {
		return out, &CompileError{
			File:   cfg.CXX,
			Output: fmt.Sprintf("compiler %q not found on PATH (install g++ or pass --cxx)", cfg.CXX),
			Err:    err,
		}
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = DefaultConfig().CacheDir
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return out, fmt.Errorf("create compile cache: %w", err)
	}

	type job struct {
		file     string
		sanitize bool
		dst      *string
	}
	jobs := []job{
		{targets.Solution, cfg.Sanitize, &out.Solution},
		{targets.Generator, false, &out.Generator},
		{targets.Brute, false, &out.Brute},
	}

	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = buildOne(ctx, cfg, jobs[i].file, jobs[i].sanitize, jobs[i].dst)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func buildOne(ctx context.Context, cfg Config, file string, sanitize bool, dst *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	src, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	flags := append([]string{}, cfg.Flags...)
	if sanitize {
		flags = append(flags, sanitizeFlags...)
	}

	h := sha256.New()
	h.Write([]byte(cfg.CXX))
	h.Write([]byte{0})
	h.Write([]byte(strings.Join(flags, "\x00")))
	h.Write([]byte{0})
	h.Write(src)
	key := hex.EncodeToString(h.Sum(nil))[:32]
	bin := filepath.Join(cfg.CacheDir, key)

	if info, err := os.Stat(bin); err == nil && !info.IsDir() {
		*dst = bin
		return nil
	}

	tmp, err := os.CreateTemp(cfg.CacheDir, ".fastgen-build-*")
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	args := append([]string{}, flags...)
	args = append(args, file, "-o", tmpPath)

	cmd := exec.CommandContext(ctx, cfg.CXX, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &CompileError{File: file, Args: args, Output: string(output), Err: err}
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod compiled binary: %w", err)
	}
	if err := os.Rename(tmpPath, bin); err != nil {
		// A concurrent build may have produced the same content-addressed
		// binary first; that is fine as long as it exists.
		if _, statErr := os.Stat(bin); statErr != nil {
			return fmt.Errorf("install compiled binary: %w", err)
		}
	}
	*dst = bin
	return nil
}

// IsCompileError reports whether err came from the compiler.
func IsCompileError(err error) bool {
	var ce *CompileError
	return errors.As(err, &ce)
}
