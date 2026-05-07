package projectstore

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
)

const AIDir = ".ai"

type Store struct {
	Root string
}

type ContentMarker struct {
	Hash        string
	GeneratedAt string
	GeneratedBy string
}

func Project(root string) Store {
	return Store{Root: filepath.Join(root, AIDir)}
}

func Global(root string) Store {
	return Store{Root: root}
}

func (s Store) SkillsDir() string {
	return filepath.Join(s.Root, "skills")
}

func (s Store) KitsDir() string {
	return filepath.Join(s.Root, "kits")
}

func (s Store) LockfilePath() string {
	return filepath.Join(s.Root, lockfile.ProjectFilename)
}

func (s Store) SkillDir(name string) string {
	return filepath.Join(s.SkillsDir(), name)
}

func (s Store) KitPath(name string) string {
	return filepath.Join(s.KitsDir(), name+".yaml")
}

func (s Store) LoadKits() (map[string]*kit.Kit, error) {
	return kit.LoadAll(s.KitsDir())
}

func (s Store) LoadProjectLockfile() (*lockfile.ProjectLockfile, error) {
	data, err := os.ReadFile(s.LockfilePath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project lockfile: %w", err)
	}
	return lockfile.ParseProject(data)
}

func (s Store) WriteProjectLockfile(lf *lockfile.ProjectLockfile) error {
	data, err := lf.Encode()
	if err != nil {
		return err
	}
	return atomicWrite(s.LockfilePath(), data)
}

func (s Store) VendoredSkills() ([]string, error) {
	entries, err := os.ReadDir(s.SkillsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project skills: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func MarkerPath(skillDir string) string {
	return filepath.Join(skillDir, lockfile.ContentHashFilename)
}

func ReadMarker(skillDir string) (ContentMarker, error) {
	data, err := os.ReadFile(MarkerPath(skillDir))
	if err != nil {
		return ContentMarker{}, err
	}
	marker := ContentMarker{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "sha256":
			marker.Hash = value
		case "generated_at":
			marker.GeneratedAt = value
		case "generated_by":
			marker.GeneratedBy = value
		}
	}
	if err := scanner.Err(); err != nil {
		return ContentMarker{}, fmt.Errorf("scan content marker: %w", err)
	}
	if marker.Hash == "" {
		return ContentMarker{}, errors.New("content marker missing sha256")
	}
	return marker, nil
}

func WriteMarker(skillDir, hash, generatedBy string, now time.Time) error {
	data := fmt.Sprintf("sha256:%s\ngenerated_at: %s\ngenerated_by: %s\n", hash, now.UTC().Format(time.RFC3339), generatedBy)
	return atomicWrite(MarkerPath(skillDir), []byte(data))
}

func VerifyMarker(skillDir string) (ContentMarker, string, error) {
	marker, err := ReadMarker(skillDir)
	if err != nil {
		return ContentMarker{}, "", err
	}
	actual, err := lockfile.HashSet(skillDir)
	if err != nil {
		return marker, "", err
	}
	if actual != marker.Hash {
		return marker, actual, fmt.Errorf("vendored content hash mismatch: expected %s, got %s", marker.Hash, actual)
	}
	return marker, actual, nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s dir: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
