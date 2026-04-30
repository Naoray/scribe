package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/workflow"
)

type nameConflictResolutionPayload struct {
	Skill  string                  `json:"skill"`
	Tool   string                  `json:"tool,omitempty"`
	Path   string                  `json:"path,omitempty"`
	Action sync.NameConflictAction `json:"action"`
	Alias  string                  `json:"alias,omitempty"`
}

func handleNameConflictError(cmd *cobra.Command, err error) error {
	var conflict *sync.NameConflictError
	if !errors.As(err, &conflict) {
		return err
	}

	wrapped := clierrors.Wrap(
		err,
		"SYNC_NAME_CONFLICT",
		clierrors.ExitConflict,
		clierrors.WithRemediation("run `scribe adopt "+conflict.Conflict.Name+"`, retry with `--alias <name>`, or skip from an interactive terminal"),
	)

	if workflow.ConflictModeForProcess(jsonFlagPassed(cmd)) == workflow.ConflictModeInteractive {
		return wrapped
	}

	resolution := conflictResolutionPayload(conflict.Conflict, conflict.Resolution)
	payload := map[string]any{"resolution": resolution}
	env := envelope.Envelope{
		Status:        envelope.StatusError,
		FormatVersion: envelope.FormatVersion,
		Data:          payload,
		Error:         conflictCLIError(wrapped),
		Meta:          envelopeMetaForCommand(cmd),
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(env); encodeErr != nil {
		return encodeErr
	}
	return clierrors.Wrap(err, "SYNC_NAME_CONFLICT", clierrors.ExitConflict, clierrors.WithRendered(true))
}

func conflictResolutionPayload(conflict sync.NameConflict, resolution sync.NameConflictResolution) nameConflictResolutionPayload {
	return nameConflictResolutionPayload{
		Skill:  conflict.Name,
		Tool:   conflict.Tool,
		Path:   conflict.Path,
		Action: resolution.Action,
		Alias:  resolution.Alias,
	}
}

func conflictCLIError(err error) *clierrors.Error {
	var ce *clierrors.Error
	if errors.As(err, &ce) {
		return ce
	}
	return &clierrors.Error{Code: "SYNC_NAME_CONFLICT", Message: err.Error(), Exit: clierrors.ExitConflict}
}

func envelopeMetaForCommand(cmd *cobra.Command) envelope.Meta {
	meta := envelope.Meta{}
	if cmd == nil {
		return meta
	}
	if version, ok := cmd.Context().Value(envelope.ScribeVersionKey).(string); ok {
		meta.ScribeVersion = version
	}
	if command, ok := cmd.Context().Value(envelope.CommandPathKey).(string); ok {
		meta.Command = command
	}
	if start, ok := cmd.Context().Value(envelope.RunEStartKey).(time.Time); ok {
		meta.DurationMS = positiveDuration(time.Since(start).Milliseconds())
	}
	if bootstrap, ok := cmd.Context().Value(envelope.BootstrapMSKey).(int64); ok {
		meta.BootstrapMS = positiveDuration(bootstrap)
	}
	return meta
}
