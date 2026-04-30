package cmd

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

func TestSchemaCommandListInputsOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"schema", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got commandSchema
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out.String())
	}
	if got.OutputSchema != nil {
		t.Fatalf("output_schema = %s, want null", string(*got.OutputSchema))
	}
	var input map[string]any
	if err := json.Unmarshal(got.InputSchema, &input); err != nil {
		t.Fatalf("input schema unmarshal: %v", err)
	}
	props := input["properties"].(map[string]any)
	if _, ok := props["remote"]; !ok {
		t.Fatalf("list input schema missing remote flag: %#v", props)
	}
}

func TestSchemaCommandAll(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"schema", "--all"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got map[string]commandSchema
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out.String())
	}
	if _, ok := got["scribe list"]; !ok {
		t.Fatalf("--all missing scribe list; keys=%v", got)
	}
}

func TestSchemaCommandUnregistered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	root.SetArgs([]string{"schema", "definitely-missing"})

	err := root.Execute()
	var ce *clierrors.Error
	if !stderrors.As(err, &ce) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if ce.Exit != clierrors.ExitNotFound {
		t.Fatalf("Exit = %d, want %d", ce.Exit, clierrors.ExitNotFound)
	}
}
