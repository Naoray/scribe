package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/mcpstatus"
)

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Inspect project MCP configuration",
	}
	cmd.AddCommand(newMCPListCommand())
	return cmd
}

func newMCPListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List project MCP declarations and client projections",
		Args:  cobra.NoArgs,
		RunE:  runMCPList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markReadOnly(markJSONSupported(cmd))
}

func runMCPList(cmd *cobra.Command, args []string) error {
	report, err := mcpstatus.Inspect(mcpstatus.InspectOptions{})
	if err != nil {
		return err
	}
	if jsonFlagPassed(cmd) {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(report); err != nil {
			return err
		}
		return r.Flush()
	}
	return printMCPList(cmd, report)
}

func printMCPList(cmd *cobra.Command, report mcpstatus.Report) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Project: %s\n", report.ProjectRoot); err != nil {
		return err
	}
	if report.ManifestPath != "" {
		if _, err := fmt.Fprintf(out, "Manifest: %s\n", report.ManifestPath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Declared MCP servers (%d): %s\n", len(report.Declarations), listOrNone(report.Declarations)); err != nil {
		return err
	}
	for _, client := range report.Clients {
		if _, err := fmt.Fprintf(out, "%s: %s", client.Name, client.State); err != nil {
			return err
		}
		if len(client.Projected) > 0 {
			if _, err := fmt.Fprintf(out, " (%s)", strings.Join(client.Projected, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if len(report.Drift) == 0 {
		_, err := fmt.Fprintln(out, "Drift: none")
		return err
	}
	if _, err := fmt.Fprintf(out, "Drift (%d):\n", len(report.Drift)); err != nil {
		return err
	}
	for _, drift := range report.Drift {
		if _, err := fmt.Fprintf(out, "- %s", drift.Kind); err != nil {
			return err
		}
		if drift.Client != "" {
			if _, err := fmt.Fprintf(out, " client=%s", drift.Client); err != nil {
				return err
			}
		}
		if drift.Server != "" {
			if _, err := fmt.Fprintf(out, " server=%s", drift.Server); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, ": %s\n", drift.Message); err != nil {
			return err
		}
	}
	return nil
}

func listOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
