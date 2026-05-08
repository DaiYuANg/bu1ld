package dsl

import (
	"context"
	"go/token"
	"slices"

	"bu1ld/internal/build"
	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"
	planocomp "github.com/arcgolabs/plano/compiler"
	planschema "github.com/arcgolabs/plano/schema"
)

func PluginDeclarations(file *File) ([]PluginDeclaration, error) {
	if file == nil || file.Result.HIR == nil {
		return nil, nil
	}
	declarations := list.NewList[PluginDeclaration]()
	forms := file.Result.HIR.Forms.Values()
	for i := range forms {
		form := forms[i]
		if form.Kind != "plugin" {
			continue
		}
		namespace, err := symbolLabel(file.Result.FileSet, form, "plugin")
		if err != nil {
			return nil, err
		}
		declaration := buildplugin.Declaration{Namespace: namespace}
		if field, ok := form.Field("id"); ok {
			declaration.ID, err = stringFieldValue(file.Result.FileSet, field)
			if err != nil {
				return nil, err
			}
		}
		if field, ok := form.Field("source"); ok {
			source, sourceErr := stringFieldValue(file.Result.FileSet, field)
			if sourceErr != nil {
				return nil, sourceErr
			}
			declaration.Source = buildplugin.Source(source)
		}
		if field, ok := form.Field("version"); ok {
			declaration.Version, err = stringFieldValue(file.Result.FileSet, field)
			if err != nil {
				return nil, err
			}
		}
		if field, ok := form.Field("path"); ok {
			declaration.Path, err = stringFieldValue(file.Result.FileSet, field)
			if err != nil {
				return nil, err
			}
		}
		declarations.Add(PluginDeclaration{
			Declaration: declaration,
			Pos:         form.Pos,
		})
	}
	return declarations.Values(), nil
}

func lowerProject(ctx context.Context, file *File, registry *buildplugin.Registry) (build.Project, error) {
	tasks := list.NewList[build.Task]()
	seen := set.NewSet[string]()
	if file == nil || file.Result.HIR == nil {
		return build.Project{Tasks: tasks, Packages: list.NewList[build.Package]()}, nil
	}
	configs, configNamespaces, err := pluginConfigurations(file, registry)
	if err != nil {
		return build.Project{}, err
	}
	configuredTasks, err := registry.Configure(ctx, configs)
	if err != nil {
		return build.Project{}, dslErrorAt(file.Result.FileSet, token.NoPos, "configure plugins: %v", err)
	}
	for _, task := range configuredTasks {
		if seen.Contains(task.Name) {
			return build.Project{}, dslErrorAt(file.Result.FileSet, token.NoPos, "duplicate task %q", task.Name)
		}
		seen.Add(task.Name)
		tasks.Add(task)
	}
	forms := file.Result.HIR.Forms.Values()
	for i := range forms {
		form := forms[i]
		items, err := lowerTopLevelForm(ctx, file.Result.FileSet, form, registry, configNamespaces)
		if err != nil {
			return build.Project{}, err
		}
		for _, task := range items {
			if seen.Contains(task.Name) {
				return build.Project{}, dslErrorAt(file.Result.FileSet, form.Pos, "duplicate task %q", task.Name)
			}
			seen.Add(task.Name)
			tasks.Add(task)
		}
	}
	return build.Project{Tasks: tasks, Packages: list.NewList[build.Package]()}, nil
}

func pluginConfigurations(file *File, registry *buildplugin.Registry) (map[string]buildplugin.PluginConfig, map[string]buildplugin.Metadata, error) {
	configNamespaces, err := registry.ConfigNamespaces()
	if err != nil {
		return nil, nil, err
	}
	configs := mapping.NewMap[string, buildplugin.PluginConfig]()
	if file == nil || file.Result.HIR == nil {
		return configs.All(), configNamespaces, nil
	}
	for _, form := range file.Result.HIR.Forms.Values() {
		if _, ok := configNamespaces[form.Kind]; !ok {
			continue
		}
		if _, exists := configs.Get(form.Kind); exists {
			return nil, nil, dslErrorAt(file.Result.FileSet, form.Pos, "duplicate plugin config %q", form.Kind)
		}
		fields, err := invocationFields(file.Result.FileSet, form)
		if err != nil {
			return nil, nil, err
		}
		configs.Set(form.Kind, buildplugin.PluginConfig{
			Namespace: form.Kind,
			Fields:    fields,
		})
	}
	return configs.All(), configNamespaces, nil
}

