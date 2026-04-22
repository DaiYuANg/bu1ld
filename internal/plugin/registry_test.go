package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryExpandsBuiltinAlias(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(LoadOptions{}, fakePlugin{})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	defer registry.Close()

	if err := registry.Declare(context.Background(), Declaration{
		Namespace: "alias",
		ID:        "builtin.fake",
		Source:    SourceBuiltin,
	}); err != nil {
		t.Fatalf("Declare() error = %v", err)
	}

	tasks, err := registry.Expand(context.Background(), Invocation{
		Namespace: "alias",
		Rule:      "echo",
		Target:    "hello",
		Fields: map[string]any{
			"message": "world",
		},
	})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got, want := tasks[0].Command.Values()[1], "world"; got != want {
		t.Fatalf("command arg = %q, want %q", got, want)
	}
}

func TestRegistryRejectsUnknownField(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(LoadOptions{}, fakePlugin{})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	defer registry.Close()

	_, err = registry.Expand(context.Background(), Invocation{
		Namespace: "fake",
		Rule:      "echo",
		Target:    "hello",
		Fields: map[string]any{
			"message": "world",
			"other":   "field",
		},
	})
	if err == nil {
		t.Fatalf("Expand() error = nil, want unknown field error")
	}
}

func TestProcessLoaderResolvesLocalAndGlobalPaths(t *testing.T) {
	t.Parallel()

	loader := NewProcessLoader(LoadOptions{
		ProjectDir: "/workspace",
		LocalDir:   "/workspace/.bu1ld/plugins",
		GlobalDir:  "/home/user/.bu1ld/plugins",
	})

	localDev, err := loader.resolvePath(Declaration{
		Namespace: "rust",
		Source:    SourceLocal,
		Path:      "./tools/rust-plugin",
	})
	if err != nil {
		t.Fatalf("resolve local dev path: %v", err)
	}
	if got, want := localDev, filepath.Join("/workspace", "./tools/rust-plugin"); got != want {
		t.Fatalf("local dev path = %q, want %q", got, want)
	}

	localInstalled, err := loader.resolvePath(Declaration{
		Namespace: "rust",
		Source:    SourceLocal,
		ID:        "org.bu1ld.rust",
		Version:   "0.1.0",
	})
	if err != nil {
		t.Fatalf("resolve local installed path: %v", err)
	}
	localWant := filepath.Join("/workspace/.bu1ld/plugins", "org.bu1ld.rust", "0.1.0", "org.bu1ld.rust")
	if localInstalled != localWant {
		t.Fatalf("local installed path = %q, want %q", localInstalled, localWant)
	}

	global, err := loader.resolvePath(Declaration{
		Namespace: "java",
		Source:    SourceGlobal,
		ID:        "org.bu1ld.java",
		Version:   "0.1.0",
	})
	if err != nil {
		t.Fatalf("resolve global path: %v", err)
	}
	want := filepath.Join("/home/user/.bu1ld/plugins", "org.bu1ld.java", "0.1.0", "org.bu1ld.java")
	if global != want {
		t.Fatalf("global path = %q, want %q", global, want)
	}
}

func TestProcessLoaderDiscoversInstalledPluginPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	discovered := filepath.Join(root, "org.bu1ld.rust", "0.1.0", "bu1ld-rust")
	if err := os.MkdirAll(filepath.Dir(discovered), 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(discovered, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	loader := NewProcessLoader(LoadOptions{LocalDir: root})
	path, err := loader.resolvePath(Declaration{
		Namespace: "rust",
		Source:    SourceLocal,
		ID:        "org.bu1ld.rust",
		Version:   "0.1.0",
	})
	if err != nil {
		t.Fatalf("resolve discovered plugin path: %v", err)
	}
	if path != discovered {
		t.Fatalf("discovered path = %q, want %q", path, discovered)
	}
}

type fakePlugin struct{}

func (p fakePlugin) Metadata() (Metadata, error) {
	return Metadata{
		ID:        "builtin.fake",
		Namespace: "fake",
		Rules: []RuleSchema{
			{
				Name: "echo",
				Fields: []FieldSchema{
					{Name: "message", Type: FieldString, Required: true},
				},
			},
		},
	}, nil
}

func (p fakePlugin) Expand(_ context.Context, invocation Invocation) ([]TaskSpec, error) {
	message, err := invocation.RequiredString("message")
	if err != nil {
		return nil, err
	}
	return []TaskSpec{
		{
			Name:    invocation.Target,
			Command: []string{"echo", message},
		},
	}, nil
}
