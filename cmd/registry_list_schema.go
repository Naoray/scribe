package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

const registryListOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "registries": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "registry": { "type": "string" },
          "visibility": {
            "type": "string",
            "enum": ["public", "private", "unknown"]
          },
          "skill_count": { "type": "integer", "minimum": 0 }
        },
        "required": ["registry", "visibility", "skill_count"],
        "additionalProperties": false
      }
    },
    "last_sync": {
      "type": ["string", "null"],
      "format": "date-time"
    }
  },
  "required": ["registries", "last_sync"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe registry list", registryListOutputSchema)
}
