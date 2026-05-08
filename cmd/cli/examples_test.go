package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
	"github.com/spf13/afero"
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
	values := list.NewList("--project-dir", projectDir)
	values.Add(args...)
	cmd.SetArgs(values.Values())
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

func copyDir(source, target string) error {
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return oops.In("bu1ld.test").Wrapf(walkErr, "walk example")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return oops.In("bu1ld.test").Wrapf(err, "resolve example path")
		}
		if rel == "." {
			return nil
		}
		out := filepath.Join(target, rel)
		if entry.IsDir() {
			if err := os.MkdirAll(out, 0o750); err != nil {
				return oops.In("bu1ld.test").With("path", out).Wrapf(err, "create example directory")
			}
			return nil
		}
		return copyFile(path, out)
	}); err != nil {
		return oops.In("bu1ld.test").Wrapf(err, "copy example directory")
	}
	return nil
}

func copyFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return oops.In("bu1ld.test").With("path", filepath.Dir(target)).Wrapf(err, "create target directory")
	}
	data, err := afero.ReadFile(afero.NewOsFs(), source)
	if err != nil {
		return oops.In("bu1ld.test").With("path", source).Wrapf(err, "read source file")
	}
	if err := afero.WriteFile(afero.NewOsFs(), target, data, 0o600); err != nil {
		return oops.In("bu1ld.test").With("path", target).Wrapf(err, "write target file")
	}
	return nil
}

func assertZipContains(t *testing.T, path, name string) {
	t.Helper()

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %q: %v", path, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Fatalf("close zip %q: %v", path, err)
		}
	}()

	for _, file := range reader.File {
		if file.Name == name {
			return
		}
	}
	t.Fatalf("zip %q does not contain %q", path, name)
}
