package lsp

import (
	"bytes"
	"slices"
	"testing"

	"bu1ld/internal/dsl"

	"github.com/arcgolabs/collectionx/list"
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
	items := server.completions("archive.z", protocol.Position{Line: 0, Character: 9}).Items

	if got, want := completionLabels(items), []string{"archive.zip"}; !equalStrings(got, want) {
		t.Fatalf("rule labels = %v, want %v", got, want)
	}
}

func TestCompletionFiltersFieldsInsideBlock(t *testing.T) {
	t.Parallel()

	server := New(dsl.NewParser(), &bytes.Buffer{}, &bytes.Buffer{})
	text := "archive.zip package {\n  ou\n}\n"
	items := server.completions(text, protocol.Position{Line: 1, Character: 4}).Items

	if got, want := completionLabels(items), []string{"out"}; !equalStrings(got, want) {
		t.Fatalf("field labels = %v, want %v", got, want)
	}
}

func completionLabels(items []protocol.CompletionItem) []string {
	labels := list.NewListWithCapacity[string](len(items))
	for i := range items {
		item := items[i]
		labels.Add(item.Label)
	}
	return labels.Values()
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}
