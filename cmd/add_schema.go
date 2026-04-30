package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var addOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "results": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "registry": { "type": "string" },
          "status": { "type": "string" },
          "version": { "type": "string" },
          "description": { "type": "string" },
          "author": { "type": "string" }
        },
        "required": ["name", "registry", "status"],
        "additionalProperties": false
      }
    },
    "installed": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "registry": { "type": "string" },
          "status": { "type": "string" },
          "error": { "type": "string" }
        },
        "required": ["name", "registry", "status"],
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe add", addOutputSchema)
}
