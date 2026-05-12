package kit_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naoray/scribe/internal/kit"
)

func TestParseYAMLRoundTripSource(t *testing.T) {
	body := []byte(`apiVersion: scribe/v1
kind: Kit
name: daily-workflow
description: Plan, capture, and close the day
skills: [plan-my-day, evaluate-day]
mcp_servers: []
source:
  registry: Naoray/skills
  rev: abc123
`)
	parsed, err := kit.ParseYAML(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Source == nil || parsed.Source.Registry != "Naoray/skills" || parsed.Source.Rev != "abc123" {
		t.Fatalf("source not parsed: %+v", parsed.Source)
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "daily-workflow.yaml")
	if err := kit.Save(dst, parsed); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := kit.Load(dst)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Source == nil || reloaded.Source.Registry != "Naoray/skills" || reloaded.Source.Rev != "abc123" {
		t.Errorf("source lost on round-trip: %+v", reloaded.Source)
	}
	if len(reloaded.Skills) != 2 {
		t.Errorf("skills lost on round-trip: %v", reloaded.Skills)
	}
}

func TestParseYAMLEmpty(t *testing.T) {
	k, err := kit.ParseYAML(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if k.Name != "" || len(k.Skills) != 0 {
		t.Errorf("expected empty kit, got %+v", k)
	}
}

func TestParseYAMLRejectsInvalidName(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"./escape",
		"name/with/slash",
		"name with space",
		"name\\..\\windows",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			body := []byte("apiVersion: scribe/v1\nkind: Kit\nname: " + name + "\n")
			if _, err := kit.ParseYAML(body); err == nil || !strings.Contains(err.Error(), "invalid kit name") {
				t.Errorf("expected invalid kit name error, got %v", err)
			}
		})
	}
}
