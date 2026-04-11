package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// DefaultMaxVersions is the default number of version snapshots to retain.
const DefaultMaxVersions = 10

// VersionInfo describes a single version snapshot.
type VersionInfo struct {
	Revision int
	Path     string
	ModTime  time.Time
}

var revFilePattern = regexp.MustCompile(`^rev-(\d+)\.md$`)

// SnapshotVersion copies current SKILL.md to versions/rev-{N}.md before content changes.
// N is the current revision number from state.
// If the SKILL.md does not exist, this is a no-op (first install).
func SnapshotVersion(skillDir string, revision int) error {
	src := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil // first install — nothing to snapshot
	}
	if err != nil {
		return fmt.Errorf("snapshot read: %w", err)
	}

	versionsDir := filepath.Join(skillDir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		return fmt.Errorf("snapshot mkdir: %w", err)
	}

	dst := filepath.Join(versionsDir, fmt.Sprintf("rev-%d.md", revision))
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return fmt.Errorf("snapshot write: %w", err)
	}

	return nil
}

// EnforceRetention deletes oldest snapshots if count exceeds maxVersions.
// If maxVersions is 0, all snapshots are kept (unlimited).
func EnforceRetention(skillDir string, maxVersions int) error {
	if maxVersions <= 0 {
		return nil
	}

	versions, err := ListVersions(skillDir)
	if err != nil {
		return err
	}

	if len(versions) <= maxVersions {
		return nil
	}

	// versions is sorted ascending by revision — delete the oldest.
	toDelete := versions[:len(versions)-maxVersions]
	for _, v := range toDelete {
		if err := os.Remove(v.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("retention delete %s: %w", v.Path, err)
		}
	}

	return nil
}

// ListVersions returns available version snapshots for a skill, sorted by revision (ascending).
func ListVersions(skillDir string) ([]VersionInfo, error) {
	versionsDir := filepath.Join(skillDir, "versions")
	entries, err := os.ReadDir(versionsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	var versions []VersionInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := revFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		rev, _ := strconv.Atoi(matches[1])
		info, err := entry.Info()
		if err != nil {
			continue
		}
		versions = append(versions, VersionInfo{
			Revision: rev,
			Path:     filepath.Join(versionsDir, entry.Name()),
			ModTime:  info.ModTime(),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Revision < versions[j].Revision
	})

	return versions, nil
}
