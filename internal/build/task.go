package build

import (
	"errors"
	"sort"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type Task struct {
	Name      string
	LocalName string
	Package   string
	WorkDir   string
	Deps      *list.List[string]
	Inputs    *list.List[string]
	Outputs   *list.List[string]
	Command   *list.List[string]
	Action    Action
}

type Action struct {
	Kind   string
	Params map[string]any
}

func NewTask(name string) Task {
	return Task{
		Name:      name,
		LocalName: name,
		Deps:      list.NewList[string](),
		Inputs:    list.NewList[string](),
		Outputs:   list.NewList[string](),
		Command:   list.NewList[string](),
	}
}

func (t Task) Validate() error {
	if t.Name == "" {
		return errors.New("task name is required")
	}
	if t.Command != nil && t.Command.Len() > 0 && t.Action.Kind != "" {
		return errors.New("task cannot define both command and action")
	}
	return nil
}

type Project struct {
	Tasks    *list.List[Task]
	Packages *list.List[Package]
}

type Package struct {
	Name string
	Dir  string
	Deps *list.List[string]
}

func NewProject(tasks ...Task) Project {
	return Project{
		Tasks:    list.NewList[Task](tasks...),
		Packages: list.NewList[Package](),
	}
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

func (p Project) PackageNames() []string {
	names := list.NewList[string]()
	if p.Packages != nil {
		p.Packages.Range(func(_ int, pkg Package) bool {
			names.Add(pkg.Name)
			return true
		})
	}
	values := names.Values()
	sort.Strings(values)
	return values
}

func QualifyTaskName(packageName, localName string) string {
	if packageName == "" || strings.Contains(localName, ":") {
		return localName
	}
	return packageName + ":" + strings.TrimPrefix(localName, ":")
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
