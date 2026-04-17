package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PackageInstallPlan describes how a staged package should self-install.
// Resolution order (first match wins):
//  1. scribe.yaml → install.command / install.uninstall
//  2. executable setup at repo root
//  3. install.sh at repo root
//  4. package.json → run `bun install` when bun is available, else `npm install`
//  5. Makefile target install (run via `make install`)
//  6. No install command → empty Command (package is tracked but no-op)
type PackageInstallPlan struct {
	// Command is the shell command to run after writing the package dir.
	// Executed from within the package directory.
	Command string
	// Uninstall, if set, is the command to run when `scribe remove <name>`
	// is invoked for this package. Best-effort.
	Uninstall string
	// Source is a short description of where the command came from
	// ("scribe.yaml", "./setup", "install.sh", "package.json", "Makefile").
	Source string
}

// packageManifestInstall is the shallow subset of scribe.yaml this package
// cares about. A full manifest is parsed elsewhere (internal/manifest); here
// we only need the install block, so we decode straight into this shape.
type packageManifestInstall struct {
	Install struct {
		Command   string `yaml:"command"`
		Uninstall string `yaml:"uninstall"`
	} `yaml:"install"`
}

// ResolvePackageInstall walks the given package directory and returns a plan
// for how to execute its self-install. See PackageInstallPlan for the
// resolution order. An absent plan (empty Command) is not an error — the
// package is tracked but triggers no shell execution.
func ResolvePackageInstall(pkgDir string) (PackageInstallPlan, error) {
	if plan, ok, err := readScribeManifestInstall(pkgDir); err != nil {
		return PackageInstallPlan{}, err
	} else if ok {
		return plan, nil
	}

	if isExecutableFile(filepath.Join(pkgDir, "setup")) {
		return PackageInstallPlan{Command: "./setup", Source: "./setup"}, nil
	}
	if fileExists(filepath.Join(pkgDir, "install.sh")) {
		return PackageInstallPlan{Command: "sh install.sh", Source: "install.sh"}, nil
	}
	if fileExists(filepath.Join(pkgDir, "package.json")) {
		return PackageInstallPlan{
			Command: "if command -v bun >/dev/null 2>&1; then bun install; else npm install; fi",
			Source:  "package.json",
		}, nil
	}
	if fileExists(filepath.Join(pkgDir, "Makefile")) {
		return PackageInstallPlan{Command: "make install", Source: "Makefile"}, nil
	}
	return PackageInstallPlan{}, nil
}

func readScribeManifestInstall(pkgDir string) (PackageInstallPlan, bool, error) {
	for _, name := range []string{"scribe.yaml", "scribe.yml"} {
		path := filepath.Join(pkgDir, name)
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return PackageInstallPlan{}, false, fmt.Errorf("read %s: %w", name, err)
		}
		var pm packageManifestInstall
		if err := yaml.Unmarshal(data, &pm); err != nil {
			// Malformed scribe.yaml shouldn't crash install — a later phase
			// can surface a warning; here we just fall through to script
			// detection.
			return PackageInstallPlan{}, false, nil
		}
		if pm.Install.Command == "" && pm.Install.Uninstall == "" {
			return PackageInstallPlan{}, false, nil
		}
		return PackageInstallPlan{
			Command:   pm.Install.Command,
			Uninstall: pm.Install.Uninstall,
			Source:    name,
		}, true, nil
	}
	return PackageInstallPlan{}, false, nil
}

// RunPackageCommand executes cmd inside pkgDir using the provided executor.
// Stdout/stderr are captured. Empty cmd is a no-op (returns nil). A non-nil
// err indicates the command failed or the context was cancelled; callers
// are expected to roll back on install errors.
func RunPackageCommand(ctx context.Context, exec CommandExecutor, pkgDir, cmd string, timeout time.Duration) (string, string, error) {
	if cmd == "" {
		return "", "", nil
	}
	if timeout == 0 {
		timeout = defaultPackageTimeout
	}
	full := fmt.Sprintf("cd %s && %s", shellQuote(pkgDir), cmd)
	return exec.Execute(ctx, full, timeout)
}

// shellQuote wraps s in single quotes so it can be safely interpolated into
// a sh -c command string. Any single quote inside is escaped via '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
