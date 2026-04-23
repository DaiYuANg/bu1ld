package plugin

import (
	"context"
	"fmt"
	"strings"

	"bu1ld/internal/build"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type LoadOptions struct {
	ProjectDir string
	LocalDir   string
	GlobalDir  string
}

type Registry struct {
	builtins      collectionx.Map[string, Plugin]
	localPlugins  collectionx.Map[string, Plugin]
	globalPlugins collectionx.Map[string, Plugin]
	active        collectionx.Map[string, Plugin]
	declarations  collectionx.Map[string, Declaration]
	loader        *ProcessLoader
}

func NewRegistry(options LoadOptions, builtins ...Plugin) (*Registry, error) {
	registry := &Registry{
		builtins:      collectionx.NewMap[string, Plugin](),
		localPlugins:  collectionx.NewMap[string, Plugin](),
		globalPlugins: collectionx.NewMap[string, Plugin](),
		active:        collectionx.NewMap[string, Plugin](),
		declarations:  collectionx.NewMap[string, Declaration](),
		loader:        NewProcessLoader(options),
	}
	for _, item := range builtins {
		if err := registry.addBuiltin(item); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) CloneWithOptions(options LoadOptions) *Registry {
	clone := &Registry{
		builtins:      collectionx.NewMap[string, Plugin](),
		localPlugins:  collectionx.NewMap[string, Plugin](),
		globalPlugins: collectionx.NewMap[string, Plugin](),
		active:        collectionx.NewMap[string, Plugin](),
		declarations:  collectionx.NewMap[string, Declaration](),
		loader:        NewProcessLoader(options),
	}
	r.builtins.Range(func(id string, item Plugin) bool {
		clone.builtins.Set(id, item)
		return true
	})
	r.localPlugins.Range(func(namespace string, item Plugin) bool {
		clone.localPlugins.Set(namespace, item)
		return true
	})
	r.globalPlugins.Range(func(namespace string, item Plugin) bool {
		clone.globalPlugins.Set(namespace, item)
		return true
	})
	r.active.Range(func(namespace string, item Plugin) bool {
		clone.active.Set(namespace, item)
		return true
	})
	return clone
}

func (r *Registry) Declare(ctx context.Context, declaration Declaration) error {
	if declaration.Namespace == "" {
		return fmt.Errorf("plugin namespace is required")
	}
	declaration = normalizeDeclaration(declaration)

	var item Plugin
	var err error
	switch declaration.Source {
	case SourceBuiltin:
		item, err = r.resolveBuiltin(declaration)
	case SourceLocal, SourceGlobal:
		item, err = r.loader.Load(ctx, declaration)
	default:
		return fmt.Errorf("unknown plugin source %q", declaration.Source)
	}
	if err != nil {
		return err
	}
	r.setScoped(declaration.Source, declaration.Namespace, item)
	r.active.Set(declaration.Namespace, item)
	r.declarations.Set(declaration.Namespace, declaration)
	return nil
}

func (r *Registry) Expand(ctx context.Context, invocation Invocation) ([]build.Task, error) {
	item, ok := r.active.Get(invocation.Namespace)
	if !ok {
		return nil, fmt.Errorf("plugin namespace %q is not registered", invocation.Namespace)
	}

	metadata, err := item.Metadata()
	if err != nil {
		return nil, fmt.Errorf("read plugin metadata for %q: %w", invocation.Namespace, err)
	}
	schema, ok := findRule(metadata, invocation.Rule)
	if !ok {
		return nil, fmt.Errorf("plugin %q does not provide rule %q", invocation.Namespace, invocation.Rule)
	}
	if err := ValidateInvocation(schema, invocation); err != nil {
		return nil, err
	}
	specs, err := item.Expand(ctx, invocation)
	if err != nil {
		return nil, err
	}
	tasks := make([]build.Task, 0, len(specs))
	for _, spec := range specs {
		tasks = append(tasks, TaskSpecToBuild(spec))
	}
	for _, task := range tasks {
		if err := task.Validate(); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (r *Registry) Schemas() ([]Metadata, error) {
	metadata := []Metadata{}
	var firstErr error
	r.active.Range(func(_ string, item Plugin) bool {
		itemMetadata, err := item.Metadata()
		if err != nil {
			firstErr = err
			return false
		}
		metadata = append(metadata, itemMetadata)
		return true
	})
	return metadata, firstErr
}

func (r *Registry) Metadata(namespace string) (Metadata, error) {
	item, ok := r.active.Get(namespace)
	if !ok {
		return Metadata{}, fmt.Errorf("plugin namespace %q is not registered", namespace)
	}
	return item.Metadata()
}

func (r *Registry) Close() {
	if r.loader != nil {
		r.loader.Close()
	}
}

func (r *Registry) addBuiltin(item Plugin) error {
	metadata, err := item.Metadata()
	if err != nil {
		return err
	}
	if metadata.ID == "" {
		return fmt.Errorf("builtin plugin id is required")
	}
	r.builtins.Set(metadata.ID, item)
	if metadata.Namespace != "" {
		r.builtins.Set("builtin."+metadata.Namespace, item)
		r.active.Set(metadata.Namespace, item)
	}
	return nil
}

func (r *Registry) setScoped(source Source, namespace string, item Plugin) {
	switch source {
	case SourceBuiltin:
		r.builtins.Set(namespace, item)
	case SourceLocal:
		r.localPlugins.Set(namespace, item)
	case SourceGlobal:
		r.globalPlugins.Set(namespace, item)
	}
}

func (r *Registry) resolveBuiltin(declaration Declaration) (Plugin, error) {
	candidates := []string{declaration.ID}
	if declaration.ID == "" {
		candidates = append(candidates, "builtin."+declaration.Namespace)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if item, ok := r.builtins.Get(candidate); ok {
			return item, nil
		}
	}
	return nil, fmt.Errorf("builtin plugin %q is not available", declaration.ID)
}

func normalizeDeclaration(declaration Declaration) Declaration {
	if declaration.Source == "" {
		switch {
		case declaration.ID == "":
			declaration.Source = SourceBuiltin
		case strings.HasPrefix(declaration.ID, "builtin."):
			declaration.Source = SourceBuiltin
		case declaration.Path != "":
			declaration.Source = SourceLocal
		default:
			declaration.Source = SourceGlobal
		}
	}
	if declaration.ID == "" && declaration.Source == SourceBuiltin {
		declaration.ID = "builtin." + declaration.Namespace
	}
	return declaration
}

func NormalizeDeclaration(declaration Declaration) Declaration {
	return normalizeDeclaration(declaration)
}

func findRule(metadata Metadata, name string) (RuleSchema, bool) {
	for _, rule := range metadata.Rules {
		if rule.Name == name {
			return rule, true
		}
	}
	return RuleSchema{}, false
}
