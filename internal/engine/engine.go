package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"time"

	"github.com/lyonbrown4d/bu1ld/internal/build"
	"github.com/lyonbrown4d/bu1ld/internal/cache"
	"github.com/lyonbrown4d/bu1ld/internal/config"
	"github.com/lyonbrown4d/bu1ld/internal/events"
	"github.com/lyonbrown4d/bu1ld/internal/graph"
	"github.com/lyonbrown4d/bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/eventx"
	"github.com/samber/oops"
)

type CommandRunner interface {
	Run(ctx context.Context, workDir string, command []string, output io.Writer) error
}

type ActionRunner interface {
	Run(ctx context.Context, workDir string, action build.Action, output io.Writer) error
}

type ActionHandler interface {
	Kind() string
	Run(ctx context.Context, workDir string, params map[string]any, output io.Writer) error
}

type DispatchActionRunner struct {
	handlers *mapping.Map[string, ActionHandler]
}

func NewActionRunner(handlers ...ActionHandler) ActionRunner {
	runner := &DispatchActionRunner{handlers: mapping.NewMap[string, ActionHandler]()}
	for _, handler := range handlers {
		if handler == nil || handler.Kind() == "" {
			continue
		}
		runner.handlers.Set(handler.Kind(), handler)
	}
	return runner
}

func (r *DispatchActionRunner) Run(ctx context.Context, workDir string, action build.Action, output io.Writer) error {
	if action.Kind == "" {
		return nil
	}
	handler, ok := r.handlers.Get(action.Kind)
	if !ok {
		return oops.In("bu1ld.engine").
			With("action", action.Kind).
			Errorf("action handler %q is not registered", action.Kind)
	}
	if err := handler.Run(ctx, workDir, action.Params, output); err != nil {
		return oops.In("bu1ld.engine").
			With("action", action.Kind).
			Wrapf(err, "run action")
	}
	return nil
}

type Engine struct {
	cfg         config.Config
	snapshotter *snapshot.Snapshotter
	store       *cache.Store
	runner      CommandRunner
	actions     ActionRunner
	output      io.Writer
	bus         eventx.BusRuntime
}

func New(
	cfg config.Config,
	snapshotter *snapshot.Snapshotter,
	store *cache.Store,
	runner CommandRunner,
	actions ActionRunner,
	bus eventx.BusRuntime,
	output io.Writer,
) *Engine {
	if actions == nil {
		actions = NewActionRunner()
	}
	return &Engine{
		cfg:         cfg,
		snapshotter: snapshotter,
		store:       store,
		runner:      runner,
		actions:     actions,
		bus:         bus,
		output:      output,
	}
}

func (e *Engine) Run(ctx context.Context, project build.Project, targets []string) error {
	plan, err := graph.Plan(project, targets)
	if err != nil {
		return oops.In("bu1ld.engine").
			With("targets", targets).
			Wrapf(err, "plan task graph")
	}

	actionKeys := mapping.NewMap[string, string]()
	for _, task := range plan.Values() {
		key, err := e.actionKey(task, actionKeys)
		if err != nil {
			return oops.In("bu1ld.engine").
				With("task", task.Name).
				Wrapf(err, "compute action key")
		}

		if !e.cfg.NoCache {
			record, hit, err := e.store.Load(key)
			if err != nil {
				return oops.In("bu1ld.engine").
					With("task", task.Name).
					With("action_key", key).
					Wrapf(err, "load cache record")
			}
			if hit {
				restored := false
				if !e.store.OutputsPresent(task) {
					if err := e.store.Restore(record); err != nil {
						return oops.In("bu1ld.engine").
							With("task", task.Name).
							With("action_key", key).
							Wrapf(err, "restore cached outputs")
					}
					restored = true
				}
				if err := e.publish(ctx, events.TaskCacheHit{Task: task.Name, Restored: restored}); err != nil {
					return err
				}
				actionKeys.Set(task.Name, key)
				continue
			}
		}

		if err := e.executeTask(ctx, task, key); err != nil {
			return err
		}
		actionKeys.Set(task.Name, key)
	}

	return nil
}

