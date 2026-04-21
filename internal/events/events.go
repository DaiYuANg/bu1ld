package events

import "time"

type TaskStarted struct {
	Task string
}

func (TaskStarted) Name() string { return "task.started" }

type TaskCacheHit struct {
	Task     string
	Restored bool
}

func (TaskCacheHit) Name() string { return "task.cache_hit" }

type TaskNoop struct {
	Task string
}

func (TaskNoop) Name() string { return "task.noop" }

type TaskCompleted struct {
	Task     string
	Duration time.Duration
}

func (TaskCompleted) Name() string { return "task.completed" }

type TaskFailed struct {
	Task     string
	Duration time.Duration
	Err      error
}

func (TaskFailed) Name() string { return "task.failed" }
