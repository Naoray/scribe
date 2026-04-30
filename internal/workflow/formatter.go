package workflow

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/Naoray/scribe/internal/cli/envelope"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
)

// Formatter absorbs all output-mode decisions. Once constructed, callers
// never branch on JSON vs text or single vs multi-registry.
type Formatter interface {
	// Sync lifecycle
	OnRegistryStart(repo string)
	OnSkillResolved(name string, status sync.SkillStatus)
	OnSkillDownloading(name string)
	OnSkillInstalled(name string, updated bool)
	OnSkillSkipped(name string, status sync.SkillStatus)
	OnSkillSkippedByDenyList(name, registry string)
	OnSkillError(name string, err error)
	OnSyncComplete(summary sync.SyncCompleteMsg)
	OnReconcileConflict(name string, conflict state.ProjectionConflict)
	OnReconcileComplete(summary sync.ReconcileCompleteMsg)
	OnLegacyFormat(repo string)

	// Connect lifecycle
	OnConnectDuplicate(repo string)
	OnConnectSaved(repo string)
	OnConnectSyncing()
	OnConnectSyncWarning(repo string, err error)
	OnConnectAvailable(repo string, count int)

	// Package lifecycle
	OnPackageInstallPrompt(name, command, source string)
	OnPackageApproved(name string)
	OnPackageDenied(name string)
	OnPackageSkipped(name, reason string)
	OnPackageInstalling(name string)
	OnPackageInstalled(name string)
	OnPackageUpdating(name string)
	OnPackageUpdated(name string)
	OnPackageError(name string, err error, stderr string)
	OnPackageHashMismatch(name, oldCmd, newCmd, source string)

	// Adoption lifecycle
	OnAdoptionSkipped(reason string)
	OnAdoptionStarted(candidateCount int)
	OnAdopted(name string, targetTools []string)
	OnAdoptionError(name string, err error)
	OnAdoptionConflictsDeferred(count int)
	OnAdoptionComplete(adopted, skipped, failed int)

	// Flush writes any buffered output (JSON mode). Text mode is a no-op.
	Flush() error
}

// NewFormatter resolves the useJSON × multiRegistry matrix once and
// returns the appropriate Formatter implementation.
func NewFormatter(useJSON bool, multiRegistry bool) Formatter {
	return NewFormatterWithWriters(useJSON, multiRegistry, os.Stdout, os.Stderr)
}

func NewFormatterForContext(ctx context.Context, useJSON bool, multiRegistry bool) Formatter {
	return NewFormatterWithWritersAndMeta(useJSON, multiRegistry, os.Stdout, os.Stderr, metaFromContext(ctx))
}

// NewFormatterWithWriters is like NewFormatter but allows injecting writers
// for testing.
func NewFormatterWithWriters(useJSON bool, multiRegistry bool, out, errOut io.Writer) Formatter {
	return NewFormatterWithWritersAndMeta(useJSON, multiRegistry, out, errOut, func() envelope.Meta {
		return envelope.Meta{}
	})
}

func NewFormatterWithWritersAndMeta(useJSON bool, multiRegistry bool, out, errOut io.Writer, meta func() envelope.Meta) Formatter {
	if useJSON {
		return newJSONFormatter(out, meta)
	}
	return newTextFormatter(out, errOut, multiRegistry)
}

func metaFromContext(ctx context.Context) func() envelope.Meta {
	return func() envelope.Meta {
		meta := envelope.Meta{}
		if ctx == nil {
			return meta
		}
		if command, ok := ctx.Value(envelope.CommandPathKey).(string); ok {
			meta.Command = command
		}
		if version, ok := ctx.Value(envelope.ScribeVersionKey).(string); ok {
			meta.ScribeVersion = version
		}
		if start, ok := ctx.Value(envelope.RunEStartKey).(time.Time); ok {
			meta.DurationMS = positiveWorkflowDuration(time.Since(start).Milliseconds())
		}
		if bootstrap, ok := ctx.Value(envelope.BootstrapMSKey).(int64); ok {
			meta.BootstrapMS = positiveWorkflowDuration(bootstrap)
		} else if bootstrapStart, ok := ctx.Value(envelope.BootstrapStartKey).(time.Time); ok {
			if runStart, ok := ctx.Value(envelope.RunEStartKey).(time.Time); ok {
				meta.BootstrapMS = positiveWorkflowDuration(runStart.Sub(bootstrapStart).Milliseconds())
			}
		}
		return meta
	}
}

func positiveWorkflowDuration(ms int64) int64 {
	if ms < 1 {
		return 1
	}
	return ms
}
