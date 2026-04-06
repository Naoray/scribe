package sync

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// CommandExecutor runs shell commands and captures output.
type CommandExecutor interface {
	Execute(ctx context.Context, command string, timeout time.Duration) (stdout, stderr string, err error)
}

// ShellExecutor runs commands via sh -c with process group management.
type ShellExecutor struct{}

func (e *ShellExecutor) Execute(ctx context.Context, command string, timeout time.Duration) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if ctx.Err() != nil {
		// Kill the entire process group on timeout/cancel.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %s", timeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}
