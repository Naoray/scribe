package targets

import (
	"fmt"
	"os"
)

// replaceSymlink atomically replaces any existing file/symlink at link
// with a new symlink pointing to target.
func replaceSymlink(link, target string) error {
	// Remove whatever is there (file, symlink, or directory with contents).
	if err := os.RemoveAll(link); err != nil {
		return fmt.Errorf("remove existing %s: %w", link, err)
	}
	return os.Symlink(target, link)
}
