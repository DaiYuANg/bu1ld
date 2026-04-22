package pluginapi

import (
	"context"
	"fmt"
	"sort"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type Metadata struct {
	ID        string
	Namespace string
	Rules     []RuleSchema
}

type FieldType string

const (
	FieldString FieldType = "string"
	FieldList   FieldType = "list"
	FieldObject FieldType = "object"
)

type FieldSchema struct {
	Name     string
	Type     FieldType
	Required bool
}

type RuleSchema struct {
	Name   string
	Fields []FieldSchema
}

type Invocation struct {
	Namespace string
	Rule      string
	Target    string
	Fields    map[string]any
}

type Plugin interface {
	Metadata() (Metadata, error)
	Expand(context.Context, Invocation) ([]TaskSpec, error)
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

func (i Invocation) OptionalString(name string, fallback string) (string, error) {
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
	default:
		return nil, fmt.Errorf("%s.%s field %q must be list", i.Namespace, i.Rule, name)
	}
}

func ValidateInvocation(schema RuleSchema, invocation Invocation) error {
	known := collectionx.NewSet[string]()
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

	names := make([]string, 0, len(invocation.Fields))
	for name := range invocation.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
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
		}
	case FieldObject:
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	default:
		return fmt.Errorf("unknown schema type %q for %s.%s field %q", field.Type, invocation.Namespace, invocation.Rule, field.Name)
	}
	return fmt.Errorf("%s.%s field %q must be %s", invocation.Namespace, invocation.Rule, field.Name, field.Type)
}

type TaskSpec struct {
	Name    string
	Deps    []string
	Inputs  []string
	Outputs []string
	Command []string
}
