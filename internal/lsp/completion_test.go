package lsp

import (
	"bytes"
	"slices"
	"testing"

	"bu1ld/internal/dsl"

	"go.lsp.dev/protocol"
)

func TestCompletionFiltersTopLevelByPrefix(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	items := server.completions("wo", protocol.Position{Line: 0, Character: 2}).Items

	if got, want := completionLabels(items), []string{"workspace"}; !equalStrings(got, want) {
		t.Fatalf("top-level labels = %v, want %v", got, want)
	}
}

func TestCompletionFiltersPluginRulesByPrefix(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	items := server.completions("go.b", protocol.Position{Line: 0, Character: 4}).Items

	if got, want := completionLabels(items), []string{"go.binary"}; !equalStrings(got, want) {
		t.Fatalf("rule labels = %v, want %v", got, want)
	}
}

func TestCompletionFiltersFieldsInsideBlock(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	text := "go.binary build {\n  ma\n}\n"
	items := server.completions(text, protocol.Position{Line: 1, Character: 4}).Items

	if got, want := completionLabels(items), []string{"main"}; !equalStrings(got, want) {
		t.Fatalf("field labels = %v, want %v", got, want)
	}
}

func completionLabels(items []protocol.CompletionItem) []string {
	labels := make([]string, 0, len(items))
	for i := range items {
		item := items[i]
		labels = append(labels, item.Label)
	}
	return labels
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}
