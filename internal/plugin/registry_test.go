package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestRegistryExpandsBuiltinAlias(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(LoadOptions{}, fakePlugin{})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	defer registry.Close()

	if declareErr := registry.Declare(context.Background(), Declaration{
		Namespace: "alias",
		ID:        "builtin.fake",
		Source:    SourceBuiltin,
	}); declareErr != nil {
		t.Fatalf("Declare() error = %v", declareErr)
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
	if got, want := localDev, filepath.Join(string(filepath.Separator), "workspace", "tools", "rust-plugin"); got != want {
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
	localWant := filepath.Join(
		string(filepath.Separator),
		"workspace",
		".bu1ld",
		"plugins",
		"org.bu1ld.rust",
		"0.1.0",
		"org.bu1ld.rust",
	)
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
	want := filepath.Join(
		string(filepath.Separator),
		"home",
		"user",
		".bu1ld",
		"plugins",
		"org.bu1ld.java",
		"0.1.0",
		"org.bu1ld.java",
	)
	if global != want {
		t.Fatalf("global path = %q, want %q", global, want)
	}
}

func TestProcessLoaderDiscoversInstalledPluginPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	discovered := filepath.Join(root, "org.bu1ld.rust", "0.1.0", "bu1ld-rust")
	if err := os.MkdirAll(filepath.Dir(discovered), 0o750); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(discovered, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	if err := afero.NewOsFs().Chmod(discovered, 0o500); err != nil {
		t.Fatalf("chmod plugin: %v", err)
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

func TestProcessLoaderResolvesExplicitManifestPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "plugin.toml")
	binary := filepath.Join(root, "bin", "bu1ld-java-plugin")
	if err := os.MkdirAll(filepath.Dir(binary), 0o750); err != nil {
		t.Fatalf("mkdir plugin bin: %v", err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}
	writePluginManifestForLoader(t, manifest, `
id = "org.bu1ld.java"
namespace = "java"
version = "0.1.0"
binary = "bin/bu1ld-java-plugin"
`)

	loader := NewProcessLoader(LoadOptions{ProjectDir: filepath.Dir(root)})
	path, err := loader.resolvePath(Declaration{
		Namespace: "java",
		Source:    SourceLocal,
		Version:   "0.1.0",
		Path:      "./" + filepath.Base(root) + "/plugin.toml",
	})
	if err != nil {
		t.Fatalf("resolve manifest path: %v", err)
	}
	if path != binary {
		t.Fatalf("resolved path = %q, want %q", path, binary)
	}
}

func TestProcessLoaderResolvesExplicitManifestDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	binary := filepath.Join(root, "bu1ld-go-plugin")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}
	writePluginManifestForLoader(t, filepath.Join(root, ManifestFileName), `
id = "org.bu1ld.go"
namespace = "go"
version = "0.1.0"
binary = "bu1ld-go-plugin"
`)

	loader := NewProcessLoader(LoadOptions{})
	path, err := loader.resolvePath(Declaration{
		Namespace: "go",
		Source:    SourceLocal,
		ID:        "org.bu1ld.go",
		Path:      root,
	})
	if err != nil {
		t.Fatalf("resolve manifest directory: %v", err)
	}
	if path != binary {
		t.Fatalf("resolved path = %q, want %q", path, binary)
	}
}

func TestValidateProcessMetadataRejectsProtocolMismatch(t *testing.T) {
	t.Parallel()

	err := validateProcessMetadata(Declaration{Namespace: "go", ID: "org.bu1ld.go"}, Metadata{
		ID:              "org.bu1ld.go",
		Namespace:       "go",
		ProtocolVersion: 99,
		Capabilities:    []string{CapabilityMetadata, CapabilityExpand},
	})
	if err == nil {
		t.Fatalf("validateProcessMetadata() error = nil, want protocol mismatch")
	}
}

func TestProcessStderrPrefixesAndKeepsTail(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	stderr := newProcessStderr("plugin-test", &output)
	if _, err := stderr.Write([]byte("failed to start\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got, want := output.String(), "[plugin:plugin-test] failed to start\n"; got != want {
		t.Fatalf("stderr output = %q, want %q", got, want)
	}
	if got, want := stderr.Tail(), "failed to start"; got != want {
		t.Fatalf("stderr tail = %q, want %q", got, want)
	}
}

func writePluginManifestForLoader(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
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
		return nil, fmt.Errorf("read fake message field: %w", err)
	}
	return []TaskSpec{
		{
			Name:    invocation.Target,
			Command: []string{"echo", message},
		},
	}, nil
}
