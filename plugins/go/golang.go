package goplugin

import (
	"context"
	"fmt"

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
				},
			},
			{
				Name: "test",
				Fields: []pluginapi.FieldSchema{
					{Name: "deps", Type: pluginapi.FieldList},
					{Name: "inputs", Type: pluginapi.FieldList},
					{Name: "srcs", Type: pluginapi.FieldList},
					{Name: "packages", Type: pluginapi.FieldList},
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

	return pluginapi.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: []string{out},
		Command: []string{"go", "build", "-o", out, mainPkg},
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

	return pluginapi.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Command: append([]string{"go", "test"}, packages...),
	}, nil
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
