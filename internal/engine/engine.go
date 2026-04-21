package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"

	"bu1ld/internal/build"
	"bu1ld/internal/cache"
	"bu1ld/internal/config"
	"bu1ld/internal/events"
	"bu1ld/internal/graph"
	"bu1ld/internal/snapshot"

	"github.com/DaiYuANg/arcgo/collectionx"
	"github.com/DaiYuANg/arcgo/eventx"
	"github.com/samber/oops"
)

type CommandRunner interface {
	Run(ctx context.Context, workDir string, command []string, output io.Writer) error
}

type ExecRunner struct{}

func NewExecRunner() CommandRunner {
	return &ExecRunner{}
}

func (r *ExecRunner) Run(ctx context.Context, workDir string, command []string, output io.Writer) error {
	if len(command) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = workDir
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

type Engine struct {
	cfg         config.Config
	snapshotter *snapshot.Snapshotter
	store       *cache.Store
	runner      CommandRunner
	output      io.Writer
	bus         eventx.BusRuntime
}

func New(
	cfg config.Config,
	snapshotter *snapshot.Snapshotter,
	store *cache.Store,
	runner CommandRunner,
	bus eventx.BusRuntime,
	output io.Writer,
) *Engine {
	return &Engine{
		cfg:         cfg,
		snapshotter: snapshotter,
		store:       store,
		runner:      runner,
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

	actionKeys := collectionx.NewMap[string, string]()
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

		startedAt := time.Now()
		if err := e.publish(ctx, events.TaskStarted{Task: task.Name}); err != nil {
			return err
		}

		if task.Command.Len() == 0 {
			if err := e.publish(ctx, events.TaskNoop{Task: task.Name}); err != nil {
				return err
			}
		} else if err := e.runner.Run(ctx, e.cfg.WorkDir, build.Values(task.Command), e.output); err != nil {
			duration := time.Since(startedAt)
			_ = e.publish(ctx, events.TaskFailed{Task: task.Name, Duration: duration, Err: err})
			return fmt.Errorf("task %s failed: %w", task.Name, err)
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
		if err := e.publish(ctx, events.TaskCompleted{Task: task.Name, Duration: duration}); err != nil {
			return err
		}
		actionKeys.Set(task.Name, key)
	}

	return nil
}

func (e *Engine) actionKey(task build.Task, actionKeys collectionx.Map[string, string]) (string, error) {
	files, err := e.snapshotter.Inputs(e.cfg.WorkDir, build.Values(task.Inputs))
	if err != nil {
		return "", err
	}

	dependencyKeys := collectionx.NewMap[string, string]()
	task.Deps.Range(func(_ int, dep string) bool {
		if key, ok := actionKeys.Get(dep); ok {
			dependencyKeys.Set(dep, key)
		}
		return true
	})

	payload := struct {
		Version       int               `json:"version"`
		TaskName      string            `json:"taskName"`
		Command       []string          `json:"command"`
		InputPatterns []string          `json:"inputPatterns"`
		Inputs        []snapshot.File   `json:"inputs"`
		Outputs       []string          `json:"outputs"`
		Dependencies  map[string]string `json:"dependencies"`
		Platform      string            `json:"platform"`
	}{
		Version:       1,
		TaskName:      task.Name,
		Command:       build.Values(task.Command),
		InputPatterns: build.Values(task.Inputs),
		Inputs:        files,
		Outputs:       build.Values(task.Outputs),
		Dependencies:  dependencyKeys.All(),
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return snapshot.HashBytes(data), nil
}

func (e *Engine) publish(ctx context.Context, event eventx.Event) error {
	if e.bus == nil {
		return nil
	}
	return e.bus.Publish(ctx, event)
}
