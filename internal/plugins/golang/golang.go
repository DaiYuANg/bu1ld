package golang

import (
	"context"

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
		return buildplugin.TaskSpec{}, err
	}
	out, err := invocation.RequiredString("out")
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, err
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
		return buildplugin.TaskSpec{}, err
	}
	inputs, err := goInputs(invocation)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}
	packages, err := invocation.OptionalList("packages", []string{"./..."})
	if err != nil {
		return buildplugin.TaskSpec{}, err
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
		return nil, err
	}
	if len(inputs) > 0 {
		return inputs, nil
	}
	return invocation.OptionalList("srcs", []string{"build.bu1ld", "go.mod", "go.sum", "**/*.go"})
}
