package mcpstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectReportsDeclarationsClientsAndDrift(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	writeFile(t, filepath.Join(home, ".scribe", "kits", "runtime.yaml"), `name: runtime
mcp_servers:
  - playwright
`)
	writeFile(t, filepath.Join(project, ".scribe.yaml"), `kits:
  - runtime
mcp:
  - mempalace
`)
	writeFile(t, filepath.Join(project, ".mcp.json"), `{
  "mcpServers": {
    "mempalace": { "command": "mempalace", "args": ["serve"] },
    "playwright": { "command": "playwright-mcp" }
  }
}
`)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{
  "enableAllProjectMcpServers": false,
  "enabledMcpjsonServers": ["mempalace"]
}
`)
	writeFile(t, filepath.Join(project, ".codex", "config.toml"), `[mcp_servers.mempalace]
command = "mempalace"
args = ["serve"]
enabled = true

[mcp_servers.manual]
command = "manual"
enabled = true
`)
	writeFile(t, filepath.Join(project, ".cursor", "mcp.json"), `{
  "mcpServers": {
    "mempalace": { "command": "wrong" }
  }
}
`)

	report, err := Inspect(InspectOptions{WorkDir: project})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	assertStrings(t, report.Declarations, []string{"mempalace", "playwright"})
	if report.Summary.Clients != 3 {
		t.Fatalf("clients = %d, want 3", report.Summary.Clients)
	}
	assertServer(t, report, "mempalace", true, true, []string{"claude", "codex", "cursor"})
	assertServer(t, report, "playwright", true, true, nil)
	assertHasDrift(t, report.Drift, DriftDeclaredMissing, "claude", "playwright")
	assertHasDrift(t, report.Drift, DriftDeclaredMissing, "codex", "playwright")
	assertHasDrift(t, report.Drift, DriftDeclaredMissing, "cursor", "playwright")
	assertHasDrift(t, report.Drift, DriftConfiguredUndeclared, "codex", "manual")
	assertHasDrift(t, report.Drift, DriftConfigMismatch, "cursor", "mempalace")
}

func TestInspectReportsUnknownClientState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".scribe.yaml"), "mcp:\n  - mempalace\n")
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{`)

	report, err := Inspect(InspectOptions{WorkDir: project})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	claude := clientByName(t, report, "claude")
	if claude.State != ClientStateUnknown {
		t.Fatalf("claude state = %q, want %q", claude.State, ClientStateUnknown)
	}
	assertHasDrift(t, report.Drift, DriftUnknownClientState, "claude", "")
}

func TestInspectDoesNotWriteClientFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".scribe.yaml"), "mcp:\n  - mempalace\n")
	files := map[string]string{
		filepath.Join(project, ".claude", "settings.json"): `{"enabledMcpjsonServers":["mempalace"]}`,
		filepath.Join(project, ".codex", "config.toml"):    "[mcp_servers.mempalace]\ncommand = \"mempalace\"\n",
		filepath.Join(project, ".cursor", "mcp.json"):      `{"mcpServers":{"mempalace":{"command":"mempalace"}}}`,
	}
	for path, content := range files {
		writeFile(t, path, content)
	}

	if _, err := Inspect(InspectOptions{WorkDir: project}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	for path, want := range files {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("%s changed:\n%s", path, got)
		}
	}
}

func TestInspectIgnoresParentProjectFilePastGitRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, ".scribe.yaml"), "kits:\n  - missing-local-kit\n")
	project := filepath.Join(parent, "repo")
	writeFile(t, filepath.Join(project, ".git"), "gitdir: /tmp/repo.git\n")

	report, err := Inspect(InspectOptions{WorkDir: project})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if report.ManifestPath != "" {
		t.Fatalf("manifest path = %q, want empty", report.ManifestPath)
	}
	assertStrings(t, report.Declarations, nil)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertServer(t *testing.T, report Report, name string, declared, defined bool, clients []string) {
	t.Helper()
	for _, server := range report.Servers {
		if server.Name != name {
			continue
		}
		if server.Declared != declared || server.Defined != defined {
			t.Fatalf("%s declared/defined = %v/%v, want %v/%v", name, server.Declared, server.Defined, declared, defined)
		}
		assertStrings(t, server.Clients, clients)
		return
	}
	t.Fatalf("server %q not found in %#v", name, report.Servers)
}

func clientByName(t *testing.T, report Report, name string) Client {
	t.Helper()
	for _, client := range report.Clients {
		if client.Name == name {
			return client
		}
	}
	t.Fatalf("client %q not found in %#v", name, report.Clients)
	return Client{}
}

func assertHasDrift(t *testing.T, drift []Drift, kind, client, server string) {
	t.Helper()
	for _, item := range drift {
		if item.Kind == kind && item.Client == client && item.Server == server {
			return
		}
	}
	t.Fatalf("missing drift kind=%s client=%s server=%s in %#v", kind, client, server, drift)
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("strings = %#v, want %#v", got, want)
		}
	}
}
