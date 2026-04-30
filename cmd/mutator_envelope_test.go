package cmd

import (
	"encoding/json"
	"testing"

	clischema "github.com/Naoray/scribe/internal/cli/schema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestAdoptDryRunEnvelopeValidatesOutputSchema(t *testing.T) {
	setupAdoptHome(t, "shape-skill", "# shape-skill\ncontent")

	env := executeEnvelopeCommand(t, []string{"adopt", "--dry-run", "--json"})
	if env.Status != "ok" {
		t.Fatalf("status = %q, want ok", env.Status)
	}
	if env.FormatVersion != "1" {
		t.Fatalf("format_version = %q, want 1", env.FormatVersion)
	}

	rawSchema, ok := clischema.Get("scribe adopt")
	if !ok {
		t.Fatal("missing adopt output schema")
	}
	schema, err := jsonschema.CompileString("adopt.schema.json", rawSchema)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var data any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if err := schema.Validate(data); err != nil {
		t.Fatalf("schema validation: %v\ndata=%s", err, string(env.Data))
	}
}
