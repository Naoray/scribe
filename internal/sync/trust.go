package sync

import (
	"crypto/sha256"
	"fmt"
)

// CommandHash computes a SHA-256 hash of the install and update commands.
func CommandHash(install, update string) string {
	h := sha256.Sum256([]byte(install + "\n" + update))
	return fmt.Sprintf("%x", h)
}

