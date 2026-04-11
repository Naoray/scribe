package discovery

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// contentHash computes a deterministic fingerprint of a skill directory's contents.
// Returns the first 8 hex chars of SHA256(sorted relative paths + file contents).
//
// skillFileHash computes SHA-256 of just the SKILL.md file, returning first 8 hex chars.
// Used for modification detection against the installed_hash stored in state.
func skillFileHash(skillDir string) (string, error) {
	path := filepath.Join(skillDir, "SKILL.md")

	// Resolve symlinks before reading.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", resolved, err)
	}

	// Normalize CRLF → LF for cross-platform determinism.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])[:8], nil
}

// Design choice: symlinks are resolved before reading, so two skills pointing to
// the same source directory produce the same hash. This is intentional — they
// represent the same content.
func contentHash(dir string) (string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(dir, path)

		// Exclude specific directories at any depth.
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Exclude .DS_Store files.
		if info.Name() == ".DS_Store" {
			return nil
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk %s: %w", dir, err)
	}

	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		absPath := filepath.Join(dir, rel)

		// Resolve symlinks before reading.
		resolved, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			continue // skip broken/circular symlinks
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			continue // skip unreadable files
		}

		// Normalize CRLF → LF for cross-platform determinism.
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

		h.Write([]byte(rel))
		h.Write(data)
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:8], nil
}
