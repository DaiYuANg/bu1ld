package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestPackagesCommand(t *testing.T) {
	t.Parallel()

	projectDir := writeMonorepoProject(t)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "packages"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "apps/api\nlibs/core\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestPackagesGraphCommand(t *testing.T) {
	t.Parallel()

	projectDir := writeMonorepoProject(t)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "packages", "graph"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"apps/api -> libs/core", "libs/core"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
}

func TestBuildAllPackagesLocalTask(t *testing.T) {
	t.Parallel()

	projectDir := writeMonorepoProject(t)

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "--no-cache", "build", "--all", ":build"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"> libs/core:build\n", "> apps/api:build\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want substring %q", got, want)
		}
	}
	if strings.Index(got, "> libs/core:build") > strings.Index(got, "> apps/api:build") {
		t.Fatalf("output = %q, want dependency package before dependent package", got)
	}
}

func TestAffectedCommandIncludesDependentPackages(t *testing.T) {
	t.Parallel()

	projectDir := writeMonorepoProject(t)
	commitMonorepoProject(t, projectDir)
	writeProjectFile(t, projectDir, "libs/core/src/message.txt", "changed\n")

	var out bytes.Buffer
	cmd := NewRootCommand(&out)
	cmd.SetArgs([]string{"--project-dir", projectDir, "affected", "--base", "HEAD"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "apps/api\nlibs/core\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func writeMonorepoProject(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	writeBuildFile(t, projectDir, `
workspace {
  name = "mono"
  packages = ["apps/*", "libs/*"]
}
`)
	writeProjectFile(t, projectDir, "libs/core/build.bu1ld", `
package {
  name = "libs/core"
}

task build {
  command = []
}
`)
	writeProjectFile(t, projectDir, "libs/core/src/message.txt", "initial\n")
	writeProjectFile(t, projectDir, "apps/api/build.bu1ld", `
package {
  name = "apps/api"
  deps = ["libs/core"]
}

task build {
  command = []
}
`)
	writeProjectFile(t, projectDir, "apps/api/src/message.txt", "initial\n")
	return projectDir
}

func writeProjectFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func commitMonorepoProject(t *testing.T, dir string) {
	t.Helper()
	repository, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	for _, path := range monorepoFiles() {
		if _, addErr := worktree.Add(path); addErr != nil {
			t.Fatalf("add %s: %v", path, addErr)
		}
	}
	if _, err = worktree.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Unix(1_700_000_000, 0),
		},
	}); err != nil {
		t.Fatalf("commit repository: %v", err)
	}
}

func monorepoFiles() []string {
	return []string{
		"build.bu1ld",
		"libs/core/build.bu1ld",
		"libs/core/src/message.txt",
		"apps/api/build.bu1ld",
		"apps/api/src/message.txt",
	}
}
