package targets

import (
	"fmt"
	"os"
)

// replaceSymlink atomically replaces any existing file/symlink at link
// with a new symlink pointing to target.
func replaceSymlink(link, target string) error {
	// Remove whatever is there (file, dir, or old symlink).
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing %s: %w", link, err)
	}
	return os.Symlink(target, link)
}
