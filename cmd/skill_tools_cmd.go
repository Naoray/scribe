package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/state"
)

func newSkillToolsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools <skill>",
		Short: "Show or change which AI tools a skill is active in",
		Long: `Show the current tool assignment for a skill, or modify it.

Without flags, prints the skill's current tool mode and active tools.
Use --enable / --disable to add or remove specific tools.
Use --reset to return to inherit mode (tracks global tool settings).

Examples:
  scribe skill tools tdd                    # show current assignment
  scribe skill tools tdd --enable cursor    # add cursor
  scribe skill tools tdd --disable gemini   # remove gemini
  scribe skill tools tdd --enable claude --disable codex
  scribe skill tools tdd --reset            # revert to global defaults`,
		Args: cobra.ExactArgs(1),
		RunE: runSkillTools,
	}
	cmd.Flags().StringSlice("enable", nil, "Enable this skill for the given tool(s)")
	cmd.Flags().StringSlice("disable", nil, "Disable this skill for the given tool(s)")
	cmd.Flags().Bool("reset", false, "Revert to inherit mode (tracks globally-enabled tools)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.MarkFlagsMutuallyExclusive("enable", "reset")
	cmd.MarkFlagsMutuallyExclusive("disable", "reset")
	return cmd
}

func runSkillTools(cmd *cobra.Command, args []string) error {
	name := args[0]
	enableFlag, _ := cmd.Flags().GetStringSlice("enable")
	disableFlag, _ := cmd.Flags().GetStringSlice("disable")
	resetFlag, _ := cmd.Flags().GetBool("reset")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	factory := newCommandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	installed, ok := st.Installed[name]
	if !ok {
		return fmt.Errorf("skill %q is not installed — run `scribe list` to see managed skills", name)
	}
	if installed.IsPackage() {
		return fmt.Errorf("skill %q is a package — per-skill tool assignment does not apply", name)
	}

	// No mutation flags → show current state.
	if len(enableFlag) == 0 && len(disableFlag) == 0 && !resetFlag {
		return renderSkillTools(name, installed, useJSON)
	}

	// Compute desired mode + tool list.
	desiredMode := installed.ToolsMode
	current := append([]string(nil), installed.Tools...)
	var desired []string

	switch {
	case resetFlag:
		desiredMode = state.ToolsModeInherit
		desired = nil

	default:
		desiredMode = state.ToolsModePinned
		desired = state.NormalizeToolSelection(current)

		if len(enableFlag) > 0 {
			desired = state.NormalizeToolSelection(append(desired, splitCSV(enableFlag)...))
		}
		if len(disableFlag) > 0 {
			drop := setOf(splitCSV(disableFlag))
			kept := desired[:0]
			for _, t := range desired {
				if !drop[t] {
					kept = append(kept, t)
				}
			}
			desired = kept
		}

		if len(desired) == 0 {
			return fmt.Errorf("cannot disable all tools for %q — use --reset to revert to global defaults", name)
		}
	}

	result, err := applySkillToolSelection(cfg, st, name, desiredMode, desired)
	if err != nil {
		return err
	}

	if useJSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	renderSkillEditText(result)
	return nil
}

func renderSkillTools(name string, skill state.InstalledSkill, useJSON bool) error {
	mode := string(skill.ToolsMode)
	if mode == "" {
		mode = "inherit"
	}
	if useJSON {
		out, _ := json.MarshalIndent(map[string]any{
			"name":       name,
			"tools_mode": mode,
			"tools":      skill.Tools,
		}, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	fmt.Printf("%s\n", name)
	fmt.Printf("  mode:  %s\n", mode)
	if len(skill.Tools) > 0 {
		fmt.Printf("  tools: %s\n", strings.Join(skill.Tools, ", "))
	} else if mode == "inherit" {
		fmt.Printf("  tools: (inherits global settings)\n")
	} else {
		fmt.Printf("  tools: (none)\n")
	}
	return nil
}
