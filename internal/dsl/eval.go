package dsl

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"bu1ld/internal/build"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type valueKind int

const (
	valueString valueKind = iota
	valueList
)

type value struct {
	kind valueKind
	text string
	list []string
}

func Evaluate(file *File) (build.Project, error) {
	tasks := collectionx.NewList[build.Task]()
	seen := collectionx.NewSet[string]()

	for _, node := range file.Tasks {
		task, err := evaluateTask(node)
		if err != nil {
			return build.Project{}, err
		}
		if seen.Contains(task.Name) {
			return build.Project{}, fmt.Errorf("dsl:%d:%d: duplicate task %q", node.Position().Line, node.Position().Column, task.Name)
		}
		seen.Add(task.Name)
		tasks.Add(task)
	}

	return build.Project{Tasks: tasks}, nil
}

func evaluateTask(node *TaskNode) (build.Task, error) {
	name, err := evaluateString(node.Name)
	if err != nil {
		return build.Task{}, err
	}
	task := build.NewTask(name)

	for _, assignment := range node.Assignments {
		values, err := evaluateStringList(assignment.Value)
		if err != nil {
			return build.Task{}, err
		}
		switch assignment.Name {
		case "deps":
			task.Deps = collectionx.NewList(values...)
		case "inputs":
			task.Inputs = collectionx.NewList(values...)
		case "outputs":
			task.Outputs = collectionx.NewList(values...)
		case "command":
			task.Command = collectionx.NewList(values...)
		default:
			return build.Task{}, fmt.Errorf(
				"dsl:%d:%d: unknown task field %q",
				assignment.Position().Line,
				assignment.Position().Column,
				assignment.Name,
			)
		}
	}

	if err := task.Validate(); err != nil {
		return build.Task{}, fmt.Errorf("dsl:%d:%d: %w", node.Position().Line, node.Position().Column, err)
	}
	return task, nil
}

func evaluateString(expr Expr) (string, error) {
	value, err := evaluate(expr)
	if err != nil {
		return "", err
	}
	if value.kind != valueString {
		return "", fmt.Errorf("dsl:%d:%d: expected string expression", expr.Position().Line, expr.Position().Column)
	}
	return value.text, nil
}

func evaluateStringList(expr Expr) ([]string, error) {
	value, err := evaluate(expr)
	if err != nil {
		return nil, err
	}
	if value.kind == valueString {
		return []string{value.text}, nil
	}
	return value.list, nil
}

func evaluate(expr Expr) (value, error) {
	switch node := expr.(type) {
	case *StringExpr:
		return value{kind: valueString, text: node.Value}, nil
	case *IdentExpr:
		return value{kind: valueString, text: node.Name}, nil
	case *ArrayExpr:
		items := collectionx.NewList[string]()
		for _, element := range node.Elements {
			value, err := evaluate(element)
			if err != nil {
				return value, err
			}
			if value.kind == valueString {
				items.Add(value.text)
			} else {
				items.Add(value.list...)
			}
		}
		return value{kind: valueList, list: items.Values()}, nil
	case *CallExpr:
		return evaluateCall(node)
	default:
		return value{}, fmt.Errorf("dsl:%d:%d: unsupported expression", expr.Position().Line, expr.Position().Column)
	}
}

func evaluateCall(call *CallExpr) (value, error) {
	switch call.Name {
	case "env":
		return evaluateEnv(call)
	case "concat":
		return evaluateConcat(call)
	case "list":
		return evaluateList(call)
	case "os":
		return evaluateNoArgString(call, runtime.GOOS)
	case "arch":
		return evaluateNoArgString(call, runtime.GOARCH)
	default:
		return value{}, fmt.Errorf("dsl:%d:%d: unknown function %q", call.Position().Line, call.Position().Column, call.Name)
	}
}

func evaluateEnv(call *CallExpr) (value, error) {
	if len(call.Args) != 1 && len(call.Args) != 2 {
		return value{}, fmt.Errorf("dsl:%d:%d: env expects 1 or 2 args", call.Position().Line, call.Position().Column)
	}
	name, err := evaluateString(call.Args[0])
	if err != nil {
		return value{}, err
	}
	fallback := ""
	if len(call.Args) == 2 {
		fallback, err = evaluateString(call.Args[1])
		if err != nil {
			return value{}, err
		}
	}
	if result := os.Getenv(name); result != "" {
		return value{kind: valueString, text: result}, nil
	}
	return value{kind: valueString, text: fallback}, nil
}

func evaluateConcat(call *CallExpr) (value, error) {
	parts := collectionx.NewList[string]()
	for _, arg := range call.Args {
		part, err := evaluateString(arg)
		if err != nil {
			return value{}, err
		}
		parts.Add(part)
	}
	return value{kind: valueString, text: strings.Join(parts.Values(), "")}, nil
}

func evaluateList(call *CallExpr) (value, error) {
	items := collectionx.NewList[string]()
	for _, arg := range call.Args {
		values, err := evaluateStringList(arg)
		if err != nil {
			return value{}, err
		}
		items.Add(values...)
	}
	return value{kind: valueList, list: items.Values()}, nil
}

func evaluateNoArgString(call *CallExpr, result string) (value, error) {
	if len(call.Args) != 0 {
		return value{}, fmt.Errorf("dsl:%d:%d: %s expects no args", call.Position().Line, call.Position().Column, call.Name)
	}
	return value{kind: valueString, text: result}, nil
}
