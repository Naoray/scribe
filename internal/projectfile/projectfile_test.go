package projectfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    *ProjectFile
		wantErr bool
	}{
		{
			name:    "empty file",
			content: "",
			want:    &ProjectFile{},
		},
		{
			name:    "minimal kits only",
			content: "kits:\n  - core\n",
			want: &ProjectFile{
				Kits: []string{"core"},
			},
		},
		{
			name: "all fields present",
			content: `kits:
  - core
snippets:
  - editor
add:
  - local/agent
remove:
  - old/skill
`,
			want: &ProjectFile{
				Kits:     []string{"core"},
				Snippets: []string{"editor"},
				Add:      []string{"local/agent"},
				Remove:   []string{"old/skill"},
			},
		},
		{
			name:    "malformed yaml",
			content: "kits: [",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), Filename)
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write project file: %v", err)
			}

			got, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Load() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLoadNonexistentPath(t *testing.T) {
	t.Parallel()

	got, err := Load(filepath.Join(t.TempDir(), Filename))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, &ProjectFile{}) {
		t.Fatalf("Load() = %#v, want empty project file", got)
	}
}

func TestFindWalksUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectPath := filepath.Join(root, Filename)
	if err := os.WriteFile(projectPath, []byte("kits:\n  - core\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}

	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

	got, err := Find(nested)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got != projectPath {
		t.Fatalf("Find() = %q, want %q", got, projectPath)
	}
}

func TestFindNotFound(t *testing.T) {
	t.Parallel()

	got, err := Find(t.TempDir())
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Find() = %q, want empty string", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "project", Filename)
	want := &ProjectFile{
		Kits:     []string{"core", "review"},
		Snippets: []string{"editor"},
		Add:      []string{"local/agent"},
		Remove:   []string{"old/skill"},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() after Save() = %#v, want %#v", got, want)
	}
}
