package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"charm.land/huh/v2"
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
  scribe tools add gemini     # force-add a builtin tool
  scribe tools add aider      # add a custom tool
  scribe tools enable cursor  # enable a tool
  scribe tools disable cursor # disable a tool`,
		Args: cobra.NoArgs,
		RunE: runToolsList,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.AddCommand(newToolsAddCommand())
	cmd.AddCommand(newToolsEnableCommand())
	cmd.AddCommand(newToolsDisableCommand())
	return cmd
}

func newToolsAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a builtin or custom tool definition",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolsAdd,
	}
	cmd.Flags().String("detect", "", "Shell command used to detect the tool")
	cmd.Flags().String("install", "", "Shell command template used to install a skill")
	cmd.Flags().String("uninstall", "", "Shell command template used to uninstall a skill")
	cmd.Flags().String("path", "", "Optional installed-path template recorded in state")
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

	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		return err
	}

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	if useJSON {
		out, err := formatToolsListJSON(statuses)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}

	fmt.Print(formatToolsList(statuses))
	return nil
}

func runToolsAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	name := strings.TrimSpace(args[0])
	detect, _ := cmd.Flags().GetString("detect")
	install, _ := cmd.Flags().GetString("install")
	uninstall, _ := cmd.Flags().GetString("uninstall")
	pathTemplate, _ := cmd.Flags().GetString("path")

	if _, ok := tools.BuiltinByName(name); ok {
		upsertToolConfig(cfg, config.ToolConfig{
			Name:    name,
			Type:    tools.ToolTypeBuiltin,
			Enabled: true,
		})
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("Tool %s added\n", name)
		return nil
	}

	if isatty.IsTerminal(os.Stdin.Fd()) {
		if strings.TrimSpace(detect) == "" {
			_ = huh.NewInput().Title("Detect command (optional)").Value(&detect).Run()
		}
		if strings.TrimSpace(install) == "" {
			if err := huh.NewInput().Title("Install command").Value(&install).Run(); err != nil {
				return err
			}
		}
		if strings.TrimSpace(uninstall) == "" {
			if err := huh.NewInput().Title("Uninstall command").Value(&uninstall).Run(); err != nil {
				return err
			}
		}
		if strings.TrimSpace(pathTemplate) == "" {
			_ = huh.NewInput().Title("Installed path template (optional)").Value(&pathTemplate).Run()
		}
	}

	if strings.TrimSpace(install) == "" || strings.TrimSpace(uninstall) == "" {
		return fmt.Errorf("custom tool %q requires --install and --uninstall", name)
	}

	upsertToolConfig(cfg, config.ToolConfig{
		Name:      name,
		Type:      tools.ToolTypeCustom,
		Enabled:   true,
		Detect:    strings.TrimSpace(detect),
		Install:   strings.TrimSpace(install),
		Uninstall: strings.TrimSpace(uninstall),
		Path:      strings.TrimSpace(pathTemplate),
	})
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("Tool %s added\n", name)
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

	for i := range cfg.Tools {
		if strings.EqualFold(cfg.Tools[i].Name, name) {
			cfg.Tools[i].Enabled = enabled
			if err := cfg.Save(); err != nil {
				return err
			}
			printToolAction(name, enabled)
			return nil
		}
	}

	if _, ok := tools.BuiltinByName(name); ok {
		upsertToolConfig(cfg, config.ToolConfig{
			Name:    name,
			Type:    tools.ToolTypeBuiltin,
			Enabled: enabled,
		})
		if err := cfg.Save(); err != nil {
			return err
		}
		printToolAction(name, enabled)
		return nil
	}

	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		return err
	}
	known := make([]string, len(statuses))
	for i, st := range statuses {
		known[i] = st.Name
	}
	return fmt.Errorf("unknown tool %q — known tools: %s", name, strings.Join(known, ", "))
}

func printToolAction(name string, enabled bool) {
	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Tool %s %s\n", name, action)
}

func formatToolsList(statuses []tools.Status) string {
	if len(statuses) == 0 {
		return "No tools detected or configured.\n"
	}

	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tTYPE\tSTATUS\tDETECTED\tSOURCE")
	for _, t := range statuses {
		status := "enabled"
		if !t.Enabled {
			status = "disabled"
		}
		detected := "n/a"
		if t.DetectKnown {
			detected = "no"
			if t.Detected {
				detected = "yes"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Name, t.Type, status, detected, t.Source)
	}
	w.Flush()
	return buf.String()
}

func formatToolsListJSON(statuses []tools.Status) (string, error) {
	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode tools JSON: %w", err)
	}
	return string(data), nil
}

func upsertToolConfig(cfg *config.Config, tc config.ToolConfig) {
	for i := range cfg.Tools {
		if strings.EqualFold(cfg.Tools[i].Name, tc.Name) {
			cfg.Tools[i] = tc
			return
		}
	}
	cfg.Tools = append(cfg.Tools, tc)
}
