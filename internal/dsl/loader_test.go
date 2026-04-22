package dsl

import (
	"os"
	"path/filepath"
	"testing"

	"bu1ld/internal/config"
)

func TestLoaderImportsBuildFiles(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `
plugin go {
  source = builtin
  id = "builtin.go"
}

import "tasks/*.bu1ld"

task root {
  command = []
}
`)
	writeDSLFile(t, projectDir, "tasks/go.bu1ld", `
go.test test {
  packages = ["./..."]
}
`)
	writeDSLFile(t, projectDir, "tasks/custom.bu1ld", `
task package {
  outputs = [$("dist/" + target)]
  command = ["sh", "-c", concat("echo ", target)]
}
`)

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	project, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, name := range []string{"package", "root", "test"} {
		if _, ok := project.FindTask(name); !ok {
			t.Fatalf("task %q not found", name)
		}
	}
	task, _ := project.FindTask("package")
	if got, want := task.Outputs.Values()[0], "dist/package"; got != want {
		t.Fatalf("package output = %q, want %q", got, want)
	}
}

func TestLoaderRejectsImportCycles(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `import "tasks/a.bu1ld"`)
	writeDSLFile(t, projectDir, "tasks/a.bu1ld", `import "../build.bu1ld"`)

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	if _, err := loader.Load(); err == nil {
		t.Fatalf("Load() error = nil, want import cycle error")
	}
}

func writeDSLFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
