package agent

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

const embeddedRendererFormatVersion = "v1"

//go:embed scribe_agent/SKILL.md.tmpl
var EmbeddedSkillTemplate []byte

var EmbeddedVersion = func() string {
	sum := sha256.Sum256(append([]byte(embeddedRendererFormatVersion+"\n"), EmbeddedSkillTemplate...))
	return hex.EncodeToString(sum[:])
}()
