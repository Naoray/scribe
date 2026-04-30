package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var statusOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "version": { "type": "string" },
    "registries": { "type": "array", "items": { "type": "string" } },
    "installed_count": { "type": "integer" },
    "last_sync": { "type": "string" }
  },
  "required": ["version", "registries", "installed_count"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe status", statusOutputSchema)
}
