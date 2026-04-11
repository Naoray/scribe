package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/config"
)

const (
	ToolTypeBuiltin = "builtin"
	ToolTypeCustom  = "custom"
)

type Status struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
	Detected    bool   `json:"detected"`
	DetectKnown bool   `json:"detect_known"`
	Source      string `json:"source"`
}

func builtinRegistry() map[string]Tool {
	registry := make(map[string]Tool)
	for _, tool := range DefaultTools() {
		registry[strings.ToLower(tool.Name())] = tool
	}
	return registry
}

func BuiltinByName(name string) (Tool, bool) {
	tool, ok := builtinRegistry()[strings.ToLower(name)]
	return tool, ok
}

func ResolveActive(cfg *config.Config) ([]Tool, error) {
	statuses, err := ResolveStatuses(cfg)
	if err != nil {
		return nil, err
	}

	var active []Tool
	for _, st := range statuses {
		if !st.Enabled {
			continue
		}
		toolCfg := findToolConfig(cfg, st.Name)
		if builtin, ok := BuiltinByName(st.Name); ok && (toolCfg == nil || toolCfg.Type != ToolTypeCustom) {
			active = append(active, builtin)
			continue
		}
		if toolCfg == nil {
			continue
		}
		custom, err := customToolFromConfig(*toolCfg)
		if err != nil {
			return nil, err
		}
		active = append(active, custom)
	}
	return active, nil
}

func ResolveStatuses(cfg *config.Config) ([]Status, error) {
	detected := DetectTools()
	builtinDetected := make(map[string]bool, len(detected))
	for _, tool := range detected {
		builtinDetected[strings.ToLower(tool.Name())] = true
	}

	statuses := make(map[string]Status)
	for _, tool := range DefaultTools() {
		name := strings.ToLower(tool.Name())
		if builtinDetected[name] {
			statuses[name] = Status{
				Name:        tool.Name(),
				Type:        ToolTypeBuiltin,
				Enabled:     true,
				Detected:    true,
				DetectKnown: true,
				Source:      "auto",
			}
		}
	}

	if cfg != nil {
		for _, tc := range cfg.Tools {
			name := strings.ToLower(tc.Name)
			if name == "" {
				continue
			}
			isBuiltin := false
			if _, ok := BuiltinByName(tc.Name); ok && tc.Type != ToolTypeCustom {
				isBuiltin = true
			}

			st := Status{
				Name:        tc.Name,
				Type:        tc.Type,
				Enabled:     tc.Enabled,
				Detected:    builtinDetected[name],
				DetectKnown: isBuiltin || strings.TrimSpace(tc.Detect) != "",
				Source:      "manual",
			}

			if st.Type == "" {
				if isBuiltin {
					st.Type = ToolTypeBuiltin
				} else {
					st.Type = ToolTypeCustom
				}
			}

			if existing, ok := statuses[name]; ok {
				st.Detected = existing.Detected
				st.DetectKnown = existing.DetectKnown || st.DetectKnown
				st.Source = "auto+manual"
			} else if st.Type == ToolTypeCustom && strings.TrimSpace(tc.Detect) != "" {
				custom, err := customToolFromConfig(tc)
				if err != nil {
					return nil, err
				}
				st.Detected = custom.Detect()
			}

			statuses[name] = st
		}
	}

	out := make([]Status, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func customToolFromConfig(tc config.ToolConfig) (CommandTool, error) {
	if strings.TrimSpace(tc.Install) == "" {
		return CommandTool{}, fmt.Errorf("tool %q is missing install command", tc.Name)
	}
	if strings.TrimSpace(tc.Uninstall) == "" {
		return CommandTool{}, fmt.Errorf("tool %q is missing uninstall command", tc.Name)
	}
	return CommandTool{
		ToolName:         tc.Name,
		DetectCommand:    tc.Detect,
		InstallCommand:   tc.Install,
		UninstallCommand: tc.Uninstall,
		PathTemplate:     tc.Path,
	}, nil
}

func findToolConfig(cfg *config.Config, name string) *config.ToolConfig {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Tools {
		if strings.EqualFold(cfg.Tools[i].Name, name) {
			return &cfg.Tools[i]
		}
	}
	return nil
}
