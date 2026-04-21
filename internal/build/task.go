package build

import (
	"fmt"
	"sort"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type Task struct {
	Name    string
	Deps    collectionx.List[string]
	Inputs  collectionx.List[string]
	Outputs collectionx.List[string]
	Command collectionx.List[string]
}

func NewTask(name string) Task {
	return Task{
		Name:    name,
		Deps:    collectionx.NewList[string](),
		Inputs:  collectionx.NewList[string](),
		Outputs: collectionx.NewList[string](),
		Command: collectionx.NewList[string](),
	}
}

func (t Task) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("task name is required")
	}
	return nil
}

type Project struct {
	Tasks collectionx.List[Task]
}

func NewProject(tasks ...Task) Project {
	return Project{Tasks: collectionx.NewList(tasks...)}
}

func (p Project) FindTask(name string) (Task, bool) {
	tasks := taskMap(p)
	return tasks.Get(name)
}

func (p Project) TaskNames() []string {
	names := collectionx.NewList[string]()
	if p.Tasks != nil {
		p.Tasks.Range(func(_ int, task Task) bool {
			names.Add(task.Name)
			return true
		})
	}
	values := names.Values()
	sort.Strings(values)
	return values
}

func Values[T any](items collectionx.List[T]) []T {
	if items == nil {
		return nil
	}
	return items.Values()
}

func taskMap(p Project) collectionx.Map[string, Task] {
	tasks := collectionx.NewMap[string, Task]()
	if p.Tasks == nil {
		return tasks
	}
	p.Tasks.Range(func(_ int, task Task) bool {
		tasks.Set(task.Name, task)
		return true
	})
	return tasks
}
