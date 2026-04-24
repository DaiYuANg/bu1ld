package plugin

import (
	"context"
	"errors"
	"strings"

	"bu1ld/internal/build"

	"github.com/arcgolabs/collectionx"
	"github.com/samber/oops"
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
		return errors.New("plugin namespace is required")
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
		return oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			Errorf("unknown plugin source %q", declaration.Source)
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
		return nil, oops.In("bu1ld.plugins").
			With("namespace", invocation.Namespace).
			Errorf("plugin namespace %q is not registered", invocation.Namespace)
	}

	metadata, err := item.Metadata()
	if err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("namespace", invocation.Namespace).
			Wrapf(err, "read plugin metadata")
	}
	schema, ok := findRule(metadata, invocation.Rule)
	if !ok {
		return nil, oops.In("bu1ld.plugins").
			With("namespace", invocation.Namespace).
			With("rule", invocation.Rule).
			Errorf("plugin %q does not provide rule %q", invocation.Namespace, invocation.Rule)
	}
	if validationErr := ValidateInvocation(schema, invocation); validationErr != nil {
		return nil, validationErr
	}
	specs, err := item.Expand(ctx, invocation)
	if err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("namespace", invocation.Namespace).
			With("rule", invocation.Rule).
			Wrapf(err, "expand plugin rule")
	}
	tasks := collectionx.NewListWithCapacity[build.Task](len(specs))
	for _, spec := range specs {
		tasks.Add(TaskSpecToBuild(spec))
	}
	for _, task := range tasks.Values() {
		if err := task.Validate(); err != nil {
			return nil, oops.In("bu1ld.plugins").
				With("namespace", invocation.Namespace).
				With("rule", invocation.Rule).
				With("task", task.Name).
				Wrapf(err, "validate expanded plugin task")
		}
	}
	return tasks.Values(), nil
}

func (r *Registry) Schemas() ([]Metadata, error) {
	metadata := collectionx.NewList[Metadata]()
	var firstErr error
	r.active.Range(func(_ string, item Plugin) bool {
		itemMetadata, metadataErr := item.Metadata()
		if metadataErr != nil {
			firstErr = oops.In("bu1ld.plugins").Wrapf(metadataErr, "read plugin metadata")
			return false
		}
		metadata.Add(itemMetadata)
		return true
	})
	return metadata.Values(), firstErr
}

func (r *Registry) Metadata(namespace string) (Metadata, error) {
	item, ok := r.active.Get(namespace)
	if !ok {
		return Metadata{}, oops.In("bu1ld.plugins").
			With("namespace", namespace).
			Errorf("plugin namespace %q is not registered", namespace)
	}
	metadata, err := item.Metadata()
	if err != nil {
		return Metadata{}, oops.In("bu1ld.plugins").
			With("namespace", namespace).
			Wrapf(err, "read plugin metadata")
	}
	return metadata, nil
}

func (r *Registry) Close() {
	if r.loader != nil {
		r.loader.Close()
	}
}

func (r *Registry) addBuiltin(item Plugin) error {
	metadata, err := item.Metadata()
	if err != nil {
		return oops.In("bu1ld.plugins").Wrapf(err, "read builtin plugin metadata")
	}
	if metadata.ID == "" {
		return errors.New("builtin plugin id is required")
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
	return nil, oops.In("bu1ld.plugins").
		With("namespace", declaration.Namespace).
		With("plugin", declaration.ID).
		New("builtin plugin is not available")
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
