package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var listOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "oneOf": [
    {
      "type": "object",
      "properties": {
        "skills": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "name": { "type": "string" },
              "description": { "type": "string" },
              "package": { "type": "string" },
              "revision": { "type": "integer" },
              "content_hash": { "type": "string" },
              "targets": { "type": "array", "items": { "type": "string" } },
              "managed": { "type": "boolean" },
              "origin": { "type": "string" },
              "path": { "type": "string" }
            },
            "required": ["name", "targets", "managed"],
            "additionalProperties": false
          }
        },
        "packages": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "name": { "type": "string" },
              "description": { "type": "string" },
              "revision": { "type": "integer" },
              "path": { "type": "string" },
              "install_cmd": { "type": "string" },
              "sources": { "type": "array", "items": { "type": "string" } }
            },
            "required": ["name"],
            "additionalProperties": false
          }
        }
      },
      "required": ["skills", "packages"],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "registries": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "registry": { "type": "string" },
              "skills": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "name": { "type": "string" },
                    "status": { "type": "string" },
                    "version": { "type": "string" },
                    "loadout_ref": { "type": "string" },
                    "maintainer": { "type": "string" },
                    "agents": { "type": "array", "items": { "type": "string" } }
                  },
                  "required": ["name", "status"],
                  "additionalProperties": false
                }
              }
            },
            "required": ["registry", "skills"],
            "additionalProperties": false
          }
        },
        "warnings": { "type": "array", "items": { "type": "string" } }
      },
      "required": ["registries"],
      "additionalProperties": false
    }
  ]
}`

func init() {
	clischema.Register("scribe list", listOutputSchema)
}
