package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrRealDirectoryExists is returned when replaceSymlink finds a real
// (non-symlink) directory at the link path. The caller should route the user
// to `scribe adopt` rather than silently destroying the directory.
var ErrRealDirectoryExists = errors.New("real directory exists at target path")

// replaceSymlink replaces any existing file or symlink at link with a new
// symlink pointing to target. It refuses to remove a real (non-symlink)
// directory and returns ErrRealDirectoryExists instead so callers can
// produce an actionable error.
func replaceSymlink(link, target string) error {
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Nothing there — proceed to create.
	case err != nil:
		return fmt.Errorf("stat %s: %w", link, err)
	case info.Mode()&os.ModeSymlink != 0:
		// Existing symlink: os.Remove removes the link itself, not the target.
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove existing symlink %s: %w", link, err)
		}
	case info.IsDir():
		// Real directory — preserve it; caller decides what to do.
		return fmt.Errorf("%w: %s", ErrRealDirectoryExists, link)
	default:
		// Regular file: remove and replace.
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove existing file %s: %w", link, err)
		}
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}
	return nil
}
