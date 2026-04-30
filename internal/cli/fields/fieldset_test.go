package fields

import (
	stderrors "errors"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type item struct {
	Name    string
	Version string
}

func TestProject(t *testing.T) {
	set := FieldSet[item]{
		"name":    func(i item) any { return i.Name },
		"version": func(i item) any { return i.Version },
	}

	got, err := Project(set, []string{"name"}, item{Name: "recap", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got["name"] != "recap" {
		t.Fatalf("name = %v", got["name"])
	}
	if _, ok := got["version"]; ok {
		t.Fatal("unexpected version field")
	}
}

func TestProjectUnknownField(t *testing.T) {
	_, err := Project(FieldSet[item]{}, []string{"missing"}, item{})
	var ce *clierrors.Error
	if !stderrors.As(err, &ce) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if ce.Exit != clierrors.ExitUsage {
		t.Fatalf("Exit = %d, want %d", ce.Exit, clierrors.ExitUsage)
	}
}
