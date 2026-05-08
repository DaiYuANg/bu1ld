package plugin

import (
	"bu1ld/internal/build"
	"bu1ld/pkg/pluginapi"

	"github.com/arcgolabs/collectionx/list"
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
type ConfigurablePlugin = pluginapi.ConfigurablePlugin
type ExecutablePlugin = pluginapi.ExecutablePlugin
type PluginConfig = pluginapi.PluginConfig
type ExecuteRequest = pluginapi.ExecuteRequest
type ExecuteResult = pluginapi.ExecuteResult
type TaskSpec = pluginapi.TaskSpec
type TaskAction = pluginapi.TaskAction

const (
	ProtocolVersion     = pluginapi.ProtocolVersion
	CapabilityMetadata  = pluginapi.CapabilityMetadata
	CapabilityExpand    = pluginapi.CapabilityExpand
	CapabilityConfigure = pluginapi.CapabilityConfigure
	CapabilityExecute   = pluginapi.CapabilityExecute

	FieldString          = pluginapi.FieldString
	FieldList            = pluginapi.FieldList
	FieldObject          = pluginapi.FieldObject
	FieldBool            = pluginapi.FieldBool
	PluginExecActionKind = pluginapi.PluginExecActionKind
)

var (
	ValidateInvocation = pluginapi.ValidateInvocation
	NormalizeMetadata  = pluginapi.NormalizeMetadata
	SupportsCapability = pluginapi.SupportsCapability
)

func TaskSpecToBuild(spec TaskSpec) build.Task {
	task := build.NewTask(spec.Name)
	task.Deps = list.NewList[string](spec.Deps...)
	task.Inputs = list.NewList[string](spec.Inputs...)
	task.Outputs = list.NewList[string](spec.Outputs...)
	task.Command = list.NewList[string](spec.Command...)
	task.Action = build.Action{
		Kind:   spec.Action.Kind,
		Params: spec.Action.Params,
	}
	return task
}
