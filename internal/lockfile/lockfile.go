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
	Filename        = "scribe.lock"
	ProjectFilename = "scribe.lock"
	ProjectKind     = "ProjectLock"
	SchemaVersion   = 1
)

type Lockfile struct {
	FormatVersion int     `yaml:"format_version"`
	Registry      string  `yaml:"registry"`
	Entries       []Entry `yaml:"entries"`
}

type ProjectLockfile struct {
	FormatVersion int            `yaml:"format_version"`
	Kind          string         `yaml:"kind"`
	GeneratedAt   string         `yaml:"generated_at,omitempty"`
	GeneratedBy   string         `yaml:"generated_by,omitempty"`
	Entries       []ProjectEntry `yaml:"entries"`
}

type Entry struct {
	Name               string `yaml:"name" json:"name"`
	SourceRegistry     string `yaml:"source_registry" json:"source_registry"`
	CommitSHA          string `yaml:"commit_sha" json:"commit_sha"`
	ContentHash        string `yaml:"content_hash" json:"content_hash"`
	InstallCommandHash string `yaml:"install_command_hash,omitempty" json:"install_command_hash,omitempty"`
}

type ProjectEntry struct {
	Entry      `yaml:",inline"`
	SourceRepo string            `yaml:"source_repo,omitempty" json:"source_repo,omitempty"`
	Path       string            `yaml:"path,omitempty" json:"path,omitempty"`
	Type       string            `yaml:"type,omitempty" json:"type,omitempty"`
	Install    string            `yaml:"install,omitempty" json:"install,omitempty"`
	Update     string            `yaml:"update,omitempty" json:"update,omitempty"`
	Installs   map[string]string `yaml:"installs,omitempty" json:"installs,omitempty"`
	Updates    map[string]string `yaml:"updates,omitempty" json:"updates,omitempty"`
}

type Update struct {
	Name        string `json:"name"`
	CurrentSHA  string `json:"current_sha"`
	LatestSHA   string `json:"latest_sha"`
	CurrentHash string `json:"current_hash"`
	LatestHash  string `json:"latest_hash"`
}

func Parse(data []byte) (*Lockfile, error) {
	kind, err := sniffKind(data)
	if err != nil {
		return nil, err
	}
	if kind == ProjectKind {
		return nil, fmt.Errorf("parse %s: project lockfile requires ParseProject", Filename)
	}
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

func ParseProject(data []byte) (*ProjectLockfile, error) {
	var lf ProjectLockfile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&lf); err != nil {
		return nil, fmt.Errorf("parse .ai/%s: %w", ProjectFilename, err)
	}
	if err := lf.Validate(); err != nil {
		return nil, err
	}
	return &lf, nil
}

func sniffKind(data []byte) (string, error) {
	var head struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return "", fmt.Errorf("parse %s discriminator: %w", Filename, err)
	}
	return strings.TrimSpace(head.Kind), nil
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

func (lf *ProjectLockfile) Encode() ([]byte, error) {
	if err := lf.Validate(); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(lf)
	if err != nil {
		return nil, fmt.Errorf("encode .ai/%s: %w", ProjectFilename, err)
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

func (lf *ProjectLockfile) Validate() error {
	if lf == nil {
		return errors.New("project lockfile is nil")
	}
	if lf.FormatVersion != SchemaVersion {
		return fmt.Errorf("unsupported project lockfile format_version %d (expected %d)", lf.FormatVersion, SchemaVersion)
	}
	if strings.TrimSpace(lf.Kind) != ProjectKind {
		return fmt.Errorf("project lockfile kind is %q (expected %q)", lf.Kind, ProjectKind)
	}
	seen := make(map[string]bool, len(lf.Entries))
	for _, entry := range lf.Entries {
		if strings.TrimSpace(entry.Name) == "" {
			return errors.New("project lockfile entry has empty name")
		}
		if seen[entry.Name] {
			return fmt.Errorf("duplicate project lockfile entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if strings.TrimSpace(entry.SourceRegistry) == "" {
			return fmt.Errorf("project lockfile entry %q missing source_registry", entry.Name)
		}
		if strings.TrimSpace(entry.CommitSHA) == "" {
			return fmt.Errorf("project lockfile entry %q missing commit_sha", entry.Name)
		}
		if strings.TrimSpace(entry.ContentHash) == "" {
			return fmt.Errorf("project lockfile entry %q has invalid content_hash", entry.Name)
		}
		if entry.InstallCommandHash != "" && strings.TrimSpace(entry.InstallCommandHash) == "" {
			return fmt.Errorf("project lockfile entry %q has invalid install_command_hash", entry.Name)
		}
		switch entry.Type {
		case "", "skill", "package":
		default:
			return fmt.Errorf("project lockfile entry %q has invalid type %q", entry.Name, entry.Type)
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

func (lf *ProjectLockfile) Entry(name string) (ProjectEntry, bool) {
	if lf == nil {
		return ProjectEntry{}, false
	}
	for _, entry := range lf.Entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return ProjectEntry{}, false
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
