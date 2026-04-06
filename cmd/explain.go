package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/discovery"
	"github.com/Naoray/scribe/internal/state"
)

var (
	explNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF"))
	explDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	explDivStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// buildLLMCmd constructs the command used to invoke the LLM CLI.
// Overridable in tests.
var buildLLMCmd = func(ctx context.Context, prompt string) *exec.Cmd {
	return exec.CommandContext(ctx, "claude", "-p", prompt)
}

func newExplainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <skill>",
		Short: "Explain what a skill does",
		Long: `Show a friendly explanation of what a skill does.

By default, uses an installed LLM (like Claude Code) to generate a casual,
developer-friendly explanation with analogies and concrete examples.

Falls back to rendering the SKILL.md directly if no LLM is available.`,
		Args: cobra.ExactArgs(1),
		RunE: runExplain,
	}
	cmd.Flags().Bool("json", false, "Output structured JSON (for agents/scripts)")
	cmd.Flags().Bool("raw", false, "Show rendered SKILL.md directly, skip AI explanation")
	cmd.MarkFlagsMutuallyExclusive("json", "raw")
	return cmd
}

func runExplain(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	rawFlag, _ := cmd.Flags().GetBool("raw")

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	skills, err := discovery.OnDisk(st)
	if err != nil {
		return fmt.Errorf("discover skills: %w", err)
	}

	skill, ok := findSkill(skills, args[0])
	if !ok {
		return fmt.Errorf("skill %q not found — run `scribe list --local` to see installed skills", args[0])
	}

	if skill.LocalPath == "" {
		return fmt.Errorf("skill %q is tracked but not on disk — try `scribe sync` first", args[0])
	}

	content, err := readSkillContent(skill.LocalPath)
	if err != nil {
		return err
	}

	isTTY := isatty.IsTerminal(os.Stdout.Fd())
	w := cmd.OutOrStdout()

	if jsonFlag {
		return explainJSON(w, skill, content)
	}

	if !isTTY {
		return renderSkillBody(w, content)
	}

	// Always show the rendered skill file first — instant feedback.
	if err := explainRendered(w, skill, content); err != nil {
		return err
	}

	if rawFlag {
		return nil
	}

	// Offer AI explanation if an LLM CLI is available and stdin is interactive.
	if _, err := detectLLMCLI(); err == nil {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			return offerAIExplanation(w, cmd.Context(), content)
		}
	}

	return nil
}

// findSkill looks up a skill by exact name or suffix match.
// Suffix matching lets users type "browse" instead of "gstack/browse".
func findSkill(skills []discovery.Skill, query string) (discovery.Skill, bool) {
	for _, sk := range skills {
		if sk.Name == query {
			return sk, true
		}
	}
	for _, sk := range skills {
		if strings.HasSuffix(sk.Name, "/"+query) {
			return sk, true
		}
	}
	return discovery.Skill{}, false
}

// readSkillContent reads the SKILL.md file from a skill directory.
func readSkillContent(skillDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	return string(data), nil
}

// detectLLMCLI finds the first available LLM CLI on the machine.
func detectLLMCLI() (string, error) {
	if _, err := exec.LookPath("claude"); err == nil {
		return "claude", nil
	}
	return "", fmt.Errorf("no supported LLM CLI found")
}

// stripFrontmatter removes YAML frontmatter (--- delimited) from content.
func stripFrontmatter(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasPrefix(s, "---") {
		return s
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return s
	}
	return strings.TrimLeft(s[end+7:], "\n")
}

