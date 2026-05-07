package sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Naoray/scribe/internal/lockfile"
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
	parts := []string{install, update}
	for _, k := range sortedMapKeys(installs) {
		parts = append(parts, fmt.Sprintf("install.%s=%s", k, installs[k]))
	}
	for _, k := range sortedMapKeys(updates) {
		parts = append(parts, fmt.Sprintf("update.%s=%s", k, updates[k]))
	}
	return lockfile.CommandHash(parts...)
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
