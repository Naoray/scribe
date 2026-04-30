package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	hookFileName = "scribe-hook.sh"
	hookEvent    = "PostToolUseFailure"
	hookMatcher  = "*"
)

// Status describes the result of a hook installer operation.
type Status int

const (
	StatusInstalled Status = iota
	StatusAlreadyInstalled
	StatusNotApplicable
)

// Installer installs the embedded Scribe hook into Claude Code.
type Installer struct {
	// HomeDir is dependency-injected for tests. If empty, os.UserHomeDir is used.
	HomeDir string
}

// Install reads ~/.claude/settings.json, adds the Scribe hook entry, and writes
// the embedded script to ~/.claude/hooks/scribe-hook.sh with 0755 permissions.
func (i *Installer) Install() (Status, error) {
	claudeDir, ok, err := i.claudeDir()
	if err != nil {
		return StatusNotApplicable, err
	}
	if !ok {
		return StatusNotApplicable, nil
	}

	scriptPath := managedScriptPath(claudeDir)
	settings, settingsMode, err := readSettings(claudeDir)
	if err != nil {
		return StatusNotApplicable, err
	}
	if err := validateHooksShape(settings); err != nil {
		return StatusNotApplicable, err
	}

	alreadyInstalled := scriptIsCurrent(scriptPath) && settingsHasManagedHook(settings, scriptPath)
	if alreadyInstalled {
		return StatusAlreadyInstalled, nil
	}

	settings, _ = addManagedHook(settings, scriptPath)
	if err := writeScript(scriptPath); err != nil {
		return StatusNotApplicable, err
	}
	if err := writeSettings(claudeDir, settings, settingsMode); err != nil {
		return StatusNotApplicable, err
	}

	return StatusInstalled, nil
}

// Uninstall removes the Scribe hook entry from settings.json and deletes the
// managed hook script. It is idempotent.
func (i *Installer) Uninstall() error {
	claudeDir, ok, err := i.claudeDir()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	scriptPath := managedScriptPath(claudeDir)
	if err := os.Remove(scriptPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove hook script: %w", err)
	}

	settings, settingsMode, err := readSettings(claudeDir)
	if err != nil {
		return err
	}
	if err := validateHooksShape(settings); err != nil {
		return err
	}
	settings, changed := removeManagedHook(settings, scriptPath)
	if !changed {
		return nil
	}
	if err := writeSettings(claudeDir, settings, settingsMode); err != nil {
		return err
	}
	return nil
}

// CurrentStatus reports whether the hook is currently installed. Because the
// Status enum intentionally has no NotInstalled value, StatusInstalled means
// Claude Code is present but the managed hook is not fully installed.
func (i *Installer) CurrentStatus() (Status, error) {
	claudeDir, ok, err := i.claudeDir()
	if err != nil {
		return StatusNotApplicable, err
	}
	if !ok {
		return StatusNotApplicable, nil
	}

	settings, _, err := readSettings(claudeDir)
	if err != nil {
		return StatusNotApplicable, err
	}
	if err := validateHooksShape(settings); err != nil {
		return StatusNotApplicable, err
	}
	scriptPath := managedScriptPath(claudeDir)
	if scriptIsCurrent(scriptPath) && settingsHasManagedHook(settings, scriptPath) {
		return StatusAlreadyInstalled, nil
	}
	return StatusInstalled, nil
}

func (i *Installer) claudeDir() (string, bool, error) {
	home := i.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", false, fmt.Errorf("home dir: %w", err)
		}
	}

	claudeDir := filepath.Join(home, ".claude")
	info, err := os.Stat(claudeDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return claudeDir, false, nil
		}
		return "", false, fmt.Errorf("stat Claude Code dir: %w", err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("%s is not a directory", claudeDir)
	}
	return claudeDir, true, nil
}

func managedScriptPath(claudeDir string) string {
	return filepath.Join(claudeDir, "hooks", hookFileName)
}

func scriptIsCurrent(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode().Perm() != 0o755 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Equal(data, Script())
}

func writeScript(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}
	if err := os.WriteFile(path, Script(), 0o755); err != nil {
		return fmt.Errorf("write hook script: %w", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod hook script: %w", err)
	}
	return nil
}

