package goplugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/bu1ld/pkg/pluginapi"
)

const (
	DefaultID                    = "org.bu1ld.go"
	Namespace                    = "go"
	DefaultGoReleaserModule      = "github.com/goreleaser/goreleaser/v2"
	DefaultGoReleaserVersion     = "v2.15.4"
	DefaultGoReleaserCommand     = "goreleaser"
	defaultGoReleaserConfig      = ".goreleaser.yaml"
	defaultGoReleaserOutput      = "dist/**"
	defaultGoReleaserReleaseMode = "snapshot"
	defaultGoGenerateOutput      = "build/generated/go"
	defaultImportedTaskPrefix    = "go."
)

type Plugin struct {
	id string
}

func New() *Plugin {
	return NewWithID(DefaultID)
}

func NewWithID(id string) *Plugin {
	if id == "" {
		id = DefaultID
	}
	return &Plugin{id: id}
}

func (p *Plugin) Metadata() (pluginapi.Metadata, error) {
	return pluginapi.Metadata{
		ID:              p.id,
		Namespace:       Namespace,
		ProtocolVersion: pluginapi.ProtocolVersion,
		Capabilities:    pluginapi.DefaultCapabilities(p),
		Rules: []pluginapi.RuleSchema{
			{
				Name: "binary",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "main", Type: pluginapi.FieldString, Required: true},
					{Name: "out", Type: pluginapi.FieldString, Required: true},
					{Name: "cacheprog", Type: pluginapi.FieldString},
				},
			},
			{
				Name: "build",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "packages", Type: pluginapi.FieldList},
					{Name: "cacheprog", Type: pluginapi.FieldString},
				},
			},
			{
				Name: "test",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "packages", Type: pluginapi.FieldList},
					{Name: "cacheprog", Type: pluginapi.FieldString},
				},
			},
			{
				Name: "generate",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "outputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "packages", Type: pluginapi.FieldList},
					{Name: "args", Type: pluginapi.FieldList},
					{Name: "run", Type: pluginapi.FieldString},
					{Name: "skip", Type: pluginapi.FieldString},
					{Name: "out", Type: pluginapi.FieldString},
					{Name: "cacheprog", Type: pluginapi.FieldString},
				},
			},
			{
				Name: "release",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "outputs", Type: pluginapi.FieldList},
					{Name: "config", Type: pluginapi.FieldString},
					{Name: "args", Type: pluginapi.FieldList},
					{Name: "mode", Type: pluginapi.FieldString},
					{Name: "command", Type: pluginapi.FieldString},
					{Name: "module", Type: pluginapi.FieldString},
					{Name: "version", Type: pluginapi.FieldString},
					{Name: "prefer_local", Type: pluginapi.FieldBool},
				},
			},
		},
		ConfigFields: []pluginapi.FieldSchema{
			{Name: "import_tasks", Type: pluginapi.FieldBool},
			{Name: "task_prefix", Type: pluginapi.FieldString},
			{Name: "generate", Type: pluginapi.FieldBool},
			{Name: "test", Type: pluginapi.FieldBool},
			{Name: "build", Type: pluginapi.FieldBool},
			{Name: "packages", Type: pluginapi.FieldList},
			{Name: "main", Type: pluginapi.FieldString},
			{Name: "out", Type: pluginapi.FieldString},
			{Name: "generate_out", Type: pluginapi.FieldString},
			{Name: "inputs", Type: pluginapi.FieldList},
			{Name: "srcs", Type: pluginapi.FieldList},
			{Name: "cacheprog", Type: pluginapi.FieldString},
		},
	}, nil
}

