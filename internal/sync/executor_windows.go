//go:build windows

package sync

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

func (e *ShellExecutor) Execute(ctx context.Context, command string, timeout time.Duration) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd", "/C", command)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if ctx.Err() != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %s", timeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}
