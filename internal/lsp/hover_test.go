package lsp

import (
	"bytes"
	"strings"
	"testing"

	"bu1ld/internal/dsl"

	"go.lsp.dev/protocol"
)

func TestHoverDescribesPluginRule(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	text := "go.binary build {\n}\n"
	hover := server.hover(text, protocol.Position{Line: 0, Character: 3})
	if hover == nil {
		t.Fatal("hover = nil, want plugin rule hover")
	}
	for _, want := range []string{"go.binary name", "builtin.go rule"} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Fatalf("hover = %q, want substring %q", hover.Contents.Value, want)
		}
	}
}

func TestHoverDescribesPluginRuleField(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	text := "go.binary build {\n  main = \"./cmd/cli\"\n}\n"
	hover := server.hover(text, protocol.Position{Line: 1, Character: 3})
	if hover == nil {
		t.Fatal("hover = nil, want field hover")
	}
	for _, want := range []string{"main = \"\"", "string required", "Field for go.binary."} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Fatalf("hover = %q, want substring %q", hover.Contents.Value, want)
		}
	}
}

func TestHoverDescribesRunAction(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	text := "task build {\n  run {\n    shell(\"go test ./...\")\n  }\n}\n"
	hover := server.hover(text, protocol.Position{Line: 2, Character: 6})
	if hover == nil {
		t.Fatal("hover = nil, want action hover")
	}
	for _, want := range []string{`shell("script")`, "POSIX shell snippet"} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Fatalf("hover = %q, want substring %q", hover.Contents.Value, want)
		}
	}
}
