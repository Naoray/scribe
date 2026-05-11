package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var mcpListOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "project_root": { "type": "string" },
    "manifest_path": { "type": "string" },
    "definitions_path": { "type": "string" },
    "declarations": { "type": "array", "items": { "type": "string" } },
    "servers": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "declared": { "type": "boolean" },
          "defined": { "type": "boolean" },
          "clients": { "type": "array", "items": { "type": "string" } },
          "drift": { "type": "array", "items": { "$ref": "#/$defs/drift" } }
        },
        "required": ["name", "declared", "defined", "clients"],
        "additionalProperties": false
      }
    },
    "clients": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "path": { "type": "string" },
          "state": { "type": "string", "enum": ["configured", "missing", "unknown"] },
          "projected": { "type": "array", "items": { "type": "string" } },
          "drift": { "type": "array", "items": { "$ref": "#/$defs/drift" } }
        },
        "required": ["name", "path", "state", "projected"],
        "additionalProperties": false
      }
    },
    "drift": { "type": "array", "items": { "$ref": "#/$defs/drift" } },
    "summary": {
      "type": "object",
      "properties": {
        "declared": { "type": "integer" },
        "defined": { "type": "integer" },
        "clients": { "type": "integer" },
        "drift": { "type": "integer" }
      },
      "required": ["declared", "defined", "clients", "drift"],
      "additionalProperties": false
    }
  },
  "required": ["project_root", "declarations", "servers", "clients", "summary"],
  "additionalProperties": false,
  "$defs": {
    "drift": {
      "type": "object",
      "properties": {
        "kind": {
          "type": "string",
          "enum": ["declared_missing", "configured_undeclared", "config_mismatch", "unknown_client_state"]
        },
        "client": { "type": "string" },
        "server": { "type": "string" },
        "path": { "type": "string" },
        "message": { "type": "string" }
      },
      "required": ["kind", "message"],
      "additionalProperties": false
    }
  }
}`

func init() {
	clischema.Register("scribe mcp list", mcpListOutputSchema)
}
