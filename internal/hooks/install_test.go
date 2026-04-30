package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstallEmptyClaudeDir(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	claudeDir := makeClaudeDir(t, home)

	status, err := (&Installer{HomeDir: home}).Install()
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if status != StatusInstalled {
		t.Fatalf("Install() status = %v, want %v", status, StatusInstalled)
	}

	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	assertInstalledScript(t, scriptPath)
	assertScribeHookCount(t, filepath.Join(claudeDir, "settings.json"), scriptPath, 1)
}

func TestInstallerInstallWithoutClaudeDir(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	status, err := (&Installer{HomeDir: home}).Install()
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if status != StatusNotApplicable {
		t.Fatalf("Install() status = %v, want %v", status, StatusNotApplicable)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("Install() created .claude or returned unexpected stat error: %v", err)
	}
}

func TestInstallerInstallAlreadyPresent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	claudeDir := makeClaudeDir(t, home)
	installer := &Installer{HomeDir: home}

	status, err := installer.Install()
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if status != StatusInstalled {
		t.Fatalf("first Install() status = %v, want %v", status, StatusInstalled)
	}

	status, err = installer.Install()
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if status != StatusAlreadyInstalled {
		t.Fatalf("second Install() status = %v, want %v", status, StatusAlreadyInstalled)
	}

	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	assertScribeHookCount(t, filepath.Join(claudeDir, "settings.json"), scriptPath, 1)
}

func TestInstallerInstallPreservesOtherSettingsAndHooks(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	claudeDir := makeClaudeDir(t, home)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	writeJSON(t, settingsPath, map[string]any{
		"theme": "dark",
		"mcpServers": map[string]any{
			"local": map[string]any{"command": "mcp-local"},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "/usr/local/bin/pre-tool"},
					},
				},
			},
			"PostToolUseFailure": []any{
				map[string]any{
					"matcher": "Write",
					"hooks": []any{
						map[string]any{"type": "command", "command": "/tmp/user-scribe-hook.sh"},
					},
				},
			},
		},
	})

	status, err := (&Installer{HomeDir: home}).Install()
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if status != StatusInstalled {
		t.Fatalf("Install() status = %v, want %v", status, StatusInstalled)
	}

	var settings map[string]any
	readJSON(t, settingsPath, &settings)
	if settings["theme"] != "dark" {
		t.Fatalf("theme = %v, want dark", settings["theme"])
	}
	if _, ok := settings["mcpServers"].(map[string]any)["local"]; !ok {
		t.Fatal("mcpServers.local was not preserved")
	}

	hooksMap := settings["hooks"].(map[string]any)
	if len(hooksMap["PreToolUse"].([]any)) != 1 {
		t.Fatal("PreToolUse hooks were not preserved")
	}
	postFailure := hooksMap["PostToolUseFailure"].([]any)
	if len(postFailure) != 2 {
		t.Fatalf("PostToolUseFailure entries = %d, want 2", len(postFailure))
	}

	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	assertScribeHookCount(t, settingsPath, scriptPath, 1)
	assertHookCommandPresent(t, settingsPath, "/tmp/user-scribe-hook.sh")
}

func TestInstallerUninstallRemovesHookAndScript(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	claudeDir := makeClaudeDir(t, home)
	installer := &Installer{HomeDir: home}
	if _, err := installer.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if err := installer.Uninstall(); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if err := installer.Uninstall(); err != nil {
		t.Fatalf("second Uninstall() error = %v", err)
	}

	scriptPath := filepath.Join(claudeDir, "hooks", "scribe-hook.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("script stat after Uninstall() error = %v, want not exist", err)
	}
	assertScribeHookCount(t, filepath.Join(claudeDir, "settings.json"), scriptPath, 0)
}

func TestInstallerCurrentStatus(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	installer := &Installer{HomeDir: home}

	status, err := installer.CurrentStatus()
	if err != nil {
		t.Fatalf("CurrentStatus() without .claude error = %v", err)
	}
	if status != StatusNotApplicable {
		t.Fatalf("CurrentStatus() without .claude = %v, want %v", status, StatusNotApplicable)
	}

	makeClaudeDir(t, home)
	status, err = installer.CurrentStatus()
	if err != nil {
		t.Fatalf("CurrentStatus() before install error = %v", err)
	}
	if status != StatusInstalled {
		t.Fatalf("CurrentStatus() before install = %v, want %v", status, StatusInstalled)
	}

	if _, err := installer.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	status, err = installer.CurrentStatus()
	if err != nil {
		t.Fatalf("CurrentStatus() after install error = %v", err)
	}
	if status != StatusAlreadyInstalled {
		t.Fatalf("CurrentStatus() after install = %v, want %v", status, StatusAlreadyInstalled)
	}
}

func TestInstallerInstallInvalidSettingsJSONErrors(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	claudeDir := makeClaudeDir(t, home)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"hooks":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&Installer{HomeDir: home}).Install()
	if err == nil {
		t.Fatal("Install() error = nil, want parse error")
	}

	data, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != `{"hooks":` {
		t.Fatalf("settings.json was overwritten on parse error: %q", data)
	}
}

func makeClaudeDir(t *testing.T, home string) string {
	t.Helper()

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return claudeDir
}

func assertInstalledScript(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed script: %v", err)
	}
	if string(data) != string(Script()) {
		t.Fatal("installed script content does not match embedded script")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat installed script: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %v, want 0755", info.Mode().Perm())
	}
}

func assertScribeHookCount(t *testing.T, settingsPath, scriptPath string, want int) {
	t.Helper()

	var settings map[string]any
	readJSON(t, settingsPath, &settings)

	count := 0
	for _, hook := range hookCommands(t, settings) {
		if hook == scriptPath {
			count++
		}
	}
	if count != want {
		t.Fatalf("managed scribe hook count = %d, want %d", count, want)
	}
}

func assertHookCommandPresent(t *testing.T, settingsPath, command string) {
	t.Helper()

	var settings map[string]any
	readJSON(t, settingsPath, &settings)
	for _, hook := range hookCommands(t, settings) {
		if hook == command {
			return
		}
	}
	t.Fatalf("hook command %q not found", command)
}

func hookCommands(t *testing.T, settings map[string]any) []string {
	t.Helper()

	hooksValue, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	entries, ok := hooksValue["PostToolUseFailure"].([]any)
	if !ok {
		return nil
	}

	var commands []string
	for _, entryValue := range entries {
		entry, ok := entryValue.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := entry["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			if !ok {
				continue
			}
			command, _ := hook["command"].(string)
			commands = append(commands, command)
		}
	}
	return commands
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON %s: %v", path, err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("parse JSON %s: %v\n%s", path, err, data)
	}
}
