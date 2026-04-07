package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
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

// CommandHash returns a deterministic hash of install+update commands.
// Used for TOFU: if the hash changes, the user must re-approve.
func CommandHash(install, update string) string {
	h := sha256.New()
	h.Write([]byte(install))
	h.Write([]byte{0})
	h.Write([]byte(update))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
