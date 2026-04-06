//go:build windows

package state

import "os"

func lockFile(path string, exclusive bool) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
}

func unlockFile(f *os.File) {
	f.Close()
}
