package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestPluginsListCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
plugin go {
  source = builtin
  id = "builtin.go"
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "plugins", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"SOURCE", "builtin", "go", "builtin.go", "binary,test", "ok"} {
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
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "id": "org.bu1ld.rust",
  "namespace": "rust",
  "version": "0.1.0",
  "binary": "bu1ld-rust",
  "rules": [{"name": "binary"}]
}`), 0o600); err != nil {
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
plugin go {
  source = builtin
  id = "builtin.go"
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
	for _, want := range []string{`"source": "builtin"`, `"namespace": "go"`, `"id": "builtin.go"`} {
		if !strings.Contains(string(lock), want) {
			t.Fatalf("lockfile = %s, want substring %q", lock, want)
		}
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
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "id": "org.bu1ld.rust",
  "namespace": "rust",
  "version": "0.1.0",
  "binary": "bu1ld-rust"
}`), 0o600); err != nil {
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
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "plugins": [
    {
      "source": "local",
      "namespace": "rust",
      "id": "org.bu1ld.rust",
      "version": "0.1.0",
      "path": "`+binary+`",
      "checksum": "sha256:wrong"
    }
  ]
}`), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
