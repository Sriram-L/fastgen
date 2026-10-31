package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

// ExecResult is the outcome of running a single binary.
type ExecResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Duration  time.Duration
	TimedOut  bool
	Canceled  bool
	Truncated bool
	Err       error
}

// Exec runs bin with args, feeding stdin, bounded by timeout (0 means no
// timeout). The child gets its own process group so timeouts and cancellation
// kill the whole tree.
func Exec(ctx context.Context, bin string, args []string, stdin []byte, timeout time.Duration, maxOut int) ExecResult {
	res := ExecResult{ExitCode: -1}
	if err := ctx.Err(); err != nil {
		res.Canceled = true
		res.Err = err
		return res
	}
	if maxOut <= 0 {
		maxOut = 4 << 20
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	stdout := newCapBuffer(maxOut)
	stderr := newCapBuffer(maxOut)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = processGroupAttr()

	if err := cmd.Start(); err != nil {
		res.Err = err
		return res
	}

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var timeoutC <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
		timeoutC = timer.C
	}

	finish := func(err error) {
		res.Duration = time.Since(start)
		res.Stdout = stdout.Bytes()
		res.Stderr = stderr.Bytes()
		res.Truncated = stdout.Truncated() || stderr.Truncated()
		res.ExitCode = exitCode(err)
	}

	select {
	case err := <-done:
		finish(err)
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				res.Err = err
			}
		}
	case <-timeoutC:
		res.TimedOut = true
		processGroupKill(cmd)
		finish(<-done)
	case <-ctx.Done():
		res.Canceled = true
		res.Err = ctx.Err()
		processGroupKill(cmd)
		finish(<-done)
	}
	return res
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type capBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func newCapBuffer(max int) *capBuffer {
	return &capBuffer{max: max}
}

func (b *capBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *capBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *capBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
