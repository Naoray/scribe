package workflow

import (
	"io"
	"os"

	"github.com/Naoray/scribe/internal/sync"
)

// Formatter absorbs all output-mode decisions. Once constructed, callers
// never branch on JSON vs text or single vs multi-registry.
type Formatter interface {
	// Sync lifecycle
	OnRegistryStart(repo string)
	OnSkillResolved(name string, status sync.SkillStatus)
	OnSkillDownloading(name string)
	OnSkillInstalled(name string, version string, updated bool)
	OnSkillSkipped(name string, status sync.SkillStatus)
	OnSkillError(name string, err error)
	OnSyncComplete(summary sync.SyncCompleteMsg)

	// Flush writes any buffered output (JSON mode). Text mode is a no-op.
	Flush() error
}

// NewFormatter resolves the useJSON × multiRegistry matrix once and
// returns the appropriate Formatter implementation.
func NewFormatter(useJSON bool, multiRegistry bool) Formatter {
	return NewFormatterWithWriters(useJSON, multiRegistry, os.Stdout, os.Stderr)
}

// NewFormatterWithWriters is like NewFormatter but allows injecting writers
// for testing.
func NewFormatterWithWriters(useJSON bool, multiRegistry bool, out, errOut io.Writer) Formatter {
	if useJSON {
		return newJSONFormatter(out)
	}
	return newTextFormatter(out, errOut, multiRegistry)
}
