package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPListJSONEnvelopeAndNoWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)
	writeMCPCommandFile(t, filepath.Join(project, ".scribe.yaml"), "mcp:\n  - mempalace\n")
	paths := map[string]string{
		filepath.Join(project, ".claude", "settings.json"): `{"enabledMcpjsonServers":["mempalace"]}`,
		filepath.Join(project, ".codex", "config.toml"):    "[mcp_servers.mempalace]\ncommand = \"mempalace\"\n",
		filepath.Join(project, ".cursor", "mcp.json"):      `{"mcpServers":{"mempalace":{"command":"mempalace"}}}`,
	}
	for path, content := range paths {
		writeMCPCommandFile(t, path, content)
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"mcp", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var env testEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s", err, stdout.String())
	}
	if env.Status != "ok" {
		t.Fatalf("status = %q, want ok", env.Status)
	}
	var data struct {
		Declarations []string `json:"declarations"`
		Summary      struct {
			Clients int `json:"clients"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Declarations) != 1 || data.Declarations[0] != "mempalace" {
		t.Fatalf("declarations = %#v, want mempalace", data.Declarations)
	}
	if data.Summary.Clients != 3 {
		t.Fatalf("clients = %d, want 3", data.Summary.Clients)
	}
	for path, want := range paths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("%s changed:\n%s", path, got)
		}
	}
}

func writeMCPCommandFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
