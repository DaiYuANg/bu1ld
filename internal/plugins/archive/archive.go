package archive

import (
	"context"
	"fmt"

	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"
)

const (
	ZipActionKind = "archive.zip"
	TarActionKind = "archive.tar"
)

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Metadata() (buildplugin.Metadata, error) {
	return buildplugin.Metadata{
		ID:        "builtin.archive",
		Namespace: "archive",
		Rules: []buildplugin.RuleSchema{
			{
				Name: "zip",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "srcs", Type: buildplugin.FieldList, Required: true},
					{Name: "out", Type: buildplugin.FieldString, Required: true},
				},
			},
			{
				Name: "tar",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "srcs", Type: buildplugin.FieldList, Required: true},
					{Name: "out", Type: buildplugin.FieldString, Required: true},
					{Name: "gzip", Type: buildplugin.FieldBool},
				},
			},
		},
	}, nil
}

func (p *Plugin) Expand(_ context.Context, invocation buildplugin.Invocation) ([]buildplugin.TaskSpec, error) {
	switch invocation.Rule {
	case "zip":
		task, err := expandZip(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	case "tar":
		task, err := expandTar(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	default:
		return nil, nil
	}
}

func expandZip(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	return expandArchive(invocation, ZipActionKind, false)
}

func expandTar(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	gzip, err := invocation.OptionalBool("gzip", false)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read archive.tar gzip field: %w", err)
	}
	return expandArchive(invocation, TarActionKind, gzip)
}

func expandArchive(invocation buildplugin.Invocation, kind string, gzip bool) (buildplugin.TaskSpec, error) {
	srcs, err := invocation.OptionalList("srcs", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read %s srcs field: %w", kind, err)
	}
	if len(srcs) == 0 {
		return buildplugin.TaskSpec{}, fmt.Errorf("%s requires at least one src", kind)
	}
	out, err := invocation.RequiredString("out")
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read %s out field: %w", kind, err)
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read %s deps field: %w", kind, err)
	}
	inputs, err := invocation.OptionalList("inputs", srcs)
	if err != nil {
		return buildplugin.TaskSpec{}, fmt.Errorf("read %s inputs field: %w", kind, err)
	}
	return buildplugin.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: []string{out},
		Action: buildplugin.TaskAction{
			Kind: kind,
			Params: map[string]any{
				"srcs": srcs,
				"out":  out,
				"gzip": gzip,
			},
		},
	}, nil
}
