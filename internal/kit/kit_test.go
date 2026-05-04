package kit

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadParsesKitYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frontend.yaml")
	data := []byte(`
name: frontend
description: Frontend defaults
skills:
  - init-react
  - init-tailwind
  - init-*
mcp_servers:
  - mempalace
  - playwright
source:
  registry: core
  rev: abc123
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := &Kit{
		Name:        "frontend",
		Description: "Frontend defaults",
		Skills:      []string{"init-react", "init-tailwind", "init-*"},
		MCPServers:  []string{"mempalace", "playwright"},
		Source: &Source{
			Registry: "core",
			Rev:      "abc123",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadAllLoadsYAMLFilesKeyedByKitName(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"frontend.yaml": "name: frontend\nskills:\n  - init-react\n",
		"backend.yaml":  "name: backend\nskills:\n  - init-go\n",
		"ignored.txt":   "name: ignored\nskills:\n  - init-ruby\n",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("LoadAll() loaded %d kits, want 2", len(got))
	}
	if got["frontend"].Skills[0] != "init-react" {
		t.Fatalf("frontend skills = %#v", got["frontend"].Skills)
	}
	if got["backend"].Skills[0] != "init-go" {
		t.Fatalf("backend skills = %#v", got["backend"].Skills)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "frontend.yaml")
	want := &Kit{
		Name:        "frontend",
		Description: "Frontend defaults",
		Skills:      []string{"init-react", "init-tailwind"},
		MCPServers:  []string{"mempalace"},
		Source: &Source{
			Registry: "core",
			Rev:      "abc123",
		},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}
