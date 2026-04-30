package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"

	scribecmd "github.com/Naoray/scribe/cmd"
	"github.com/Naoray/scribe/internal/cli/schema"
)

func main() {
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	tmplPath := filepath.Join(root, "docs", "agent", "CLAUDE.md.tmpl")
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		fatal(fmt.Errorf("read template: %w", err))
	}
	tmpl, err := template.New("CLAUDE.md").Parse(string(raw))
	if err != nil {
		fatal(fmt.Errorf("parse template: %w", err))
	}
	data := struct {
		CommandTable string
	}{
		CommandTable: schema.RenderMarkdown(scribecmd.RootCommandForDocs(), schema.All()),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		fatal(fmt.Errorf("render template: %w", err))
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), buf.Bytes(), 0o644); err != nil {
		fatal(fmt.Errorf("write CLAUDE.md: %w", err))
	}
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve generator path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..")), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
