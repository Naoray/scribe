package schema

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/cobra"
)

func TestInputSchemaIsValidJSONSchema202012(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("json", false, "Output JSON")
	cmd.Flags().String("name", "", "Name")

	raw := InputSchema(cmd)
	if _, err := jsonschema.CompileString("input.schema.json", raw); err != nil {
		t.Fatalf("compile schema: %v\n%s", err, raw)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %v", decoded["$schema"])
	}
}
