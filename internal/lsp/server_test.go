package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"bu1ld/internal/dsl"
)

func TestServerPublishesDiagnostics(t *testing.T) {
	t.Parallel()

	var in bytes.Buffer
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":  "file:///workspace/build.bu1ld",
				"text": "task build {\n  command = []\n}\n",
			},
		},
	})
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///workspace/build.bu1ld"},
			"contentChanges": []map[string]any{
				{"text": "task build {\n  unknown = []\n}\n"},
			},
		},
	})

	var out bytes.Buffer
	server := New(dsl.NewParser(), &in, &out)
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"textDocumentSync":1`,
		`"completionProvider"`,
		`"diagnostics":[]`,
		`unknown task field`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %s, want substring %q", got, want)
		}
	}
}

func TestServerCompletesPluginRulesAndFields(t *testing.T) {
	t.Parallel()

	text := strings.Join([]string{
		"plugin go {",
		"  source = builtin",
		"  id = \"builtin.go\"",
		"}",
		"",
		"go.binary build {",
		"  ",
		"}",
		"",
	}, "\n")

	var in bytes.Buffer
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":  "file:///workspace/build.bu1ld",
				"text": text,
			},
		},
	})
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///workspace/build.bu1ld"},
			"position":     map[string]any{"line": 8, "character": 0},
		},
	})
	writeTestMessage(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///workspace/build.bu1ld"},
			"position":     map[string]any{"line": 6, "character": 2},
		},
	})

	var out bytes.Buffer
	server := New(dsl.NewParser(), &in, &out)
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"label":"import"`,
		`"label":"go.binary"`,
		`"label":"go.test"`,
		`"label":"main"`,
		`"label":"out"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %s, want substring %q", got, want)
		}
	}
}

func writeTestMessage(t *testing.T, out *bytes.Buffer, message any) {
	t.Helper()

	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(payload), payload); err != nil {
		t.Fatalf("write message: %v", err)
	}
}