func (p *Plugin) Configure(_ context.Context, config pluginapi.PluginConfig) ([]pluginapi.TaskSpec, error) {
	fields := config.Fields
	if fields == nil {
		fields = map[string]any{}
	}
	importTasks, err := configBool(fields, "import_tasks", true)
	if err != nil {
		return nil, err
	}
	if !importTasks || !looksLikeGoProject(projectDirectory()) {
		return nil, nil
	}

	prefix, err := configString(fields, "task_prefix", defaultImportedTaskPrefix)
	if err != nil {
		return nil, err
	}
	packages, err := configList(fields, "packages", []string{"./..."})
	if err != nil {
		return nil, err
	}
	inputs, err := goConfigInputs(fields)
	if err != nil {
		return nil, err
	}
	cacheprog, err := configString(fields, "cacheprog", "")
	if err != nil {
		return nil, err
	}

	tasks := []pluginapi.TaskSpec{}
	generateEnabled, err := configBool(fields, "generate", true)
	if err != nil {
		return nil, err
	}
	testEnabled, err := configBool(fields, "test", true)
	if err != nil {
		return nil, err
	}
	buildEnabled, err := configBool(fields, "build", true)
	if err != nil {
		return nil, err
	}

	if generateEnabled {
		out, err := configString(fields, "generate_out", defaultGoGenerateOutput)
		if err != nil {
			return nil, err
		}
		out = defaultString(out, defaultGoGenerateOutput)
		tasks = append(tasks, pluginapi.TaskSpec{
			Name:    prefix + "generate",
			Inputs:  inputs,
			Outputs: []string{goGenerateOutputPattern(out)},
			Action: pluginAction("generate", map[string]any{
				"packages":  packages,
				"out":       out,
				"cacheprog": cacheprog,
			}),
		})
	}
	if testEnabled {
		deps := []string{}
		if generateEnabled {
			deps = append(deps, prefix+"generate")
		}
		tasks = append(tasks, pluginapi.TaskSpec{
			Name:   prefix + "test",
			Deps:   deps,
			Inputs: inputs,
			Action: pluginAction("test", map[string]any{
				"packages":  packages,
				"cacheprog": cacheprog,
			}),
		})
	}
	if buildEnabled {
		deps := []string{}
		if testEnabled {
			deps = append(deps, prefix+"test")
		}
		mainPkg, err := configString(fields, "main", "")
		if err != nil {
			return nil, err
		}
		out, err := configString(fields, "out", "")
		if err != nil {
			return nil, err
		}
		if mainPkg != "" && out != "" {
			tasks = append(tasks, pluginapi.TaskSpec{
				Name:    prefix + "build",
				Deps:    deps,
				Inputs:  inputs,
				Outputs: []string{out},
				Action: pluginAction("binary", map[string]any{
					"main":      mainPkg,
					"out":       out,
					"cacheprog": cacheprog,
				}),
			})
		} else {
			tasks = append(tasks, pluginapi.TaskSpec{
				Name:   prefix + "build",
				Deps:   deps,
				Inputs: inputs,
				Action: pluginAction("build", map[string]any{
					"packages":  packages,
					"cacheprog": cacheprog,
				}),
			})
		}
	}
	return tasks, nil
}

func (p *Plugin) Expand(_ context.Context, invocation pluginapi.Invocation) ([]pluginapi.TaskSpec, error) {
	switch invocation.Rule {
	case "binary":
		task, err := expandBinary(invocation)
		if err != nil {
			return nil, err
		}
		return []pluginapi.TaskSpec{task}, nil
	case "build":
		task, err := expandBuild(invocation)
		if err != nil {
			return nil, err
		}
		return []pluginapi.TaskSpec{task}, nil
	case "test":
		task, err := expandTest(invocation)
		if err != nil {
			return nil, err
		}
		return []pluginapi.TaskSpec{task}, nil
	case "generate":
		task, err := expandGenerate(invocation)
		if err != nil {
			return nil, err
		}
		return []pluginapi.TaskSpec{task}, nil
	case "release":
		task, err := expandRelease(invocation)
		if err != nil {
			return nil, err
		}
		return []pluginapi.TaskSpec{task}, nil
	default:
		return nil, nil
	}
}

func expandBinary(invocation pluginapi.Invocation) (pluginapi.TaskSpec, error) {
	mainPkg, err := invocation.RequiredString("main")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.binary main field: %w", err)
	}
	out, err := invocation.RequiredString("out")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.binary out field: %w", err)
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.binary deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return pluginapi.TaskSpec{}, err
	}
	cacheprog, err := invocation.OptionalString("cacheprog", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.binary cacheprog field: %w", err)
	}

	return pluginapi.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: []string{out},
		Action: pluginAction("binary", map[string]any{
			"main":      mainPkg,
			"out":       out,
			"cacheprog": cacheprog,
		}),
	}, nil
}

func expandBuild(invocation pluginapi.Invocation) (pluginapi.TaskSpec, error) {
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.build deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return pluginapi.TaskSpec{}, err
	}
	packages, err := invocation.OptionalList("packages", []string{"./..."})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.build packages field: %w", err)
	}
	cacheprog, err := invocation.OptionalString("cacheprog", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.build cacheprog field: %w", err)
	}

	return pluginapi.TaskSpec{
		Name:   invocation.Target,
		Deps:   deps,
		Inputs: inputs,
		Action: pluginAction("build", map[string]any{
			"packages":  packages,
			"cacheprog": cacheprog,
		}),
	}, nil
}

