package goplugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"bu1ld/pkg/pluginapi"
)

const (
	DefaultID = "org.bu1ld.go"
	Namespace = "go"
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
		ID:        p.id,
		Namespace: Namespace,
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
				Name: "test",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "packages", Type: pluginapi.FieldList},
					{Name: "cacheprog", Type: pluginapi.FieldString},
				},
			},
		},
	}, nil
}

func (p *Plugin) Expand(_ context.Context, invocation pluginapi.Invocation) ([]pluginapi.TaskSpec, error) {
	switch invocation.Rule {
	case "binary":
		task, err := expandBinary(invocation)
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
	case "test":
		packages, err := actionList(request.Params, "packages", []string{"./..."})
		if err != nil {
			return pluginapi.ExecuteResult{}, err
		}
		return runGo(ctx, request.WorkDir, append([]string{"test"}, packages...), request.Params)
	default:
		return pluginapi.ExecuteResult{}, fmt.Errorf("unknown go action %q", request.Action)
	}
}

func runGo(ctx context.Context, workDir string, args []string, params map[string]any) (pluginapi.ExecuteResult, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = workDir
	cmd.Env = goEnv(params)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(output.String())
		if text == "" {
			return pluginapi.ExecuteResult{}, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
		}
		return pluginapi.ExecuteResult{}, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, text)
	}
	return pluginapi.ExecuteResult{Output: output.String()}, nil
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
			"bu1ld-go-cacheprog",
			"--remote-cache-url",
			quoteCacheprogArg(remoteURL),
		}, " ")
	}
	return ""
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
