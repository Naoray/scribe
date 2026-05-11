package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

const kitListOutputSchema = `{
  "type": "object",
  "required": ["kits"],
  "properties": {
    "kits": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "description", "skills_count"],
        "properties": {
          "name": {"type": "string"},
          "description": {"type": "string"},
          "skills_count": {"type": "integer"},
          "skills": {"type": "array", "items": {"type": "string"}},
          "registry": {"type": "string"},
          "path": {"type": "string"},
          "author": {"type": "string"},
          "remote": {"type": "boolean"},
          "installed_locally": {"type": "boolean"}
        }
      }
    }
  }
}`

const kitShowOutputSchema = `{
  "type": "object",
  "required": ["name", "description", "skills"],
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "skills": {"type": "array", "items": {"type": "string"}},
    "registry": {"type": "string"},
    "source": {
      "type": "object",
      "properties": {
        "registry": {"type": "string"},
        "rev": {"type": "string"}
      }
    },
    "refs": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["raw", "skill", "origin", "connected"],
        "properties": {
          "raw": {"type": "string"},
          "skill": {"type": "string"},
          "origin": {"type": "string", "enum": ["same_registry", "cross_registry", "local"]},
          "registry": {"type": "string"},
          "connected": {"type": "boolean"},
          "glob": {"type": "boolean"},
          "local": {"type": "boolean"},
          "source": {"type": "string"},
          "reason": {"type": "string"}
        }
      }
    }
  }
}`

func init() {
	clischema.Register("scribe kit list", kitListOutputSchema)
	clischema.Register("scribe kit show", kitShowOutputSchema)
}