func expandTest(invocation pluginapi.Invocation) (pluginapi.TaskSpec, error) {
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.test deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return pluginapi.TaskSpec{}, err
	}
	packages, err := invocation.OptionalList("packages", []string{"./..."})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.test packages field: %w", err)
	}
	cacheprog, err := invocation.OptionalString("cacheprog", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.test cacheprog field: %w", err)
	}

	return pluginapi.TaskSpec{
		Name:   invocation.Target,
		Deps:   deps,
		Inputs: inputs,
		Action: pluginAction("test", map[string]any{
			"packages":  packages,
			"cacheprog": cacheprog,
		}),
	}, nil
}

func expandGenerate(invocation pluginapi.Invocation) (pluginapi.TaskSpec, error) {
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return pluginapi.TaskSpec{}, err
	}
	out, err := invocation.OptionalString("out", defaultGoGenerateOutput)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate out field: %w", err)
	}
	out = defaultString(out, defaultGoGenerateOutput)
	outputs, err := invocation.OptionalList("outputs", []string{goGenerateOutputPattern(out)})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate outputs field: %w", err)
	}
	packages, err := invocation.OptionalList("packages", []string{"./..."})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate packages field: %w", err)
	}
	args, err := invocation.OptionalList("args", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate args field: %w", err)
	}
	run, err := invocation.OptionalString("run", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate run field: %w", err)
	}
	skip, err := invocation.OptionalString("skip", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate skip field: %w", err)
	}
	cacheprog, err := invocation.OptionalString("cacheprog", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.generate cacheprog field: %w", err)
	}

	return pluginapi.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: outputs,
		Action: pluginAction("generate", map[string]any{
			"packages":  packages,
			"args":      args,
			"run":       run,
			"skip":      skip,
			"out":       out,
			"cacheprog": cacheprog,
		}),
	}, nil
}

func expandRelease(invocation pluginapi.Invocation) (pluginapi.TaskSpec, error) {
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release deps field: %w", err)
	}
	inputs, err := invocation.OptionalList("inputs", []string{defaultGoReleaserConfig, "go.mod", "go.sum", "**/*.go"})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release inputs field: %w", err)
	}
	outputs, err := invocation.OptionalList("outputs", []string{defaultGoReleaserOutput})
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release outputs field: %w", err)
	}
	config, err := invocation.OptionalString("config", defaultGoReleaserConfig)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release config field: %w", err)
	}
	args, err := invocation.OptionalList("args", nil)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release args field: %w", err)
	}
	mode, err := invocation.OptionalString("mode", defaultGoReleaserReleaseMode)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release mode field: %w", err)
	}
	command, err := invocation.OptionalString("command", "")
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release command field: %w", err)
	}
	module, err := invocation.OptionalString("module", DefaultGoReleaserModule)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release module field: %w", err)
	}
	version, err := invocation.OptionalString("version", DefaultGoReleaserVersion)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release version field: %w", err)
	}
	preferLocal, err := invocation.OptionalBool("prefer_local", true)
	if err != nil {
		return pluginapi.TaskSpec{}, fmt.Errorf("read go.release prefer_local field: %w", err)
	}

	if len(args) == 0 {
		args = goReleaserModeArgs(mode)
	}
	return pluginapi.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: outputs,
		Action: pluginAction("release", map[string]any{
			"config":       config,
			"args":         args,
			"command":      command,
			"module":       module,
			"version":      version,
			"prefer_local": preferLocal,
		}),
	}, nil
}

func (p *Plugin) Execute(ctx context.Context, request pluginapi.ExecuteRequest) (pluginapi.ExecuteResult, error) {
	switch request.Action {
	case "binary":
		mainPkg, err := actionString(request.Params, "main", true, "")
		if err != nil {
			return pluginapi.ExecuteResult{}, err
		}
		out, err := actionString(request.Params, "out", true, "")
		if err != nil {
			return pluginapi.ExecuteResult{}, err
		}
		return runGo(ctx, request.WorkDir, []string{"build", "-o", out, mainPkg}, request.Params)
	case "build":
		packages, err := actionList(request.Params, "packages", []string{"./..."})
		if err != nil {
			return pluginapi.ExecuteResult{}, err
		}
		return runGo(ctx, request.WorkDir, append([]string{"build"}, packages...), request.Params)
	case "test":
		packages, err := actionList(request.Params, "packages", []string{"./..."})
		if err != nil {
			return pluginapi.ExecuteResult{}, err
		}
		return runGo(ctx, request.WorkDir, append([]string{"test"}, packages...), request.Params)
	case "generate":
		return runGoGenerate(ctx, request.WorkDir, request.Params)
	case "release":
		return runGoReleaser(ctx, request.WorkDir, request.Params)
	default:
		return pluginapi.ExecuteResult{}, fmt.Errorf("unknown go action %q", request.Action)
	}
}

