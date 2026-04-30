package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var doctorOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "skill": { "type": "string" },
    "fix": { "type": "boolean" },
    "issues": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "skill": { "type": "string" },
          "tool": { "type": "string" },
          "kind": { "type": "string" },
          "status": { "type": "string" },
          "message": { "type": "string" }
        },
        "required": ["skill", "kind", "status", "message"],
        "additionalProperties": false
      }
    }
  },
  "required": ["fix", "issues"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe doctor", doctorOutputSchema)
}
