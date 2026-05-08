package pluginapi

import (
	"context"
	"fmt"
	"slices"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"
)

type Metadata struct {
	ID            string        `json:"id"`
	Namespace     string        `json:"namespace"`
	Rules         []RuleSchema  `json:"rules"`
	ConfigFields  []FieldSchema `json:"config_fields,omitempty"`
	AutoConfigure bool          `json:"auto_configure,omitempty"`
}

type FieldType string

const (
	FieldString FieldType = "string"
	FieldList   FieldType = "list"
	FieldObject FieldType = "object"
	FieldBool   FieldType = "bool"
)

type FieldSchema struct {
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
}

type RuleSchema struct {
	Name   string        `json:"name"`
	Fields []FieldSchema `json:"fields"`
}

type Invocation struct {
	Namespace string         `json:"namespace"`
	Rule      string         `json:"rule"`
	Target    string         `json:"target"`
	Fields    map[string]any `json:"fields"`
}

type Plugin interface {
	Metadata() (Metadata, error)
	Expand(context.Context, Invocation) ([]TaskSpec, error)
}

type ConfigurablePlugin interface {
	Configure(context.Context, PluginConfig) ([]TaskSpec, error)
}

type ExecutablePlugin interface {
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
}

type PluginConfig struct {
	Namespace string         `json:"namespace"`
	Fields    map[string]any `json:"fields"`
}

type ExecuteRequest struct {
	Namespace string         `json:"namespace"`
	Action    string         `json:"action"`
	WorkDir   string         `json:"work_dir"`
	Params    map[string]any `json:"params"`
}

type ExecuteResult struct {
	Output string `json:"output,omitempty"`
}

func (i Invocation) RequiredString(name string) (string, error) {
	value, ok := i.Fields[name]
	if !ok {
		return "", fmt.Errorf("%s.%s requires field %q", i.Namespace, i.Rule, name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s field %q must be string", i.Namespace, i.Rule, name)
	}
	return text, nil
}

func (i Invocation) OptionalString(name, fallback string) (string, error) {
	value, ok := i.Fields[name]
	if !ok {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s field %q must be string", i.Namespace, i.Rule, name)
	}
	return text, nil
}

func (i Invocation) OptionalList(name string, fallback []string) ([]string, error) {
	value, ok := i.Fields[name]
	if !ok {
		return fallback, nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return typed, nil
	case []any:
		values := list.NewListWithCapacity[string](len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s.%s field %q must be list", i.Namespace, i.Rule, name)
			}
			values.Add(text)
		}
		return values.Values(), nil
	default:
		return nil, fmt.Errorf("%s.%s field %q must be list", i.Namespace, i.Rule, name)
	}
}

func (i Invocation) OptionalBool(name string, fallback bool) (bool, error) {
	value, ok := i.Fields[name]
	if !ok {
		return fallback, nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s.%s field %q must be bool", i.Namespace, i.Rule, name)
	}
	return enabled, nil
}

func (i Invocation) OptionalObject(name string, fallback map[string]any) (map[string]any, error) {
	value, ok := i.Fields[name]
	if !ok {
		return fallback, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s field %q must be object", i.Namespace, i.Rule, name)
	}
	return object, nil
}

func ValidateInvocation(schema RuleSchema, invocation Invocation) error {
	known := set.NewSet[string]()
	for _, field := range schema.Fields {
		known.Add(field.Name)
		value, ok := invocation.Fields[field.Name]
		if !ok {
			if field.Required {
				return fmt.Errorf("%s.%s requires field %q", invocation.Namespace, invocation.Rule, field.Name)
			}
			continue
		}
		if err := validateFieldType(invocation, field, value); err != nil {
			return err
		}
	}

	names := mapping.NewMapFrom(invocation.Fields).Keys()
	slices.Sort(names)
	for _, name := range names {
		if !known.Contains(name) {
			return fmt.Errorf("unknown %s.%s field %q", invocation.Namespace, invocation.Rule, name)
		}
	}
	return nil
}

func validateFieldType(invocation Invocation, field FieldSchema, value any) error {
	switch field.Type {
	case FieldString:
		if _, ok := value.(string); ok {
			return nil
		}
	case FieldList:
		switch value.(type) {
		case string, []string:
			return nil
		case []any:
			if _, err := invocation.OptionalList(field.Name, nil); err == nil {
				return nil
			}
		}
	case FieldObject:
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	case FieldBool:
		if _, ok := value.(bool); ok {
			return nil
		}
	default:
		return fmt.Errorf("unknown schema type %q for %s.%s field %q", field.Type, invocation.Namespace, invocation.Rule, field.Name)
	}
	return fmt.Errorf("%s.%s field %q must be %s", invocation.Namespace, invocation.Rule, field.Name, field.Type)
}

type TaskSpec struct {
	Name    string     `json:"name"`
	Deps    []string   `json:"deps,omitempty"`
	Inputs  []string   `json:"inputs,omitempty"`
	Outputs []string   `json:"outputs,omitempty"`
	Command []string   `json:"command,omitempty"`
	Action  TaskAction `json:"action,omitempty"`
}

type TaskAction struct {
	Kind   string         `json:"kind,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}
