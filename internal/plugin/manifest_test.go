package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveManifestPathUsesManifestBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "org.bu1ld.rust", "0.1.0")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	writePluginManifest(t, filepath.Join(dir, ManifestFileName), `{
  "id": "org.bu1ld.rust",
  "namespace": "rust",
  "version": "0.1.0",
  "binary": "bu1ld-rust",
  "rules": [{"name": "binary"}]
}`)
	binary := filepath.Join(dir, "bu1ld-rust")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}
	if err := os.Chmod(binary, 0o500); err != nil {
		t.Fatalf("chmod plugin binary: %v", err)
	}

	path, ok, err := ResolveManifestPath(root, Declaration{
		ID:      "org.bu1ld.rust",
		Version: "0.1.0",
	})
	if err != nil {
		t.Fatalf("ResolveManifestPath() error = %v", err)
	}
	if !ok {
		t.Fatalf("ResolveManifestPath() ok = false")
	}
	if path != binary {
		t.Fatalf("path = %q, want %q", path, binary)
	}
}

func TestDiscoverManifests(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "org.bu1ld.rust", "0.1.0")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	writePluginManifest(t, filepath.Join(dir, ManifestFileName), `{
  "id": "org.bu1ld.rust",
  "namespace": "rust",
  "version": "0.1.0",
  "binary": "bu1ld-rust"
}`)

	manifests, err := DiscoverManifests(root)
	if err != nil {
		t.Fatalf("DiscoverManifests() error = %v", err)
	}
	if got, want := len(manifests), 1; got != want {
		t.Fatalf("manifest count = %d, want %d", got, want)
	}
	if got, want := manifests[0].Manifest.ID, "org.bu1ld.rust"; got != want {
		t.Fatalf("manifest id = %q, want %q", got, want)
	}
}

func writePluginManifest(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
