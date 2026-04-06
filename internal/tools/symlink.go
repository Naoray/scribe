package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// replaceSymlink atomically replaces any existing file/symlink at link
// with a new symlink pointing to target.
func replaceSymlink(link, target string) error {
	// Remove whatever is there (file, dir, or old symlink).
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove existing %s: %w", link, err)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}
	return nil
}
