package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"sort"
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

// CommandHash returns a deterministic hash of all install/update commands,
// including per-tool overrides. Used for TOFU: if the hash changes, the user
// must re-approve. Adding per-tool commands to a previously global-only entry
// will change the hash and trigger re-approval.
func CommandHash(install, update string, installs, updates map[string]string) string {
	h := sha256.New()
	h.Write([]byte(install))
	h.Write([]byte{0})
	h.Write([]byte(update))
	h.Write([]byte{0})
	for _, k := range sortedMapKeys(installs) {
		fmt.Fprintf(h, "%s=%s\x00", k, installs[k])
	}
	for _, k := range sortedMapKeys(updates) {
		fmt.Fprintf(h, "%s=%s\x00", k, updates[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
