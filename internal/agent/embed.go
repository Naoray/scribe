package agent

import (
	"crypto/sha256"
	"encoding/hex"

	scribe "github.com/Naoray/scribe"
)

const embeddedRendererFormatVersion = "v2"

// EmbeddedSkillTemplate is the scribe-agent SKILL.md template. It lives at the
// repo root (SKILL.md.tmpl) and is embedded via the root package.
var EmbeddedSkillTemplate = scribe.AgentSkillTemplate

// EmbeddedClaudeTemplate is the generated agent-facing CLI contract.
var EmbeddedClaudeTemplate = scribe.AgentClaudeTemplate

var EmbeddedVersion = func() string {
	blob := append([]byte(embeddedRendererFormatVersion+"\n"), EmbeddedSkillTemplate...)
	blob = append(blob, []byte("\n---CLAUDE.md---\n")...)
	blob = append(blob, EmbeddedClaudeTemplate...)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}()
