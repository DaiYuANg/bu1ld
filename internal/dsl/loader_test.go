package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"
)

func TestLoaderImportsBuildFiles(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `
plugin go {
  source = builtin
  id = "builtin.go"
}

import "tasks/**/*.bu1ld"

task root {
  command = []
}
`)
	writeDSLFile(t, projectDir, "tasks/go/test.bu1ld", `
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

func TestLoaderUsesConfigCacheWhenBuildFilesUnchanged(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `
task actual {
  command = []
}
`)

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	buildFile, err := cleanAbsPath(filepath.Join(projectDir, "build.bu1ld"))
	if err != nil {
		t.Fatalf("cleanAbsPath() error = %v", err)
	}
	checksum, err := buildplugin.ChecksumFile(buildFile)
	if err != nil {
		t.Fatalf("ChecksumFile() error = %v", err)
	}
	writeConfigCacheRecord(t, loader, configCacheRecord{
		Version:   configCacheVersion,
		BuildFile: buildFile,
		Files: []configCacheFile{{
			Path:     buildFile,
			Checksum: checksum,
		}},
		Project: configCacheProject{
			Tasks: []configCacheTask{{Name: "cached"}},
		},
	})

	project, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := project.FindTask("cached"); !ok {
		t.Fatalf("cached task not found")
	}
	if _, ok := project.FindTask("actual"); ok {
		t.Fatalf("actual task found, want cached project")
	}
}

func TestLoaderBypassesConfigCacheWhenNoCache(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `
task actual {
  command = []
}
`)

	cacheLoader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	buildFile, err := cleanAbsPath(filepath.Join(projectDir, "build.bu1ld"))
	if err != nil {
		t.Fatalf("cleanAbsPath() error = %v", err)
	}
	checksum, err := buildplugin.ChecksumFile(buildFile)
	if err != nil {
		t.Fatalf("ChecksumFile() error = %v", err)
	}
	writeConfigCacheRecord(t, cacheLoader, configCacheRecord{
		Version:   configCacheVersion,
		BuildFile: buildFile,
		Files: []configCacheFile{{
			Path:     buildFile,
			Checksum: checksum,
		}},
		Project: configCacheProject{
			Tasks: []configCacheTask{{Name: "cached"}},
		},
	})

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld", NoCache: true}, NewParser())
	project, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := project.FindTask("actual"); !ok {
		t.Fatalf("actual task not found")
	}
	if _, ok := project.FindTask("cached"); ok {
		t.Fatalf("cached task found with NoCache enabled")
	}
}

func TestLoaderInvalidatesConfigCacheWhenImportedGlobChanges(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeDSLFile(t, projectDir, "build.bu1ld", `import "tasks/**/*.bu1ld"`)
	writeDSLFile(t, projectDir, "tasks/a.bu1ld", `
task a {
  command = []
}
`)

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	project, err := loader.Load()
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	if _, ok := project.FindTask("a"); !ok {
		t.Fatalf("task a not found")
	}

	writeDSLFile(t, projectDir, "tasks/nested/b.bu1ld", `
task b {
  command = []
}
`)

	loader = NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	project, err = loader.Load()
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if _, ok := project.FindTask("b"); !ok {
		t.Fatalf("task b not found after glob import changed")
	}
}

func TestLoaderInvalidatesConfigCacheWhenEnvChanges(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("BU1LD_CACHE_INPUT", "first")
	t.Setenv("BU1LD_CACHE_SCRIPT_INPUT", "script-first")
	writeDSLFile(t, projectDir, "build.bu1ld", `
task envtask {
  inputs = [env("BU1LD_CACHE_INPUT", "fallback"), $(env("BU1LD_CACHE_SCRIPT_INPUT", "fallback"))]
  command = []
}
`)

	loader := NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	project, err := loader.Load()
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	task, ok := project.FindTask("envtask")
	if !ok {
		t.Fatalf("envtask not found")
	}
	if got, want := task.Inputs.Values()[0], "first"; got != want {
		t.Fatalf("first input = %q, want %q", got, want)
	}
	if got, want := task.Inputs.Values()[1], "script-first"; got != want {
		t.Fatalf("first script input = %q, want %q", got, want)
	}

	t.Setenv("BU1LD_CACHE_SCRIPT_INPUT", "script-second")
	loader = NewLoader(config.Config{WorkDir: projectDir, BuildFile: "build.bu1ld"}, NewParser())
	project, err = loader.Load()
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	task, ok = project.FindTask("envtask")
	if !ok {
		t.Fatalf("envtask not found after env changed")
	}
	if got, want := task.Inputs.Values()[0], "first"; got != want {
		t.Fatalf("second input = %q, want %q", got, want)
	}
	if got, want := task.Inputs.Values()[1], "script-second"; got != want {
		t.Fatalf("second script input = %q, want %q", got, want)
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

func writeConfigCacheRecord(t *testing.T, loader *Loader, record configCacheRecord) {
	t.Helper()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	data = append(data, '\n')
	path := loader.configCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
