package plugin

import (
	"context"
	"fmt"
	"io"
)

type ExecuteHandler struct {
	options LoadOptions
}

func NewExecuteHandler(options LoadOptions) *ExecuteHandler {
	return &ExecuteHandler{options: options}
}

func (h *ExecuteHandler) Kind() string {
	return PluginExecActionKind
}

func (h *ExecuteHandler) Run(ctx context.Context, workDir string, params map[string]any, output io.Writer) error {
	namespace, err := stringActionParam(params, "namespace", true)
	if err != nil {
		return err
	}
	action, err := stringActionParam(params, "action", true)
	if err != nil {
		return err
	}
	source, err := stringActionParam(params, "source", false)
	if err != nil {
		return err
	}
	id, err := stringActionParam(params, "id", false)
	if err != nil {
		return err
	}
	version, err := stringActionParam(params, "version", false)
	if err != nil {
		return err
	}
	path, err := stringActionParam(params, "path", false)
	if err != nil {
		return err
	}
	image, err := stringActionParam(params, "image", false)
	if err != nil {
		return err
	}
	pull, err := stringActionParam(params, "pull", false)
	if err != nil {
		return err
	}
	network, err := stringActionParam(params, "network", false)
	if err != nil {
		return err
	}
	containerWorkDir, err := stringActionParam(params, "work_dir", false)
	if err != nil {
		return err
	}
	payload, err := objectActionParam(params, "params")
	if err != nil {
		return err
	}

	loader := NewProcessLoader(h.options)
	defer loader.Close()

	item, err := loader.Load(ctx, NormalizeDeclaration(Declaration{
		Namespace: namespace,
		ID:        id,
		Source:    Source(source),
		Version:   version,
		Path:      path,
		Image:     image,
		Pull:      pull,
		Network:   network,
		WorkDir:   containerWorkDir,
	}))
	if err != nil {
		return fmt.Errorf("load plugin %q for action %q: %w", namespace, action, err)
	}
	executable, ok := item.(ExecutablePlugin)
	if !ok {
		return fmt.Errorf("plugin %q does not support execute", namespace)
	}
	result, err := executable.Execute(ctx, ExecuteRequest{
		Namespace: namespace,
		Action:    action,
		WorkDir:   workDir,
		Params:    payload,
	})
	if err != nil {
		return fmt.Errorf("execute plugin %q action %q: %w", namespace, action, err)
	}
	if result.Output != "" {
		if _, err := io.WriteString(output, result.Output); err != nil {
			return fmt.Errorf("write plugin output: %w", err)
		}
	}
	return nil
}

func stringActionParam(params map[string]any, name string, required bool) (string, error) {
	value, ok := params[name]
	if !ok {
		if required {
			return "", fmt.Errorf("plugin.exec requires param %q", name)
		}
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("plugin.exec param %q must be string", name)
	}
	return text, nil
}

func objectActionParam(params map[string]any, name string) (map[string]any, error) {
	value, ok := params[name]
	if !ok || value == nil {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("plugin.exec param %q must be object", name)
	}
	return object, nil
}
