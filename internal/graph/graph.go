package graph

import (
	"fmt"
	"strings"

	"bu1ld/internal/build"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type visitState int

const (
	unvisited visitState = iota
	visiting
	visited
)

func Plan(project build.Project, targets []string) (collectionx.List[build.Task], error) {
	if len(targets) == 0 {
		targets = []string{"build"}
	}

	tasks := collectionx.NewMap[string, build.Task]()
	if project.Tasks != nil {
		project.Tasks.Range(func(_ int, task build.Task) bool {
			tasks.Set(task.Name, task)
			return true
		})
	}

	states := collectionx.NewMap[string, visitState]()
	ordered := collectionx.NewList[build.Task]()

	var visit func(name string, stack collectionx.List[string]) error
	visit = func(name string, stack collectionx.List[string]) error {
		task, ok := tasks.Get(name)
		if !ok {
			return fmt.Errorf("unknown task %q", name)
		}

		state, ok := states.Get(name)
		if ok {
			switch state {
			case visited:
				return nil
			case visiting:
				stack.Add(name)
				return fmt.Errorf("cycle detected: %s", strings.Join(stack.Values(), " -> "))
			}
		}

		states.Set(name, visiting)
		nextStack := collectionx.NewList(stack.Values()...)
		nextStack.Add(name)

		task.Deps.Range(func(_ int, dep string) bool {
			if err := visit(dep, nextStack); err != nil {
				states.Set(name, unvisited)
				panic(err)
			}
			return true
		})

		states.Set(name, visited)
		ordered.Add(task)
		return nil
	}

	for _, target := range targets {
		if err := runVisit(visit, target); err != nil {
			return nil, err
		}
	}

	return ordered, nil
}

func runVisit(visit func(string, collectionx.List[string]) error, target string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recoveredErr, ok := recovered.(error); ok {
				err = recoveredErr
				return
			}
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return visit(target, collectionx.NewList[string]())
}
