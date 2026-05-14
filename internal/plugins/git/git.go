package git

import (
	"context"
	"path/filepath"

	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"

	"github.com/samber/oops"
)

const InfoActionKind = "git.info"

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Metadata() (buildplugin.Metadata, error) {
	return buildplugin.Metadata{
		ID:        "builtin.git",
		Namespace: "git",
		Rules: []buildplugin.RuleSchema{
			{
				Name: "info",
				Fields: []buildplugin.FieldSchema{
					{Name: "deps", Type: buildplugin.FieldList},
					{Name: "inputs", Type: buildplugin.FieldList},
					{Name: "repo", Type: buildplugin.FieldString},
					{Name: "out", Type: buildplugin.FieldString, Required: true},
					{Name: "include_dirty", Type: buildplugin.FieldBool},
				},
			},
		},
	}, nil
}

func (p *Plugin) Expand(_ context.Context, invocation buildplugin.Invocation) ([]buildplugin.TaskSpec, error) {
	switch invocation.Rule {
	case "info":
		task, err := expandInfo(invocation)
		if err != nil {
			return nil, err
		}
		return []buildplugin.TaskSpec{task}, nil
	default:
		return nil, nil
	}
}

func expandInfo(invocation buildplugin.Invocation) (buildplugin.TaskSpec, error) {
	repo, err := invocation.OptionalString("repo", ".")
	if err != nil {
		return buildplugin.TaskSpec{}, oops.In("bu1ld.git").Wrapf(err, "read git.info repo field")
	}
	out, err := invocation.RequiredString("out")
	if err != nil {
		return buildplugin.TaskSpec{}, oops.In("bu1ld.git").Wrapf(err, "read git.info out field")
	}
	deps, err := invocation.OptionalList("deps", nil)
	if err != nil {
		return buildplugin.TaskSpec{}, oops.In("bu1ld.git").Wrapf(err, "read git.info deps field")
	}
	inputs, err := gitInputs(invocation, repo)
	if err != nil {
		return buildplugin.TaskSpec{}, err
	}
	includeDirty, err := invocation.OptionalBool("include_dirty", true)
	if err != nil {
		return buildplugin.TaskSpec{}, oops.In("bu1ld.git").Wrapf(err, "read git.info include_dirty field")
	}
	return buildplugin.TaskSpec{
		Name:    invocation.Target,
		Deps:    deps,
		Inputs:  inputs,
		Outputs: []string{out},
		Action: buildplugin.TaskAction{
			Kind: InfoActionKind,
			Params: map[string]any{
				"repo":          repo,
				"out":           out,
				"include_dirty": includeDirty,
			},
		},
	}, nil
}

func gitInputs(invocation buildplugin.Invocation, repo string) ([]string, error) {
	inputs, err := invocation.OptionalList("inputs", nil)
	if err != nil {
		return nil, oops.In("bu1ld.git").Wrapf(err, "read git.info inputs field")
	}
	if len(inputs) > 0 {
		return inputs, nil
	}
	gitDir := filepath.ToSlash(filepath.Join(repo, ".git"))
	if repo == "." {
		gitDir = ".git"
	}
	return []string{
		filepath.ToSlash(filepath.Join(gitDir, "HEAD")),
		filepath.ToSlash(filepath.Join(gitDir, "index")),
		filepath.ToSlash(filepath.Join(gitDir, "packed-refs")),
		filepath.ToSlash(filepath.Join(gitDir, "refs", "**", "*")),
	}, nil
}