func (e *Engine) executeTask(ctx context.Context, task build.Task, key string) error {
	startedAt := time.Now()
	if err := e.publish(ctx, events.TaskStarted{Task: task.Name}); err != nil {
		return err
	}

	if err := e.runTaskBody(ctx, task); err != nil {
		duration := time.Since(startedAt)
		taskErr := oops.In("bu1ld.engine").
			With("task", task.Name).
			With("action_key", key).
			With("duration", duration.String()).
			Wrapf(err, "run task")
		if publishErr := e.publish(ctx, events.TaskFailed{Task: task.Name, Duration: duration, Err: err}); publishErr != nil {
			return errors.Join(taskErr, publishErr)
		}
		return taskErr
	}

	if !e.cfg.NoCache {
		if err := e.store.Save(task, key); err != nil {
			return oops.In("bu1ld.engine").
				With("task", task.Name).
				With("action_key", key).
				Wrapf(err, "save cache record")
		}
	}

	duration := time.Since(startedAt)
	return e.publish(ctx, events.TaskCompleted{Task: task.Name, Duration: duration})
}

func (e *Engine) runTaskBody(ctx context.Context, task build.Task) error {
	workDir := e.taskWorkDir(task)
	if task.Command.Len() > 0 {
		if err := e.runner.Run(ctx, workDir, build.Values(task.Command), e.output); err != nil {
			return oops.In("bu1ld.engine").
				With("task", task.Name).
				With("work_dir", workDir).
				Wrapf(err, "run task command")
		}
		return nil
	}
	if task.Action.Kind != "" {
		if err := e.actions.Run(ctx, workDir, task.Action, e.output); err != nil {
			return oops.In("bu1ld.engine").
				With("task", task.Name).
				With("work_dir", workDir).
				With("action", task.Action.Kind).
				Wrapf(err, "run task action")
		}
		return nil
	}
	return e.publish(ctx, events.TaskNoop{Task: task.Name})
}

func (e *Engine) actionKey(task build.Task, actionKeys *mapping.Map[string, string]) (string, error) {
	files, err := e.snapshotter.Inputs(e.taskWorkDir(task), build.Values(task.Inputs))
	if err != nil {
		return "", fmt.Errorf("snapshot task inputs for %q: %w", task.Name, err)
	}

	dependencyKeys := mapping.NewMap[string, string]()
	task.Deps.Range(func(_ int, dep string) bool {
		if key, ok := actionKeys.Get(dep); ok {
			dependencyKeys.Set(dep, key)
		}
		return true
	})

	payload := struct {
		Version       int               `json:"version"`
		TaskName      string            `json:"taskName"`
		LocalName     string            `json:"localName"`
		Package       string            `json:"package"`
		WorkDir       string            `json:"workDir"`
		Command       []string          `json:"command"`
		Action        build.Action      `json:"action"`
		InputPatterns []string          `json:"inputPatterns"`
		Inputs        []snapshot.File   `json:"inputs"`
		Outputs       []string          `json:"outputs"`
		Dependencies  map[string]string `json:"dependencies"`
		Platform      string            `json:"platform"`
	}{
		Version:       1,
		TaskName:      task.Name,
		LocalName:     task.LocalName,
		Package:       task.Package,
		WorkDir:       task.WorkDir,
		Command:       build.Values(task.Command),
		Action:        task.Action,
		InputPatterns: build.Values(task.Inputs),
		Inputs:        files,
		Outputs:       build.Values(task.Outputs),
		Dependencies:  dependencyKeys.All(),
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal action key payload for %q: %w", task.Name, err)
	}
	return snapshot.HashBytes(data), nil
}

func (e *Engine) taskWorkDir(task build.Task) string {
	if task.WorkDir == "" {
		return e.cfg.WorkDir
	}
	return filepath.Join(e.cfg.WorkDir, filepath.FromSlash(task.WorkDir))
}

func (e *Engine) publish(ctx context.Context, event eventx.Event) error {
	if e.bus == nil {
		return nil
	}
	if err := e.bus.Publish(ctx, event); err != nil {
		return fmt.Errorf("publish event %T: %w", event, err)
	}
	return nil
}
