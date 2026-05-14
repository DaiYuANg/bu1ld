package app

import (
	"cmp"
	"context"
	"io"
	"strings"

	"github.com/lyonbrown4d/bu1ld/internal/build"
	"github.com/lyonbrown4d/bu1ld/internal/cache"
	"github.com/lyonbrown4d/bu1ld/internal/config"
	"github.com/lyonbrown4d/bu1ld/internal/dsl"
	"github.com/lyonbrown4d/bu1ld/internal/engine"
	"github.com/lyonbrown4d/bu1ld/internal/graph"
	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

type CommandKind string

const (
	CommandBuild                   CommandKind = "build"
	CommandTest                    CommandKind = "test"
	CommandInit                    CommandKind = "init"
	CommandDoctor                  CommandKind = "doctor"
	CommandGraph                   CommandKind = "graph"
	CommandTasks                   CommandKind = "tasks"
	CommandPackages                CommandKind = "packages"
	CommandPackagesGraph           CommandKind = "packages.graph"
	CommandAffected                CommandKind = "affected"
	CommandClean                   CommandKind = "clean"
	CommandPluginsList             CommandKind = "plugins.list"
	CommandPluginsDoctor           CommandKind = "plugins.doctor"
	CommandPluginsLock             CommandKind = "plugins.lock"
	CommandPluginsSearch           CommandKind = "plugins.search"
	CommandPluginsInfo             CommandKind = "plugins.info"
	CommandPluginsInstall          CommandKind = "plugins.install"
	CommandPluginsUpdate           CommandKind = "plugins.update"
	CommandPluginsRegistryValidate CommandKind = "plugins.registry.validate"
	CommandPluginsPublish          CommandKind = "plugins.publish"
	CommandDaemonStatus            CommandKind = "daemon.status"
	CommandDaemonStart             CommandKind = "daemon.start"
	CommandDaemonStop              CommandKind = "daemon.stop"
	CommandServerStatus            CommandKind = "server.status"
	CommandServerCoordinator       CommandKind = "server.coordinator"
	CommandServerWorker            CommandKind = "server.worker"
)

type CommandRequest struct {
	Kind               CommandKind
	Targets            []string
	AllPackages        bool
	BaseRef            string
	ForceWrite         bool
	ListenAddr         string
	PluginQuery        string
	PluginRef          string
	PluginGlobal       bool
	RegistrySource     string
	PluginManifestPath string
	PluginAssetURL     string
	PluginOS           string
	PluginArch         string
	PluginFormat       string
	PluginSHA256       string
	PluginStatus       string
	PluginBu1ld        string
}

type App struct {
	request  CommandRequest
	cfg      config.Config
	loader   *dsl.Loader
	registry *buildplugin.Registry
	builder  *engine.Engine
	store    *cache.Store
	output   io.Writer
}

type appServices struct {
	cfg      config.Config
	loader   *dsl.Loader
	registry *buildplugin.Registry
	builder  *engine.Engine
	store    *cache.Store
	output   io.Writer
}

func newAppServices(
	cfg config.Config,
	loader *dsl.Loader,
	registry *buildplugin.Registry,
	builder *engine.Engine,
	store *cache.Store,
	output io.Writer,
) appServices {
	return appServices{
		cfg:      cfg,
		loader:   loader,
		registry: registry,
		builder:  builder,
		store:    store,
		output:   output,
	}
}

func New(
	request CommandRequest,
	services appServices,
) (*App, error) {
	return &App{
		request:  request,
		cfg:      services.cfg,
		loader:   services.loader,
		registry: services.registry,
		builder:  services.builder,
		store:    services.store,
		output:   services.output,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	handler, ok := a.handlers()[a.request.Kind]
	if !ok {
		return oops.In("bu1ld.app").
			With("command", a.request.Kind).
			Errorf("unknown command kind %q", a.request.Kind)
	}
	return handler(ctx)
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

func (a *App) expandTargets(project build.Project) []string {
	if !a.request.AllPackages {
		return normalizeRootTargets(a.request.Targets)
	}
	localTargets := normalizeLocalTargets(a.request.Targets)
	targets := list.NewList[string]()
	for _, pkgName := range project.PackageNames() {
		for _, local := range localTargets {
			targets.Add(build.QualifyTaskName(pkgName, local))
		}
	}
	return targets.Values()
}

func normalizeRootTargets(targets []string) []string {
	values := list.NewList[string]()
	for _, target := range targets {
		values.Add(strings.TrimPrefix(target, ":"))
	}
	return values.Values()
}

func normalizeLocalTargets(targets []string) []string {
	if len(targets) == 0 {
		targets = []string{":build"}
	}
	values := list.NewList[string]()
	for _, target := range targets {
		values.Add(strings.TrimPrefix(target, ":"))
	}
	return values.Values()
}

func (a *App) printPackages(project build.Project) error {
	for _, name := range project.PackageNames() {
		if err := writeLine(a.output, name); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) printPackageGraph(project build.Project) error {
	if project.Packages == nil {
		return nil
	}
	for _, pkg := range project.Packages.Values() {
		deps := build.Values(pkg.Deps)
		line := pkg.Name
		if len(deps) > 0 {
			line += " -> " + strings.Join(deps, ", ")
		}
		if err := writeLine(a.output, line); err != nil {
			return err
		}
	}
	return nil
}
