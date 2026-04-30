package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type File struct {
	Path    string
	Content []byte
}

func HashFiles(files []File) (string, error) {
	if len(files) == 0 {
		return "", errors.New("cannot hash empty file tree")
	}
	clean := make([]File, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		p := filepath.ToSlash(filepath.Clean(file.Path))
		if p == "." || p == "" || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
			return "", fmt.Errorf("invalid file path %q", file.Path)
		}
		if seen[p] {
			return "", fmt.Errorf("duplicate file path %q", p)
		}
		seen[p] = true
		clean = append(clean, File{Path: p, Content: file.Content})
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].Path < clean[j].Path })

	h := sha256.New()
	for _, file := range clean {
		_, _ = h.Write([]byte(file.Path))
		_, _ = h.Write([]byte{0})
		sum := sha256.Sum256(file.Content)
		_, _ = h.Write([]byte(hex.EncodeToString(sum[:])))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func HashDir(root string) (string, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "versions":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, File{Path: rel, Content: data})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash directory %s: %w", root, err)
	}
	return HashFiles(files)
}

func CommandHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		if part == "" {
			continue
		}
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
