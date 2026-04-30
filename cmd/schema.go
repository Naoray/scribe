package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/cli/output"
	clischema "github.com/Naoray/scribe/internal/cli/schema"
)

type commandSchema struct {
	InputSchema  json.RawMessage  `json:"input_schema"`
	OutputSchema *json.RawMessage `json:"output_schema"`
}

func newSchemaCommand(root *cobra.Command) *cobra.Command {
	var all bool
	var markdown bool
	cmd := &cobra.Command{
		Use:   "schema [command]",
		Short: "Print JSON Schema for command inputs and outputs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if markdown {
				_, err := fmt.Fprint(cmd.OutOrStdout(), clischema.RenderMarkdown(root, clischema.All()))
				return err
			}
			if all {
				return writeAllSchemas(cmd, root)
			}
			target := "schema"
			if len(args) == 1 {
				target = args[0]
			}
			found, err := findCommand(root, target)
			if err != nil {
				return err
			}
			return writeCommandSchema(cmd, found, target)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Print schemas for all commands")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Render command schema summary as Markdown")
	return markJSONSupported(cmd)
}

func writeAllSchemas(cmd *cobra.Command, root *cobra.Command) error {
	out := map[string]commandSchema{}
	walkCommandsForSchema(root, func(c *cobra.Command) {
		if c.Hidden {
			return
		}
		out[c.CommandPath()] = schemaForCommand(c)
	})
	return renderSchemaResult(cmd, cmd.CommandPath(), out)
}

func writeCommandSchema(cmd *cobra.Command, target *cobra.Command, requested string) error {
	command := cmd.CommandPath()
	if requested != "" && requested != cmd.Name() {
		command += " " + requested
	}
	return renderSchemaResult(cmd, command, schemaForCommand(target))
}

func renderSchemaResult(cmd *cobra.Command, command string, data any) error {
	if !jsonFlagPassed(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(data)
	}

	renderer := output.New(envFromArgs(os.Args), os.Stdout, os.Stderr)
	renderer.SetMeta("command", command)
	if version, ok := cmd.Context().Value(envelope.ScribeVersionKey).(string); ok {
		renderer.SetMeta("scribe_version", version)
	}
	if start, ok := cmd.Context().Value(envelope.RunEStartKey).(time.Time); ok {
		duration := time.Since(start).Milliseconds()
		if duration < 1 {
			duration = 1
		}
		renderer.SetMeta("duration_ms", duration)
	}
	if err := renderer.Result(data); err != nil {
		return err
	}
	return renderer.Flush()
}

func schemaForCommand(cmd *cobra.Command) commandSchema {
	input := json.RawMessage(clischema.InputSchema(cmd))
	var output *json.RawMessage
	if raw, ok := clischema.Get(cmd.CommandPath()); ok {
		msg := json.RawMessage(raw)
		output = &msg
	}
	return commandSchema{
		InputSchema:  input,
		OutputSchema: output,
	}
}

func findCommand(root *cobra.Command, path string) (*cobra.Command, error) {
	path = strings.TrimSpace(strings.TrimPrefix(path, root.CommandPath()+" "))
	var found *cobra.Command
	walkCommandsForSchema(root, func(cmd *cobra.Command) {
		if found != nil {
			return
		}
		if cmd.CommandPath() == path || cmd.CommandPath() == root.CommandPath()+" "+path || cmd.Name() == path {
			found = cmd
		}
	})
	if found != nil {
		return found, nil
	}
	return nil, &clierrors.Error{
		Code:        "SCHEMA_COMMAND_NOT_FOUND",
		Message:     "schema command not found: " + path,
		Remediation: "registered commands: " + strings.Join(schemaCommandPaths(root), ", "),
		Exit:        clierrors.ExitNotFound,
	}
}

func schemaCommandPaths(root *cobra.Command) []string {
	var paths []string
	walkCommandsForSchema(root, func(cmd *cobra.Command) {
		if !cmd.Hidden {
			paths = append(paths, cmd.CommandPath())
		}
	})
	sort.Strings(paths)
	return paths
}

func walkCommandsForSchema(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommandsForSchema(child, visit)
	}
}
