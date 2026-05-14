package dsl

import (
	"cmp"
	"path/filepath"
	"slices"

	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

func (l *Loader) configCachePlugins(files []*File) ([]configCachePlugin, error) {
	loader := buildplugin.NewProcessLoader(l.LoadOptions())
	items := list.NewList[configCachePlugin]()
	for _, file := range files {
		declarations, err := PluginDeclarations(file)
		if err != nil {
			return nil, err
		}
		for _, item := range declarations {
			plugin, ok, pluginErr := l.configCachePlugin(loader, item)
			if pluginErr != nil {
				return nil, pluginErr
			}
			if ok {
				items.Add(plugin)
			}
		}
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCachePlugin) int {
		return cmp.Compare(configCachePluginKey(left), configCachePluginKey(right))
	})
	return values, nil
}

func (l *Loader) configCachePlugin(
	loader *buildplugin.ProcessLoader,
	item PluginDeclaration,
) (configCachePlugin, bool, error) {
	declaration := buildplugin.NormalizeDeclaration(item.Declaration)
	if declaration.Source != buildplugin.SourceLocal && declaration.Source != buildplugin.SourceGlobal {
		return configCachePlugin{}, false, nil
	}
	path, err := loader.ResolvePath(declaration)
	if err != nil {
		return configCachePlugin{}, false, oops.In("bu1ld.dsl").
			With("plugin", declaration.Namespace).
			With("source", declaration.Source).
			Wrapf(err, "resolve plugin path")
	}
	checksum, err := buildplugin.ChecksumFile(path)
	if err != nil {
		return configCachePlugin{}, false, oops.In("bu1ld.dsl").
			With("plugin", declaration.Namespace).
			With("path", path).
			Wrapf(err, "checksum plugin binary")
	}
	return configCachePlugin{
		Source:       declaration.Source,
		Namespace:    declaration.Namespace,
		ID:           declaration.ID,
		Version:      declaration.Version,
		DeclaredPath: declaration.Path,
		Path:         filepath.Clean(path),
		Checksum:     checksum,
	}, true, nil
}

func (l *Loader) configCachePluginValid(item configCachePlugin) bool {
	if item.Path == "" || item.Checksum == "" {
		return false
	}
	loader := buildplugin.NewProcessLoader(l.LoadOptions())
	declaration := buildplugin.Declaration{
		Source:    item.Source,
		Namespace: item.Namespace,
		ID:        item.ID,
		Version:   item.Version,
		Path:      item.DeclaredPath,
	}
	path, err := loader.ResolvePath(declaration)
	if err != nil || filepath.Clean(path) != filepath.Clean(item.Path) {
		return false
	}
	checksum, err := buildplugin.ChecksumFile(path)
	return err == nil && checksum == item.Checksum
}

func (l *Loader) configCachePluginsValid(plugins []configCachePlugin) bool {
	for _, plugin := range plugins {
		if !l.configCachePluginValid(plugin) {
			return false
		}
	}
	return true
}

func configCachePluginKey(item configCachePlugin) string {
	return string(item.Source) + "\x00" + item.Namespace + "\x00" + item.ID + "\x00" + item.Version + "\x00" + item.Path
}
