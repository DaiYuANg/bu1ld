package graph

import (
	"fmt"
	"strings"

	"bu1ld/internal/build"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type visitState int

const (
	unvisited visitState = iota
	visiting
	visited
)

func Plan(project build.Project, targets []string) (*list.List[build.Task], error) {
	if len(targets) == 0 {
		targets = []string{"build"}
	}

	tasks := mapping.NewMap[string, build.Task]()
	if project.Tasks != nil {
		project.Tasks.Range(func(_ int, task build.Task) bool {
			tasks.Set(task.Name, task)
			return true
		})
	}

	states := mapping.NewMap[string, visitState]()
	ordered := list.NewList[build.Task]()

	var visit func(name string, stack *list.List[string]) error
	visit = func(name string, stack *list.List[string]) error {
		task, ok := tasks.Get(name)
		if !ok {
			return fmt.Errorf("unknown task %q", name)
		}

		state, ok := states.Get(name)
		if ok {
			switch state {
			case unvisited:
				// Continue traversal below.
			case visited:
				return nil
			case visiting:
				stack.Add(name)
				return fmt.Errorf("cycle detected: %s", strings.Join(stack.Values(), " -> "))
			}
		}

		states.Set(name, visiting)
		nextStack := list.NewList[string](stack.Values()...)
		nextStack.Add(name)

		for _, dep := range build.Values(task.Deps) {
			if err := visit(dep, nextStack); err != nil {
				states.Set(name, unvisited)
				return err
			}
		}

		states.Set(name, visited)
		ordered.Add(task)
		return nil
	}

	for _, target := range targets {
		if err := visit(target, list.NewList[string]()); err != nil {
			return nil, err
		}
	}

	return ordered, nil
}
