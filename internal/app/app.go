package app

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/cache"
	"bu1ld/internal/config"
	"bu1ld/internal/dsl"
	"bu1ld/internal/engine"
	"bu1ld/internal/events"
	"bu1ld/internal/fsx"
	"bu1ld/internal/graph"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/golang"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type CommandKind string

const (
	CommandBuild             CommandKind = "build"
	CommandTest              CommandKind = "test"
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
	Kind    CommandKind
	Targets []string
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
		tasks := collectionx.NewList[build.Task]()
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

func RunCommand(ctx context.Context, cfg config.Config, output io.Writer, request CommandRequest) (err error) {
	spec := NewDixApp(cfg, output, request)
	runtime, err := spec.Start(ctx)
	if err != nil {
		return oops.In("bu1ld.app").
			With("command", request.Kind).
			Wrapf(err, "start dix runtime")
	}
	defer func() {
		stopCtx := context.WithoutCancel(ctx)
		if stopErr := runtime.Stop(stopCtx); stopErr != nil {
			stopErr = oops.In("bu1ld.app").
				With("command", request.Kind).
				Wrapf(stopErr, "stop dix runtime")
			if err == nil {
				err = stopErr
				return
			}
			err = errors.Join(err, stopErr)
		}
	}()

	app, err := dix.ResolveAs[*App](runtime.Container())
	if err != nil {
		return oops.In("bu1ld.app").
			With("command", request.Kind).
			Wrapf(err, "resolve app service")
	}
	return app.Run(ctx)
}

func NewDixApp(cfg config.Config, output io.Writer, request CommandRequest) *dix.App {
	return dix.New(
		"bu1ld "+string(request.Kind),
		dix.Modules(
			coreModule(cfg, output),
			commandModule(request),
		),
		dix.LoggerFrom(func(c *dix.Container) (*slog.Logger, error) {
			return dix.ResolveAs[*slog.Logger](c)
		}),
	)
}

func coreModule(cfg config.Config, output io.Writer) dix.Module {
	return dix.NewModule("core",
		dix.WithModuleProviders(
			dix.Value[config.Config](cfg),
			dix.Value[io.Writer](output),
			dix.ProviderErr1[*slog.Logger, config.Config](newLogger),
			dix.Provider0[afero.Fs](fsx.NewOsFS),
			dix.Provider0[*dsl.Parser](dsl.NewParser),
			dix.Provider3[*dsl.Loader, config.Config, afero.Fs, *dsl.Parser](dsl.NewLoader),
			dix.Provider1[*snapshot.Snapshotter, afero.Fs](snapshot.NewSnapshotter),
			dix.Provider2[*cache.Store, config.Config, afero.Fs](cache.NewStore),
			dix.Provider0[engine.CommandRunner](engine.NewExecRunner),
			dix.ProviderErr1[eventx.BusRuntime, io.Writer](newEventBus),
			dix.ProviderErr1[*buildplugin.Registry, *dsl.Loader](newPluginRegistry),
			dix.Provider6[*engine.Engine, config.Config, *snapshot.Snapshotter, *cache.Store, engine.CommandRunner, eventx.BusRuntime, io.Writer](engine.New),
			dix.ProviderErr6[*App, CommandRequest, *dsl.Loader, *buildplugin.Registry, *engine.Engine, *cache.Store, io.Writer](New),
		),
		dix.WithModuleHooks(
			dix.OnStop[eventx.BusRuntime](func(_ context.Context, bus eventx.BusRuntime) error {
				return bus.Close()
			}),
			dix.OnStop[*buildplugin.Registry](func(_ context.Context, registry *buildplugin.Registry) error {
				registry.Close()
				return nil
			}),
			dix.OnStop[*slog.Logger](func(_ context.Context, logger *slog.Logger) error {
				return logx.Close(logger)
			}),
		),
	)
}

func commandModule(request CommandRequest) dix.Module {
	return dix.NewModule("command."+string(request.Kind),
		dix.WithModuleProviders(
			dix.Value[CommandRequest](request),
		),
	)
}

func newLogger(cfg config.Config) (*slog.Logger, error) {
	logger, err := logx.New(
		logx.WithConsole(false),
		logx.WithFile(cfg.LogPath()),
		logx.WithLevelString(cfg.LogLevel),
	)
	if err != nil {
		return nil, oops.In("bu1ld.app").
			With("file", cfg.LogPath()).
			Wrapf(err, "create logger")
	}
	return logger, nil
}

func newEventBus(output io.Writer) (eventx.BusRuntime, error) {
	bus := eventx.New()
	if err := subscribeConsole(bus, output); err != nil {
		if closeErr := bus.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	return bus, nil
}

func newPluginRegistry(loader *dsl.Loader) (*buildplugin.Registry, error) {
	registry, err := buildplugin.NewRegistry(loader.LoadOptions(), golang.New())
	if err != nil {
		return nil, oops.In("bu1ld.app").Wrapf(err, "create plugin registry")
	}
	return registry, nil
}

func subscribeConsole(bus eventx.BusRuntime, output io.Writer) error {
	if _, err := eventx.Subscribe[events.TaskStarted](bus, func(_ context.Context, event events.TaskStarted) error {
		return writef(output, "> %s\n", event.Task)
	}); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "subscribe task started events")
	}

	if _, err := eventx.Subscribe[events.TaskCacheHit](bus, func(_ context.Context, event events.TaskCacheHit) error {
		status := "FROM-CACHE"
		if event.Restored {
			status = "RESTORED"
		}
		return writef(output, "  %s %s\n", status, event.Task)
	}); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "subscribe task cache hit events")
	}

	if _, err := eventx.Subscribe[events.TaskNoop](bus, func(_ context.Context, event events.TaskNoop) error {
		return writef(output, "  NOOP %s\n", event.Task)
	}); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "subscribe task noop events")
	}

	if _, err := eventx.Subscribe[events.TaskCompleted](bus, func(_ context.Context, event events.TaskCompleted) error {
		return writef(output, "  DONE %s (%s)\n", event.Task, event.Duration.Round(1_000_000))
	}); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "subscribe task completed events")
	}

	if _, err := eventx.Subscribe[events.TaskFailed](bus, func(_ context.Context, event events.TaskFailed) error {
		return writef(output, "  FAILED %s (%s): %v\n", event.Task, event.Duration.Round(1_000_000), event.Err)
	}); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "subscribe task failed events")
	}

	return nil
}
