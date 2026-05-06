package dsl

import (
	"slices"

	"bu1ld/internal/build"

	"github.com/arcgolabs/collectionx/list"
)

func configCacheProjectFromBuild(project build.Project) configCacheProject {
	packages := list.NewList[configCacheProjectPackage]()
	if project.Packages != nil {
		project.Packages.Range(func(_ int, pkg build.Package) bool {
			packages.Add(configCacheProjectPackage{
				Name: pkg.Name,
				Dir:  pkg.Dir,
				Deps: slices.Clone(build.Values(pkg.Deps)),
			})
			return true
		})
	}
	tasks := list.NewList[configCacheTask]()
	if project.Tasks != nil {
		project.Tasks.Range(func(_ int, task build.Task) bool {
			tasks.Add(configCacheTaskFromBuild(task))
			return true
		})
	}
	return configCacheProject{Packages: packages.Values(), Tasks: tasks.Values()}
}

func configCacheTaskFromBuild(task build.Task) configCacheTask {
	return configCacheTask{
		Name:    task.Name,
		Deps:    slices.Clone(build.Values(task.Deps)),
		Inputs:  slices.Clone(build.Values(task.Inputs)),
		Outputs: slices.Clone(build.Values(task.Outputs)),
		Command: slices.Clone(build.Values(task.Command)),
		Action: configCacheAction{
			Kind:   task.Action.Kind,
			Params: task.Action.Params,
		},
		Local:   task.LocalName,
		Package: task.Package,
		WorkDir: task.WorkDir,
	}
}

func projectFromConfigCache(cached configCacheProject) build.Project {
	packages := list.NewList[build.Package]()
	for _, item := range cached.Packages {
		packages.Add(build.Package{
			Name: item.Name,
			Dir:  item.Dir,
			Deps: list.NewList[string](item.Deps...),
		})
	}
	tasks := list.NewList[build.Task]()
	for i := range cached.Tasks {
		item := cached.Tasks[i]
		tasks.Add(taskFromConfigCache(item))
	}
	return build.Project{Packages: packages, Tasks: tasks}
}

func taskFromConfigCache(item configCacheTask) build.Task {
	task := build.NewTask(item.Name)
	task.Deps = list.NewList[string](item.Deps...)
	task.Inputs = list.NewList[string](item.Inputs...)
	task.Outputs = list.NewList[string](item.Outputs...)
	task.Command = list.NewList[string](item.Command...)
	task.Action = build.Action{
		Kind:   item.Action.Kind,
		Params: item.Action.Params,
	}
	task.LocalName = item.Local
	if task.LocalName == "" {
		task.LocalName = item.Name
	}
	task.Package = item.Package
	task.WorkDir = item.WorkDir
	return task
}