func runGoGenerate(ctx context.Context, workDir string, params map[string]any) (pluginapi.ExecuteResult, error) {
	packages, err := actionList(params, "packages", []string{"./..."})
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	args, err := actionList(params, "args", nil)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	run, err := actionString(params, "run", false, "")
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	skip, err := actionString(params, "skip", false, "")
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	out, err := actionString(params, "out", false, defaultGoGenerateOutput)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	out = defaultString(out, defaultGoGenerateOutput)
	env, err := goGenerateEnv(workDir, out, params)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}

	goArgs := []string{"generate"}
	if strings.TrimSpace(run) != "" {
		goArgs = append(goArgs, "-run", run)
	}
	if strings.TrimSpace(skip) != "" {
		goArgs = append(goArgs, "-skip", skip)
	}
	goArgs = append(goArgs, args...)
	goArgs = append(goArgs, packages...)
	return runCommand(ctx, workDir, "go", goArgs, env)
}

func runGoReleaser(ctx context.Context, workDir string, params map[string]any) (pluginapi.ExecuteResult, error) {
	args, err := actionList(params, "args", goReleaserModeArgs(defaultGoReleaserReleaseMode))
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	config, err := actionString(params, "config", false, defaultGoReleaserConfig)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	args = goReleaserArgs(config, args)

	command, err := actionString(params, "command", false, "")
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	preferLocal, err := actionBool(params, "prefer_local", true)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	if command == "" && preferLocal {
		if path, lookErr := exec.LookPath(DefaultGoReleaserCommand); lookErr == nil {
			command = path
		}
	}
	if command != "" {
		return runCommand(ctx, workDir, command, args, os.Environ())
	}

	module, err := actionString(params, "module", false, DefaultGoReleaserModule)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	version, err := actionString(params, "version", false, DefaultGoReleaserVersion)
	if err != nil {
		return pluginapi.ExecuteResult{}, err
	}
	if strings.TrimSpace(version) != "" {
		module += "@" + strings.TrimSpace(version)
	}
	goArgs := append([]string{"run", module}, args...)
	return runCommand(ctx, workDir, "go", goArgs, os.Environ())
}

func runGo(ctx context.Context, workDir string, args []string, params map[string]any) (pluginapi.ExecuteResult, error) {
	return runCommand(ctx, workDir, "go", args, goEnv(params))
}

func goGenerateEnv(workDir, out string, params map[string]any) ([]string, error) {
	env := goEnv(params)
	absOut := out
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(workDir, filepath.FromSlash(out))
	}
	if err := os.MkdirAll(absOut, 0o750); err != nil {
		return nil, fmt.Errorf("create go.generate output directory %q: %w", absOut, err)
	}
	env = appendEnv(env, "BU1LD_GO_GENERATE_OUT", absOut)
	env = appendEnv(env, "BU1LD_GO_GENERATE_REL_OUT", out)
	return env, nil
}

func runCommand(ctx context.Context, workDir, name string, args []string, env []string) (pluginapi.ExecuteResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
	cmd.Env = env

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(output.String())
		if text == "" {
			return pluginapi.ExecuteResult{}, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return pluginapi.ExecuteResult{}, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, text)
	}
	return pluginapi.ExecuteResult{Output: output.String()}, nil
}

func goReleaserModeArgs(mode string) []string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "release":
		return []string{"release", "--clean"}
	case "check":
		return []string{"check"}
	case "", "snapshot":
		return []string{"release", "--snapshot", "--clean", "--skip=publish"}
	default:
		return []string{mode}
	}
}

func goReleaserArgs(config string, args []string) []string {
	if strings.TrimSpace(config) == "" || hasConfigArg(args) {
		return args
	}
	return append([]string{"--config", strings.TrimSpace(config)}, args...)
}

