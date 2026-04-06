package provider

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/Naoray/scribe/internal/manifest"
)

const marketplacePath = ".claude-plugin/marketplace.json"

// marketplaceFile is the JSON structure of .claude-plugin/marketplace.json.
type marketplaceFile struct {
	Name    string              `json:"name"`
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplacePlugin struct {
	Name   string   `json:"name"`
	Source string   `json:"source"`
	Skills []string `json:"skills"`
}

// ParseMarketplace parses a marketplace.json byte slice and returns catalog entries.
// Each plugin's skills are flattened into individual entries with Group set to the plugin name.
func ParseMarketplace(data []byte, owner, repo string) ([]manifest.Entry, error) {
	var mf marketplaceFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parse marketplace.json: %w", err)
	}

	var entries []manifest.Entry
	source := fmt.Sprintf("github:%s/%s@HEAD", owner, repo)

	for _, plugin := range mf.Plugins {
		// Resolve the plugin source path (strip leading "./").
		pluginDir := strings.TrimPrefix(plugin.Source, "./")

		for _, skillPath := range plugin.Skills {
			// Skill name is the last path component.
			skillName := path.Base(skillPath)

			entries = append(entries, manifest.Entry{
				Name:   skillName,
				Source: source,
				Path:   path.Join(pluginDir, skillPath),
				Author: owner,
				Group:  plugin.Name,
			})
		}
	}

	return entries, nil
}
