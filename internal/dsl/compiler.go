package dsl

import (
	"errors"
	"fmt"
	"strings"

	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	planocomp "github.com/arcgolabs/plano/compiler"
	planschema "github.com/arcgolabs/plano/schema"
	"github.com/samber/oops"
	"mvdan.cc/sh/v3/syntax"
)

const planoParseCacheEntries = 64

func newCompiler(
	registry *buildplugin.Registry,
	readFile func(string) ([]byte, error),
	lookupEnv func(string) (string, bool),
) (*planocomp.Compiler, error) {
	compiler := planocomp.New(planocomp.Options{
		LookupEnv:         lookupEnv,
		ReadFile:          readFile,
		ParseCacheEntries: planoParseCacheEntries,
	})
	compiler.RegisterConst("builtin", string(buildplugin.SourceBuiltin))
	compiler.RegisterConst("local", string(buildplugin.SourceLocal))
	compiler.RegisterConst("global", string(buildplugin.SourceGlobal))
	compiler.RegisterConst("container", string(buildplugin.SourceContainer))

	if err := compiler.RegisterForms(planschema.FormSpecs(
		workspaceFormSpec(),
		packageFormSpec(),
		pluginFormSpec(),
		toolchainFormSpec(),
		taskFormSpec(),
		runFormSpec(),
	)); err != nil {
		return nil, oops.In("bu1ld.dsl").Wrapf(err, "register core plano forms")
	}
	if err := compiler.RegisterActions(planocomp.ActionSpecs(
		execActionSpec(),
		shellActionSpec(),
	)); err != nil {
		return nil, oops.In("bu1ld.dsl").Wrapf(err, "register plano actions")
	}
	if err := registerPluginRuleForms(compiler, registry); err != nil {
		return nil, err
	}
	return compiler, nil
}

func registerPluginRuleForms(compiler *planocomp.Compiler, registry *buildplugin.Registry) error {
	schemas, err := registry.Schemas()
	if err != nil {
		return oops.In("bu1ld.dsl").Wrapf(err, "read plugin schemas")
	}
	for _, metadata := range schemas {
		if metadata.AutoConfigure || len(metadata.ConfigFields) > 0 {
			spec := planschema.FormSpec{
				Name:      metadata.Namespace,
				LabelKind: planschema.LabelNone,
				BodyMode:  planschema.BodyFieldOnly,
				Fields:    planschema.Fields(pluginFieldSpecs(metadata.ConfigFields)...),
				Docs:      metadata.ID + " plugin configuration.",
			}
			if err := compiler.RegisterForm(spec); err != nil {
				return oops.In("bu1ld.dsl").
					With("plugin", metadata.ID).
					With("namespace", metadata.Namespace).
					Wrapf(err, "register plugin config form")
			}
		}
		for _, rule := range metadata.Rules {
			spec := planschema.FormSpec{
				Name:         metadata.Namespace + "." + rule.Name,
				LabelKind:    planschema.LabelSymbol,
				LabelRefKind: "task",
				Declares:     "task",
				BodyMode:     planschema.BodyScript,
				Fields:       pluginRuleFields(rule),
				Docs:         metadata.ID + " rule " + rule.Name,
			}
			if err := compiler.RegisterForm(spec); err != nil {
				return oops.In("bu1ld.dsl").
					With("plugin", metadata.ID).
					With("namespace", metadata.Namespace).
					With("rule", rule.Name).
					Wrapf(err, "register plugin rule form")
			}
		}
	}
	return nil
}

func workspaceFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:      "workspace",
		LabelKind: planschema.LabelNone,
		BodyMode:  planschema.BodyFieldOnly,
		Fields: planschema.Fields(
			planschema.FieldSpec{Name: "name", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "default", Type: planschema.RefType{Kind: "task"}},
			planschema.FieldSpec{Name: "packages", Type: planschema.ListType{Elem: planschema.TypePath}},
		),
		Docs: "Workspace metadata and default target.",
	}
}

func packageFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:      "package",
		LabelKind: planschema.LabelNone,
		BodyMode:  planschema.BodyFieldOnly,
		Fields: planschema.Fields(
			planschema.FieldSpec{Name: "name", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "deps", Type: planschema.ListType{Elem: planschema.TypeString}},
		),
		Docs: "Package metadata for monorepo workspaces.",
	}
}

func pluginFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:      "plugin",
		LabelKind: planschema.LabelSymbol,
		BodyMode:  planschema.BodyFieldOnly,
		Fields: planschema.Fields(
			planschema.FieldSpec{Name: "source", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "id", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "version", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "path", Type: planschema.TypePath},
			planschema.FieldSpec{Name: "image", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "pull", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "network", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "work_dir", Type: planschema.TypePath},
		),
		Docs: "Declare a builtin, local, global, or container bu1ld plugin.",
	}
}

func toolchainFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:      "toolchain",
		LabelKind: planschema.LabelSymbol,
		BodyMode:  planschema.BodyFieldOnly,
		Fields: planschema.Fields(
			planschema.FieldSpec{Name: "version", Type: planschema.TypeString},
			planschema.FieldSpec{Name: "settings", Type: planschema.MapType{Elem: planschema.TypeAny}},
		),
		Docs: "Declare toolchain configuration.",
	}
}

func taskFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:         "task",
		LabelKind:    planschema.LabelSymbol,
		LabelRefKind: "task",
		Declares:     "task",
		BodyMode:     planschema.BodyScript,
		Fields: planschema.Fields(
			planschema.FieldSpec{Name: "deps", Type: taskDependencyListType()},
			planschema.FieldSpec{Name: "inputs", Type: planschema.ListType{Elem: planschema.TypePath}},
			planschema.FieldSpec{Name: "outputs", Type: planschema.ListType{Elem: planschema.TypePath}},
			planschema.FieldSpec{Name: "command", Type: planschema.ListType{Elem: planschema.TypeString}},
		),
		NestedForms: planschema.NestedForms("run"),
		Docs:        "Declare a task with typed fields and script-capable body.",
	}
}

func runFormSpec() planschema.FormSpec {
	return planschema.FormSpec{
		Name:      "run",
		LabelKind: planschema.LabelNone,
		BodyMode:  planschema.BodyCallOnly,
		Docs:      "Run one build action.",
	}
}

func execActionSpec() planocomp.ActionSpec {
	return planocomp.ActionSpec{
		Name:         "exec",
		MinArgs:      1,
		MaxArgs:      -1,
		ArgTypes:     planschema.Types(planschema.TypeString),
		VariadicType: planschema.TypeString,
		Docs:         "Execute an external command.",
	}
}

func shellActionSpec() planocomp.ActionSpec {
	return planocomp.ActionSpec{
		Name:     "shell",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: planschema.Types(planschema.TypeString),
		Validate: func(args list.List[any]) error {
			if args.Len() != 1 {
				return errors.New("shell expects exactly one script argument")
			}
			script, ok := args.Get(0)
			if !ok {
				return errors.New("shell expects exactly one script argument")
			}
			text, ok := script.(string)
			if !ok {
				return errors.New("shell expects string argument")
			}
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(text), "shell"); err != nil {
				return fmt.Errorf("shell syntax error: %w", err)
			}
			return nil
		},
		Docs: "Execute a POSIX shell snippet.",
	}
}

func pluginRuleFields(rule buildplugin.RuleSchema) *mapping.OrderedMap[string, planschema.FieldSpec] {
	return planschema.Fields(pluginFieldSpecs(rule.Fields)...)
}

func pluginFieldSpecs(fields []buildplugin.FieldSchema) []planschema.FieldSpec {
	return list.MapList[buildplugin.FieldSchema, planschema.FieldSpec](list.NewList(fields...), func(_ int, field buildplugin.FieldSchema) planschema.FieldSpec {
		return planschema.FieldSpec{
			Name:     field.Name,
			Type:     pluginFieldType(field),
			Required: field.Required,
		}
	}).Values()
}

func pluginFieldType(field buildplugin.FieldSchema) planschema.Type {
	switch field.Type {
	case buildplugin.FieldString:
		if isPathStringField(field.Name) {
			return planschema.TypePath
		}
		return planschema.TypeString
	case buildplugin.FieldList:
		if field.Name == "deps" {
			return taskDependencyListType()
		}
		if isPathListField(field.Name) {
			return planschema.ListType{Elem: planschema.TypePath}
		}
		return planschema.ListType{Elem: planschema.TypeString}
	case buildplugin.FieldObject:
		return planschema.MapType{Elem: planschema.TypeAny}
	case buildplugin.FieldBool:
		return planschema.TypeBool
	default:
		return planschema.TypeAny
	}
}

func taskDependencyListType() planschema.Type {
	return planschema.ListType{Elem: planschema.TypeAny}
}

func isPathStringField(name string) bool {
	switch name {
	case "main", "out", "path", "build_dir", "classes_dir", "jar":
		return true
	default:
		return false
	}
}

func isPathListField(name string) bool {
	switch name {
	case "inputs", "outputs", "srcs", "resources":
		return true
	default:
		return false
	}
}
