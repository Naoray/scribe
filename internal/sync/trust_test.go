package sync

import "testing"

func TestCommandHash(t *testing.T) {
	h1 := CommandHash("npm install", "npm update")
	h2 := CommandHash("npm install", "npm update")
	if h1 != h2 {
		t.Error("same commands should produce same hash")
	}

	h3 := CommandHash("npm install --force", "npm update")
	if h1 == h3 {
		t.Error("different commands should produce different hash")
	}

	h4 := CommandHash("npm install", "")
	if h4 == "" {
		t.Error("expected non-empty hash")
	}
}

