package build

import (
	"errors"
	"sort"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type Task struct {
	Name    string
	Deps    *list.List[string]
	Inputs  *list.List[string]
	Outputs *list.List[string]
	Command *list.List[string]
}

func NewTask(name string) Task {
	return Task{
		Name:    name,
		Deps:    list.NewList[string](),
		Inputs:  list.NewList[string](),
		Outputs: list.NewList[string](),
		Command: list.NewList[string](),
	}
}

func (t Task) Validate() error {
	if t.Name == "" {
		return errors.New("task name is required")
	}
	return nil
}

type Project struct {
	Tasks *list.List[Task]
}

func NewProject(tasks ...Task) Project {
	return Project{Tasks: list.NewList[Task](tasks...)}
}

func (p Project) FindTask(name string) (Task, bool) {
	tasks := taskMap(p)
	return tasks.Get(name)
}

func (p Project) TaskNames() []string {
	names := list.NewList[string]()
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

func Values[T any](items *list.List[T]) []T {
	if items == nil {
		return nil
	}
	return items.Values()
}

func taskMap(p Project) *mapping.Map[string, Task] {
	tasks := mapping.NewMap[string, Task]()
	if p.Tasks == nil {
		return tasks
	}
	p.Tasks.Range(func(_ int, task Task) bool {
		tasks.Set(task.Name, task)
		return true
	})
	return tasks
}
