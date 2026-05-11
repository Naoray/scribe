package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/source"
)

type sourceFlagValues struct {
	source         string
	repo           string
	url            string
	ref            string
	path           string
	id             string
	registryAlias  string
	usedRegistryAs bool
}

func addSourceFlags(cmd *cobra.Command, includeRegistryAlias bool) {
	cmd.Flags().String("source", "", "Source shorthand, URL, or local path")
	cmd.Flags().String("repo", "", "GitHub owner/repo source")
	cmd.Flags().String("url", "", "Git or file source URL")
	cmd.Flags().String("ref", "", "Git ref for the source")
	cmd.Flags().String("path", "", "Subdirectory path within the source")
	cmd.Flags().String("id", "", "Stable source ID")
	if includeRegistryAlias {
		cmd.Flags().String("registry", "", "Legacy alias for --source")
	}
}

func readSourceFlags(cmd *cobra.Command) (sourceFlagValues, error) {
	values := sourceFlagValues{}
	values.source, _ = cmd.Flags().GetString("source")
	values.repo, _ = cmd.Flags().GetString("repo")
	values.url, _ = cmd.Flags().GetString("url")
	values.ref, _ = cmd.Flags().GetString("ref")
	values.path, _ = cmd.Flags().GetString("path")
	values.id, _ = cmd.Flags().GetString("id")

	if f := cmd.Flags().Lookup("registry"); f != nil {
		registry, _ := cmd.Flags().GetString("registry")
		if values.source != "" && registry != "" {
			return values, fmt.Errorf("--source and --registry cannot be used together")
		}
		if values.source == "" {
			values.source = registry
			values.registryAlias = registry
			values.usedRegistryAs = true
		}
	}

	return values, nil
}

func (v sourceFlagValues) hasAny() bool {
	return v.source != "" || v.repo != "" || v.url != "" || v.ref != "" || v.path != "" || v.id != ""
}

func (v sourceFlagValues) hasTyped() bool {
	return v.hasAny() && !v.usedRegistryAs
}

func sourceSpecFromFlags(v sourceFlagValues) (source.SourceSpec, source.SourceIdentity, string, error) {
	if !v.hasAny() {
		return source.SourceSpec{}, source.SourceIdentity{}, "", nil
	}

	var spec source.SourceSpec
	var err error
	switch {
	case v.source != "":
		if v.repo != "" || v.url != "" {
			return source.SourceSpec{}, source.SourceIdentity{}, "", fmt.Errorf("--source cannot be combined with --repo or --url")
		}
		spec, err = source.ParseSourceArg(v.source)
	case v.repo != "":
		if v.url != "" {
			return source.SourceSpec{}, source.SourceIdentity{}, "", fmt.Errorf("--repo and --url cannot be used together")
		}
		spec = source.SourceSpec{Type: source.SourceGitHub, Repo: v.repo}
	case v.url != "":
		spec, err = source.ParseSourceArg(v.url)
	case v.path != "":
		spec = source.SourceSpec{Type: source.SourceLocal, Path: v.path}
	default:
		return source.SourceSpec{}, source.SourceIdentity{}, "", fmt.Errorf("--ref, --path, and --id require --source, --repo, or --url")
	}
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, "", err
	}

	if v.ref != "" {
		spec.Ref = v.ref
	}
	if v.path != "" && spec.Type != source.SourceLocal {
		spec.Path = v.path
	}
	if v.id != "" {
		spec.ID = v.id
	}

	spec, ident, err := source.Canonicalize(spec)
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, "", err
	}
	display := sourceDisplay(spec, ident)
	return spec, ident, display, nil
}

func sourceDisplay(spec source.SourceSpec, ident source.SourceIdentity) string {
	if spec.ID != "" {
		return spec.ID
	}
	if spec.Type == source.SourceGitHub && spec.Path == "" && spec.Ref == "" {
		return spec.Repo
	}
	if ident.Key != "" {
		return ident.Key
	}
	if spec.Repo != "" {
		return spec.Repo
	}
	if spec.URL != "" {
		return spec.URL
	}
	return spec.Path
}

func parseInstallRefForCommand(ref string) (source.SourceSpec, source.SourceIdentity, string, string, error) {
	parsed, err := source.ParseInstallRef(ref)
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, "", "", err
	}
	spec, ident, err := source.Canonicalize(parsed.Source)
	if err != nil {
		return source.SourceSpec{}, source.SourceIdentity{}, "", "", err
	}
	if spec.Type == source.SourceGitHub && spec.Path == "" && spec.Ref == "" {
		ident.Key = spec.Repo
	}
	return spec, ident, sourceDisplay(spec, ident), strings.TrimSpace(parsed.Skill), nil
}
