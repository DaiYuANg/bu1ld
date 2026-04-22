package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/cache"
	"bu1ld/internal/config"
	"bu1ld/internal/dsl"
	"bu1ld/internal/engine"
	"bu1ld/internal/events"
	"bu1ld/internal/graph"
	"bu1ld/internal/snapshot"

	"github.com/DaiYuANg/arcgo/dix"
	"github.com/DaiYuANg/arcgo/eventx"
	"github.com/DaiYuANg/arcgo/logx"
	"github.com/samber/oops"
)

type CommandKind string

const (
	CommandBuild             CommandKind = "build"
	CommandTest              CommandKind = "test"
	CommandGraph             CommandKind = "graph"
	CommandTasks             CommandKind = "tasks"
	CommandClean             CommandKind = "clean"
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
	request CommandRequest
	loader  *dsl.Loader
	engine  *engine.Engine
	store   *cache.Store
	output  io.Writer
}

func New(
	request CommandRequest,
	loader *dsl.Loader,
	engine *engine.Engine,
	store *cache.Store,
	output io.Writer,
	bus eventx.BusRuntime,
) (*App, error) {
	if err := subscribeConsole(bus, output); err != nil {
		return nil, err
	}
	return &App{
		request: request,
		loader:  loader,
		engine:  engine,
		store:   store,
		output:  output,
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
		return a.engine.Run(ctx, project, a.request.Targets)
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
		_, err := fmt.Fprintln(a.output, "cache cleaned")
		return err
	case CommandDaemonStatus:
		_, err := fmt.Fprintln(a.output, "daemon status: unavailable (not implemented)")
		return err
	case CommandDaemonStart, CommandDaemonStop:
		return oops.In("bu1ld.daemon").
			With("command", a.request.Kind).
			New("daemon runtime is reserved for the next build engine iteration")
	case CommandServerStatus:
		_, err := fmt.Fprintln(a.output, "server status: unavailable (not implemented)")
		return err
	case CommandServerCoordinator, CommandServerWorker:
		return oops.In("bu1ld.server").
			With("command", a.request.Kind).
			New("distributed build server is reserved for a future implementation")
	default:
		return fmt.Errorf("unknown command kind %q", a.request.Kind)
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
		if _, err := fmt.Fprintln(a.output, line); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) graphTasks(project build.Project) ([]build.Task, error) {
	if len(a.request.Targets) == 0 {
		tasks := make([]build.Task, 0, len(project.TaskNames()))
		for _, name := range project.TaskNames() {
			task, _ := project.FindTask(name)
			tasks = append(tasks, task)
		}
		return tasks, nil
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
		if _, err := fmt.Fprintln(a.output, name); err != nil {
			return err
		}
	}
	return nil
}

func RunCommand(ctx context.Context, cfg config.Config, output io.Writer, request CommandRequest) error {
	spec := NewDixApp(cfg, output, request)
	runtime, err := spec.Start(ctx)
	if err != nil {
		return oops.In("bu1ld.app").
			With("command", request.Kind).
			Wrapf(err, "start dix runtime")
	}
	defer func() {
		stopCtx := context.WithoutCancel(ctx)
		_ = runtime.Stop(stopCtx)
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
			dix.Value(cfg),
			dix.Value[io.Writer](output),
			dix.ProviderErr1(newLogger),
			dix.Provider0(dsl.NewParser),
			dix.Provider2(dsl.NewLoader),
			dix.Provider0(snapshot.NewSnapshotter),
			dix.Provider2(cache.NewStore),
			dix.Provider0[engine.CommandRunner](engine.NewExecRunner),
			dix.Provider0[eventx.BusRuntime](func() eventx.BusRuntime {
				return eventx.New()
			}),
			dix.Provider6(engine.New),
			dix.ProviderErr6(New),
		),
		dix.WithModuleHooks(
			dix.OnStop[eventx.BusRuntime](func(_ context.Context, bus eventx.BusRuntime) error {
				return bus.Close()
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
			dix.Value(request),
		),
	)
}

func newLogger(cfg config.Config) (*slog.Logger, error) {
	return logx.New(
		logx.WithConsole(false),
		logx.WithFile(cfg.LogPath()),
		logx.WithLevelString(cfg.LogLevel),
	)
}

func subscribeConsole(bus eventx.BusRuntime, output io.Writer) error {
	if _, err := eventx.Subscribe[events.TaskStarted](bus, func(_ context.Context, event events.TaskStarted) error {
		_, err := fmt.Fprintf(output, "> %s\n", event.Task)
		return err
	}); err != nil {
		return err
	}

	if _, err := eventx.Subscribe[events.TaskCacheHit](bus, func(_ context.Context, event events.TaskCacheHit) error {
		status := "FROM-CACHE"
		if event.Restored {
			status = "RESTORED"
		}
		_, err := fmt.Fprintf(output, "  %s %s\n", status, event.Task)
		return err
	}); err != nil {
		return err
	}

	if _, err := eventx.Subscribe[events.TaskNoop](bus, func(_ context.Context, event events.TaskNoop) error {
		_, err := fmt.Fprintf(output, "  NOOP %s\n", event.Task)
		return err
	}); err != nil {
		return err
	}

	if _, err := eventx.Subscribe[events.TaskCompleted](bus, func(_ context.Context, event events.TaskCompleted) error {
		_, err := fmt.Fprintf(output, "  DONE %s (%s)\n", event.Task, event.Duration.Round(1_000_000))
		return err
	}); err != nil {
		return err
	}

	if _, err := eventx.Subscribe[events.TaskFailed](bus, func(_ context.Context, event events.TaskFailed) error {
		_, err := fmt.Fprintf(output, "  FAILED %s (%s): %v\n", event.Task, event.Duration.Round(1_000_000), event.Err)
		return err
	}); err != nil {
		return err
	}

	return nil
}
