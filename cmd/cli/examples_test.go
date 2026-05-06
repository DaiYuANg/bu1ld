package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveExampleEndToEnd(t *testing.T) {
	t.Parallel()

	projectDir := copyExample(t, "archive-basic")

	tasksOutput := executeCLI(t, projectDir, "tasks")
	for _, want := range []string{"package_tgz", "package_zip"} {
		if !strings.Contains(tasksOutput, want) {
			t.Fatalf("tasks output = %q, want %q", tasksOutput, want)
		}
	}

	graphOutput := executeCLI(t, projectDir, "graph", "package_zip")
	if got, want := graphOutput, "package_zip\n"; got != want {
		t.Fatalf("graph output = %q, want %q", got, want)
	}

	firstBuild := executeCLI(t, projectDir, "build", "package_zip")
	for _, want := range []string{"> package_zip\n", "  DONE package_zip"} {
		if !strings.Contains(firstBuild, want) {
			t.Fatalf("first build output = %q, want %q", firstBuild, want)
		}
	}
	assertZipContains(t, filepath.Join(projectDir, "dist", "source.zip"), "src/message.txt")

	secondBuild := executeCLI(t, projectDir, "build", "package_zip")
	if !strings.Contains(secondBuild, "FROM-CACHE package_zip") {
		t.Fatalf("second build output = %q, want cache hit", secondBuild)
	}
}

func TestInitCommandEndToEnd(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(init) error = %v\noutput:\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "initialized bu1ld project") {
		t.Fatalf("init output = %q, want initialized message", got)
	}

	tasksOutput := executeCLI(t, projectDir, "tasks")
	for _, want := range []string{"build", "package_zip"} {
		if !strings.Contains(tasksOutput, want) {
			t.Fatalf("tasks output = %q, want %q", tasksOutput, want)
		}
	}

	buildOutput := executeCLI(t, projectDir, "build")
	for _, want := range []string{"> package_zip\n", "  DONE package_zip", "> build\n", "  NOOP build"} {
		if !strings.Contains(buildOutput, want) {
			t.Fatalf("build output = %q, want %q", buildOutput, want)
		}
	}
	assertZipContains(t, filepath.Join(projectDir, "dist", "source.zip"), "src/message.txt")
}

func TestInitCommandRefusesExistingStarterFiles(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
task build {
  command = []
}
`)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute(init) error = nil, want existing file error")
	}
	if got, want := err.Error(), "rerun with --force"; !strings.Contains(got, want) {
		t.Fatalf("Execute(init) error = %q, want substring %q", got, want)
	}
}

func executeCLI(t *testing.T, projectDir string, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs(append([]string{"--project-dir", projectDir}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v\noutput:\n%s", args, err, out.String())
	}
	return out.String()
}

func copyExample(t *testing.T, name string) string {
	t.Helper()

	source := filepath.Join("..", "..", "examples", name)
	target := t.TempDir()
	if err := copyDir(source, target); err != nil {
		t.Fatalf("copy example %q: %v", name, err)
	}
	return target
}

func copyDir(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		out := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		return copyFile(path, out)
	})
}

func copyFile(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func assertZipContains(t *testing.T, path string, name string) {
	t.Helper()

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %q: %v", path, err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == name {
			return
		}
	}
	t.Fatalf("zip %q does not contain %q", path, name)
}
