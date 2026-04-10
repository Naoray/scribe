package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Method represents how scribe was installed.
type Method int

const (
	MethodHomebrew  Method = iota
	MethodGoInstall
	MethodCurlBinary
)

// String returns a human-readable label for the install method.
func (m Method) String() string {
	switch m {
	case MethodHomebrew:
		return "homebrew"
	case MethodGoInstall:
		return "go install"
	case MethodCurlBinary:
		return "direct binary"
	default:
		return "unknown"
	}
}

// Swappable for testing.
var (
	executablePath = os.Executable
	evalSymlinks   = filepath.EvalSymlinks
	brewListCheck  = defaultBrewListCheck
)

func defaultBrewListCheck(name string) bool {
	return exec.Command("brew", "list", name).Run() == nil
}

// DetectMethod determines how scribe was installed by inspecting the
// executable path. Uses path heuristics first (instant), falls back
// to `brew list` for ambiguous paths.
func DetectMethod() Method {
	exePath, err := executablePath()
	if err != nil {
		return MethodCurlBinary
	}

	resolved, err := evalSymlinks(exePath)
	if err != nil {
		resolved = exePath
	}

	if isHomebrewPath(resolved) {
		return MethodHomebrew
	}
	if strings.Contains(resolved, "/go/bin/") {
		return MethodGoInstall
	}

	// Ambiguous path — ask Homebrew directly.
	if brewListCheck("scribe") {
		return MethodHomebrew
	}

	return MethodCurlBinary
}

func isHomebrewPath(path string) bool {
	return strings.Contains(path, "/Cellar/") ||
		strings.Contains(path, "/opt/homebrew/") ||
		strings.Contains(path, "/linuxbrew/")
}

// NeedsUpgrade compares the current version against the latest release tag.
// Returns (isDevBuild, needsUpgrade).
// Dev builds return (true, false) — caller should skip upgrade.
// Equal versions return (false, false).
// Different versions return (false, true).
func NeedsUpgrade(current, latestTag string) (isDevBuild bool, needsUpgrade bool) {
	if current == "dev" || current == "" {
		return true, false
	}
	latest := strings.TrimPrefix(latestTag, "v")
	return false, current != latest
}
