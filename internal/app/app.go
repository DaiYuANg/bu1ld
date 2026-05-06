package app

import (
	"cmp"
	"context"
	"io"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/cache"
	"bu1ld/internal/dsl"
	"bu1ld/internal/engine"
	"bu1ld/internal/graph"
	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

type CommandKind string

const (
	CommandBuild             CommandKind = "build"
	CommandTest              CommandKind = "test"
	CommandInit              CommandKind = "init"
	CommandDoctor            CommandKind = "doctor"
	CommandGraph             CommandKind = "graph"
	CommandTasks             CommandKind = "tasks"
	CommandClean             CommandKind = "clean"
	CommandPluginsList       CommandKind = "plugins.list"
	CommandPluginsDoctor     CommandKind = "plugins.doctor"
	CommandPluginsLock       CommandKind = "plugins.lock"
	CommandDaemonStatus      CommandKind = "daemon.status"
	CommandDaemonStart       CommandKind = "daemon.start"
	CommandDaemonStop        CommandKind = "daemon.stop"
	CommandServerStatus      CommandKind = "server.status"
	CommandServerCoordinator CommandKind = "server.coordinator"
	CommandServerWorker      CommandKind = "server.worker"
)

type CommandRequest struct {
	Kind       CommandKind
	Targets    []string
	ForceWrite bool
}

type App struct {
	request  CommandRequest
	loader   *dsl.Loader
	registry *buildplugin.Registry
	engine   *engine.Engine
	store    *cache.Store
	output   io.Writer
}

func New(
	request CommandRequest,
	loader *dsl.Loader,
	registry *buildplugin.Registry,
	engine *engine.Engine,
	store *cache.Store,
	output io.Writer,
) (*App, error) {
	return &App{
		request:  request,
		loader:   loader,
		registry: registry,
		engine:   engine,
		store:    store,
		output:   output,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	switch a.request.Kind {
	case CommandInit:
		return a.initProject()
	case CommandDoctor:
		return a.doctor(ctx)
	case CommandBuild, CommandTest:
		project, err := a.loader.Load()
		if err != nil {
			return oops.In("bu1ld.app").
				With("command", a.request.Kind).
				Wrapf(err, "load project")
		}
		if err := a.engine.Run(ctx, project, a.request.Targets); err != nil {
			return oops.In("bu1ld.app").
				With("command", a.request.Kind).
				With("targets", a.request.Targets).
				Wrapf(err, "run build graph")
		}
		return nil
	case CommandGraph:
		project, err := a.loadProject()
		if err != nil {
			return err
		}
		return a.printGraph(project)
	case CommandTasks:
		project, err := a.loadProject()
		if err != nil {
			return err
		}
		return a.printTasks(project)
	case CommandClean:
		if err := a.store.Clean(); err != nil {
			return oops.In("bu1ld.app").
				With("command", a.request.Kind).
				Wrapf(err, "clean cache")
		}
		return writeLine(a.output, "cache cleaned")
	case CommandPluginsList:
		return a.printPlugins(ctx, false)
	case CommandPluginsDoctor:
		return a.printPluginsDoctor(ctx)
	case CommandPluginsLock:
		return a.writePluginsLock(ctx)
	case CommandDaemonStatus:
		return writeLine(a.output, "daemon status: unavailable (not implemented)")
	case CommandDaemonStart, CommandDaemonStop:
		return oops.In("bu1ld.daemon").
			With("command", a.request.Kind).
			New("daemon runtime is reserved for the next build engine iteration")
	case CommandServerStatus:
		return writeLine(a.output, "server status: unavailable (not implemented)")
	case CommandServerCoordinator, CommandServerWorker:
		return oops.In("bu1ld.server").
			With("command", a.request.Kind).
			New("distributed build server is reserved for a future implementation")
	default:
		return oops.In("bu1ld.app").
			With("command", a.request.Kind).
			Errorf("unknown command kind %q", a.request.Kind)
	}
}

func (a *App) loadProject() (build.Project, error) {
	project, err := a.loader.Load()
	if err != nil {
		return build.Project{}, oops.In("bu1ld.app").
			With("command", a.request.Kind).
			Wrapf(err, "load project")
	}
	return project, nil
}

func (a *App) printGraph(project build.Project) error {
	tasks, err := a.graphTasks(project)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		deps := build.Values(task.Deps)
		line := task.Name
		if len(deps) > 0 {
			line += " -> " + strings.Join(deps, ", ")
		}
		if err := writeLine(a.output, line); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) graphTasks(project build.Project) ([]build.Task, error) {
	if len(a.request.Targets) == 0 {
		tasks := list.NewList[build.Task]()
		if project.Tasks != nil {
			project.Tasks.Range(func(_ int, task build.Task) bool {
				tasks.Add(task)
				return true
			})
		}
		tasks.Sort(func(left, right build.Task) int {
			return cmp.Compare(left.Name, right.Name)
		})
		return tasks.Values(), nil
	}

	plan, err := graph.Plan(project, a.request.Targets)
	if err != nil {
		return nil, oops.In("bu1ld.app").
			With("command", a.request.Kind).
			With("targets", a.request.Targets).
			Wrapf(err, "plan graph")
	}
	return plan.Values(), nil
}

func (a *App) printTasks(project build.Project) error {
	for _, name := range project.TaskNames() {
		if err := writeLine(a.output, name); err != nil {
			return err
		}
	}
	return nil
}
