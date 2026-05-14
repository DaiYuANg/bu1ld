package dsl

import (
	"testing"

	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"

	"github.com/spf13/afero"
)

func TestRawPluginDeclarationsParsesContainerFields(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	const path = "build.bu1ld"
	if err := afero.WriteFile(fs, path, []byte(`
plugin go {
  source = container
  id = "org.bu1ld.go"
  version = "0.1.3"
  image = "ghcr.io/example/bu1ld-go-plugin:0.1.3"
  pull = "never"
  network = "none"
  work_dir = "/repo"
}
`), 0o600); err != nil {
		t.Fatalf("write build file: %v", err)
	}

	declarations, err := RawPluginDeclarations(fs, path)
	if err != nil {
		t.Fatalf("RawPluginDeclarations() error = %v", err)
	}
	if got, want := len(declarations), 1; got != want {
		t.Fatalf("declaration count = %d, want %d", got, want)
	}
	declaration := declarations[0].Declaration
	if got, want := declaration.Source, buildplugin.SourceContainer; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if got, want := declaration.Image, "ghcr.io/example/bu1ld-go-plugin:0.1.3"; got != want {
		t.Fatalf("image = %q, want %q", got, want)
	}
	if got, want := declaration.Pull, "never"; got != want {
		t.Fatalf("pull = %q, want %q", got, want)
	}
	if got, want := declaration.Network, "none"; got != want {
		t.Fatalf("network = %q, want %q", got, want)
	}
	if got, want := declaration.WorkDir, "/repo"; got != want {
		t.Fatalf("work_dir = %q, want %q", got, want)
	}
}
