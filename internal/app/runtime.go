package app

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"bu1ld/internal/cache"
	"bu1ld/internal/config"
	"bu1ld/internal/dsl"
	"bu1ld/internal/engine"
	"bu1ld/internal/events"
	"bu1ld/internal/fsx"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/archive"
	"bu1ld/internal/plugins/docker"
	"bu1ld/internal/plugins/golang"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

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
			dix.Provider6[*engine.Engine, config.Config, *snapshot.Snapshotter, *cache.Store, engine.CommandRunner, eventx.BusRuntime, io.Writer](newEngine),
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

func newActionRunner() engine.ActionRunner {
	return engine.NewActionRunner(
		docker.NewImageHandler(),
		archive.NewZipHandler(),
		archive.NewTarHandler(),
	)
}

func newEngine(
	cfg config.Config,
	snapshotter *snapshot.Snapshotter,
	store *cache.Store,
	runner engine.CommandRunner,
	bus eventx.BusRuntime,
	output io.Writer,
) *engine.Engine {
	return engine.New(cfg, snapshotter, store, runner, newActionRunner(), bus, output)
}

func newPluginRegistry(loader *dsl.Loader) (*buildplugin.Registry, error) {
	registry, err := buildplugin.NewRegistry(loader.LoadOptions(), golang.New(), docker.New(), archive.New())
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