func lowerTopLevelForm(
	ctx context.Context,
	fset *token.FileSet,
	form planocomp.HIRForm,
	registry *buildplugin.Registry,
	configNamespaces map[string]buildplugin.Metadata,
) ([]build.Task, error) {
	switch form.Kind {
	case "workspace", "package", "plugin", "toolchain":
		return nil, nil
	case "task":
		task, err := lowerTask(fset, form)
		if err != nil {
			return nil, err
		}
		return []build.Task{task}, nil
	default:
		if _, ok := configNamespaces[form.Kind]; ok {
			return nil, nil
		}
		namespace, rule, ok := splitQualifiedKind(form.Kind)
		if !ok {
			return nil, dslErrorAt(fset, form.Pos, "unknown form %q", form.Kind)
		}
		return lowerPluginRuleForm(ctx, fset, form, registry, namespace, rule)
	}
}

func lowerTask(fset *token.FileSet, form planocomp.HIRForm) (build.Task, error) {
	name, err := symbolLabel(fset, form, "task")
	if err != nil {
		return build.Task{}, err
	}
	task := build.NewTask(name)
	hasCommand := false

	for _, field := range form.Fields.Values() {
		switch field.Name {
		case "deps":
			values, valueErr := stringListValue(fset, field)
			if valueErr != nil {
				return build.Task{}, valueErr
			}
			task.Deps = list.NewList[string](values...)
		case "inputs":
			values, valueErr := stringListValue(fset, field)
			if valueErr != nil {
				return build.Task{}, valueErr
			}
			task.Inputs = list.NewList[string](values...)
		case "outputs":
			values, valueErr := stringListValue(fset, field)
			if valueErr != nil {
				return build.Task{}, valueErr
			}
			task.Outputs = list.NewList[string](values...)
		case "command":
			values, valueErr := stringListValue(fset, field)
			if valueErr != nil {
				return build.Task{}, valueErr
			}
			hasCommand = true
			task.Command = list.NewList[string](values...)
		default:
			return build.Task{}, dslErrorAt(fset, field.Pos, "unknown task field %q", field.Name)
		}
	}

	runForms := form.Forms.Values()
	if hasCommand && len(runForms) > 0 {
		return build.Task{}, dslErrorAt(fset, runForms[0].Pos, "task cannot define both command and run block")
	}
	if len(runForms) > 1 {
		return build.Task{}, dslErrorAt(fset, runForms[1].Pos, "task run block supports one action in this version")
	}
	if len(runForms) == 1 {
		command, commandErr := lowerRunForm(fset, runForms[0])
		if commandErr != nil {
			return build.Task{}, commandErr
		}
		task.Command = list.NewList[string](command...)
	}

	if err := task.Validate(); err != nil {
		return build.Task{}, dslErrorAt(fset, form.Pos, "%s", err.Error())
	}
	return task, nil
}

func lowerRunForm(fset *token.FileSet, form planocomp.HIRForm) ([]string, error) {
	if form.Kind != "run" {
		return nil, dslErrorAt(fset, form.Pos, "task nested form %q is not supported", form.Kind)
	}
	calls := form.Calls.Values()
	if len(calls) == 0 {
		return nil, nil
	}
	if len(calls) > 1 {
		return nil, dslErrorAt(fset, calls[1].Pos, "task run block supports one action in this version")
	}
	return lowerActionCall(fset, calls[0])
}

func lowerActionCall(fset *token.FileSet, call planocomp.HIRCall) ([]string, error) {
	switch call.Name {
	case "exec":
		command := list.NewList[string]()
		for _, arg := range call.Args.Values() {
			text, err := stringValue(fset, call.Pos, arg.Value)
			if err != nil {
				return nil, err
			}
			command.Add(text)
		}
		values := command.Values()
		if len(values) == 0 {
			return nil, dslErrorAt(fset, call.Pos, "exec requires at least one argument")
		}
		return values, nil
	case "shell":
		if len(call.Args.Values()) != 1 {
			return nil, dslErrorAt(fset, call.Pos, "shell expects exactly one script argument")
		}
		script, err := stringValue(fset, call.Pos, call.Args.Values()[0].Value)
		if err != nil {
			return nil, err
		}
		return []string{"sh", "-c", script}, nil
	default:
		return nil, dslErrorAt(fset, call.Pos, "unknown run action %q", call.Name)
	}
}

func lowerPluginRuleForm(
	ctx context.Context,
	fset *token.FileSet,
	form planocomp.HIRForm,
	registry *buildplugin.Registry,
	namespace string,
	rule string,
) ([]build.Task, error) {
	target, err := symbolLabel(fset, form, form.Kind)
	if err != nil {
		return nil, err
	}
	fields, err := invocationFields(fset, form)
	if err != nil {
		return nil, err
	}
	tasks, err := registry.Expand(ctx, buildplugin.Invocation{
		Namespace: namespace,
		Rule:      rule,
		Target:    target,
		Fields:    fields,
	})
	if err != nil {
		return nil, dslErrorAt(fset, form.Pos, "expand rule %s.%s: %v", namespace, rule, err)
	}
	return tasks, nil
}

