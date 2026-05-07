package goplugin

import (
	"context"
	"strings"
	"testing"

	"bu1ld/pkg/pluginapi"
)

func TestMetadataUsesExternalPluginID(t *testing.T) {
	t.Parallel()

	metadata, err := New().Metadata()
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if got, want := metadata.ID, "org.bu1ld.go"; got != want {
		t.Fatalf("metadata id = %q, want %q", got, want)
	}
	if got, want := metadata.Namespace, "go"; got != want {
		t.Fatalf("metadata namespace = %q, want %q", got, want)
	}
}

func TestExpandBinary(t *testing.T) {
	t.Parallel()

	tasks, err := New().Expand(context.Background(), pluginapi.Invocation{
		Namespace: "go",
		Rule:      "binary",
		Target:    "build",
		Fields: map[string]any{
			"main": "./cmd/cli",
			"out":  "dist/bu1ld",
		},
	})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got, want := len(tasks), 1; got != want {
		t.Fatalf("task count = %d, want %d", got, want)
	}
	if got, want := strings.Join(tasks[0].Command, " "), "go build -o dist/bu1ld ./cmd/cli"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}
