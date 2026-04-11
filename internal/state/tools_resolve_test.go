package state

import (
	"reflect"
	"testing"
)

func TestEffectiveTools_InheritReturnsAvailable(t *testing.T) {
	sk := InstalledSkill{Tools: []string{"claude"}} // stale cache — ignored
	got := sk.EffectiveTools([]string{"claude", "cursor"})
	want := []string{"claude", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEffectiveTools_PinnedIntersectsAvailable(t *testing.T) {
	sk := InstalledSkill{
		ToolsMode: ToolsModePinned,
		Tools:     []string{"claude", "codex"}, // codex not available
	}
	got := sk.EffectiveTools([]string{"claude", "cursor"})
	want := []string{"claude"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEffectiveTools_PinnedPreservesOrder(t *testing.T) {
	sk := InstalledSkill{
		ToolsMode: ToolsModePinned,
		Tools:     []string{"cursor", "claude"},
	}
	got := sk.EffectiveTools([]string{"claude", "cursor"})
	want := []string{"cursor", "claude"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEffectiveTools_PackagesBypass(t *testing.T) {
	sk := InstalledSkill{
		Type:      "package",
		ToolsMode: ToolsModePinned,
		Tools:     []string{"claude"},
	}
	got := sk.EffectiveTools([]string{"claude", "cursor"})
	want := []string{"claude", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("packages should bypass per-skill routing: got %v, want %v", got, want)
	}
}

func TestEffectiveTools_PinnedEmptyYieldsEmpty(t *testing.T) {
	sk := InstalledSkill{ToolsMode: ToolsModePinned, Tools: nil}
	got := sk.EffectiveTools([]string{"claude", "cursor"})
	if len(got) != 0 {
		t.Errorf("pinned with empty Tools should yield empty list, got %v", got)
	}
}

func TestNormalizeToolSelection_DedupePreservesOrder(t *testing.T) {
	got := NormalizeToolSelection([]string{"claude", "cursor", "claude", "", "codex"})
	want := []string{"claude", "cursor", "codex"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