func invocationFields(fset *token.FileSet, form planocomp.HIRForm) (map[string]any, error) {
	fields := mapping.NewMapWithCapacity[string, any](form.Fields.Len())
	for _, field := range form.Fields.Values() {
		value, err := invocationFieldValue(fset, field)
		if err != nil {
			return nil, err
		}
		fields.Set(field.Name, value)
	}
	return fields.All(), nil
}

func invocationFieldValue(fset *token.FileSet, field planocomp.HIRField) (any, error) {
	switch field.Expected.(type) {
	case planschema.ListType:
		return stringListValue(fset, field)
	case planschema.MapType:
		return anyMapValue(fset, field.Pos, field.Value)
	default:
		if _, ok := field.Value.(bool); ok {
			return field.Value, nil
		}
		return stringValue(fset, field.Pos, field.Value)
	}
}

func symbolLabel(fset *token.FileSet, form planocomp.HIRForm, owner string) (string, error) {
	if form.Label == nil || form.Label.Value == "" {
		return "", dslErrorAt(fset, form.Pos, "%s requires symbol label", owner)
	}
	return form.Label.Value, nil
}

func stringFieldValue(fset *token.FileSet, field planocomp.HIRField) (string, error) {
	return stringValue(fset, field.Pos, field.Value)
}

func stringListValue(fset *token.FileSet, field planocomp.HIRField) ([]string, error) {
	return stringListFromValue(fset, field.Pos, field.Value)
}

func stringValue(fset *token.FileSet, pos token.Pos, value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case planschema.Ref:
		return typed.Name, nil
	default:
		return "", dslErrorAt(fset, pos, "expected string value, got %T", value)
	}
}

func stringListFromValue(fset *token.FileSet, pos token.Pos, value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{typed}, nil
	case []string:
		return slices.Clone(typed), nil
	case planschema.Ref:
		return []string{typed.Name}, nil
	case []any:
		values := list.NewList[string]()
		for _, item := range typed {
			text, err := stringValue(fset, pos, item)
			if err != nil {
				return nil, err
			}
			values.Add(text)
		}
		return values.Values(), nil
	default:
		return nil, dslErrorAt(fset, pos, "expected list value, got %T", value)
	}
}

func anyValue(fset *token.FileSet, pos token.Pos, value any) (any, error) {
	switch typed := value.(type) {
	case nil, string, bool, int64, float64:
		return typed, nil
	case planschema.Ref:
		return typed.Name, nil
	case []any:
		values := list.NewListWithCapacity[any](len(typed))
		for _, item := range typed {
			next, err := anyValue(fset, pos, item)
			if err != nil {
				return nil, err
			}
			values.Add(next)
		}
		return values.Values(), nil
	case map[string]any:
		out := mapping.NewMapWithCapacity[string, any](len(typed))
		for key, item := range typed {
			next, err := anyValue(fset, pos, item)
			if err != nil {
				return nil, err
			}
			out.Set(key, next)
		}
		return out.All(), nil
	case *mapping.OrderedMap[string, any]:
		out := mapping.NewMapWithCapacity[string, any](typed.Len())
		var rangeErr error
		typed.Range(func(key string, item any) bool {
			next, err := anyValue(fset, pos, item)
			if err != nil {
				rangeErr = err
				return false
			}
			out.Set(key, next)
			return true
		})
		if rangeErr != nil {
			return nil, rangeErr
		}
		return out.All(), nil
	default:
		return nil, dslErrorAt(fset, pos, "unsupported value %T", value)
	}
}

func anyMapValue(fset *token.FileSet, pos token.Pos, value any) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := mapping.NewMapWithCapacity[string, any](len(typed))
		for key, item := range typed {
			next, err := anyValue(fset, pos, item)
			if err != nil {
				return nil, err
			}
			out.Set(key, next)
		}
		return out.All(), nil
	case *mapping.OrderedMap[string, any]:
		out := mapping.NewMapWithCapacity[string, any](typed.Len())
		var rangeErr error
		typed.Range(func(key string, item any) bool {
			next, err := anyValue(fset, pos, item)
			if err != nil {
				rangeErr = err
				return false
			}
			out.Set(key, next)
			return true
		})
		if rangeErr != nil {
			return nil, rangeErr
		}
		return out.All(), nil
	default:
		return nil, dslErrorAt(fset, pos, "expected object value, got %T", value)
	}
}
