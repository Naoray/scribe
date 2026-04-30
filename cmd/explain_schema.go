package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var explainOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "description": { "type": "string" },
    "revision": { "type": "integer" },
    "targets": { "type": "array", "items": { "type": "string" } },
    "path": { "type": "string" },
    "content": { "type": "string" }
  },
  "required": ["name", "content"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe explain", explainOutputSchema)
}