func explainJSON(w io.Writer, skill discovery.Skill, content string) error {
	out := struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Version     string   `json:"version,omitempty"`
		Source      string   `json:"source,omitempty"`
		Targets     []string `json:"targets,omitempty"`
		Path        string   `json:"path,omitempty"`
		Content     string   `json:"content"`
	}{
		Name:        skill.Name,
		Description: skill.Description,
		Version:     skill.Version,
		Source:      skill.Source,
		Targets:     skill.Targets,
		Path:        skill.LocalPath,
		Content:     content,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func printSkillHeader(w io.Writer, skill discovery.Skill) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, explNameStyle.Render(skill.Name))
	if skill.Description != "" {
		fmt.Fprintln(w, explDimStyle.Render(skill.Description))
	}
	type kv struct{ key, value string }
	var meta []kv
	if skill.Version != "" {
		meta = append(meta, kv{"Version", skill.Version})
	}
	if skill.Source != "" {
		meta = append(meta, kv{"Source", skill.Source})
	}
	if len(skill.Targets) > 0 {
		meta = append(meta, kv{"Agents", strings.Join(skill.Targets, ", ")})
	}
	for _, m := range meta {
		fmt.Fprintf(w, "%s %s\n", explDimStyle.Render(m.key+":"), m.value)
	}
	fmt.Fprintln(w, explDivStyle.Render(strings.Repeat("─", 60)))
}

func renderSkillBody(w io.Writer, content string) error {
	body := stripFrontmatter(content)
	rendered, err := glamour.Render(body, "auto")
	if err != nil {
		fmt.Fprintln(w, body)
		return nil
	}
	fmt.Fprint(w, rendered)
	return nil
}

func explainRendered(w io.Writer, skill discovery.Skill, content string) error {
	printSkillHeader(w, skill)
	return renderSkillBody(w, content)
}

const explainSystemPrompt = `You're explaining a coding agent skill to a developer who's never seen it before.

Rules:
- Explain what the skill does in 2-3 short paragraphs
- Use a concrete analogy or metaphor to make it click — like explaining it to a teammate over coffee
- Focus on: when you'd reach for this skill, what it saves you from doing manually
- Be casual and direct — no marketing speak, no filler
- If the skill has specific triggers or flags, mention them briefly
- End with a one-liner on when NOT to use it (if applicable)`

const (
	spinnerInterval = 80 * time.Millisecond
	clearLine       = "\r\033[K"
)

// spinState drives a braille spinner on an io.Writer until stop() is called.
type spinState struct {
	once   sync.Once
	stopCh chan struct{}
	done   chan struct{}
}

func startSpinner(w io.Writer, label string) *spinState {
	s := &spinState{stopCh: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		fmt.Fprintf(w, "  %s  %s", frames[0], label)
		for {
			select {
			case <-s.stopCh:
				fmt.Fprint(w, clearLine) // erase spinner line
				return
			case <-ticker.C:
				i++
				fmt.Fprintf(w, "\r  %s  %s", frames[i%len(frames)], label)
			}
		}
	}()
	return s
}

func (s *spinState) stop() {
	s.once.Do(func() {
		close(s.stopCh)
		<-s.done
	})
}

// offerAIExplanation prompts the user to optionally get an AI-generated explanation.
// The skill file has already been rendered above; this is an opt-in upgrade.
func offerAIExplanation(w io.Writer, ctx context.Context, content string) error {
	var want bool
	err := huh.NewConfirm().
		Title("✨ Get a better explanation with AI?").
		Affirmative("Yes").
		Negative("No").
		Value(&want).
		Run()
	if err != nil || !want {
		return nil
	}
	fmt.Fprintln(w)
	return runAIExplanation(w, ctx, content)
}

// runAIExplanation calls the LLM, buffers the output, and renders it as markdown.
// If the LLM fails, it returns nil — the caller has already shown the skill file.
func runAIExplanation(w io.Writer, ctx context.Context, content string) error {
	prompt := fmt.Sprintf(
		"%s\n\nHere's the skill file:\n\n---\n%s",
		explainSystemPrompt,
		stripFrontmatter(content),
	)

	c := buildLLMCmd(ctx, prompt)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = io.Discard // avoid concurrent writes with spinner on os.Stderr

	var spin *spinState
	if isatty.IsTerminal(os.Stderr.Fd()) {
		spin = startSpinner(os.Stderr, "Thinking...")
	}
	runErr := c.Run()
	if spin != nil {
		spin.stop()
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, explDimStyle.Render("AI explanation unavailable."))
		return nil
	}

	rendered, renderErr := glamour.Render(buf.String(), "auto")
	if renderErr != nil {
		fmt.Fprint(w, buf.String())
	} else {
		fmt.Fprint(w, rendered)
	}
	return nil
}
