package sync_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/Naoray/scribe/internal/sync"
)

func TestShellExecutor_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	stdout, stderr, err := exec.Execute(context.Background(), "echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "hello\n")
	}
	if stderr != "" {
		t.Errorf("stderr: got %q, want empty", stderr)
	}
}

func TestShellExecutor_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	_, stderr, err := exec.Execute(context.Background(), "echo fail >&2 && exit 1", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if stderr != "fail\n" {
		t.Errorf("stderr: got %q, want %q", stderr, "fail\n")
	}
}

func TestShellExecutor_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	exec := &sync.ShellExecutor{}
	_, _, err := exec.Execute(context.Background(), "sleep 60", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestShellExecutor_ContextCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := &sync.ShellExecutor{}
	_, _, err := exec.Execute(ctx, "sleep 60", 5*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
