package pluginregistry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmbeddedRegistry(t *testing.T) {
	t.Parallel()

	index, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	goPlugin, ok := index.Find("org.bu1ld.go")
	if !ok {
		t.Fatalf("embedded registry missing org.bu1ld.go")
	}
	if got, want := goPlugin.Namespace, "go"; got != want {
		t.Fatalf("Namespace = %q, want %q", got, want)
	}
	latest, ok := goPlugin.LatestVersion()
	if !ok {
		t.Fatalf("org.bu1ld.go has no latest version")
	}
	if got, want := latest.Version, "0.1.0"; got != want {
		t.Fatalf("Version = %q, want %q", got, want)
	}
}

func TestLoadExternalRegistryResolvesRelativeAssets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plugins.toml"), `
version = 1

[[plugins]]
id = "org.example.echo"
file = "plugins/org.example.echo.toml"
`)
	writeFile(t, filepath.Join(root, "plugins", "org.example.echo.toml"), `
id = "org.example.echo"
namespace = "echo"

[[versions]]
version = "0.1.0"

[[versions.assets]]
url = "../assets/echo"
format = "dir"
`)

	index, err := Load(context.Background(), LoadOptions{Source: root})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	plugin, ok := index.Find("org.example.echo")
	if !ok {
		t.Fatalf("registry missing org.example.echo")
	}
	version, ok := plugin.LatestVersion()
	if !ok {
		t.Fatalf("org.example.echo has no latest version")
	}
	asset, ok := version.Asset("", "")
	if !ok {
		t.Fatalf("org.example.echo has no generic asset")
	}
	want := filepath.Join(root, "assets", "echo")
	if got := filepath.Clean(asset.URL); got != want {
		t.Fatalf("Asset URL = %q, want %q", got, want)
	}
}

func TestInstallDirAsset(t *testing.T) {
	t.Parallel()

	assetDir := t.TempDir()
	writeFile(t, filepath.Join(assetDir, "plugin.toml"), `
id = "org.example.echo"
namespace = "echo"
version = "0.1.0"
binary = "echo"
`)
	writeFile(t, filepath.Join(assetDir, "echo"), "#!/bin/sh\n")

	index, err := newIndex(1, []Plugin{
		{
			ID:        "org.example.echo",
			Namespace: "echo",
			Versions: []PluginVersion{
				{
					Version: "0.1.0",
					Assets: []PluginAsset{
						{OS: "testos", Arch: "testarch", URL: assetDir, Format: "dir"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("newIndex() error = %v", err)
	}

	result, err := Install(context.Background(), index, InstallOptions{
		Ref:    "org.example.echo",
		Root:   t.TempDir(),
		GOOS:   "testos",
		GOARCH: "testarch",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, want := result.ID, "org.example.echo"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "plugin.toml")); err != nil {
		t.Fatalf("installed manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "echo")); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
