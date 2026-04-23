package dsl

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"bu1ld/internal/build"
	buildplugin "bu1ld/internal/plugin"

	"github.com/DaiYuANg/arcgo/collectionx"
	"github.com/expr-lang/expr"
	"mvdan.cc/sh/v3/syntax"
)

type valueKind int

const (
	valueString valueKind = iota
	valueList
	valueObject
)

type value struct {
	kind   valueKind
	text   string
	list   []string
	object collectionx.Map[string, value]
}

type evalContext struct {
	context  context.Context
	registry *buildplugin.Registry
	vars     map[string]any
	envs     *envTracker
}

type PluginDeclaration struct {
	Declaration buildplugin.Declaration
	Pos         Position
}

type envDependency struct {
	Name  string
	Value string
}

func Evaluate(file *File) (build.Project, error) {
	return EvaluateWithRegistry(file, NewParser().registry.CloneWithOptions(buildplugin.LoadOptions{}))
}

func EvaluateWithRegistry(file *File, registry *buildplugin.Registry) (build.Project, error) {
	project, _, err := evaluateWithRegistry(file, registry)
	return project, err
}

func evaluateWithRegistry(file *File, registry *buildplugin.Registry) (build.Project, []envDependency, error) {
	defer registry.Close()

	tasks := collectionx.NewList[build.Task]()
	seen := collectionx.NewSet[string]()
	envs := newEnvTracker()
	ctx := evalContext{
		context:  context.Background(),
		registry: registry,
		vars:     map[string]any{},
		envs:     envs,
	}

	declarations, err := pluginDeclarations(file, ctx)
	if err != nil {
		return build.Project{}, nil, err
	}
	for _, item := range declarations {
		if err := registry.Declare(ctx.context, item.Declaration); err != nil {
			return build.Project{}, nil, fmt.Errorf("dsl:%d:%d: %w", item.Pos.Line, item.Pos.Column, err)
		}
	}

	for _, statement := range file.Statements {
		if isPluginDeclaration(statement) {
			continue
		}
		generated, err := evaluateStatement(statement, ctx)
		if err != nil {
			return build.Project{}, nil, err
		}
		for _, task := range generated {
			if seen.Contains(task.Name) {
				return build.Project{}, nil, fmt.Errorf(
					"dsl:%d:%d: duplicate task %q",
					statement.Position().Line,
					statement.Position().Column,
					task.Name,
				)
			}
			seen.Add(task.Name)
			tasks.Add(task)
		}
	}

	return build.Project{Tasks: tasks}, envs.Values(), nil
}

func PluginDeclarations(file *File) ([]PluginDeclaration, error) {
	return pluginDeclarations(file, evalContext{
		context: context.Background(),
		vars:    map[string]any{},
	})
}

func pluginDeclarations(file *File, ctx evalContext) ([]PluginDeclaration, error) {
	declarations := []PluginDeclaration{}
	for _, statement := range file.Statements {
		node, ok := statement.(*BlockNode)
		if !ok || node.Kind != "plugin" {
			continue
		}
		declaration, err := evaluatePluginDeclaration(node, ctx)
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, PluginDeclaration{
			Declaration: declaration,
			Pos:         node.Position(),
		})
	}
	return declarations, nil
}

func isPluginDeclaration(statement Statement) bool {
	node, ok := statement.(*BlockNode)
	return ok && node.Kind == "plugin"
}

func evaluateStatement(statement Statement, ctx evalContext) ([]build.Task, error) {
	switch node := statement.(type) {
	case *ImportNode:
		return nil, nil
	case *BlockNode:
		return evaluateBlock(node, ctx)
	case *RuleNode:
		return evaluateRule(node, ctx)
	default:
		return nil, fmt.Errorf("dsl:%d:%d: unsupported statement", statement.Position().Line, statement.Position().Column)
	}
}

func evaluateBlock(node *BlockNode, ctx evalContext) ([]build.Task, error) {
	switch node.Kind {
	case "workspace":
		if err := evaluateWorkspace(node, ctx); err != nil {
			return nil, err
		}
		return nil, nil
	case "plugin":
		return nil, nil
	case "toolchain":
		if err := evaluateToolchain(node, ctx); err != nil {
			return nil, err
		}
		return nil, nil
	case "task":
		task, err := evaluateTask(node, ctx)
		if err != nil {
			return nil, err
		}
		return []build.Task{task}, nil
	default:
		if namespace, rule, ok := splitRuleKind(node.Kind); ok {
			return evaluatePluginRuleBlock(node, namespace, rule, ctx)
		}
		return nil, fmt.Errorf("dsl:%d:%d: unknown block %q", node.Position().Line, node.Position().Column, node.Kind)
	}
}

