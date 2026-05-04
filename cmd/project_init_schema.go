package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var projectInitOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "kits": {
      "type": "array",
      "items": { "type": "string" }
    },
    "project_file": { "type": "string" },
    "gitignore_updated": { "type": "boolean" }
  },
  "required": ["kits", "project_file", "gitignore_updated"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe project init", projectInitOutputSchema)
}
