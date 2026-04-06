//go:build unix

package state

import (
	"fmt"
	"os"
	"syscall"
)

// lockFile acquires an advisory flock on the given path.
// Use exclusive=true for writes, exclusive=false (shared) for reads.
func lockFile(path string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	lockType := syscall.LOCK_SH
	if exclusive {
		lockType = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), lockType); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return f, nil
}

// unlockFile releases the advisory lock and closes the file.
func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}
