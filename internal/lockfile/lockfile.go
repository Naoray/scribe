package lockfile

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Filename      = "scribe.lock"
	SchemaVersion = 1
)

type Lockfile struct {
	FormatVersion int     `yaml:"format_version"`
	Registry      string  `yaml:"registry"`
	Entries       []Entry `yaml:"entries"`
}

type Entry struct {
	Name               string `yaml:"name" json:"name"`
	SourceRegistry     string `yaml:"source_registry" json:"source_registry"`
	CommitSHA          string `yaml:"commit_sha" json:"commit_sha"`
	ContentHash        string `yaml:"content_hash" json:"content_hash"`
	InstallCommandHash string `yaml:"install_command_hash,omitempty" json:"install_command_hash,omitempty"`
}

type Update struct {
	Name        string `json:"name"`
	CurrentSHA  string `json:"current_sha"`
	LatestSHA   string `json:"latest_sha"`
	CurrentHash string `json:"current_hash"`
	LatestHash  string `json:"latest_hash"`
}

func Parse(data []byte) (*Lockfile, error) {
	var lf Lockfile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", Filename, err)
	}
	if err := lf.Validate(); err != nil {
		return nil, err
	}
	return &lf, nil
}

func (lf *Lockfile) Encode() ([]byte, error) {
	if err := lf.Validate(); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(lf)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", Filename, err)
	}
	return data, nil
}

func (lf *Lockfile) Validate() error {
	if lf == nil {
		return errors.New("lockfile is nil")
	}
	if lf.FormatVersion != SchemaVersion {
		return fmt.Errorf("unsupported lockfile format_version %d (expected %d)", lf.FormatVersion, SchemaVersion)
	}
	if strings.TrimSpace(lf.Registry) == "" {
		return errors.New("lockfile registry is required")
	}
	seen := make(map[string]bool, len(lf.Entries))
	for _, entry := range lf.Entries {
		if strings.TrimSpace(entry.Name) == "" {
			return errors.New("lockfile entry has empty name")
		}
		if seen[entry.Name] {
			return fmt.Errorf("duplicate lockfile entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if strings.TrimSpace(entry.SourceRegistry) == "" {
			return fmt.Errorf("lockfile entry %q missing source_registry", entry.Name)
		}
		if strings.TrimSpace(entry.CommitSHA) == "" {
			return fmt.Errorf("lockfile entry %q missing commit_sha", entry.Name)
		}
		if strings.TrimSpace(entry.ContentHash) == "" {
			return fmt.Errorf("lockfile entry %q has invalid content_hash", entry.Name)
		}
		if entry.InstallCommandHash != "" && strings.TrimSpace(entry.InstallCommandHash) == "" {
			return fmt.Errorf("lockfile entry %q has invalid install_command_hash", entry.Name)
		}
	}
	return nil
}

func (lf *Lockfile) Entry(name string) (Entry, bool) {
	if lf == nil {
		return Entry{}, false
	}
	for _, entry := range lf.Entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

func Diff(current, latest *Lockfile) []Update {
	latestByName := map[string]Entry{}
	if latest != nil {
		for _, entry := range latest.Entries {
			latestByName[entry.Name] = entry
		}
	}

	currentByName := map[string]Entry{}
	if current != nil {
		for _, entry := range current.Entries {
			currentByName[entry.Name] = entry
		}
	}

	names := make([]string, 0, len(latestByName))
	for name := range latestByName {
		names = append(names, name)
	}
	sort.Strings(names)

	updates := make([]Update, 0)
	for _, name := range names {
		next := latestByName[name]
		prev := currentByName[name]
		if prev.CommitSHA == next.CommitSHA &&
			prev.ContentHash == next.ContentHash &&
			prev.InstallCommandHash == next.InstallCommandHash {
			continue
		}
		updates = append(updates, Update{
			Name:        name,
			CurrentSHA:  prev.CommitSHA,
			LatestSHA:   next.CommitSHA,
			CurrentHash: prev.ContentHash,
			LatestHash:  next.ContentHash,
		})
	}
	return updates
}
