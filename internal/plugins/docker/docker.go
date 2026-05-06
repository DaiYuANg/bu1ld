package docker

import (
	"context"
	"fmt"
	"path/filepath"

	buildplugin "bu1ld/internal/plugin"

	"github.com/samber/oops"
)

const ImageActionKind = "docker.image"

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Metadata() (buildplugin.Metadata, error) {
	return buildplugin.Metadata{
		ID:        "builtin.docker",
		Namespace: "docker",
		Rules: []buildplugin.RuleSchema{
			{
				Name: "image",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "outputs", Type: buildplugin.FieldList},
					{Name: "context", Type: buildplugin.FieldString},
					{Name: "dockerfile", Type: buildplugin.FieldString},
					{Name: "tags", Type: buildplugin.FieldList, Required: true},
					{Name: "build_args", Type: buildplugin.FieldObject},
					{Name: "platforms", Type: buildplugin.FieldList},
					{Name: "target", Type: buildplugin.FieldString},
					{Name: "output", Type: buildplugin.FieldString},
					{Name: "push", Type: buildplugin.FieldBool},
					{Name: "load", Type: buildplugin.FieldBool},
				},
			},
		},
	}, nil
}

func (p *Plugin) Expand(_ context.Context, invocation buildplugin.Invocation) ([]buildplugin.TaskSpec, error) {
	switch invocation.Rule {
	case "image":
		task, err := expandImage(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	default:
		return nil, nil
	}
}

func expandImage(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	contextDir, err := invocation.OptionalString("context", ".")
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read docker.image context field: %w", err)
	}
	dockerfile, err := invocation.OptionalString("dockerfile", "Dockerfile")
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read docker.image dockerfile field: %w", err)
	}
	tags, err := invocation.OptionalList("tags", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read docker.image tags field: %w", err)
	}
	if len(tags) == 0 {
		return buildplugin.TaskSpec{}, oops.In("bu1ld.docker").New("docker.image requires at least one tag")
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read docker.image deps field: %w", err)
	}
	inputs, err := dockerInputs(invocation, contextDir, dockerfile)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}
	outputs, err := invocation.OptionalList("outputs", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read docker.image outputs field: %w", err)
	}
	action, err := imageAction(invocation, contextDir, dockerfile, tags)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}

	return buildplugin.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: outputs,
		Action:  action,
	}, nil
}

func dockerInputs(invocation buildplugin.Invocation, contextDir, dockerfile string) ([]string, error) {
	inputs, err := invocation.OptionalList("inputs", nil)
	if err != nil {
		return nil, fmt.Errorf("read docker.image inputs field: %w", err)
	}
	if len(inputs) > 0 {
		return inputs, nil
	}
	contextPattern := filepath.ToSlash(filepath.Join(contextDir, "**", "*"))
	if contextDir == "." {
		contextPattern = "**/*"
	}
	return []string{dockerfile, contextPattern}, nil
}

func imageAction(
	invocation buildplugin.Invocation,
	contextDir string,
	dockerfile string,
	tags []string,
) (buildplugin.TaskAction, error) {
	platforms, err := invocation.OptionalList("platforms", nil)
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image platforms field: %w", err)
	}

	buildArgs, err := invocation.OptionalObject("build_args", nil)
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image build_args field: %w", err)
	}

	target, err := invocation.OptionalString("target", "")
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image target field: %w", err)
	}

	output, err := invocation.OptionalString("output", "")
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image output field: %w", err)
	}

	push, err := invocation.OptionalBool("push", false)
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image push field: %w", err)
	}
	load, err := invocation.OptionalBool("load", false)
	if err != nil {
		return buildplugin.TaskAction{}, fmt.Errorf("read docker.image load field: %w", err)
	}
	if push && load {
		return buildplugin.TaskAction{}, oops.In("bu1ld.docker").New("docker.image cannot set push and load together")
	}

	return buildplugin.TaskAction{
		Kind: ImageActionKind,
		Params: map[string]any{
			"context":    contextDir,
			"dockerfile": dockerfile,
			"tags":       tags,
			"build_args": buildArgs,
			"platforms":  platforms,
			"target":     target,
			"output":     output,
			"push":       push,
			"load":       load,
		},
	}, nil
}
