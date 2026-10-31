package runner

import (
	"context"
	"testing"
	"time"
)

func TestExecCapturesStreamsAndExit(t *testing.T) {
	res := Exec(context.Background(), "/bin/sh", []string{"-c", "cat; echo oops >&2; exit 3"}, []byte("hello\n"), time.Second, 1<<20)
	if res.TimedOut || res.Canceled || res.Err != nil {
		t.Fatalf("unexpected failure: %+v", res)
	}
	if res.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", res.ExitCode)
	}
	if string(res.Stdout) != "hello\n" {
		t.Fatalf("Stdout = %q", res.Stdout)
	}
	if string(res.Stderr) != "oops\n" {
		t.Fatalf("Stderr = %q", res.Stderr)
	}
}

func TestExecTimeoutKillsTree(t *testing.T) {
	start := time.Now()
	res := Exec(context.Background(), "/bin/sh", []string{"-c", "sleep 30 & wait"}, nil, 150*time.Millisecond, 1<<20)
	elapsed := time.Since(start)
	if !res.TimedOut {
		t.Fatalf("expected timeout, got %+v", res)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout kill took too long: %s", elapsed)
	}
}

func TestExecCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Exec(ctx, "/bin/sh", []string{"-c", "echo never"}, nil, time.Second, 1<<20)
	if !res.Canceled {
		t.Fatalf("expected cancel, got %+v", res)
	}
}

func TestExecOutputCap(t *testing.T) {
	res := Exec(context.Background(), "/bin/sh", []string{"-c", "yes x"}, nil, 100*time.Millisecond, 1024)
	if !res.Truncated {
		t.Fatalf("expected truncated output, got %d bytes", len(res.Stdout))
	}
	if len(res.Stdout) > 1024 {
		t.Fatalf("captured %d bytes, want <= 1024", len(res.Stdout))
	}
}
