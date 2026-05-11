package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryIndexJSONEnvelope(t *testing.T) {
	home := t.TempDir()
	indexDir := filepath.Join(home, ".scribe", "index")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatalf("mkdir index dir: %v", err)
	}
	data := `{
  "version": 1,
  "updated_at": "2026-05-11T08:42:11Z",
  "registries": [
    {
      "repo": "acme/skills",
      "source_repo": "acme/skills",
      "visibility": "public",
      "default_branch": "main",
      "head_sha": "abc123",
      "manifest_present": true,
      "manifest": {"api_version": "scribe/v1", "kind": "Registry", "team_name": "Acme", "present": true},
      "skill_count": 2,
      "kit_count": 1,
      "last_fetched_at": "2026-05-11T08:42:11Z"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(indexDir, "registries.json"), []byte(data), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	stdout, stderr, code := runScribeHelperWithHome(t, home, []string{"registry", "index", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d\nstderr=%s\nstdout=%s", code, stderr, stdout)
	}
	var env struct {
		Data struct {
			Registries []struct {
				Repo       string `json:"repo"`
				HeadSHA    string `json:"head_sha"`
				SkillCount int    `json:"skill_count"`
				KitCount   int    `json:"kit_count"`
			} `json:"registries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nstdout=%s", err, stdout)
	}
	if len(env.Data.Registries) != 1 || env.Data.Registries[0].Repo != "acme/skills" {
		t.Fatalf("registries = %#v", env.Data.Registries)
	}
	if env.Data.Registries[0].HeadSHA != "abc123" || env.Data.Registries[0].SkillCount != 2 || env.Data.Registries[0].KitCount != 1 {
		t.Fatalf("registry = %#v", env.Data.Registries[0])
	}
}
