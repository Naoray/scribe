package prereq

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/paths"
)

// AuthResult describes the GitHub authentication status.
type AuthResult struct {
	OK     bool   `json:"ok"`
	Method string `json:"method,omitempty"` // "gh_cli", "GITHUB_TOKEN", "config", ""
}

// DirResult describes the ~/.scribe/ directory status.
type DirResult struct {
	OK   bool   `json:"ok"`
	Path string `json:"path"`
}

// ConnectionsResult describes existing team connections.
type ConnectionsResult struct {
	Repos []string `json:"repos,omitempty"`
}

// Result holds all prerequisite check outcomes.
type Result struct {
	GitHubAuth  AuthResult        `json:"github_auth"`
	ScribeDir   DirResult         `json:"scribe_dir"`
	Connections ConnectionsResult `json:"connections"`
	ConfigErr   string            `json:"config_error,omitempty"`
}

// Check runs all prerequisite checks and returns the result.
// ConfigErr is non-nil if the config file exists but is malformed.
func Check() Result {
	cfg, err := config.Load()
	r := Result{
		GitHubAuth:  checkAuth(cfg),
		ScribeDir:   checkDir(),
		Connections: checkConnections(cfg),
	}
	if err != nil {
		r.ConfigErr = err.Error()
	}
	return r
}

func checkAuth(cfg *config.Config) AuthResult {
	// 1. gh auth token (with timeout to avoid keychain hangs)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output(); err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return AuthResult{OK: true, Method: "gh_cli"}
		}
	}
	// 2. GITHUB_TOKEN env
	if os.Getenv("GITHUB_TOKEN") != "" {
		return AuthResult{OK: true, Method: "GITHUB_TOKEN"}
	}
	// 3. Config file token
	if cfg != nil && cfg.Token != "" {
		return AuthResult{OK: true, Method: "config"}
	}
	return AuthResult{OK: false}
}

func checkDir() DirResult {
	dir, err := paths.ScribeDir()
	if err != nil {
		return DirResult{OK: false, Path: "~/.scribe"}
	}
	_, err = os.Stat(dir)
	return DirResult{OK: err == nil, Path: dir}
}

func checkConnections(cfg *config.Config) ConnectionsResult {
	if cfg == nil {
		return ConnectionsResult{}
	}
	return ConnectionsResult{Repos: cfg.TeamRepos()}
}
