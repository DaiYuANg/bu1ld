package golang

import (
	"context"
	"fmt"

	buildplugin "bu1ld/internal/plugin"
)

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Metadata() (buildplugin.Metadata, error) {
	return buildplugin.Metadata{
		ID:        "builtin.go",
		Namespace: "go",
		Rules: []buildplugin.RuleSchema{
			{
				Name: "binary",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "srcs", Type: buildplugin.FieldList},
					{Name: "main", Type: buildplugin.FieldString, Required: true},
					{Name: "out", Type: buildplugin.FieldString, Required: true},
				},
			},
			{
				Name: "test",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "srcs", Type: buildplugin.FieldList},
					{Name: "packages", Type: buildplugin.FieldList},
				},
			},
		},
	}, nil
}

func (p *Plugin) Expand(_ context.Context, invocation buildplugin.Invocation) ([]buildplugin.TaskSpec, error) {
	switch invocation.Rule {
	case "binary":
		task, err := expandBinary(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	case "test":
		task, err := expandTest(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	default:
		return nil, nil
	}
}

func expandBinary(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	mainPkg, err := invocation.RequiredString("main")
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read go.binary main field: %w", err)
	}
	out, err := invocation.RequiredString("out")
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read go.binary out field: %w", err)
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read go.binary deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}

	return buildplugin.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: []string{out},
		Command: []string{"go", "build", "-o", out, mainPkg},
	}, nil
}

func expandTest(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read go.test deps field: %w", err)
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}
	packages, err := invocation.OptionalList("packages", []string{"./..."})
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read go.test packages field: %w", err)
	}

	return buildplugin.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Command: append([]string{"go", "test"}, packages...),
	}, nil
}

func goInputs(invocation buildplugin.Invocation) ([]string, error) {
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