func readSettings(claudeDir string) (map[string]any, fs.FileMode, error) {
	path := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, 0o644, nil
		}
		return nil, 0, fmt.Errorf("read settings.json: %w", err)
	}

	settings := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, 0, fmt.Errorf("parse settings.json: %w", err)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("stat settings.json: %w", err)
	}
	return settings, info.Mode().Perm(), nil
}

func writeSettings(claudeDir string, settings map[string]any, mode fs.FileMode) error {
	path := filepath.Join(claudeDir, "settings.json")
	if mode == 0 {
		mode = 0o644
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings.json: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(claudeDir, ".settings.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp settings.json: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp settings.json: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp settings.json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp settings.json: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("save settings.json: %w", err)
	}
	return nil
}

// validateHooksShape returns an error when settings.json contains a `hooks`
// field of the wrong type. It permits the field to be absent (a fresh config)
// or a JSON object with `PostToolUseFailure` either absent or shaped as a JSON
// array. Any other shape is reported instead of being silently overwritten so
// the user's hand-edited config is preserved until they fix it.
func validateHooksShape(settings map[string]any) error {
	hooksValue, exists := settings["hooks"]
	if !exists {
		return nil
	}
	hooksMap, ok := hooksValue.(map[string]any)
	if !ok {
		return fmt.Errorf("settings.json `hooks` is not a JSON object (got %T); please fix manually before reinstalling the scribe hook", hooksValue)
	}
	eventValue, exists := hooksMap[hookEvent]
	if !exists {
		return nil
	}
	if _, ok := eventValue.([]any); !ok {
		return fmt.Errorf("settings.json `hooks.%s` is not a JSON array (got %T); please fix manually before reinstalling the scribe hook", hookEvent, eventValue)
	}
	return nil
}

func settingsHasManagedHook(settings map[string]any, scriptPath string) bool {
	for _, hook := range postFailureHookCommands(settings) {
		if hook == scriptPath {
			return true
		}
	}
	return false
}

func addManagedHook(settings map[string]any, scriptPath string) (map[string]any, bool) {
	if settingsHasManagedHook(settings, scriptPath) {
		return settings, false
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = map[string]any{}
		settings["hooks"] = hooksMap
	}

	entries, _ := hooksMap[hookEvent].([]any)
	entries = append(entries, map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": scriptPath,
			},
		},
	})
	hooksMap[hookEvent] = entries

	return settings, true
}

func removeManagedHook(settings map[string]any, scriptPath string) (map[string]any, bool) {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return settings, false
	}

	entries, ok := hooksMap[hookEvent].([]any)
	if !ok {
		return settings, false
	}

	changed := false
	filteredEntries := make([]any, 0, len(entries))
	for _, entryValue := range entries {
		entry, ok := entryValue.(map[string]any)
		if !ok {
			filteredEntries = append(filteredEntries, entryValue)
			continue
		}

		hooksList, ok := entry["hooks"].([]any)
		if !ok {
			filteredEntries = append(filteredEntries, entryValue)
			continue
		}

		filteredHooks := make([]any, 0, len(hooksList))
		for _, hookValue := range hooksList {
			hook, ok := hookValue.(map[string]any)
			if !ok {
				filteredHooks = append(filteredHooks, hookValue)
				continue
			}
			command, _ := hook["command"].(string)
			if command == scriptPath {
				changed = true
				continue
			}
			filteredHooks = append(filteredHooks, hookValue)
		}

		if len(filteredHooks) == 0 {
			changed = true
			continue
		}
		entry["hooks"] = filteredHooks
		filteredEntries = append(filteredEntries, entry)
	}

	if !changed {
		return settings, false
	}
	if len(filteredEntries) == 0 {
		delete(hooksMap, hookEvent)
	} else {
		hooksMap[hookEvent] = filteredEntries
	}
	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	}

	return settings, true
}

func postFailureHookCommands(settings map[string]any) []string {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	entries, ok := hooksMap[hookEvent].([]any)
	if !ok {
		return nil
	}

	var commands []string
	for _, entryValue := range entries {
		entry, ok := entryValue.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := entry["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hookValue := range hooksList {
			hook, ok := hookValue.(map[string]any)
			if !ok {
				continue
			}
			command, _ := hook["command"].(string)
			if command != "" {
				commands = append(commands, command)
			}
		}
	}
	return commands
}
