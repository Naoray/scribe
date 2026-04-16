package agent

import (
	"bytes"
	"fmt"
	"os/exec"
	"text/template"
	"time"

	"github.com/Naoray/scribe/internal/state"
)

type scribeAgentRenderContext struct {
	HasScribeBinary         bool
	HasScribeAgentInstalled bool
	NeedsBootstrap          bool
	ShowDailyUpgradePrompt  bool
}

var scribeAgentTemplate = template.Must(template.New("scribe-agent").Parse(string(EmbeddedSkillTemplate)))

func buildScribeAgentRenderContext(st *state.State) scribeAgentRenderContext {
	return buildScribeAgentRenderContextAt(st, time.Now().UTC())
}

func buildScribeAgentRenderContextAt(st *state.State, now time.Time) scribeAgentRenderContext {
	hasBinary := hasScribeBinary()
	hasInstalled := st != nil && st.Installed != nil
	if hasInstalled {
		_, hasInstalled = st.Installed["scribe-agent"]
	}
	needsBootstrap := !hasBinary || !hasInstalled
	showPrompt := !needsBootstrap && !st.ScribeBinaryUpdateCooldownFresh(now)
	return scribeAgentRenderContext{
		HasScribeBinary:         hasBinary,
		HasScribeAgentInstalled: hasInstalled,
		NeedsBootstrap:          needsBootstrap,
		ShowDailyUpgradePrompt:  showPrompt,
	}
}

func buildScribeAgentRenderContextForStore(store string, st *state.State) scribeAgentRenderContext {
	_ = store
	return buildScribeAgentRenderContextAt(st, time.Now().UTC())
}

func buildScribeAgentRenderContextForStoreAt(store string, st *state.State, now time.Time) scribeAgentRenderContext {
	_ = store
	return buildScribeAgentRenderContextAt(st, now)
}

func renderScribeAgentMarkdown(ctx scribeAgentRenderContext) ([]byte, error) {
	var buf bytes.Buffer
	if err := scribeAgentTemplate.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("render scribe-agent skill: %w", err)
	}
	return buf.Bytes(), nil
}

// RenderSkillTemplate parses tmpl as a Go text/template and renders it with
// the same context that EnsureScribeAgent uses. Use this when upgrading from
// a fetched SKILL.md.tmpl so the bootstrap section is shown only when needed.
func RenderSkillTemplate(tmpl []byte, store string, st *state.State) ([]byte, error) {
	t, err := template.New("scribe-agent").Parse(string(tmpl))
	if err != nil {
		return nil, fmt.Errorf("parse scribe-agent template: %w", err)
	}
	ctx := buildScribeAgentRenderContextForStore(store, st)
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("render scribe-agent skill: %w", err)
	}
	return buf.Bytes(), nil
}

func renderScribeAgentForStore(store string, st *state.State) ([]byte, scribeAgentRenderContext, error) {
	ctx := buildScribeAgentRenderContextForStore(store, st)
	rendered, err := renderScribeAgentMarkdown(ctx)
	if err != nil {
		return nil, scribeAgentRenderContext{}, err
	}
	return rendered, ctx, nil
}

func hasScribeBinary() bool {
	_, err := exec.LookPath("scribe")
	return err == nil
}
