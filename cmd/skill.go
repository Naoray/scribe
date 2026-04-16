package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func newSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Inspect or modify individual installed skills",
		Long:  `Per-skill management for choosing tools and repairing managed drift.`,
	}
	cmd.AddCommand(newSkillEditCommand(), newSkillToolsCommand(), newSkillRepairCommand())
	return cmd
}

func newSkillEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Change which AI tools a skill installs into",
		Long: `Pin a skill to a specific subset of AI tools, or return it to inherit mode
(where it tracks globally-enabled tools).

Examples:
  scribe skill edit commit --tools claude,cursor   # pin to claude + cursor
  scribe skill edit commit --add codex             # add codex, keep the rest
  scribe skill edit commit --remove cursor         # drop cursor
  scribe skill edit commit --inherit               # revert to global defaults
  scribe skill edit commit --json                  # machine output`,
		Args: cobra.ExactArgs(1),
		RunE: runSkillEdit,
	}
	cmd.Flags().StringSlice("tools", nil, "Comma-separated list of tools to pin this skill to")
	cmd.Flags().StringSlice("add", nil, "Add one or more tools to the current pin set")
	cmd.Flags().StringSlice("remove", nil, "Remove one or more tools from the current pin set")
	cmd.Flags().Bool("inherit", false, "Return the skill to inherit mode (tracks global tool settings)")
	cmd.Flags().Bool("pin", false, "Keep the current Tools list but mark as pinned")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.MarkFlagsMutuallyExclusive("tools", "inherit")
	cmd.MarkFlagsMutuallyExclusive("tools", "add")
	cmd.MarkFlagsMutuallyExclusive("tools", "remove")
	cmd.MarkFlagsMutuallyExclusive("inherit", "pin")
	cmd.MarkFlagsMutuallyExclusive("inherit", "add")
	cmd.MarkFlagsMutuallyExclusive("inherit", "remove")
	return cmd
}

type skillEditResult struct {
	Name      string   `json:"name"`
	ToolsMode string   `json:"tools_mode"`
	Tools     []string `json:"tools"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
}

type skillRepairResult struct {
	Name   string `json:"name"`
	Tool   string `json:"tool"`
	Source string `json:"source"`
}

func newSkillRepairCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair <name>",
		Short: "Resolve managed drift for a skill projection",
		Args:  cobra.ExactArgs(1),
		RunE:  runSkillRepair,
	}
	cmd.Flags().String("tool", "", "Tool projection to repair")
	cmd.Flags().String("from", "managed", "Conflict winner: managed or tool")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	if err := cmd.MarkFlagRequired("tool"); err != nil {
		panic(err)
	}
	return cmd
}

func runSkillEdit(cmd *cobra.Command, args []string) error {
	name := args[0]

	toolsFlag, _ := cmd.Flags().GetStringSlice("tools")
	addFlag, _ := cmd.Flags().GetStringSlice("add")
	removeFlag, _ := cmd.Flags().GetStringSlice("remove")
	inheritFlag, _ := cmd.Flags().GetBool("inherit")
	pinFlag, _ := cmd.Flags().GetBool("pin")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())

	if len(toolsFlag) == 0 && len(addFlag) == 0 && len(removeFlag) == 0 && !inheritFlag && !pinFlag {
		return fmt.Errorf("skill edit: supply one of --tools, --add, --remove, --inherit, or --pin")
	}

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
		return fmt.Errorf("skill %q is not installed (run `scribe list` to see managed skills)", name)
	}
	if installed.Type == "package" {
		return fmt.Errorf("skill %q is a package — per-skill tool pinning does not apply", name)
	}

	currentTools := append([]string(nil), installed.Tools...)

	// Compute desired tool list + mode.
	var desired []string
	desiredMode := installed.ToolsMode

	switch {
	case inheritFlag:
		desiredMode = state.ToolsModeInherit
		desired = nil
	case len(toolsFlag) > 0:
		desiredMode = state.ToolsModePinned
		desired = state.NormalizeToolSelection(splitCSV(toolsFlag))
	case pinFlag && len(addFlag) == 0 && len(removeFlag) == 0:
		desiredMode = state.ToolsModePinned
		desired = state.NormalizeToolSelection(currentTools)
	default:
		// --add / --remove (with optional --pin implied)
		desiredMode = state.ToolsModePinned
		desired = state.NormalizeToolSelection(currentTools)
		if len(addFlag) > 0 {
			desired = state.NormalizeToolSelection(append(desired, splitCSV(addFlag)...))
		}
		if len(removeFlag) > 0 {
			drop := make(map[string]bool, len(removeFlag))
			for _, t := range splitCSV(removeFlag) {
				drop[t] = true
			}
			kept := desired[:0]
			for _, t := range desired {
				if !drop[t] {
					kept = append(kept, t)
				}
			}
			desired = kept
		}
	}
	if desiredMode == state.ToolsModePinned && len(desired) == 0 {
		return fmt.Errorf("cannot pin skill %q to zero tools — use --inherit to revert", name)
	}

	result, err := applySkillToolSelection(cfg, st, args[0], desiredMode, desired)
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

func renderSkillEditText(r skillEditResult) {
	fmt.Printf("Updated %s\n", r.Name)
	fmt.Printf("  mode:  %s\n", r.ToolsMode)
	fmt.Printf("  tools: %s\n", strings.Join(r.Tools, ", "))
	if len(r.Added) > 0 {
		fmt.Printf("  +     %s\n", strings.Join(r.Added, ", "))
	}
	if len(r.Removed) > 0 {
		fmt.Printf("  -     %s\n", strings.Join(r.Removed, ", "))
	}
}

func runSkillRepair(cmd *cobra.Command, args []string) error {
	toolName, _ := cmd.Flags().GetString("tool")
	source, _ := cmd.Flags().GetString("from")
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

	name := args[0]
	result, err := applySkillRepair(cfg, st, name, toolName, source)
	if err != nil {
		return err
	}
	if useJSON {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	fmt.Printf("Repaired %s for %s using %s as the source of truth\n", name, toolName, source)
	return nil
}

// splitCSV flattens slices like ["a,b", "c"] into ["a", "b", "c"] so users
// can pass either --tools a,b or --tools a --tools b.
func splitCSV(in []string) []string {
	var out []string
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func setOf(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func mustStoreDir() string {
	d, err := tools.StoreDir()
	if err != nil {
		panic(fmt.Sprintf("resolve store dir: %v", err))
	}
	return d
}
