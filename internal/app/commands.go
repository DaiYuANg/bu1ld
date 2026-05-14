package app

import (
	"context"

	"github.com/lyonbrown4d/bu1ld/internal/build"

	"github.com/samber/oops"
)

type commandHandler func(context.Context) error

func (a *App) handlers() map[CommandKind]commandHandler {
	return map[CommandKind]commandHandler{
		CommandInit:          func(context.Context) error { return a.initProject() },
		CommandDoctor:        a.doctor,
		CommandBuild:         a.runBuild,
		CommandTest:          a.runBuild,
		CommandGraph:         a.runGraph,
		CommandTasks:         a.runTasks,
		CommandPackages:      a.runPackages,
		CommandPackagesGraph: a.runPackagesGraph,
		CommandAffected:      a.runAffected,
		CommandClean:         a.runClean,
		CommandPluginsList:   func(ctx context.Context) error { return a.printPlugins(ctx, false) },
		CommandPluginsDoctor: a.printPluginsDoctor,
		CommandPluginsLock:   a.writePluginsLock,
		CommandPluginsSearch: a.printPluginRegistrySearch,
		CommandPluginsInfo:   a.printPluginRegistryInfo,
		CommandPluginsInstall: func(ctx context.Context) error {
			return a.installRegistryPlugin(ctx, a.request.ForceWrite, "installed")
		},
		CommandPluginsUpdate: func(ctx context.Context) error {
			return a.installRegistryPlugin(ctx, true, "updated")
		},
		CommandPluginsRegistryValidate: a.validatePluginRegistry,
		CommandPluginsPublish:          a.printPluginPublishSnippet,
		CommandDaemonStatus: func(context.Context) error {
			return writeLine(a.output, "daemon status: unavailable (not implemented)")
		},
		CommandDaemonStart: a.runDaemonReserved,
		CommandDaemonStop:  a.runDaemonReserved,
		CommandServerStatus: func(context.Context) error {
			return writeLine(a.output, "server status: unavailable (not implemented)")
		},
		CommandServerCoordinator: a.runServerCoordinator,
		CommandServerWorker:      a.runServerReserved,
	}
}

func (a *App) runBuild(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	targets := a.expandTargets(project)
	if err := a.builder.Run(ctx, project, targets); err != nil {
		return oops.In("bu1ld.app").
			With("command", a.request.Kind).
			With("targets", targets).
			Wrapf(err, "run build graph")
	}
	return nil
}

func (a *App) runGraph(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	return a.printGraph(project)
}

func (a *App) runTasks(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	return a.printTasks(project)
}

func (a *App) runPackages(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	return a.printPackages(project)
}

func (a *App) runPackagesGraph(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	return a.printPackageGraph(project)
}

func (a *App) runAffected(ctx context.Context) error {
	project, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	return a.printAffected(ctx, project)
}

func (a *App) runClean(context.Context) error {
	if err := a.store.Clean(); err != nil {
		return oops.In("bu1ld.app").
			With("command", a.request.Kind).
			Wrapf(err, "clean cache")
	}
	return writeLine(a.output, "cache cleaned")
}

func (a *App) runDaemonReserved(context.Context) error {
	return oops.In("bu1ld.daemon").
		With("command", a.request.Kind).
		New("daemon runtime is reserved for the next build engine iteration")
}

func (a *App) runServerReserved(context.Context) error {
	return oops.In("bu1ld.server").
		With("command", a.request.Kind).
		New("distributed build server is reserved for a future implementation")
}

func (a *App) loadProject(ctx context.Context) (build.Project, error) {
	project, err := a.loader.LoadContext(ctx)
	if err != nil {
		return build.Project{}, oops.In("bu1ld.app").
			With("command", a.request.Kind).
			Wrapf(err, "load project")
	}
	return project, nil
}
