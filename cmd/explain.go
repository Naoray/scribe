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
	return cmd
}

func runExplain(cmd *cobra.Command, args []string) error {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	rawFlag, _ := cmd.Flags().GetBool("raw")
	if jsonFlag && rawFlag {
		return fmt.Errorf("if any flags in the group [json raw] are set none of the others can be; [json raw] were all set")
	}
	factory := newCommandFactory()

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	skills, err := discovery.OnDisk(st)
	if err != nil {
		return fmt.Errorf("discover skills: %w", err)
	}

	skill, ok := findSkill(skills, args[0])
	if !ok {
		return fmt.Errorf("skill %q not found — run `scribe list` to see installed skills", args[0])
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

	if rawFlag {
		return explainRendered(w, skill, content)
	}

	// Default interactive mode: show preview, expand on Enter.
	body := stripFrontmatter(content)
	preview, hasMore := extractPreview(body, previewParagraphs)

	printSkillHeader(w, skill)
	if err := renderMarkdownTo(w, preview); err != nil {
		return err
	}

	stdinTTY := isatty.IsTerminal(os.Stdin.Fd())

	if hasMore && stdinTTY {
		if promptExpand(w) {
			fullRendered, err := renderMarkdownString(body)
			if err != nil {
				return err
			}
			if err := showInPager(fullRendered); err != nil {
				// Pager failed — fall back to inline print.
				fmt.Fprint(w, fullRendered)
			}
		}
	}

	// Offer AI explanation if an LLM CLI is available and stdin is interactive.
	if _, err := detectLLMCLI(); err == nil {
		if stdinTTY {
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
		Revision    int      `json:"revision,omitempty"`
		Targets     []string `json:"targets,omitempty"`
		Path        string   `json:"path,omitempty"`
		Content     string   `json:"content"`
	}{
		Name:        skill.Name,
		Description: skill.Description,
		Revision:    skill.Revision,
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
	if skill.Revision > 0 {
		meta = append(meta, kv{"Revision", fmt.Sprintf("rev %d", skill.Revision)})
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
	return renderMarkdownTo(w, body)
}

// previewParagraphs is the number of markdown paragraphs to show before
// prompting the user to expand. Paragraph = text separated by blank lines.
const previewParagraphs = 3

// extractPreview splits a markdown body at paragraph boundaries (blank lines).
// It returns the first maxParagraphs paragraphs and whether more content exists.
func extractPreview(body string, maxParagraphs int) (preview string, hasMore bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", false
	}
	parts := strings.SplitN(body, "\n\n", maxParagraphs+1)
	if len(parts) <= maxParagraphs {
		return body, false
	}
	return strings.Join(parts[:maxParagraphs], "\n\n"), true
}

// renderMarkdownTo renders a markdown string through glamour to the writer.
func renderMarkdownTo(w io.Writer, md string) error {
	rendered, err := glamour.Render(md, "auto")
	if err != nil {
		fmt.Fprintln(w, md)
		return nil
	}
	fmt.Fprint(w, rendered)
	return nil
}

// renderMarkdownString renders markdown to a string for use with a pager.
func renderMarkdownString(md string) (string, error) {
	rendered, err := glamour.Render(md, "auto")
	if err != nil {
		return md, nil
	}
	return rendered, nil
}

// promptExpand shows a hint and waits for the user to press Enter or q.
// Returns false on EOF/error (e.g. Ctrl-D) so closing stdin skips the pager.
func promptExpand(w io.Writer) bool {
	fmt.Fprintln(w, explDimStyle.Render("  ↵ Enter to read more · q to skip"))
	// Read a single line without buffering ahead — a bufio.NewReader would
	// consume bytes past the newline, stealing input from the huh prompt
	// that may follow (offerAIExplanation).
	buf := make([]byte, 0, 64)
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			return false // EOF or error → skip
		}
		if b[0] == '\n' {
			break
		}
		buf = append(buf, b[0])
	}
	return strings.TrimSpace(string(buf)) != "q"
}

// showInPager pipes content through the system pager (less, more, or $PAGER).
func showInPager(content string) error {
	pager := os.Getenv("PAGER")
	if pager == "" {
		if _, err := exec.LookPath("less"); err == nil {
			pager = "less -RFX"
		} else if _, err := exec.LookPath("more"); err == nil {
			pager = "more"
		} else {
			// No pager available — caller should fall back to inline print.
			return fmt.Errorf("no pager found")
		}
	}

	parts := strings.Fields(pager)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
