package output

import (
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/fields"
)

const fieldsFlagName = "fields"

func AttachFieldsFlag[T any](cmd *cobra.Command, _ fields.FieldSet[T]) {
	cmd.Flags().String(fieldsFlagName, "", "Comma-separated fields to include in JSON output")
}
