package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/projectfile"
)

type projectEditOptions struct {
	kind    string
	action  string
	field   string
	noSync  bool
	block   bool
	json    bool
	mutator func(*projectfile.ProjectFile) *[]string
}

type projectEditOutput struct {
	ProjectRoot string                  `json:"project_root"`
	ProjectFile string                  `json:"project_file"`
	Kind        string                  `json:"kind"`
	Action      string                  `json:"action"`
	Field       string                  `json:"field"`
	Synced      bool                    `json:"synced"`
	Results     []projectEditNameResult `json:"results"`
	List        []string                `json:"list"`
}

type projectEditNameResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Changed bool   `json:"changed"`
}

func newProjectKitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kit",
		Short: "Manage .scribe.yaml kit entries",
	}
	cmd.AddCommand(
		newProjectEditCommand(projectEditOptions{kind: "kit", action: "add", field: "kits", mutator: func(pf *projectfile.ProjectFile) *[]string { return &pf.Kits }}),
		newProjectEditCommand(projectEditOptions{kind: "kit", action: "remove", field: "kits", mutator: func(pf *projectfile.ProjectFile) *[]string { return &pf.Kits }}),
	)
	return cmd
}

func newProjectSnippetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snippet",
		Short: "Manage .scribe.yaml snippet entries",
	}
	cmd.AddCommand(
		newProjectEditCommand(projectEditOptions{kind: "snippet", action: "add", field: "snippets", mutator: func(pf *projectfile.ProjectFile) *[]string { return &pf.Snippets }}),
		newProjectEditCommand(projectEditOptions{kind: "snippet", action: "remove", field: "snippets", mutator: func(pf *projectfile.ProjectFile) *[]string { return &pf.Snippets }}),
	)
	return cmd
}

func newProjectEditCommand(opts projectEditOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   opts.action + " <name>...",
		Short: projectEditShort(opts),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			if opts.kind == "skill" && opts.action == "remove" {
				opts.block, _ = cmd.Flags().GetBool("block")
				if opts.block {
					opts.field = "remove"
					opts.mutator = func(pf *projectfile.ProjectFile) *[]string { return &pf.Remove }
				}
			}
			opts.noSync, _ = cmd.Flags().GetBool("no-sync")
			return runProjectEdit(cmd, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.noSync, "no-sync", false, "Only edit .scribe.yaml; do not run project sync")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	if opts.kind == "skill" && opts.action == "remove" {
		cmd.Flags().BoolVar(&opts.block, "block", false, "Add the skill to remove: to suppress a kit-provided skill")
	}
	return markJSONSupported(cmd)
}

func projectEditShort(opts projectEditOptions) string {
	if opts.action == "add" {
		return "Add " + opts.kind + " entries to .scribe.yaml"
	}
	return "Remove " + opts.kind + " entries from .scribe.yaml"
}

func runProjectEdit(cmd *cobra.Command, args []string, opts projectEditOptions) error {
	for _, name := range args {
		if err := validateProjectEditName(opts.kind, name); err != nil {
			return err
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	projectPath, err := projectfile.Find(wd)
	if err != nil {
		return err
	}
	if projectPath == "" {
		return clierrors.Wrap(errors.New("no .scribe.yaml found"), "PROJECT_FILE_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithRemediation("Run inside a scribe project or create .scribe.yaml first."),
		)
	}

	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return err
	}
	list := opts.mutator(pf)
	out := projectEditOutput{
		ProjectRoot: filepath.Dir(projectPath),
		ProjectFile: projectPath,
		Kind:        opts.kind,
		Action:      opts.action,
		Field:       opts.field,
		Synced:      !opts.noSync && opts.kind != "snippet",
	}
	for _, name := range args {
		var changed bool
		if opts.action == "add" || (opts.kind == "skill" && opts.block) {
			*list, changed = projectfile.AddUnique(*list, name)
			out.Results = append(out.Results, projectEditNameResult{Name: name, Status: addStatus(changed), Changed: changed})
			continue
		}
		*list, changed = projectfile.RemoveName(*list, name)
		out.Results = append(out.Results, projectEditNameResult{Name: name, Status: removeStatus(changed), Changed: changed})
	}
	out.List = cloneProjectEditList(*list)

	if err := projectfile.Save(projectPath, pf); err != nil {
		return err
	}
	if !opts.noSync && opts.kind != "snippet" {
		if err := runProjectEditSync(cmd, opts.json); err != nil {
			return err
		}
	}

	if opts.json {
		return renderMutatorEnvelope(cmd, out, envelope.StatusOK)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: %s %s (%s)\n", projectfile.Filename, opts.kind, opts.action, strings.Join(args, ", "))
	if !opts.noSync && opts.kind == "snippet" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Snippet projection was not run; run `scribe sync` to project snippets.")
	}
	return nil
}

func validateProjectEditName(kind, name string) error {
	if kind == "kit" {
		return validateKitName(name)
	}
	return validateProjectRefName(kind, name)
}

func validateProjectRefName(kind, name string) error {
	trimmed := strings.TrimSpace(name)
	invalid := trimmed == "" ||
		trimmed != name ||
		strings.HasPrefix(trimmed, "/") ||
		strings.Contains(trimmed, `\`) ||
		strings.Contains(trimmed, "..")
	if invalid {
		return invalidProjectRefName(kind, name)
	}

	if idx := strings.LastIndex(trimmed, ":"); idx >= 0 {
		qualifier := trimmed[:idx]
		refName := trimmed[idx+1:]
		if qualifier == "" || refName == "" || strings.Contains(refName, "/") {
			return invalidProjectRefName(kind, name)
		}
		if strings.Contains(qualifier, "/") {
			parts := strings.Split(qualifier, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return invalidProjectRefName(kind, name)
			}
		}
		return nil
	}

	if strings.Contains(trimmed, "/") {
		return invalidProjectRefName(kind, name)
	}
	return nil
}

func invalidProjectRefName(kind, name string) error {
	code := "PROJECT_REF_INVALID"
	if kind == "skill" {
		code = "SKILL_REF_INVALID"
	} else if kind == "snippet" {
		code = "SNIPPET_REF_INVALID"
	}
	return clierrors.Wrap(fmt.Errorf("invalid %s ref %q", kind, name), code, clierrors.ExitValid,
		clierrors.WithResource(name),
		clierrors.WithRemediation("Use a local name, registry:name, or owner/repo:name without path traversal."),
	)
}

func cloneProjectEditList(list []string) []string {
	if len(list) == 0 {
		return []string{}
	}
	return append([]string(nil), list...)
}

func runProjectEditSync(cmd *cobra.Command, jsonMode bool) error {
	syncCmd := cmd
	if jsonMode {
		syncCmd = &cobra.Command{}
		syncCmd.SetOut(io.Discard)
		syncCmd.SetErr(cmd.ErrOrStderr())
		syncCmd.SetContext(cmd.Context())
	}
	return runProjectSync(syncCmd, &projectSyncOptions{})
}

func addStatus(changed bool) string {
	if changed {
		return "added"
	}
	return "present"
}

func removeStatus(changed bool) string {
	if changed {
		return "removed"
	}
	return "absent"
}
