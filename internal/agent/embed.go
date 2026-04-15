package agent

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed scribe_agent/SKILL.md
var EmbeddedSkillMD []byte

var EmbeddedVersion = func() string {
	sum := sha256.Sum256(EmbeddedSkillMD)
	return hex.EncodeToString(sum[:])
}()
