package lockfile

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const ContentHashFilename = ".scribe-content-hash"

// HashSet hashes the project-share content set for root.
func HashSet(root string) (string, error) {
	files, err := hashSetFiles(root)
	if err != nil {
		return "", err
	}
	return HashFiles(files)
}

func hashSetFiles(root string) ([]File, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve hash root: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if gitRoot, ok := findGitRoot(root); ok {
		files, err := gitHashSetFiles(root, gitRoot)
		if err != nil {
			return nil, err
		}
		return files, nil
	}
	return walkHashSetFiles(root)
}

func findGitRoot(root string) (string, bool) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	gitRoot := strings.TrimSpace(string(out))
	if gitRoot == "" {
		return "", false
	}
	return gitRoot, true
}

func gitHashSetFiles(root, gitRoot string) ([]File, error) {
	rel, err := filepath.Rel(gitRoot, root)
	if err != nil {
		return nil, fmt.Errorf("resolve git hash path: %w", err)
	}
	cmd := exec.Command("git", "-C", gitRoot, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", filepath.ToSlash(rel))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git ls-files hash set: %w: %s", err, strings.TrimSpace(string(out)))
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]File, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := filepath.Join(gitRoot, filepath.FromSlash(string(part)))
		file, ok, err := readHashSetFile(root, path)
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func walkHashSetFiles(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && hashSetDenied(root, path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if hashSetDenied(root, path, false) {
			return nil
		}
		file, ok, err := readHashSetFile(root, path)
		if err != nil {
			return err
		}
		if ok {
			files = append(files, file)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk hash set %s: %w", root, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func readHashSetFile(root, path string) (File, bool, error) {
	if hashSetDenied(root, path, false) {
		return File{}, false, nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, fmt.Errorf("stat hash file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return File{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, false, fmt.Errorf("read hash file %s: %w", path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return File{}, false, err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return File{Path: filepath.ToSlash(rel), Content: data}, true, nil
}

func hashSetDenied(root, path string, dir bool) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		switch part {
		case ".git", "versions", ".idea", ".vscode", "node_modules":
			return true
		}
	}
	name := parts[len(parts)-1]
	if dir {
		return false
	}
	switch name {
	case ".DS_Store", "Thumbs.db", ContentHashFilename, ".scribe-base.md":
		return true
	}
	for _, pattern := range []string{"*.swp", "*.swo", "*.bak.*"} {
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}
