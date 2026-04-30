package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var updateOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "registries": {
      "type": "array",
      "items": { "$ref": "#/$defs/registry_plan" }
    },
    "updates": {
      "type": "array",
      "items": { "$ref": "#/$defs/update" }
    },
    "applied": { "type": "boolean" },
    "commits": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "registry": { "type": "string" },
          "sha": { "type": "string" },
          "url": { "type": "string" }
        },
        "required": ["registry", "sha", "url"],
        "additionalProperties": false
      }
    }
  },
  "required": ["registries", "updates", "applied"],
  "additionalProperties": false,
  "$defs": {
    "registry_plan": {
      "type": "object",
      "properties": {
        "registry": { "type": "string" },
        "updates": {
          "type": "array",
          "items": { "$ref": "#/$defs/update" }
        }
      },
      "required": ["registry", "updates"],
      "additionalProperties": false
    },
    "update": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "current_sha": { "type": "string" },
        "latest_sha": { "type": "string" },
        "current_hash": { "type": "string" },
        "latest_hash": { "type": "string" }
      },
      "required": ["name", "current_sha", "latest_sha", "current_hash", "latest_hash"],
      "additionalProperties": false
    }
  }
}`

func init() {
	clischema.Register("scribe update", updateOutputSchema)
}
