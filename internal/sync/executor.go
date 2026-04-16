package sync

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// CommandExecutor runs shell commands and captures output.
type CommandExecutor interface {
	Execute(ctx context.Context, command string, timeout time.Duration) (stdout, stderr string, err error)
}

// ShellExecutor runs commands via a platform shell with process group management where supported.
type ShellExecutor struct{}

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
