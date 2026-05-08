package plugin

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"bu1ld/internal/build"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

type LoadOptions struct {
	ProjectDir       string
	LocalDir         string
	GlobalDir        string
	Env              []string
	HandshakeTimeout time.Duration
	Stderr           io.Writer
}

type Registry struct {
	builtins      *mapping.Map[string, Plugin]
	localPlugins  *mapping.Map[string, Plugin]
	globalPlugins *mapping.Map[string, Plugin]
	active        *mapping.Map[string, Plugin]
	declarations  *mapping.Map[string, Declaration]
	loader        *ProcessLoader
}

func NewRegistry(options LoadOptions, builtins ...Plugin) (*Registry, error) {
	registry := &Registry{
		builtins:      mapping.NewMap[string, Plugin](),
		localPlugins:  mapping.NewMap[string, Plugin](),
		globalPlugins: mapping.NewMap[string, Plugin](),
		active:        mapping.NewMap[string, Plugin](),
		declarations:  mapping.NewMap[string, Declaration](),
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
		builtins:      mapping.NewMap[string, Plugin](),
		localPlugins:  mapping.NewMap[string, Plugin](),
		globalPlugins: mapping.NewMap[string, Plugin](),
		active:        mapping.NewMap[string, Plugin](),
		declarations:  mapping.NewMap[string, Declaration](),
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

	metadata, err := pluginMetadata(item)
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
	tasks := list.NewListWithCapacity[build.Task](len(specs))
	for i := range specs {
		spec := specs[i]
		spec = r.decoratePluginAction(invocation.Namespace, spec)
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

func (r *Registry) Configure(ctx context.Context, configs map[string]PluginConfig) ([]build.Task, error) {
	tasks := list.NewList[build.Task]()
	var firstErr error
	r.active.Range(func(namespace string, item Plugin) bool {
		metadata, err := pluginMetadata(item)
		if err != nil {
			firstErr = oops.In("bu1ld.plugins").
				With("namespace", namespace).
				Wrapf(err, "read plugin metadata")
			return false
		}
		config, hasConfig := configs[namespace]
		if !metadata.AutoConfigure && !hasConfig {
			return true
		}
		if config.Namespace == "" {
			config.Namespace = namespace
		}
		if config.Fields == nil {
			config.Fields = map[string]any{}
		}
		if err := ValidateInvocation(RuleSchema{Name: "config", Fields: metadata.ConfigFields}, Invocation{
			Namespace: namespace,
			Rule:      "config",
			Fields:    config.Fields,
		}); err != nil {
			firstErr = err
			return false
		}
		configurable, ok := item.(ConfigurablePlugin)
		if !ok {
			firstErr = oops.In("bu1ld.plugins").
				With("namespace", namespace).
				New("plugin does not support configure")
			return false
		}
		specs, err := configurable.Configure(ctx, config)
		if err != nil {
			firstErr = oops.In("bu1ld.plugins").
				With("namespace", namespace).
				Wrapf(err, "configure plugin")
			return false
		}
		for i := range specs {
			spec := r.decoratePluginAction(namespace, specs[i])
			tasks.Add(TaskSpecToBuild(spec))
		}
		return true
	})
	if firstErr != nil {
		return nil, firstErr
	}
	for _, task := range tasks.Values() {
		if err := task.Validate(); err != nil {
			return nil, oops.In("bu1ld.plugins").
				With("task", task.Name).
				Wrapf(err, "validate configured plugin task")
		}
	}
	return tasks.Values(), nil
}

func (r *Registry) Schemas() ([]Metadata, error) {
	metadata := list.NewList[Metadata]()
	var firstErr error
	r.active.Range(func(_ string, item Plugin) bool {
		itemMetadata, metadataErr := pluginMetadata(item)
		if metadataErr != nil {
			firstErr = oops.In("bu1ld.plugins").Wrapf(metadataErr, "read plugin metadata")
			return false
		}
		metadata.Add(itemMetadata)
		return true
	})
	return metadata.Values(), firstErr
}

func (r *Registry) ConfigNamespaces() (map[string]Metadata, error) {
	items := mapping.NewMap[string, Metadata]()
	schemas, err := r.Schemas()
	if err != nil {
		return nil, err
	}
	for _, metadata := range schemas {
		if metadata.Namespace == "" {
			continue
		}
		if metadata.AutoConfigure || len(metadata.ConfigFields) > 0 {
			items.Set(metadata.Namespace, metadata)
		}
	}
	return items.All(), nil
}

func (r *Registry) Metadata(namespace string) (Metadata, error) {
	item, ok := r.active.Get(namespace)
	if !ok {
		return Metadata{}, oops.In("bu1ld.plugins").
			With("namespace", namespace).
			Errorf("plugin namespace %q is not registered", namespace)
	}
	metadata, err := pluginMetadata(item)
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

func pluginMetadata(item Plugin) (Metadata, error) {
	metadata, err := item.Metadata()
	if err != nil {
		return Metadata{}, err
	}
	return NormalizeMetadata(item, metadata), nil
}

func (r *Registry) addBuiltin(item Plugin) error {
	metadata, err := pluginMetadata(item)
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

func (r *Registry) decoratePluginAction(namespace string, spec TaskSpec) TaskSpec {
	if spec.Action.Kind != PluginExecActionKind {
		return spec
	}
	if spec.Action.Params == nil {
		spec.Action.Params = map[string]any{}
	}
	if _, ok := spec.Action.Params["namespace"]; !ok {
		spec.Action.Params["namespace"] = namespace
	}
	if declaration, ok := r.declarations.Get(namespace); ok {
		if _, exists := spec.Action.Params["source"]; !exists && declaration.Source != "" {
			spec.Action.Params["source"] = string(declaration.Source)
		}
		if _, exists := spec.Action.Params["id"]; !exists && declaration.ID != "" {
			spec.Action.Params["id"] = declaration.ID
		}
		if _, exists := spec.Action.Params["version"]; !exists && declaration.Version != "" {
			spec.Action.Params["version"] = declaration.Version
		}
		if _, exists := spec.Action.Params["path"]; !exists && declaration.Path != "" {
			spec.Action.Params["path"] = declaration.Path
		}
	}
	return spec
}

func (r *Registry) resolveBuiltin(declaration Declaration) (Plugin, error) {
	candidates := list.NewList[string](declaration.ID)
	if declaration.ID == "" {
		candidates.Add("builtin." + declaration.Namespace)
	}
	for _, candidate := range candidates.Values() {
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
	return findRuleOption(metadata, name).Get()
}

func findRuleOption(metadata Metadata, name string) mo.Option[RuleSchema] {
	return list.NewList(metadata.Rules...).FirstWhere(func(_ int, rule RuleSchema) bool {
		if rule.Name == name {
			return true
		}
		return false
	})
}
