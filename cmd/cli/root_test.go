package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
task "test" {
  command = []
}

task "build" {
  deps = ["test"]
  command = []
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "graph"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "build -> test\ntest\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestGraphCommandWithTarget(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
task "prepare" {
  command = []
}

task "test" {
  command = []
}

task "build" {
  deps = ["prepare", "test"]
  command = []
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "graph", "build"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "prepare\ntest\nbuild -> prepare, test\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTasksCommand(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
task "test" {
  command = []
}

task "build" {
  command = []
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "tasks"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "build\ntest\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestBuildCommandRunsNoopTask(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
task "build" {
  command = []
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "--no-cache", "build"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"> build\n", "  NOOP build\n", "  DONE build"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestDaemonStatusCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"daemon", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "daemon status") {
		t.Fatalf("output = %q, want daemon status", got)
	}
}

func TestServerStatusCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"server", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "server status") {
		t.Fatalf("output = %q, want server status", got)
	}
}

func writeBuildFile(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "build.bu1ld"), []byte(content), 0o644); err != nil {
		t.Fatalf("write build file: %v", err)
	}
}
