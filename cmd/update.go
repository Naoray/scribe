package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/registry"
	isync "github.com/Naoray/scribe/internal/sync"
)

type updateOutput struct {
	lockPlanOutput
	Applied bool           `json:"applied"`
	Commits []updateCommit `json:"commits,omitempty"`
}

type updateCommit struct {
	Registry string `json:"registry"`
	SHA      string `json:"sha"`
	URL      string `json:"url"`
}

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh registry lockfiles after review",
		Args:  cobra.NoArgs,
		RunE:  runUpdate,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().Bool("apply", false, "Write refreshed lockfiles to registries")
	return markJSONSupported(cmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	jsonFlag := jsonFlagPassed(cmd)
	apply, _ := cmd.Flags().GetBool("apply")
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return err
	}
	client, err := factory.Client()
	if err != nil {
		return err
	}
	provider, err := factory.Provider()
	if err != nil {
		return err
	}
	st, err := factory.State()
	if err != nil {
		return err
	}
	repos := cfg.TeamRepos()
	plan, latest, err := buildLockPlan(cmd.Context(), repos, isync.WrapGitHubClient(client), provider, st)
	if err != nil {
		return err
	}
	out := updateOutput{lockPlanOutput: plan, Applied: false}
	if apply {
		pusher := registry.NewGitHubPusher(client)
		for _, repo := range repos {
			lf := latest[repo]
			if lf == nil {
				continue
			}
			commit, err := pushLockfile(cmd.Context(), pusher, repo, lf)
			if err != nil {
				return err
			}
			out.Commits = append(out.Commits, commit)
		}
		out.Applied = true
	}
	if jsonFlag {
		return renderMutatorEnvelope(cmd, out, envelope.StatusOK)
	}
	if !apply {
		if len(out.Updates) == 0 {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "No lockfile updates available.")
			return err
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out.lockPlanOutput)
	}
	for _, commit := range out.Commits {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", commit.Registry, commit.URL)
	}
	return nil
}

type lockfilePusher interface {
	LatestCommitSHA(ctx context.Context, owner, repo, branch string) (string, error)
	PushFilesAtomic(ctx context.Context, owner, repo, branch string, files map[string][]byte, message, expectedHead string) (registry.CommitResult, error)
}

func pushLockfile(ctx context.Context, pusher lockfilePusher, repo string, lf *lockfile.Lockfile) (updateCommit, error) {
	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return updateCommit{}, err
	}
	data, err := lf.Encode()
	if err != nil {
		return updateCommit{}, err
	}
	head, err := pusher.LatestCommitSHA(ctx, owner, repoName, "main")
	if err != nil {
		return updateCommit{}, err
	}
	commit, err := pusher.PushFilesAtomic(ctx, owner, repoName, "main", map[string][]byte{
		lockfile.Filename: data,
	}, "Update scribe.lock", head)
	if err != nil {
		return updateCommit{}, err
	}
	return updateCommit{Registry: repo, SHA: commit.SHA, URL: commit.URL}, nil
}
