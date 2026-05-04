package budget

import (
	"strings"
	"testing"
)

func TestEstimateDescriptionBytes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name: "counts UTF-8 bytes",
			content: "---\n" +
				"name: utf8\n" +
				"description: é\n" +
				"---\n\n" +
				"你好\n",
			want: len([]byte("é")) + len("\n\n") + len([]byte("你好")),
		},
		{
			name: "empty description still counts first body paragraph",
			content: "---\n" +
				"name: empty-description\n" +
				"description: \"\"\n" +
				"---\n\n" +
				"First body paragraph.\n",
			want: len("First body paragraph."),
		},
		{
			name: "uses only first body paragraph",
			content: "---\n" +
				"name: multi-paragraph\n" +
				"description: Frontmatter description.\n" +
				"---\n\n" +
				"First paragraph spans\n" +
				"two lines.\n\n" +
				"Second paragraph is ignored.\n",
			want: len("Frontmatter description.") + len("\n\n") + len("First paragraph spans two lines."),
		},
		{
			name: "no frontmatter counts first body paragraph",
			content: "First paragraph.\n\n" +
				"Second paragraph.\n",
			want: len("First paragraph."),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateDescriptionBytes(Skill{Name: tt.name, Content: []byte(tt.content)})
			if got != tt.want {
				t.Fatalf("EstimateDescriptionBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckBudgetThresholdBoundaries(t *testing.T) {
	tests := []struct {
		name string
		used int
		want Status
	}{
		{name: "below warn threshold is silent", used: 3807, want: StatusSilent},
		{name: "warn threshold warns", used: 3808, want: StatusWarn},
		{name: "below hard limit warns", used: 5439, want: StatusWarn},
		{name: "hard limit refuses", used: 5440, want: StatusRefuse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckBudget([]Skill{skillWithDescription("one", tt.used)}, "codex")
			if result.Status != tt.want {
				t.Fatalf("Status = %s, want %s", result.Status, tt.want)
			}
			if result.Used != tt.used {
				t.Fatalf("Used = %d, want %d", result.Used, tt.used)
			}
		})
	}
}

func TestCheckBudgetOverflowBreakdown(t *testing.T) {
	skills := []Skill{
		skillWithDescription("a", 3000),
		skillWithDescription("b", 2500),
		skillWithDescription("c", 100),
	}

	result := CheckBudget(skills, "codex")
	if result.Status != StatusRefuse {
		t.Fatalf("Status = %s, want %s", result.Status, StatusRefuse)
	}
	if result.Used != 5600 {
		t.Fatalf("Used = %d, want 5600", result.Used)
	}
	want := []Overflow{
		{Skill: "b", Bytes: 60},
		{Skill: "c", Bytes: 100},
	}
	if len(result.Overflow) != len(want) {
		t.Fatalf("Overflow = %#v, want %#v", result.Overflow, want)
	}
	for i := range want {
		if result.Overflow[i] != want[i] {
			t.Fatalf("Overflow[%d] = %#v, want %#v", i, result.Overflow[i], want[i])
		}
	}
}

func TestCheckProjectionBudgetShortensCodexDescriptions(t *testing.T) {
	skills := []Skill{
		skillWithSentenceDescription("a", strings.Repeat("a", 3000)+". "+strings.Repeat("ignored", 100)),
		skillWithSentenceDescription("b", strings.Repeat("b", 3000)+". "+strings.Repeat("ignored", 100)),
	}

	raw := CheckBudget(skills, "codex")
	if raw.Status != StatusRefuse {
		t.Fatalf("raw Status = %s, want %s", raw.Status, StatusRefuse)
	}

	projected := CheckProjectionBudget(skills, "codex")
	if projected.Status == StatusRefuse {
		t.Fatalf("projected Status = %s, want non-refuse; used %d", projected.Status, projected.Used)
	}
	if projected.Used >= raw.Used {
		t.Fatalf("projected Used = %d, want less than raw %d", projected.Used, raw.Used)
	}
}

func TestCheckProjectionBudgetKeepsClaudeRawDescriptions(t *testing.T) {
	skills := []Skill{
		skillWithSentenceDescription("a", strings.Repeat("a", 3000)+". "+strings.Repeat("ignored", 100)),
		skillWithSentenceDescription("b", strings.Repeat("b", 5200)+". "+strings.Repeat("ignored", 100)),
	}

	result := CheckProjectionBudget(skills, "claude")
	if result.Status != StatusRefuse {
		t.Fatalf("Status = %s, want %s", result.Status, StatusRefuse)
	}
}

func skillWithDescription(name string, bytes int) Skill {
	return Skill{
		Name: name,
		Content: []byte("---\n" +
			"name: " + name + "\n" +
			"description: " + strings.Repeat("x", bytes) + "\n" +
			"---\n"),
	}
}

func skillWithSentenceDescription(name, description string) Skill {
	return Skill{
		Name: name,
		Content: []byte("---\n" +
			"name: " + name + "\n" +
			"description: " + description + "\n" +
			"---\n"),
	}
}
