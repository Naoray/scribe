package scribe

import _ "embed"

// AgentSkillTemplate is the scribe-agent SKILL.md template embedded at build time.
// It is rendered at install time by internal/agent based on local state.
//
//go:embed SKILL.md.tmpl
var AgentSkillTemplate []byte
