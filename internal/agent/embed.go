package agent

import (
	"crypto/sha256"
	"encoding/hex"

	scribe "github.com/Naoray/scribe"
)

const embeddedRendererFormatVersion = "v1"

// EmbeddedSkillTemplate is the scribe-agent SKILL.md template. It lives at the
// repo root (SKILL.md.tmpl) and is embedded via the root package.
var EmbeddedSkillTemplate = scribe.AgentSkillTemplate

var EmbeddedVersion = func() string {
	sum := sha256.Sum256(append([]byte(embeddedRendererFormatVersion+"\n"), EmbeddedSkillTemplate...))
	return hex.EncodeToString(sum[:])
}()
