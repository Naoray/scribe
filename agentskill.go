package scribe

import _ "embed"

// AgentSkillTemplate is the scribe SKILL.md template embedded at build time.
// It is rendered at install time by internal/agent based on local state.
//
//go:embed SKILL.md.tmpl
var AgentSkillTemplate []byte

// AgentClaudeTemplate is the generated agent-facing CLI contract embedded at build time.
//
//go:embed CLAUDE.md
var AgentClaudeTemplate []byte
