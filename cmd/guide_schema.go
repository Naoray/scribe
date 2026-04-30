package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var guideOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "status": { "type": "string" },
    "prerequisites": {
      "type": "object",
      "properties": {
        "github_auth": { "type": "object" },
        "scribe_dir": { "type": "object" },
        "connections": { "type": "object" }
      },
      "required": ["github_auth", "scribe_dir", "connections"],
      "additionalProperties": true
    },
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "command": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["command", "description"],
        "additionalProperties": false
      }
    }
  },
  "required": ["status", "prerequisites", "steps"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe guide", guideOutputSchema)
}