func evaluateWorkspace(node *BlockNode, ctx evalContext) error {
	if node.Name != nil {
		return fmt.Errorf("dsl:%d:%d: workspace does not take a block name; use workspace { name = ... }", node.Name.Position().Line, node.Name.Position().Column)
	}
	fields := newFieldSet("workspace", node.Assignments, ctx)
	if _, err := fields.optionalString("name", ""); err != nil {
		return err
	}
	if _, err := fields.optionalString("default", ""); err != nil {
		return err
	}
	return fields.rejectUnknown("name", "default")
}

func evaluatePluginDeclaration(node *BlockNode, ctx evalContext) (buildplugin.Declaration, error) {
	namespace, err := evaluateSymbolName("plugin", node.Name, node.Position(), ctx)
	if err != nil {
		return buildplugin.Declaration{}, err
	}
	fields := newFieldSet("plugin", node.Assignments, ctx)
	id, err := fields.optionalString("id", "")
	if err != nil {
		return buildplugin.Declaration{}, err
	}
	source, err := fields.optionalString("source", "")
	if err != nil {
		return buildplugin.Declaration{}, err
	}
	version, err := fields.optionalString("version", "")
	if err != nil {
		return buildplugin.Declaration{}, err
	}
	path, err := fields.optionalString("path", "")
	if err != nil {
		return buildplugin.Declaration{}, err
	}
	if err := fields.rejectUnknown("id", "source", "version", "path"); err != nil {
		return buildplugin.Declaration{}, err
	}
	return buildplugin.Declaration{
		Namespace: namespace,
		ID:        id,
		Source:    buildplugin.Source(source),
		Version:   version,
		Path:      path,
	}, nil
}

func evaluateToolchain(node *BlockNode, ctx evalContext) error {
	if _, err := evaluateSymbolName("toolchain", node.Name, node.Position(), ctx); err != nil {
		return err
	}
	fields := newFieldSet("toolchain", node.Assignments, ctx)
	if _, err := fields.optionalString("version", ""); err != nil {
		return err
	}
	if _, err := fields.optionalValue("settings"); err != nil {
		return err
	}
	return fields.rejectUnknown("version", "settings")
}

