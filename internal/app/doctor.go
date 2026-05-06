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

	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	if graphErr := validateProjectGraph(project); graphErr != nil {
		return oops.In("bu1ld.doctor").Wrapf(graphErr, "check task graph")
	}

	entries, err := a.pluginEntries(ctx)
	if err != nil {
		return oops.In("bu1ld.doctor").Wrapf(err, "check plugins")
	}
	issues := pluginIssues(entries)

	if err := a.writeDoctorSummary(project, entries); err != nil {
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

func (a *App) writeDoctorSummary(project build.Project, entries []pluginEntry) error {
	if err := writef(a.output, "tasks: %d\n", len(project.TaskNames())); err != nil {
		return err
	}
	if err := writeLine(a.output, "task graph: ok"); err != nil {
		return err
	}
	return writef(a.output, "plugins: %d checked\n", len(entries))
}

func validateProjectGraph(project build.Project) error {
	targets := project.TaskNames()
	if len(targets) == 0 {
		return nil
	}
	_, err := graph.Plan(project, targets)
	if err != nil {
		return oops.In("bu1ld.doctor").Wrapf(err, "plan task graph")
	}
	return nil
}

func pluginIssues(entries []pluginEntry) *list.List[pluginEntry] {
	issues := list.NewList[pluginEntry]()
	for i := range entries {
		entry := entries[i]
		if entry.Err != nil {
			issues.Add(entry)
		}
	}
	return issues
}

func writePluginIssues(a *App, entries []pluginEntry) error {
	for i := range entries {
		entry := entries[i]
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
