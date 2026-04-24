package plugin

import (
	"bu1ld/internal/build"
	"bu1ld/pkg/pluginapi"

	"github.com/arcgolabs/collectionx"
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceLocal   Source = "local"
	SourceGlobal  Source = "global"
)

type Declaration struct {
	Namespace string
	ID        string
	Source    Source
	Version   string
	Path      string
}

type Metadata = pluginapi.Metadata
type FieldType = pluginapi.FieldType
type FieldSchema = pluginapi.FieldSchema
type RuleSchema = pluginapi.RuleSchema
type Invocation = pluginapi.Invocation
type Plugin = pluginapi.Plugin
type TaskSpec = pluginapi.TaskSpec

const (
	FieldString = pluginapi.FieldString
	FieldList   = pluginapi.FieldList
	FieldObject = pluginapi.FieldObject
)

var ValidateInvocation = pluginapi.ValidateInvocation

func TaskSpecToBuild(spec TaskSpec) build.Task {
	task := build.NewTask(spec.Name)
	task.Deps = collectionx.NewList[string](spec.Deps...)
	task.Inputs = collectionx.NewList[string](spec.Inputs...)
	task.Outputs = collectionx.NewList[string](spec.Outputs...)
	task.Command = collectionx.NewList[string](spec.Command...)
	return task
}
