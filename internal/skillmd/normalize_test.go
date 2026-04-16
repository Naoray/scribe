package skillmd

import (
	"strings"
	"testing"
)

func TestNormalizeAddsMissingFrontmatterFromDirectoryName(t *testing.T) {
	doc, normalized, err := Normalize("ascii", []byte("# ASCII Diagram Generator\n\nCreate ASCII diagrams.\n"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if doc.Name != "ascii" {
		t.Fatalf("Name = %q, want %q", doc.Name, "ascii")
	}
	if doc.Description != "Create ASCII diagrams." {
		t.Fatalf("Description = %q, want %q", doc.Description, "Create ASCII diagrams.")
	}
	if !doc.Changed {
		t.Fatal("Changed = false, want true")
	}

	content := string(normalized)
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("normalized content missing frontmatter:\n%s", content)
	}
	if !strings.Contains(content, "name: ascii\n") {
		t.Fatalf("normalized content missing name:\n%s", content)
	}
	if !strings.Contains(content, "description: Create ASCII diagrams.\n") {
		t.Fatalf("normalized content missing description:\n%s", content)
	}
	if !strings.Contains(content, "# ASCII Diagram Generator\n") {
		t.Fatalf("normalized content missing body:\n%s", content)
	}
}

func TestNormalizeFillsMissingDescriptionFromFirstParagraph(t *testing.T) {
	content := []byte(`---
name: ascii
---

# ASCII Diagram Generator

Create ASCII diagrams for flows,
architectures, and processes.
`)

	doc, normalized, err := Normalize("ascii", content)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if doc.Name != "ascii" {
		t.Fatalf("Name = %q, want %q", doc.Name, "ascii")
	}
	if doc.Description != "Create ASCII diagrams for flows, architectures, and processes." {
		t.Fatalf("Description = %q", doc.Description)
	}
	if !doc.Changed {
		t.Fatal("Changed = false, want true")
	}
	if !strings.Contains(string(normalized), "description: Create ASCII diagrams for flows, architectures, and processes.\n") {
		t.Fatalf("normalized content missing extracted description:\n%s", normalized)
	}
}

func TestNormalizeSkipsHeadingsListsAndCodeFences(t *testing.T) {
	body := strings.Join([]string{
		"# ASCII Diagram Generator",
		"",
		"- ignore this bullet",
		"* ignore this bullet too",
		"> ignore this quote",
		"| ignore this table row |",
		"```",
		"still ignore this",
		"```",
		"",
		"Create ASCII diagrams for flows, architectures, and processes.",
	}, "\n")

	if got := ExtractFallbackDescription(body); got != "Create ASCII diagrams for flows, architectures, and processes." {
		t.Fatalf("ExtractFallbackDescription = %q", got)
	}
}

func TestNormalizeRejectsUnrecoverableFrontmatter(t *testing.T) {
	_, _, err := Normalize("ascii", []byte(`---
name: ascii

# missing closing delimiter
`))
	if err == nil {
		t.Fatal("Normalize returned nil error for unrecoverable frontmatter")
	}
}

func TestNormalizeCanonicalizesExistingFrontmatter(t *testing.T) {
	input := []byte(`---
description: Create ASCII diagrams.
name: ascii
---

Body
`)

	_, normalized, err := Normalize("ascii", input)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if string(normalized) == string(input) {
		t.Fatal("Normalize preserved non-canonical frontmatter; want deterministic rewrite")
	}
	if !strings.HasPrefix(string(normalized), "---\n") {
		t.Fatalf("normalized content missing frontmatter:\n%s", normalized)
	}
	if !strings.Contains(string(normalized), "name: ascii\n") {
		t.Fatalf("normalized content missing name:\n%s", normalized)
	}
	if !strings.Contains(string(normalized), "description: Create ASCII diagrams.\n") {
		t.Fatalf("normalized content missing description:\n%s", normalized)
	}
}

func TestNormalizePreservesExtraFrontmatterKeys(t *testing.T) {
	input := []byte(`---
license: MIT
description: Create ASCII diagrams.
name: ascii
tags:
  - diagrams
  - ascii
---

Body
`)

	_, normalized, err := Normalize("ascii", input)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	content := string(normalized)
	if !strings.Contains(content, "name: ascii\n") {
		t.Fatalf("normalized content missing name:\n%s", content)
	}
	if !strings.Contains(content, "description: Create ASCII diagrams.\n") {
		t.Fatalf("normalized content missing description:\n%s", content)
	}
	if !strings.Contains(content, "license: MIT\n") {
		t.Fatalf("normalized content dropped extra key:\n%s", content)
	}
	if !strings.Contains(content, "- diagrams\n") || !strings.Contains(content, "- ascii\n") {
		t.Fatalf("normalized content dropped extra sequence:\n%s", content)
	}
}
