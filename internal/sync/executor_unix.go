//go:build !windows

package sync

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func (e *ShellExecutor) Execute(ctx context.Context, command string, timeout time.Duration) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Cancel runs while the process is still alive, so we can SIGKILL the
	// entire process group (children, grandchildren) instead of just the
	// shell parent that exec.CommandContext would otherwise kill.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if ctx.Err() != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %s", timeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}
