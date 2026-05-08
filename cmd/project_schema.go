package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var projectSyncOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "project_root": {"type": "string"},
    "kits_written": {"type": "array", "items": {"type": "string"}},
    "skills_vendored": {"type": "array", "items": {"type": "string"}},
    "registry_pinned": {"type": "array", "items": {"type": "string"}},
    "bootstrap_skipped": {"type": "array", "items": {"type": "string"}},
    "drift": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["project_root"],
  "additionalProperties": false
}`

var projectSkillOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "path": {"type": "string"},
    "origin": {"type": "string"}
  },
  "required": ["name", "path", "origin"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe project sync", projectSyncOutputSchema)
	clischema.Register("scribe project skill create", projectSkillOutputSchema)
	clischema.Register("scribe project skill claim", projectSkillOutputSchema)
}