func goGenerateOutputPattern(out string) string {
	out = strings.TrimRight(defaultString(out, defaultGoGenerateOutput), `/\`)
	return out + "/**"
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func hasConfigArg(args []string) bool {
	for _, arg := range args {
		if arg == "-f" || arg == "--config" || strings.HasPrefix(arg, "--config=") {
			return true
		}
	}
	return false
}

func goEnv(params map[string]any) []string {
	env := os.Environ()
	cacheprog := cacheprogValue(params)
	if cacheprog == "" {
		return env
	}
	return appendEnv(env, "GOCACHEPROG", cacheprog)
}

func cacheprogValue(params map[string]any) string {
	if value, err := actionString(params, "cacheprog", false, ""); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	for _, name := range []string{"BU1LD_GO__CACHEPROG", "BU1LD_GO_CACHEPROG"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	if remoteURL := firstEnv("BU1LD_GO__REMOTE_CACHE_URL", "BU1LD_GO_REMOTE_CACHE_URL", "BU1LD_REMOTE_CACHE__URL", "BU1LD_REMOTE_CACHE_URL"); remoteURL != "" {
		return strings.Join([]string{
			quoteCacheprogArg(defaultCacheprogCommand()),
			"cacheprog",
			"--remote-cache-url",
			quoteCacheprogArg(remoteURL),
		}, " ")
	}
	return ""
}

func defaultCacheprogCommand() string {
	executable, err := os.Executable()
	if err != nil || strings.TrimSpace(executable) == "" {
		return "bu1ld-go-plugin"
	}
	return executable
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func quoteCacheprogArg(value string) string {
	if strings.ContainsAny(value, " \t\r\n\"") {
		return strconv.Quote(value)
	}
	return value
}

func appendEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func pluginAction(action string, params map[string]any) pluginapi.TaskAction {
	return pluginapi.TaskAction{
		Kind: pluginapi.PluginExecActionKind,
		Params: map[string]any{
			"namespace": Namespace,
			"action":    action,
			"params":    params,
		},
	}
}

func actionString(params map[string]any, name string, required bool, fallback string) (string, error) {
	value, ok := params[name]
	if !ok || value == nil {
		if required {
			return "", fmt.Errorf("go action requires param %q", name)
		}
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("go action param %q must be string", name)
	}
	return text, nil
}

func actionList(params map[string]any, name string, fallback []string) ([]string, error) {
	value, ok := params[name]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return typed, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("go action param %q must be list", name)
			}
			values = append(values, text)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("go action param %q must be list", name)
	}
}

func actionBool(params map[string]any, name string, fallback bool) (bool, error) {
	value, ok := params[name]
	if !ok || value == nil {
		return fallback, nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("go action param %q must be bool", name)
	}
	return enabled, nil
}

func goInputs(invocation pluginapi.Invocation) ([]string, error) {
	inputs, err := invocation.OptionalList("inputs", nil)
	if err != nil {
		return nil, fmt.Errorf("read go inputs field: %w", err)
	}
	if len(inputs) > 0 {
		return inputs, nil
	}
	srcs, err := invocation.OptionalList("srcs", []string{"build.bu1ld", "go.mod", "go.sum", "**/*.go"})
	if err != nil {
		return nil, fmt.Errorf("read go srcs field: %w", err)
	}
	return srcs, nil
}

func goConfigInputs(fields map[string]any) ([]string, error) {
	inputs, err := configList(fields, "inputs", nil)
	if err != nil {
		return nil, fmt.Errorf("read go config inputs field: %w", err)
	}
	if len(inputs) > 0 {
		return inputs, nil
	}
	srcs, err := configList(fields, "srcs", []string{"build.bu1ld", "go.mod", "go.sum", "go.work", "go.work.sum", "**/*.go"})
	if err != nil {
		return nil, fmt.Errorf("read go config srcs field: %w", err)
	}
	return srcs, nil
}

func looksLikeGoProject(root string) bool {
	return fileExists(filepath.Join(root, "go.mod")) || fileExists(filepath.Join(root, "go.work"))
}

func projectDirectory() string {
	if value := strings.TrimSpace(os.Getenv("BU1LD_PROJECT_DIR")); value != "" {
		return value
	}
	return "."
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func configString(fields map[string]any, name, fallback string) (string, error) {
	value, ok := fields[name]
	if !ok || value == nil {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("go config field %q must be string", name)
	}
	return text, nil
}

func configBool(fields map[string]any, name string, fallback bool) (bool, error) {
	value, ok := fields[name]
	if !ok || value == nil {
		return fallback, nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("go config field %q must be bool", name)
	}
	return enabled, nil
}

func configList(fields map[string]any, name string, fallback []string) ([]string, error) {
	value, ok := fields[name]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return typed, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("go config field %q must be list", name)
			}
			values = append(values, text)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("go config field %q must be list", name)
	}
}
