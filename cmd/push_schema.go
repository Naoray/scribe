package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var pushOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "skill": { "type": "string" },
    "registry": { "type": "string" },
    "commit_sha": { "type": "string" },
    "commit_url": { "type": "string" }
  },
  "required": ["skill", "registry", "commit_sha", "commit_url"],
  "additionalProperties": false
}`

func init() {
	clischema.Register("scribe push", pushOutputSchema)
}
