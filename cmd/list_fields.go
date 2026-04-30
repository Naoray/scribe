package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/fields"
	"github.com/Naoray/scribe/internal/cli/output"
	"github.com/Naoray/scribe/internal/workflow"
)

var listFieldSet = fields.FieldSet[workflow.ListOutput]{
	"skills": func(out workflow.ListOutput) any {
		return out["skills"]
	},
	"packages": func(out workflow.ListOutput) any {
		return out["packages"]
	},
	"registries": func(out workflow.ListOutput) any {
		return out["registries"]
	},
	"warnings": func(out workflow.ListOutput) any {
		return out["warnings"]
	},
}

func attachListFields(cmd *cobra.Command) {
	output.AttachFieldsFlag(cmd, listFieldSet)
}

func projectListOutput(cmd *cobra.Command, out workflow.ListOutput) (any, error) {
	fieldsFlag, _ := cmd.Flags().GetString("fields")
	if strings.TrimSpace(fieldsFlag) == "" {
		return out, nil
	}
	return fields.Project(listFieldSet, strings.Split(fieldsFlag, ","), out)
}
