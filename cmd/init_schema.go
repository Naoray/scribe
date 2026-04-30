package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var initOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "package": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "description": { "type": "string" },
        "author": { "type": "string" }
      },
      "required": ["name"],
      "additionalProperties": false
    },
    "skills": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "path": { "type": "string" }
        },
        "required": ["name", "path"],
        "additionalProperties": false
      }
    },
    "scribe_file": { "type": "string" }
  },
  "required": ["package", "skills", "scribe_file"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe init", initOutputSchema)
}
