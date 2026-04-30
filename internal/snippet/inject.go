package snippet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	targetClaude = "claude"
	targetCodex  = "codex"
	targetCursor = "cursor"
	targetAll    = "all"
)

// Inject materializes snippets into targetPath using scribe-managed marker blocks.
func Inject(targetPath string, snippets []*Snippet) error {
	data, err := os.ReadFile(targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read injection target: %w", err)
	}

	updated := data
	changed := false
	for _, s := range snippets {
		if s == nil {
			continue
		}
		block := []byte(managedBlock(s))
		match, ok := findManagedBlock(updated, s.Name)
		if !ok {
			updated = appendBlock(updated, block)
			changed = true
			continue
		}
		if match.hash == BodyHash(s.Body) {
			continue
		}
		next := make([]byte, 0, len(updated)-match.end+match.start+len(block))
		next = append(next, updated[:match.start]...)
		next = append(next, block...)
		next = append(next, updated[match.end:]...)
		updated = next
		changed = true
	}

	if !changed {
		return nil
	}
	return atomicWrite(targetPath, updated)
}

// Remove deletes a snippet's managed block from targetPath while preserving all other content.
func Remove(targetPath, snippetName string) error {
	data, err := os.ReadFile(targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read injection target: %w", err)
	}

	match, ok := findManagedBlock(data, snippetName)
	if !ok {
		return nil
	}
	updated := make([]byte, 0, len(data)-(match.end-match.start))
	updated = append(updated, data[:match.start]...)
	updated = append(updated, data[match.end:]...)
	return atomicWrite(targetPath, updated)
}

// TargetsFor resolves a snippet's target names to existing agent rule files in projectRoot.
func TargetsFor(s *Snippet, projectRoot string) []string {
	if s == nil {
		return nil
	}

	targets := s.Targets
	if len(targets) == 0 || containsTarget(targets, targetAll) {
		targets = []string{targetClaude, targetCodex, targetCursor}
	}

	seen := make(map[string]bool, len(targets))
	var paths []string
	for _, target := range targets {
		path, ok := targetPath(projectRoot, target)
		if !ok || seen[path] {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	return paths
}

// BodyHash returns the sha256 hex digest of body.
func BodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

type blockMatch struct {
	start int
	end   int
	hash  string
}

func findManagedBlock(data []byte, name string) (blockMatch, bool) {
	startNeedle := []byte("<!-- scribe:start name=" + name + " hash=")
	searchFrom := 0
	for {
		relStart := bytes.Index(data[searchFrom:], startNeedle)
		if relStart < 0 {
			return blockMatch{}, false
		}
		start := searchFrom + relStart
		startLineEnd := bytes.IndexByte(data[start:], '\n')
		if startLineEnd < 0 {
			return blockMatch{}, false
		}
		startLineEnd += start

		startLine := data[start:startLineEnd]
		hash, ok := parseStartHash(startLine, string(startNeedle))
		if !ok {
			searchFrom = startLineEnd + 1
			continue
		}

		endNeedle := []byte("<!-- scribe:end name=" + name + " -->")
		relEnd := bytes.Index(data[startLineEnd+1:], endNeedle)
		if relEnd < 0 {
			return blockMatch{}, false
		}
		endStart := startLineEnd + 1 + relEnd
		end := endStart + len(endNeedle)
		if end < len(data) && data[end] == '\r' {
			end++
		}
		if end < len(data) && data[end] == '\n' {
			end++
		}
		return blockMatch{start: start, end: end, hash: hash}, true
	}
}

func parseStartHash(line []byte, prefix string) (string, bool) {
	text := string(bytes.TrimSuffix(line, []byte("\r")))
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, " -->") {
		return "", false
	}
	hash := strings.TrimSuffix(strings.TrimPrefix(text, prefix), " -->")
	if hash == "" {
		return "", false
	}
	return hash, true
}

func managedBlock(s *Snippet) string {
	var b strings.Builder
	b.WriteString("<!-- scribe:start name=")
	b.WriteString(s.Name)
	b.WriteString(" hash=")
	b.WriteString(BodyHash(s.Body))
	b.WriteString(" -->\n")
	b.WriteString(s.Body)
	if !strings.HasSuffix(s.Body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("<!-- scribe:end name=")
	b.WriteString(s.Name)
	b.WriteString(" -->\n")
	return b.String()
}

func appendBlock(data, block []byte) []byte {
	if len(data) == 0 {
		return append([]byte{}, block...)
	}
	updated := append([]byte{}, data...)
	if !bytes.HasSuffix(updated, []byte("\n")) {
		updated = append(updated, '\n')
	}
	return append(updated, block...)
}

func targetPath(projectRoot, target string) (string, bool) {
	switch target {
	case targetClaude:
		return filepath.Join(projectRoot, "CLAUDE.md"), true
	case targetCodex:
		return filepath.Join(projectRoot, "AGENTS.md"), true
	case targetCursor:
		return filepath.Join(projectRoot, ".cursorrules"), true
	default:
		return "", false
	}
}

func containsTarget(targets []string, target string) bool {
	for _, candidate := range targets {
		if candidate == target {
			return true
		}
	}
	return false
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write target: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save target: %w", err)
	}
	return nil
}
