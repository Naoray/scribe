package kit

import (
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/projectfile"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name            string
		projectFile     *projectfile.ProjectFile
		availableKits   map[string]*Kit
		installedSkills []string
		want            []string
		wantErr         string
	}{
		{
			name:        "empty project file",
			projectFile: &projectfile.ProjectFile{},
			want:        []string{},
		},
		{
			name: "single kit returns its skills",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"frontend"},
			},
			availableKits: map[string]*Kit{
				"frontend": {Name: "frontend", Skills: []string{"init-react", "init-tailwind"}},
			},
			want: []string{"init-react", "init-tailwind"},
		},
		{
			name: "multiple kits union without duplicates",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"frontend", "backend"},
			},
			availableKits: map[string]*Kit{
				"frontend": {Name: "frontend", Skills: []string{"init-react", "shared"}},
				"backend":  {Name: "backend", Skills: []string{"init-go", "shared"}},
			},
			want: []string{"init-go", "init-react", "shared"},
		},
		{
			name: "glob expands against installed skills",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"all-init"},
			},
			availableKits: map[string]*Kit{
				"all-init": {Name: "all-init", Skills: []string{"init-*"}},
			},
			installedSkills: []string{"init-react", "init-go", "audit-tests"},
			want:            []string{"init-go", "init-react"},
		},
		{
			name: "add applies after kits",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"frontend"},
				Add:  []string{"audit-tests"},
			},
			availableKits: map[string]*Kit{
				"frontend": {Name: "frontend", Skills: []string{"init-react"}},
			},
			want: []string{"audit-tests", "init-react"},
		},
		{
			name: "remove applies after add and kits",
			projectFile: &projectfile.ProjectFile{
				Kits:   []string{"frontend"},
				Add:    []string{"audit-tests"},
				Remove: []string{"init-react", "audit-tests"},
			},
			availableKits: map[string]*Kit{
				"frontend": {Name: "frontend", Skills: []string{"init-react", "init-tailwind"}},
			},
			want: []string{"init-tailwind"},
		},
		{
			name: "missing kit returns error with name",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"missing"},
			},
			wantErr: "missing",
		},
		{
			name: "glob matching nothing contributes nothing without error",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"none"},
			},
			availableKits: map[string]*Kit{
				"none": {Name: "none", Skills: []string{"init-*"}},
			},
			installedSkills: []string{"audit-tests"},
			want:            []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.projectFile, tt.availableKits, tt.installedSkills)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Resolve() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("Resolve() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveMCPServers(t *testing.T) {
	tests := []struct {
		name          string
		projectFile   *projectfile.ProjectFile
		availableKits map[string]*Kit
		want          []string
		wantErr       string
	}{
		{
			name:        "empty project file",
			projectFile: &projectfile.ProjectFile{},
			want:        []string{},
		},
		{
			name: "single kit returns its MCP servers",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"agent-runtime"},
			},
			availableKits: map[string]*Kit{
				"agent-runtime": {Name: "agent-runtime", MCPServers: []string{"mempalace", "playwright"}},
			},
			want: []string{"mempalace", "playwright"},
		},
		{
			name: "multiple kits union without duplicates",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"memory", "browser"},
			},
			availableKits: map[string]*Kit{
				"memory":  {Name: "memory", MCPServers: []string{"mempalace", "github"}},
				"browser": {Name: "browser", MCPServers: []string{"playwright", "github"}},
			},
			want: []string{"github", "mempalace", "playwright"},
		},
		{
			name: "missing kit returns error with name",
			projectFile: &projectfile.ProjectFile{
				Kits: []string{"missing"},
			},
			wantErr: "missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveMCPServers(tt.projectFile, tt.availableKits)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolveMCPServers() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveMCPServers() error = %v", err)
			}
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("ResolveMCPServers() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
