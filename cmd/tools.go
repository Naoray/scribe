package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/tools"
)

func newToolsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Manage AI tool targets for skill installation",
		Long: `Show detected AI tools and their enabled/disabled status.

Use enable/disable to control which tools receive skill installations
during sync.

Examples:
  scribe tools                # list tools and status
  scribe tools enable cursor  # enable a tool
  scribe tools disable cursor # disable a tool`,
		Args: cobra.NoArgs,
		RunE: runToolsList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.AddCommand(newToolsEnableCommand())
	cmd.AddCommand(newToolsDisableCommand())
	return cmd
}

func newToolsEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a tool for skill installation",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolsEnable,
	}
}

func newToolsDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a tool for skill installation",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolsDisable,
	}
}

func runToolsList(cmd *cobra.Command, _ []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ensureToolsPopulated(cfg)

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	if useJSON {
		out, err := formatToolsListJSON(cfg.Tools)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}

	fmt.Print(formatToolsList(cfg.Tools))
	return nil
}

func runToolsEnable(cmd *cobra.Command, args []string) error {
	return setToolEnabled(args[0], true)
}

func runToolsDisable(cmd *cobra.Command, args []string) error {
	return setToolEnabled(args[0], false)
}

func setToolEnabled(name string, enabled bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ensureToolsPopulated(cfg)

	found := false
	for i := range cfg.Tools {
		if strings.EqualFold(cfg.Tools[i].Name, name) {
			cfg.Tools[i].Enabled = enabled
			found = true
			break
		}
	}

	if !found {
		known := make([]string, len(cfg.Tools))
		for i, t := range cfg.Tools {
			known[i] = t.Name
		}
		return fmt.Errorf("unknown tool %q — known tools: %s", name, strings.Join(known, ", "))
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Tool %s %s\n", name, action)
	return nil
}

// ensureToolsPopulated auto-populates cfg.Tools from detected tools if empty.
func ensureToolsPopulated(cfg *config.Config) {
	if len(cfg.Tools) > 0 {
		return
	}
	detected := tools.DetectTools()
	for _, t := range detected {
		cfg.Tools = append(cfg.Tools, config.ToolConfig{
			Name:    t.Name(),
			Enabled: true,
		})
	}
}

// formatToolsList returns a tab-formatted table of tools and their statuses.
func formatToolsList(toolCfgs []config.ToolConfig) string {
	if len(toolCfgs) == 0 {
		return "No tools detected. Install Claude or Cursor and try again.\n"
	}

	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSTATUS")
	for _, t := range toolCfgs {
		status := "enabled"
		if !t.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(w, "%s\t%s\n", t.Name, status)
	}
	w.Flush()
	return buf.String()
}

// formatToolsListJSON returns JSON-encoded tool configs.
func formatToolsListJSON(toolCfgs []config.ToolConfig) (string, error) {
	data, err := json.MarshalIndent(toolCfgs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode tools JSON: %w", err)
	}
	return string(data), nil
}
