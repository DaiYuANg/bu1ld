package goplugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lyonbrown4d/bu1ld/pkg/pluginapi"
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
	if got, want := metadata.ProtocolVersion, pluginapi.ProtocolVersion; got != want {
		t.Fatalf("metadata protocol version = %d, want %d", got, want)
	}
	if !pluginapi.SupportsCapability(metadata, pluginapi.CapabilityExecute) {
		t.Fatalf("metadata capabilities = %#v, want execute", metadata.Capabilities)
	}
	for _, want := range []string{"binary", "build", "test", "generate", "release"} {
		if !metadataHasRule(metadata, want) {
			t.Fatalf("metadata missing rule %q", want)
		}
	}
	if got, want := metadata.ConfigFields[0].Name, "import_tasks"; got != want {
		t.Fatalf("first config field = %q, want %q", got, want)
	}
}

func TestConfigureImportsGoToolchainTasks(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	t.Setenv("BU1LD_PROJECT_DIR", workDir)

	tasks, err := New().Configure(context.Background(), pluginapi.PluginConfig{
		Namespace: "go",
		Fields: map[string]any{
			"main": "./cmd/app",
			"out":  "dist/app",
		},
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if got, want := taskNames(tasks), "go.generate,go.test,go.build"; got != want {
		t.Fatalf("task names = %q, want %q", got, want)
	}
	build := tasks[2]
	if got, want := build.Action.Params["action"], "binary"; got != want {
		t.Fatalf("build action = %q, want %q", got, want)
	}
	if got, want := strings.Join(build.Deps, ","), "go.test"; got != want {
		t.Fatalf("build deps = %q, want %q", got, want)
	}
	if got, want := strings.Join(tasks[0].Outputs, ","), "build/generated/go/**"; got != want {
		t.Fatalf("generate outputs = %q, want %q", got, want)
	}
}

func TestConfigureSkipsWhenNoGoProject(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	t.Setenv("BU1LD_PROJECT_DIR", t.TempDir())
	tasks, err := New().Configure(context.Background(), pluginapi.PluginConfig{Namespace: "go"})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want empty", tasks)
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

func TestExpandReleaseDefaultsToEmbeddedGoReleaser(t *testing.T) {
	tasks, err := New().Expand(context.Background(), pluginapi.Invocation{
		Namespace: "go",
		Rule:      "release",
		Target:    "release",
		Fields:    map[string]any{},
	})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got, want := len(tasks), 1; got != want {
		t.Fatalf("task count = %d, want %d", got, want)
	}
	task := tasks[0]
	if got, want := task.Outputs, []string{"dist/**"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("outputs = %#v, want %#v", got, want)
	}
	if got, want := task.Action.Kind, pluginapi.PluginExecActionKind; got != want {
		t.Fatalf("action kind = %q, want %q", got, want)
	}
	params, ok := task.Action.Params["params"].(map[string]any)
	if !ok {
		t.Fatalf("action params type = %T, want map[string]any", task.Action.Params["params"])
	}
	if got, want := params["config"], ".goreleaser.yaml"; got != want {
		t.Fatalf("config = %q, want %q", got, want)
	}
	args, ok := params["args"].([]string)
	if !ok {
		t.Fatalf("args type = %T, want []string", params["args"])
	}
	if got, want := strings.Join(args, " "), "release --snapshot --clean --skip=publish"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if got, want := params["module"], DefaultGoReleaserModule; got != want {
		t.Fatalf("module = %q, want %q", got, want)
	}
	if got, want := params["version"], DefaultGoReleaserVersion; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func TestExpandGenerateDefaultsToBuildGeneratedGo(t *testing.T) {
	tasks, err := New().Expand(context.Background(), pluginapi.Invocation{
		Namespace: "go",
		Rule:      "generate",
		Target:    "generate",
		Fields:    map[string]any{},
	})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got, want := len(tasks), 1; got != want {
		t.Fatalf("task count = %d, want %d", got, want)
	}
	task := tasks[0]
	if got, want := task.Outputs, []string{"build/generated/go/**"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("outputs = %#v, want %#v", got, want)
	}
	if got, want := task.Action.Kind, pluginapi.PluginExecActionKind; got != want {
		t.Fatalf("action kind = %q, want %q", got, want)
	}
	params, ok := task.Action.Params["params"].(map[string]any)
	if !ok {
		t.Fatalf("action params type = %T, want map[string]any", task.Action.Params["params"])
	}
	if got, want := params["out"], "build/generated/go"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
	packages, ok := params["packages"].([]string)
	if !ok {
		t.Fatalf("packages type = %T, want []string", params["packages"])
	}
	if got, want := strings.Join(packages, " "), "./..."; got != want {
		t.Fatalf("packages = %q, want %q", got, want)
	}
}

func TestExecuteInjectsGOCACHEPROGFromEnvironment(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	envFile := filepath.Join(workDir, "gocacheprog.txt")
	writeFakeGo(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BU1LD_GO__CACHEPROG", "custom-cacheprog --remote http://127.0.0.1:19876")
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
	if got, want := strings.TrimSpace(string(data)), "custom-cacheprog --remote http://127.0.0.1:19876"; got != want {
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
	want := strings.Join([]string{
		quoteCacheprogArg(defaultCacheprogCommand()),
		"cacheprog",
		"--remote-cache-url",
		"http://127.0.0.1:19876",
	}, " ")
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("GOCACHEPROG = %q, want %q", got, want)
	}
}

func TestExecuteGenerateCreatesOutputDirAndPassesEnv(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	argsFile := filepath.Join(workDir, "go-generate-args.txt")
	outFile := filepath.Join(workDir, "go-generate-out.txt")
	relOutFile := filepath.Join(workDir, "go-generate-rel-out.txt")
	writeFakeGoGenerate(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BU1LD_FAKE_GO_ARGS", argsFile)
	t.Setenv("BU1LD_FAKE_GO_OUT", outFile)
	t.Setenv("BU1LD_FAKE_GO_REL_OUT", relOutFile)

	_, err := New().Execute(context.Background(), pluginapi.ExecuteRequest{
		Action:  "generate",
		WorkDir: workDir,
		Params: map[string]any{
			"packages": []any{"./pkg"},
			"args":     []any{"-x"},
			"run":      "mock",
			"out":      "build/generated/go",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	expectedOut := filepath.Join(workDir, "build", "generated", "go")
	if info, err := os.Stat(expectedOut); err != nil || !info.IsDir() {
		t.Fatalf("generated output dir missing: info=%v err=%v", info, err)
	}
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile(args) error = %v", err)
	}
	gotArgs := strings.TrimSpace(string(argsData))
	for _, want := range []string{"generate", "-run", "mock", "-x", "./pkg"} {
		if !strings.Contains(gotArgs, want) {
			t.Fatalf("go args = %q, want to contain %q", gotArgs, want)
		}
	}
	outData, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	if got, want := filepath.Clean(strings.TrimSpace(string(outData))), expectedOut; got != want {
		t.Fatalf("BU1LD_GO_GENERATE_OUT = %q, want %q", got, want)
	}
	relOutData, err := os.ReadFile(relOutFile)
	if err != nil {
		t.Fatalf("ReadFile(rel out) error = %v", err)
	}
	if got, want := strings.TrimSpace(string(relOutData)), "build/generated/go"; got != want {
		t.Fatalf("BU1LD_GO_GENERATE_REL_OUT = %q, want %q", got, want)
	}
}

func TestExecuteReleaseFallsBackToGoRunGoReleaser(t *testing.T) {
	restoreEnv(t, goEnvKeys()...)

	workDir := t.TempDir()
	binDir := t.TempDir()
	argsFile := filepath.Join(workDir, "goreleaser-args.txt")
	writeFakeGoArgs(t, binDir)

	t.Setenv("PATH", binDir)
	t.Setenv("BU1LD_FAKE_GO_ARGS", argsFile)

	_, err := New().Execute(context.Background(), pluginapi.ExecuteRequest{
		Action:  "release",
		WorkDir: workDir,
		Params: map[string]any{
			"config":       ".goreleaser.yaml",
			"args":         []any{"check"},
			"version":      "v2.15.4",
			"prefer_local": false,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := strings.TrimSpace(string(data))
	for _, want := range []string{
		"run",
		"github.com/goreleaser/goreleaser/v2@v2.15.4",
		"--config",
		".goreleaser.yaml",
		"check",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("go args = %q, want to contain %q", got, want)
		}
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
		"BU1LD_FAKE_GO_ARGS",
		"BU1LD_FAKE_GO_OUT",
		"BU1LD_FAKE_GO_REL_OUT",
		"BU1LD_GO_GENERATE_OUT",
		"BU1LD_GO_GENERATE_REL_OUT",
		"BU1LD_PROJECT_DIR",
	}
}

func metadataHasRule(metadata pluginapi.Metadata, name string) bool {
	for _, rule := range metadata.Rules {
		if rule.Name == name {
			return true
		}
	}
	return false
}

func taskNames(tasks []pluginapi.TaskSpec) string {
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, task.Name)
	}
	return strings.Join(names, ",")
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

func writeFakeGoArgs(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "go.bat")
		content := "@echo off\r\necho %*> \"%BU1LD_FAKE_GO_ARGS%\"\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
		return
	}

	path := filepath.Join(dir, "go")
	content := "#!/bin/sh\nprintf '%s' \"$*\" > \"$BU1LD_FAKE_GO_ARGS\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func writeFakeGoGenerate(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "go.bat")
		content := "@echo off\r\necho %*> \"%BU1LD_FAKE_GO_ARGS%\"\r\necho %BU1LD_GO_GENERATE_OUT%> \"%BU1LD_FAKE_GO_OUT%\"\r\necho %BU1LD_GO_GENERATE_REL_OUT%> \"%BU1LD_FAKE_GO_REL_OUT%\"\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
		return
	}

	path := filepath.Join(dir, "go")
	content := "#!/bin/sh\nprintf '%s' \"$*\" > \"$BU1LD_FAKE_GO_ARGS\"\nprintf '%s' \"$BU1LD_GO_GENERATE_OUT\" > \"$BU1LD_FAKE_GO_OUT\"\nprintf '%s' \"$BU1LD_GO_GENERATE_REL_OUT\" > \"$BU1LD_FAKE_GO_REL_OUT\"\n"
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
