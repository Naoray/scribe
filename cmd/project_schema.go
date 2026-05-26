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

var projectEditOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "project_root": {"type": "string"},
    "project_file": {"type": "string"},
    "kind": {"type": "string", "enum": ["kit", "snippet", "skill"]},
    "action": {"type": "string", "enum": ["add", "remove"]},
    "field": {"type": "string", "enum": ["kits", "snippets", "add", "remove"]},
    "synced": {"type": "boolean"},
    "results": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "status": {"type": "string", "enum": ["added", "present", "removed", "absent"]},
          "changed": {"type": "boolean"}
        },
        "required": ["name", "status", "changed"],
        "additionalProperties": false
      }
    },
    "list": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["project_root", "project_file", "kind", "action", "field", "synced", "results", "list"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe project sync", projectSyncOutputSchema)
	clischema.Register("scribe project skill create", projectSkillOutputSchema)
	clischema.Register("scribe project skill claim", projectSkillOutputSchema)
	for _, command := range []string{
		"scribe project kit add",
		"scribe project kit remove",
		"scribe project snippet add",
		"scribe project snippet remove",
		"scribe project skill add",
		"scribe project skill remove",
	} {
		clischema.Register(command, projectEditOutputSchema)
	}
}
