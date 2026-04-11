package cmd

import (
	"context"
	"fmt"
	"os"

	gogithub "github.com/google/go-github/v69/github"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/upgrade"
)

func newUpgradeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade scribe to the latest version",
		Long:  "Detects how scribe was installed and upgrades using the appropriate method.",
		Args:  cobra.NoArgs,
		RunE:  runUpgrade,
	}
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	factory := newCommandFactory()

	// Dev builds should not attempt self-upgrade.
	isDevBuild, _ := upgrade.NeedsUpgrade(Version, "")
	if isDevBuild {
		fmt.Println("Running development build, skipping upgrade.")
		return nil
	}

	// Detect install method.
	method := upgrade.DetectMethod()
	fmt.Printf("Installed via: %s\n", method)

	// Fetch latest release.
	_, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}
	release, err := client.LatestRelease(ctx, "Naoray", "scribe")
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	latestTag := release.GetTagName()
	_, needsUpgrade := upgrade.NeedsUpgrade(Version, latestTag)
	if !needsUpgrade {
		fmt.Printf("Already up to date (%s)\n", latestTag)
		return nil
	}

	fmt.Printf("Upgrading v%s → %s...\n", Version, latestTag)

	isTTY := isatty.IsTerminal(os.Stdout.Fd())
	return doUpgrade(ctx, method, release, client, isTTY)
}

func doUpgrade(ctx context.Context, method upgrade.Method, release *gogithub.RepositoryRelease, client *github.Client, isTTY bool) error {
	var spin *spinState
	if isTTY {
		spin = startSpinner(os.Stdout, "Downloading and installing...")
	}

	var upgradeErr error
	switch method {
	case upgrade.MethodHomebrew:
		if spin != nil {
			spin.stop()
		}
		// Brew has its own progress output — don't wrap with spinner.
		_, upgradeErr = upgrade.UpgradeHomebrew(ctx)
	case upgrade.MethodGoInstall:
		_, upgradeErr = upgrade.UpgradeGoInstall(ctx)
		if spin != nil {
			spin.stop()
		}
	case upgrade.MethodCurlBinary:
		upgradeErr = upgrade.UpgradeBinary(ctx, release, client)
		if spin != nil {
			spin.stop()
		}
	}

	if upgradeErr != nil {
		return fmt.Errorf("upgrade failed: %w", upgradeErr)
	}

	fmt.Printf("Successfully upgraded to %s\n", release.GetTagName())
	return nil
}
