package budget

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const warnRatio = 0.70

type Status string

const (
	StatusSilent Status = "silent"
	StatusWarn   Status = "warn"
	StatusRefuse Status = "refuse"
)

type Skill struct {
	Name    string
	Content []byte
}

type Overflow struct {
	Skill string
	Bytes int
}

type Result struct {
	Agent     string
	Limit     int
	Used      int
	Status    Status
	Overflow  []Overflow
	Missing   []string
	WarnRatio float64
}

func (r Result) Percent() int {
	if r.Limit <= 0 {
		return 0
	}
	return (r.Used*100 + r.Limit - 1) / r.Limit
}

func EstimateDescriptionBytes(skill Skill) int {
	description, body := splitSkill(skill.Content)
	firstParagraph := extractFirstParagraph(body)
	return len([]byte(strings.TrimSpace(description))) + len([]byte(firstParagraph))
}

func CheckBudget(skills []Skill, agent string) Result {
	limit := AgentBudgets[strings.ToLower(agent)]
	result := Result{
		Agent:     strings.ToLower(agent),
		Limit:     limit,
		WarnRatio: warnRatio,
	}
	if limit <= 0 {
		result.Status = StatusSilent
		return result
	}

	ordered := append([]Skill(nil), skills...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Name < ordered[j].Name
	})

	for _, skill := range ordered {
		size := EstimateDescriptionBytes(skill)
		before := result.Used
		result.Used += size
		if result.Used >= limit {
			overflow := result.Used - limit
			if before > limit {
				overflow = size
			}
			result.Overflow = append(result.Overflow, Overflow{
				Skill: skill.Name,
				Bytes: overflow,
			})
		}
	}

	switch {
	case result.Used >= limit:
		result.Status = StatusRefuse
	case float64(result.Used) >= float64(limit)*warnRatio:
		result.Status = StatusWarn
	default:
		result.Status = StatusSilent
	}
	return result
}

func FormatResult(result Result) string {
	if result.Limit <= 0 {
		return ""
	}
	return fmt.Sprintf("%s budget: %d%% (%d / %d bytes)", title(result.Agent), result.Percent(), result.Used, result.Limit)
}

func FormatOverflow(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s skill budget exceeded\n", title(result.Agent))
	fmt.Fprintf(&b, "estimated: %d / %d bytes (%d%%)\n", result.Used, result.Limit, result.Percent())
	if len(result.Overflow) > 0 {
		b.WriteString("overflow caused by:\n")
		for _, item := range result.Overflow {
			fmt.Fprintf(&b, "  %s adds %d bytes over budget\n", item.Skill, item.Bytes)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

type frontmatter struct {
	Description string `yaml:"description"`
}

func splitSkill(content []byte) (string, string) {
	text := normalizeLineEndings(string(content))
	if !strings.HasPrefix(text, "---\n") {
		return "", text
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", text
	}
	rawFrontmatter := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rawFrontmatter), &fm); err != nil {
		return "", body
	}
	return fm.Description, body
}

func extractFirstParagraph(body string) string {
	lines := strings.Split(normalizeLineEndings(body), "\n")
	var paragraph []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	return strings.Join(strings.Fields(strings.Join(paragraph, " ")), " ")
}

func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