func evaluateTask(node *BlockNode, ctx evalContext) (build.Task, error) {
	name, err := evaluateSymbolName("task", node.Name, node.Position(), ctx)
	if err != nil {
		return build.Task{}, err
	}
	taskCtx := ctx.with("target", name)
	task := build.NewTask(name)
	hasCommand := false

	for _, assignment := range node.Assignments {
		values, err := evaluateStringList(assignment.Value, taskCtx)
		if err != nil {
			return build.Task{}, fieldError("task", assignment, err)
		}
		switch assignment.Name {
		case "deps":
			task.Deps = collectionx.NewList(values...)
		case "inputs":
			task.Inputs = collectionx.NewList(values...)
		case "outputs":
			task.Outputs = collectionx.NewList(values...)
		case "command":
			hasCommand = true
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
	if hasCommand && len(node.Actions) > 0 {
		return build.Task{}, fmt.Errorf("dsl:%d:%d: task cannot define both command and run block", node.Actions[0].Position().Line, node.Actions[0].Position().Column)
	}
	command, err := evaluateActions(node.Actions, taskCtx)
	if err != nil {
		return build.Task{}, err
	}
	if len(command) > 0 {
		task.Command = collectionx.NewList(command...)
	}

	if err := task.Validate(); err != nil {
		return build.Task{}, fmt.Errorf("dsl:%d:%d: %w", node.Position().Line, node.Position().Column, err)
	}
	return task, nil
}

func evaluateActions(actions []*ActionNode, ctx evalContext) ([]string, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	if len(actions) > 1 {
		action := actions[1]
		return nil, fmt.Errorf("dsl:%d:%d: task run block supports one action in this version", action.Position().Line, action.Position().Column)
	}
	action := actions[0]
	switch action.Call.Name {
	case "exec":
		items := collectionx.NewList[string]()
		for _, arg := range action.Call.Args {
			value, err := evaluateString(arg, ctx)
			if err != nil {
				return nil, err
			}
			items.Add(value)
		}
		if len(items.Values()) == 0 {
			return nil, fmt.Errorf("dsl:%d:%d: exec requires at least one argument", action.Position().Line, action.Position().Column)
		}
		return items.Values(), nil
	case "shell":
		if len(action.Call.Args) != 1 {
			return nil, fmt.Errorf("dsl:%d:%d: shell expects exactly one script argument", action.Position().Line, action.Position().Column)
		}
		script, err := evaluateString(action.Call.Args[0], ctx)
		if err != nil {
			return nil, err
		}
		if err := validateShellScript(script, action.Position()); err != nil {
			return nil, err
		}
		return []string{"sh", "-c", script}, nil
	default:
		return nil, fmt.Errorf("dsl:%d:%d: unknown run action %q", action.Position().Line, action.Position().Column, action.Call.Name)
	}
}

func validateShellScript(script string, pos Position) error {
	if _, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(script), "shell"); err != nil {
		return fmt.Errorf("dsl:%d:%d: shell syntax error: %w", pos.Line, pos.Column, err)
	}
	return nil
}

func evaluateRule(node *RuleNode, ctx evalContext) ([]build.Task, error) {
	return nil, fmt.Errorf("dsl:%d:%d: call-style top-level rule %q is not supported; use namespace.rule target { ... }", node.Position().Line, node.Position().Column, node.Call.Name)
}

func evaluateSymbolName(owner string, name Expr, pos Position, ctx evalContext) (string, error) {
	if name == nil {
		return "", fmt.Errorf("dsl:%d:%d: %s block requires a symbol name", pos.Line, pos.Column, owner)
	}
	if _, ok := name.(*IdentExpr); !ok {
		return "", fmt.Errorf("dsl:%d:%d: %s block name must be a symbol", name.Position().Line, name.Position().Column, owner)
	}
	return evaluateString(name, ctx)
}

func evaluatePluginRuleBlock(node *BlockNode, namespace string, rule string, ctx evalContext) ([]build.Task, error) {
	target, err := evaluateSymbolName(node.Kind, node.Name, node.Position(), ctx)
	if err != nil {
		return nil, err
	}
	ruleCtx := ctx.with("target", target)
	fields, err := evaluateFields(node.Assignments, ruleCtx)
	if err != nil {
		return nil, err
	}
	return ctx.registry.Expand(ctx.context, buildplugin.Invocation{
		Namespace: namespace,
		Rule:      rule,
		Target:    target,
		Fields:    fields,
	})
}

func evaluateFields(assignments []*AssignmentNode, ctx evalContext) (map[string]any, error) {
	fields := map[string]any{}
	for _, assignment := range assignments {
		value, err := evaluate(assignment.Value, ctx)
		if err != nil {
			return nil, fieldError("rule", assignment, err)
		}
		fields[assignment.Name] = valueAny(value)
	}
	return fields, nil
}

func valueAny(item value) any {
	switch item.kind {
	case valueString:
		return item.text
	case valueList:
		return item.list
	case valueObject:
		object := map[string]any{}
		item.object.Range(func(key string, value value) bool {
			object[key] = valueAny(value)
			return true
		})
		return object
	default:
		return nil
	}
}

func splitRuleKind(kind string) (string, string, bool) {
	namespace, rule, ok := strings.Cut(kind, ".")
	if !ok || namespace == "" || rule == "" {
		return "", "", false
	}
	return namespace, rule, true
}

func evaluateString(expr Expr, ctx evalContext) (string, error) {
	value, err := evaluate(expr, ctx)
	if err != nil {
		return "", err
	}
	if value.kind != valueString {
		return "", fmt.Errorf("dsl:%d:%d: expected string expression", expr.Position().Line, expr.Position().Column)
	}
	return value.text, nil
}

func evaluateStringList(expr Expr, ctx evalContext) ([]string, error) {
	value, err := evaluate(expr, ctx)
	if err != nil {
		return nil, err
	}
	if value.kind == valueString {
		return []string{value.text}, nil
	}
	if value.kind != valueList {
		return nil, fmt.Errorf("dsl:%d:%d: expected string or list expression", expr.Position().Line, expr.Position().Column)
	}
	return value.list, nil
}

func evaluate(exprNode Expr, ctx evalContext) (value, error) {
	switch node := exprNode.(type) {
	case *StringExpr:
		return value{kind: valueString, text: node.Value}, nil
	case *ScriptExpr:
		return evaluateScript(node, ctx)
	case *IdentExpr:
		if item, ok := ctx.vars[node.Name]; ok {
			return valueFromAny(item, node.Position())
		}
		return value{kind: valueString, text: node.Name}, nil
	case *ArrayExpr:
		items := collectionx.NewList[string]()
		for _, element := range node.Elements {
			value, err := evaluate(element, ctx)
			if err != nil {
				return value, err
			}
			if value.kind == valueString {
				items.Add(value.text)
			} else if value.kind == valueList {
				items.Add(value.list...)
			} else {
				return value, fmt.Errorf("dsl:%d:%d: object values cannot be used in arrays", element.Position().Line, element.Position().Column)
			}
		}
		return value{kind: valueList, list: items.Values()}, nil
	case *ObjectExpr:
		object := collectionx.NewMap[string, value]()
		for _, entry := range node.Entries {
			value, err := evaluate(entry.Value, ctx)
			if err != nil {
				return value, err
			}
			object.Set(entry.Key, value)
		}
		return value{kind: valueObject, object: object}, nil
	case *CallExpr:
		return evaluateCall(node, ctx)
	default:
		return value{}, fmt.Errorf("dsl:%d:%d: unsupported expression", exprNode.Position().Line, exprNode.Position().Column)
	}
}

func evaluateCall(call *CallExpr, ctx evalContext) (value, error) {
	switch call.Name {
	case "env":
		return evaluateEnv(call, ctx)
	case "concat":
		return evaluateConcat(call, ctx)
	case "list":
		return evaluateList(call, ctx)
	case "os":
		return evaluateNoArgString(call, runtime.GOOS)
	case "arch":
		return evaluateNoArgString(call, runtime.GOARCH)
	default:
		return value{}, fmt.Errorf("dsl:%d:%d: unknown function %q", call.Position().Line, call.Position().Column, call.Name)
	}
}

func evaluateScript(script *ScriptExpr, ctx evalContext) (value, error) {
	output, err := expr.Eval(script.Source, ctx.exprEnv())
	if err != nil {
		return value{}, fmt.Errorf("dsl:%d:%d: evaluate expression %q: %w", script.Position().Line, script.Position().Column, script.Source, err)
	}
	return valueFromAny(output, script.Position())
}

func evaluateEnv(call *CallExpr, ctx evalContext) (value, error) {
	if len(call.Args) != 1 && len(call.Args) != 2 {
		return value{}, fmt.Errorf("dsl:%d:%d: env expects 1 or 2 args", call.Position().Line, call.Position().Column)
	}
	name, err := evaluateString(call.Args[0], ctx)
	if err != nil {
		return value{}, err
	}
	fallback := ""
	if len(call.Args) == 2 {
		fallback, err = evaluateString(call.Args[1], ctx)
		if err != nil {
			return value{}, err
		}
	}
	if result := ctx.readEnv(name); result != "" {
		return value{kind: valueString, text: result}, nil
	}
	return value{kind: valueString, text: fallback}, nil
}

func evaluateConcat(call *CallExpr, ctx evalContext) (value, error) {
	parts := collectionx.NewList[string]()
	for _, arg := range call.Args {
		part, err := evaluateString(arg, ctx)
		if err != nil {
			return value{}, err
		}
		parts.Add(part)
	}
	return value{kind: valueString, text: strings.Join(parts.Values(), "")}, nil
}

func evaluateList(call *CallExpr, ctx evalContext) (value, error) {
	items := collectionx.NewList[string]()
	for _, arg := range call.Args {
		values, err := evaluateStringList(arg, ctx)
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

func valueFromAny(item any, pos Position) (value, error) {
	switch typed := item.(type) {
	case string:
		return value{kind: valueString, text: typed}, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return value{kind: valueString, text: fmt.Sprint(typed)}, nil
	case []string:
		return value{kind: valueList, list: typed}, nil
	case []any:
		items := collectionx.NewList[string]()
		for _, element := range typed {
			scalar, ok := scalarString(element)
			if !ok {
				return value{}, fmt.Errorf("dsl:%d:%d: expression list values must be scalar", pos.Line, pos.Column)
			}
			items.Add(scalar)
		}
		return value{kind: valueList, list: items.Values()}, nil
	case map[string]any:
		object := collectionx.NewMap[string, value]()
		for key, element := range typed {
			value, err := valueFromAny(element, pos)
			if err != nil {
				return value, err
			}
			object.Set(key, value)
		}
		return value{kind: valueObject, object: object}, nil
	default:
		return value{}, fmt.Errorf("dsl:%d:%d: expression returned unsupported value %T", pos.Line, pos.Column, item)
	}
}

func scalarString(item any) (string, bool) {
	switch typed := item.(type) {
	case string:
		return typed, true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(typed), true
	default:
		return "", false
	}
}

func (c evalContext) with(key string, item any) evalContext {
	vars := map[string]any{}
	for k, v := range c.vars {
		vars[k] = v
	}
	vars[key] = item
	return evalContext{
		context:  c.context,
		registry: c.registry,
		vars:     vars,
		envs:     c.envs,
	}
}

func (c evalContext) exprEnv() map[string]any {
	env := map[string]any{
		"os":     runtime.GOOS,
		"arch":   runtime.GOARCH,
		"env":    c.readEnv,
		"concat": exprConcat,
		"list":   exprList,
	}
	for key, item := range c.vars {
		env[key] = item
	}
	return env
}

func (c evalContext) readEnv(name string, fallback ...string) string {
	result := os.Getenv(name)
	if c.envs != nil {
		c.envs.Set(name, result)
	}
	if result != "" {
		return result
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

func exprConcat(items ...any) string {
	parts := collectionx.NewList[string]()
	for _, item := range items {
		parts.Add(fmt.Sprint(item))
	}
	return strings.Join(parts.Values(), "")
}

func exprList(items ...any) []any {
	return items
}

type envTracker struct {
	items collectionx.Map[string, string]
}

func newEnvTracker() *envTracker {
	return &envTracker{items: collectionx.NewMap[string, string]()}
}

func (t *envTracker) Set(name string, value string) {
	if t == nil || name == "" {
		return
	}
	t.items.Set(name, value)
}

func (t *envTracker) Values() []envDependency {
	if t == nil {
		return nil
	}
	items := collectionx.NewList[envDependency]()
	t.items.Range(func(name string, value string) bool {
		items.Add(envDependency{Name: name, Value: value})
		return true
	})
	values := items.Values()
	sort.SliceStable(values, func(i, j int) bool {
		return values[i].Name < values[j].Name
	})
	return values
}

type fieldSet struct {
	rule        string
	assignments collectionx.Map[string, *AssignmentNode]
	ctx         evalContext
}

func newFieldSet(rule string, assignments []*AssignmentNode, ctx evalContext) fieldSet {
	items := collectionx.NewMap[string, *AssignmentNode]()
	for _, assignment := range assignments {
		items.Set(assignment.Name, assignment)
	}
	return fieldSet{rule: rule, assignments: items, ctx: ctx}
}

func (s fieldSet) requiredString(name string) (string, error) {
	assignment, ok := s.assignments.Get(name)
	if !ok {
		return "", fmt.Errorf("dsl: %s requires field %q", s.rule, name)
	}
	return s.string(assignment)
}

func (s fieldSet) optionalString(name string, fallback string) (string, error) {
	assignment, ok := s.assignments.Get(name)
	if !ok {
		return fallback, nil
	}
	return s.string(assignment)
}

func (s fieldSet) optionalList(name string, fallback []string) ([]string, error) {
	assignment, ok := s.assignments.Get(name)
	if !ok {
		return fallback, nil
	}
	return s.list(assignment)
}

func (s fieldSet) optionalValue(name string) (value, error) {
	assignment, ok := s.assignments.Get(name)
	if !ok {
		return value{}, nil
	}
	item, err := evaluate(assignment.Value, s.ctx)
	if err != nil {
		return value{}, fieldError(s.rule, assignment, err)
	}
	return item, nil
}

func (s fieldSet) string(assignment *AssignmentNode) (string, error) {
	item, err := evaluateString(assignment.Value, s.ctx)
	if err != nil {
		return "", fieldError(s.rule, assignment, err)
	}
	return item, nil
}

func (s fieldSet) list(assignment *AssignmentNode) ([]string, error) {
	items, err := evaluateStringList(assignment.Value, s.ctx)
	if err != nil {
		return nil, fieldError(s.rule, assignment, err)
	}
	return items, nil
}

func (s fieldSet) rejectUnknown(allowed ...string) error {
	known := collectionx.NewSet(allowed...)
	var unknown *AssignmentNode
	s.assignments.Range(func(name string, assignment *AssignmentNode) bool {
		if known.Contains(name) {
			return true
		}
		unknown = assignment
		return false
	})
	if unknown == nil {
		return nil
	}
	return fmt.Errorf("dsl:%d:%d: unknown %s field %q", unknown.Position().Line, unknown.Position().Column, s.rule, unknown.Name)
}

func fieldError(owner string, assignment *AssignmentNode, err error) error {
	return fmt.Errorf(
		"dsl:%d:%d: %s field %q: %w",
		assignment.Position().Line,
		assignment.Position().Column,
		owner,
		assignment.Name,
		err,
	)
}
