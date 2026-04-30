package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var adoptOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "dry_run": { "type": "boolean" },
    "adopt": { "type": "array" },
    "conflicts": { "type": "array" },
    "registries": {
      "type": "array",
      "items": { "$ref": "#/$defs/registry_result" }
    },
    "summary": { "$ref": "#/$defs/summary" },
    "adoption": {
      "type": "object",
      "properties": {
        "skipped": { "type": "string" },
        "skills": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "name": { "type": "string" },
              "tools": { "type": "array", "items": { "type": "string" } },
              "error": { "type": "string" }
            },
            "required": ["name"],
            "additionalProperties": false
          }
        },
        "conflicts_deferred": { "type": "integer" },
        "adopted": { "type": "integer" },
        "failed": { "type": "integer" }
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false,
  "$defs": {
    "summary": {
      "type": "object",
      "properties": {
        "installed": { "type": "integer" },
        "updated": { "type": "integer" },
        "skipped": { "type": "integer" },
        "failed": { "type": "integer" }
      },
      "required": ["installed", "updated", "skipped", "failed"],
      "additionalProperties": false
    },
    "registry_result": {
      "type": "object",
      "properties": {
        "registry": { "type": "string" },
        "skills": { "type": "array" }
      },
      "required": ["registry", "skills"],
      "additionalProperties": true
    }
  }
}`

func init() {
	clischema.Register("scribe adopt", adoptOutputSchema)
}
