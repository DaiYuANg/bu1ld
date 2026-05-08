package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestPluginsListCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
plugin archive {
  source = builtin
  id = "builtin.archive"
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"SOURCE", "builtin", "archive", "builtin.archive", "tar,zip", "ok"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestPluginsListIncludesInstalledManifest(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, ``)
	pluginDir := filepath.Join(projectDir, ".bu1ld", "plugins", "org.bu1ld.rust", "0.1.0")
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`
id = "org.bu1ld.rust"
namespace = "rust"
version = "0.1.0"
binary = "bu1ld-rust"

[[rules]]
name = "binary"
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	pluginBinary := filepath.Join(pluginDir, "bu1ld-rust")
	if err := os.WriteFile(pluginBinary, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}
	if err := afero.NewOsFs().Chmod(pluginBinary, 0o500); err != nil {
		t.Fatalf("chmod plugin binary: %v", err)
	}

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"local", "rust", "org.bu1ld.rust", "0.1.0", "binary", "installed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestPluginsLockCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
plugin archive {
  source = builtin
  id = "builtin.archive"
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "lock"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "wrote") {
		t.Fatalf("output = %q, want wrote", got)
	}

	lock, err := afero.ReadFile(afero.NewOsFs(), filepath.Join(projectDir, "bu1ld.lock"))
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	for _, want := range []string{`"source": "builtin"`, `"namespace": "archive"`, `"id": "builtin.archive"`} {
		if !strings.Contains(string(lock), want) {
			t.Fatalf("lockfile = %s, want substring %q", lock, want)
		}
	}
}

func TestPluginsSearchCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, ``)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "search", "go"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "org.bu1ld.go", "go", "0.1.2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestPluginsInfoCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, ``)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "info", "org.bu1ld.java"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"id: org.bu1ld.java", "namespace: java", "JPMS", "0.1.2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestPluginsInstallCommandInstallsFromExternalRegistry(t *testing.T) {
	projectDir := t.TempDir()
	registryDir := t.TempDir()
	writeBuildFile(t, projectDir, ``)
	t.Setenv("BU1LD_PLUGIN_REGISTRY", registryDir)

	writeRegistryTestFile(t, filepath.Join(registryDir, "plugins.toml"), `
version = 1

[[plugins]]
id = "org.example.echo"
file = "plugins/org.example.echo.toml"
`)
	writeRegistryTestFile(t, filepath.Join(registryDir, "plugins", "org.example.echo.toml"), fmt.Sprintf(`
id = "org.example.echo"
namespace = "echo"
description = "Test plugin"

[[versions]]
version = "0.1.0"

[[versions.assets]]
os = "%s"
arch = "%s"
url = "../assets/echo"
format = "dir"
`, runtime.GOOS, runtime.GOARCH))
	writeRegistryTestFile(t, filepath.Join(registryDir, "assets", "echo", "plugin.toml"), `
id = "org.example.echo"
namespace = "echo"
version = "0.1.0"
binary = "echo"
`)
	writeRegistryTestFile(t, filepath.Join(registryDir, "assets", "echo", "echo"), "#!/bin/sh\n")

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "install", "org.example.echo"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "installed org.example.echo@0.1.0") {
		t.Fatalf("output = %q, want install message", got)
	}

	manifest := filepath.Join(projectDir, ".bu1ld", "plugins", "org.example.echo", "0.1.0", "plugin.toml")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("installed manifest missing: %v", err)
	}
}

func TestPluginsDoctorReportsLockChecksumMismatch(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, ``)
	pluginDir := filepath.Join(projectDir, ".bu1ld", "plugins", "org.bu1ld.rust", "0.1.0")
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	binary := filepath.Join(pluginDir, "bu1ld-rust")
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`
id = "org.bu1ld.rust"
namespace = "rust"
version = "0.1.0"
binary = "bu1ld-rust"
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}
	if err := afero.NewOsFs().Chmod(binary, 0o500); err != nil {
		t.Fatalf("chmod plugin binary: %v", err)
	}
	if err := writeWrongPluginLock(projectDir, binary); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "doctor"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want lock mismatch")
	}

	got := out.String()
	for _, want := range []string{"lock-mismatch", "checksum", "org.bu1ld.rust"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestPluginsDoctorReportsMissingLocalPlugin(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
plugin rust {
  source = local
  id = "org.bu1ld.rust"
  version = "0.1.0"
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "doctor"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want doctor issue")
	}

	got := out.String()
	for _, want := range []string{"local plugins:", "org.bu1ld.rust", "missing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func writeWrongPluginLock(projectDir, binary string) error {
	path := filepath.Join(projectDir, "bu1ld.lock")
	lock := map[string]any{
		"version": 1,
		"plugins": []map[string]string{
			{
				"source":    "local",
				"namespace": "rust",
				"id":        "org.bu1ld.rust",
				"version":   "0.1.0",
				"path":      binary,
				"checksum":  "sha256:wrong",
			},
		},
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal wrong plugin lock: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeRegistryTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
