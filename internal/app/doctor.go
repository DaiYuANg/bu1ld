package app

import (
	"context"

	"bu1ld/internal/build"
	"bu1ld/internal/graph"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

func (a *App) doctor(ctx context.Context) error {
	options := a.loader.LoadOptions()
	if err := writef(a.output, "project: %s\n", options.ProjectDir); err != nil {
		return err
	}
	if err := writef(a.output, "build file: %s\n", a.loader.BuildFilePath()); err != nil {
		return err
	}

	project, err := a.loadProject()
	if err != nil {
		return err
	}
	if err := validateProjectGraph(project); err != nil {
		return oops.In("bu1ld.doctor").Wrapf(err, "check task graph")
	}

	entries, err := a.pluginEntries(ctx)
	if err != nil {
		return oops.In("bu1ld.doctor").Wrapf(err, "check plugins")
	}
	issues := pluginIssues(entries)

	if err := writef(a.output, "tasks: %d\n", len(project.TaskNames())); err != nil {
		return err
	}
	if err := writeLine(a.output, "task graph: ok"); err != nil {
		return err
	}
	if err := writef(a.output, "plugins: %d checked\n", len(entries)); err != nil {
		return err
	}
	if issues.Len() > 0 {
		if err := writePluginIssues(a, issues.Values()); err != nil {
			return err
		}
		return oops.In("bu1ld.doctor").
			With("plugin_issues", issues.Len()).
			New("doctor found plugin issues")
	}

	return writeLine(a.output, "doctor: ok")
}

func validateProjectGraph(project build.Project) error {
	targets := project.TaskNames()
	if len(targets) == 0 {
		return nil
	}
	_, err := graph.Plan(project, targets)
	return err
}

func pluginIssues(entries []pluginEntry) *list.List[pluginEntry] {
	issues := list.NewList[pluginEntry]()
	for _, entry := range entries {
		if entry.Err != nil {
			issues.Add(entry)
		}
	}
	return issues
}

func writePluginIssues(a *App, entries []pluginEntry) error {
	for _, entry := range entries {
		if err := writef(
			a.output,
			"plugin issue: %s %s %s %s: %v\n",
			entry.Source,
			emptyDash(entry.Namespace),
			emptyDash(entry.ID),
			entry.Status,
			entry.Err,
		); err != nil {
			return err
		}
	}
	return nil
}
