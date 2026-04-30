package output

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/fields"
)

func TestAttachFieldsFlag(t *testing.T) {
	attached := &cobra.Command{Use: "attached"}
	AttachFieldsFlag(attached, fields.FieldSet[string]{"name": func(s string) any { return s }})
	if attached.Flags().Lookup("fields") == nil {
		t.Fatal("attached command missing --fields")
	}

	unattached := &cobra.Command{Use: "unattached"}
	if unattached.Flags().Lookup("fields") != nil {
		t.Fatal("unattached command has --fields")
	}
}
