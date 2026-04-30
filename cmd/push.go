package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/discovery"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type pushOutput struct {
	Skill     string `json:"skill"`
	Registry  string `json:"registry"`
	CommitSHA string `json:"commit_sha"`
	CommitURL string `json:"commit_url"`
}

type pushAuthClient interface {
	IsAuthenticated() bool
	AuthenticatedUser(ctx context.Context) (gh.AuthenticatedUser, error)
}

var (
	pushGitConfigEmail = gitConfigUserEmail
	newRegistryPusher  = registry.NewGitHubPusher
)

func newPushCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <skill>",
		Short: "Push local skill edits to their source registry",
		Args:  cobra.ExactArgs(1),
		RunE:  runPush,
	}
	return markJSONSupported(cmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	factory := commandFactory()
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	client, err := factory.Client()
	if err != nil {
		return err
	}
	result, err := runPushWithDeps(cmd.Context(), args[0], st, client, newRegistryPusher(client), pushGitConfigEmail)
	if err != nil {
		return err
	}

	out := pushOutput{
		Skill:     result.Skill,
		Registry:  result.Registry,
		CommitSHA: result.CommitSHA,
		CommitURL: result.CommitURL,
	}
	if jsonFlagPassed(cmd) {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(out); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed %s to %s at %s\n", out.Skill, out.Registry, out.CommitSHA)
	return nil
}

func runPushWithDeps(ctx context.Context, skillName string, st *state.State, auth pushAuthClient, pusher registry.GitHubPusher, gitEmail func(context.Context) string) (registry.PushResult, error) {
	installed, ok := st.Installed[skillName]
	if !ok {
		err := fmt.Errorf("skill %q not found", skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "SKILL_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithResource(skillName),
			clierrors.WithRemediation("Run `scribe list` to see installed skills."),
		)
	}
	if installed.IsPackage() {
		err := fmt.Errorf("skill %q is a package and cannot be pushed with this command", skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_UNSUPPORTED_PACKAGE", clierrors.ExitConflict,
			clierrors.WithResource(skillName),
		)
	}

	source, ok := pushSource(installed)
	if !ok {
		err := fmt.Errorf("skill %q has no registry source metadata", skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_SOURCE_MISSING", clierrors.ExitPerm,
			clierrors.WithResource(skillName),
			clierrors.WithRemediation("install or sync this skill from a registry before pushing"),
		)
	}
	if strings.TrimSpace(source.Author) == "" {
		err := fmt.Errorf("skill %q has no recorded author", skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_AUTHOR_MISSING", clierrors.ExitPerm,
			clierrors.WithResource(skillName),
			clierrors.WithRemediation("re-sync the skill from a registry that declares an author"),
		)
	}
	if auth == nil || !auth.IsAuthenticated() {
		err := errors.New("GitHub authentication required")
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_AUTH_REQUIRED", clierrors.ExitPerm,
			clierrors.WithRemediation("run `gh auth login` or set GITHUB_TOKEN"),
		)
	}
	if !authorMatches(ctx, source.Author, auth, gitEmail) {
		err := fmt.Errorf("local user is not the author of %q", skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_AUTHOR_MISMATCH", clierrors.ExitPerm,
			clierrors.WithResource(skillName),
			clierrors.WithRemediation("only the recorded skill author may push updates"),
		)
	}

	storeDir, err := tools.StoreDir()
	if err != nil {
		return registry.PushResult{}, fmt.Errorf("resolve store dir: %w", err)
	}
	skillDir := filepath.Join(storeDir, skillName)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return registry.PushResult{}, clierrors.Wrap(err, "SKILL_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithResource(skillName),
				clierrors.WithRemediation("Run `scribe sync` before pushing this skill."),
			)
		}
		return registry.PushResult{}, fmt.Errorf("stat SKILL.md: %w", err)
	}

	meta, err := discovery.ReadSkillMetadata(skillDir)
	if err != nil {
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_VALIDATION_FAILED", clierrors.ExitValid,
			clierrors.WithResource(skillName),
		)
	}
	if err := discovery.ValidateAgentSkillMetadata(meta); err != nil {
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_VALIDATION_FAILED", clierrors.ExitValid,
			clierrors.WithResource(skillName),
		)
	}
	if meta.Name != skillName {
		err := fmt.Errorf("SKILL.md name %q does not match requested skill %q", meta.Name, skillName)
		return registry.PushResult{}, clierrors.Wrap(err, "PUSH_VALIDATION_FAILED", clierrors.ExitValid,
			clierrors.WithResource(skillName),
		)
	}

	return registry.PushSkill(ctx, pusher, skillName, skillDir, source)
}

func pushSource(installed state.InstalledSkill) (state.SkillSource, bool) {
	for _, source := range installed.Sources {
		if source.PushRegistry() != "" {
			return source, true
		}
	}
	return state.SkillSource{}, false
}

func authorMatches(ctx context.Context, author string, auth pushAuthClient, gitEmail func(context.Context) string) bool {
	want := normalizeIdentity(author)
	if want == "" {
		return false
	}
	if gitEmail != nil && normalizeIdentity(gitEmail(ctx)) == want {
		return true
	}
	user, err := auth.AuthenticatedUser(ctx)
	if err == nil {
		if normalizeIdentity(user.Login) == want {
			return true
		}
		for _, email := range user.Emails {
			if normalizeIdentity(email) == want {
				return true
			}
		}
	}
	return false
}

func normalizeIdentity(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.Contains(value, "<") && strings.Contains(value, ">") {
		before, after, ok := strings.Cut(value, "<")
		if ok {
			_ = before
			email, _, ok := strings.Cut(after, ">")
			if ok {
				return strings.TrimSpace(email)
			}
		}
	}
	return value
}

func gitConfigUserEmail(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "config", "user.email").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
