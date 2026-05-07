package goplugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bu1ld/pkg/pluginapi"
)

func TestMetadataUsesExternalPluginID(t *testing.T) {
	metadata, err := New().Metadata()
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if got, want := metadata.ID, "org.bu1ld.go"; got != want {
		t.Fatalf("metadata id = %q, want %q", got, want)
	}
	if got, want := metadata.Namespace, "go"; got != want {
		t.Fatalf("metadata namespace = %q, want %q", got, want)
	}
}

func TestExpandBinary(t *testing.T) {
	tasks, err := New().Expand(context.Background(), pluginapi.Invocation{
		Namespace: "go",
		Rule:      "binary",
		Target:    "build",
		Fields: map[string]any{
			"main": "./cmd/cli",
			"out":  "dist/bu1ld",
		},
	})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got, want := len(tasks), 1; got != want {
		t.Fatalf("task count = %d, want %d", got, want)
	}
	action := tasks[0].Action
	if got, want := action.Kind, pluginapi.PluginExecActionKind; got != want {
		t.Fatalf("action kind = %q, want %q", got, want)
	}
	if got, want := action.Params["namespace"], "go"; got != want {
		t.Fatalf("action namespace = %q, want %q", got, want)
	}
	if got, want := action.Params["action"], "binary"; got != want {
		t.Fatalf("action = %q, want %q", got, want)
	}
	params, ok := action.Params["params"].(map[string]any)
	if !ok {
		t.Fatalf("action params type = %T, want map[string]any", action.Params["params"])
	}
	if got, want := params["main"], "./cmd/cli"; got != want {
		t.Fatalf("main param = %q, want %q", got, want)
	}
	if got, want := params["out"], "dist/bu1ld"; got != want {
		t.Fatalf("out param = %q, want %q", got, want)
	}
}

func TestExecuteInjectsGOCACHEPROGFromEnvironment(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	envFile := filepath.Join(workDir, "gocacheprog.txt")
	writeFakeGo(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BU1LD_GO__CACHEPROG", "bu1ld-go-cacheprog --remote http://127.0.0.1:19876")
	t.Setenv("BU1LD_FAKE_GO_ENV", envFile)

	_, err := New().Execute(context.Background(), pluginapi.ExecuteRequest{
		Action:  "test",
		WorkDir: workDir,
		Params: map[string]any{
			"packages": []any{"./..."},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := strings.TrimSpace(string(data)), "bu1ld-go-cacheprog --remote http://127.0.0.1:19876"; got != want {
		t.Fatalf("GOCACHEPROG = %q, want %q", got, want)
	}
}

func TestExecuteCacheprogParamOverridesEnvironment(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	envFile := filepath.Join(workDir, "gocacheprog.txt")
	writeFakeGo(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BU1LD_GO__CACHEPROG", "env-cacheprog")
	t.Setenv("BU1LD_FAKE_GO_ENV", envFile)

	_, err := New().Execute(context.Background(), pluginapi.ExecuteRequest{
		Action:  "binary",
		WorkDir: workDir,
		Params: map[string]any{
			"main":      "./cmd/app",
			"out":       "dist/app",
			"cacheprog": "field-cacheprog",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := strings.TrimSpace(string(data)), "field-cacheprog"; got != want {
		t.Fatalf("GOCACHEPROG = %q, want %q", got, want)
	}
}

func TestExecuteDerivesGOCACHEPROGFromRemoteCacheEnvironment(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	envFile := filepath.Join(workDir, "gocacheprog.txt")
	writeFakeGo(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BU1LD_REMOTE_CACHE__URL", "http://127.0.0.1:19876")
	t.Setenv("BU1LD_FAKE_GO_ENV", envFile)

	_, err := New().Execute(context.Background(), pluginapi.ExecuteRequest{
		Action:  "test",
		WorkDir: workDir,
		Params: map[string]any{
			"packages": []any{"./..."},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := strings.TrimSpace(string(data)), "bu1ld-go-cacheprog --remote-cache-url http://127.0.0.1:19876"; got != want {
		t.Fatalf("GOCACHEPROG = %q, want %q", got, want)
	}
}

func goEnvKeys() []string {
	return []string{
		"BU1LD_GO__CACHEPROG",
		"BU1LD_GO_CACHEPROG",
		"BU1LD_GO__REMOTE_CACHE_URL",
		"BU1LD_GO_REMOTE_CACHE_URL",
		"BU1LD_REMOTE_CACHE__URL",
		"BU1LD_REMOTE_CACHE_URL",
		"GOCACHEPROG",
		"PATH",
		"BU1LD_FAKE_GO_ENV",
	}
}

func writeFakeGo(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "go.bat")
		content := "@echo off\r\necho %GOCACHEPROG%> \"%BU1LD_FAKE_GO_ENV%\"\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
		return
	}

	path := filepath.Join(dir, "go")
	content := "#!/bin/sh\nprintf '%s' \"$GOCACHEPROG\" > \"$BU1LD_FAKE_GO_ENV\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func restoreEnv(t *testing.T, keys ...string) {
	t.Helper()

	values := map[string]string{}
	present := map[string]bool{}
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		values[key] = value
		present[key] = ok
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%s) error = %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, values[key])
				continue
			}
			_ = os.Unsetenv(key)
		}
	})
}
