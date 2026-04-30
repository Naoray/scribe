package cmd

const mutatorWorkflowOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
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
                "action": { "type": "string" },
                "status": { "type": "string" },
                "version": { "type": "string" },
                "error": { "type": "string" }
              },
              "required": ["name", "action"],
              "additionalProperties": false
            }
          }
        },
        "required": ["registry", "skills"],
        "additionalProperties": false
      }
    },
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
    "adoption": { "type": "object" },
    "reconcile": { "type": "object" },
    "skipped_by_deny_list": { "type": "array" }
  },
  "required": ["registries", "summary"],
  "additionalProperties": false
}`
