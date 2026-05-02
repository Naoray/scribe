package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var migrateGlobalToProjectsOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["dry_run", "found_global_links", "found_skills", "selected_projects", "planned_project_file_writes", "planned_global_link_removals", "wrote_project_files", "removed_global_links", "skipped_global_links", "project_files", "removed_links", "candidate_projects"],
  "properties": {
    "dry_run": {"type": "boolean"},
    "found_global_links": {"type": "integer"},
    "found_skills": {"type": "integer"},
    "selected_projects": {"type": "integer"},
    "planned_project_file_writes": {"type": "integer"},
    "planned_global_link_removals": {"type": "integer"},
    "wrote_project_files": {"type": "integer"},
    "removed_global_links": {"type": "integer"},
    "skipped_global_links": {"type": "integer"},
    "project_files": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["project", "file", "added_skills", "skills", "changed"],
        "properties": {
          "project": {"type": "string"},
          "file": {"type": "string"},
          "added_skills": {"type": "array", "items": {"type": "string"}},
          "skills": {"type": "array", "items": {"type": "string"}},
          "changed": {"type": "boolean"},
          "budget_per_agent": {
            "type": "object",
            "additionalProperties": {
              "type": "object",
              "additionalProperties": true
            }
          }
        },
        "additionalProperties": false
      }
    },
    "removed_links": {
      "type": "array",
      "items": {"$ref": "#/$defs/global_link"}
    },
    "skipped_links": {
      "type": "array",
      "items": {"$ref": "#/$defs/global_link"}
    },
    "candidate_projects": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["path", "source"],
        "properties": {
          "path": {"type": "string"},
          "source": {"type": "string"}
        },
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false,
  "$defs": {
    "global_link": {
      "type": "object",
      "required": ["tool", "skill", "path", "canonical_path"],
      "properties": {
        "tool": {"type": "string"},
        "skill": {"type": "string"},
        "path": {"type": "string"},
        "canonical_path": {"type": "string"}
      },
      "additionalProperties": false
    }
  }
}`

func init() {
	clischema.Register("scribe migrate global-to-projects", migrateGlobalToProjectsOutputSchema)
}
