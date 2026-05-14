package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"
	"github.com/lyonbrown4d/bu1ld/internal/pluginregistry"

	"github.com/arcgolabs/collectionx/list"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func (a *App) printPluginRegistrySearch(ctx context.Context) error {
	index, err := a.loadPluginRegistry(ctx)
	if err != nil {
		return err
	}
	plugins := index.Search(a.request.PluginQuery)
	writer := tabwriter.NewWriter(a.output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tNAMESPACE\tVERSION\tDESCRIPTION"); err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "write plugin registry search")
	}
	for _, plugin := range plugins {
		version := plugin.LatestVersionOption().OrElse(pluginregistry.PluginVersion{Version: "-"}).Version
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\n",
			plugin.ID,
			plugin.Namespace,
			version,
			emptyDash(plugin.Description),
		); err != nil {
			return oops.In("bu1ld.plugin_registry").Wrapf(err, "write plugin registry search")
		}
	}
	if err := writer.Flush(); err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "flush plugin registry search")
	}
	return nil
}

func (a *App) printPluginRegistryInfo(ctx context.Context) error {
	index, err := a.loadPluginRegistry(ctx)
	if err != nil {
		return err
	}
	id, version, err := pluginregistry.ParseRef(a.request.PluginRef)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "parse plugin reference")
	}
	plugin, ok := index.FindOption(id).Get()
	if !ok {
		return oops.In("bu1ld.plugin_registry").
			With("plugin", id).
			Errorf("plugin %q was not found in the registry", id)
	}

	if err := writef(a.output, "id: %s\n", plugin.ID); err != nil {
		return err
	}
	if err := writef(a.output, "namespace: %s\n", plugin.Namespace); err != nil {
		return err
	}
	if plugin.Owner != "" {
		if err := writef(a.output, "owner: %s\n", plugin.Owner); err != nil {
			return err
		}
	}
	if plugin.Description != "" {
		if err := writef(a.output, "description: %s\n", plugin.Description); err != nil {
			return err
		}
	}
	if plugin.Homepage != "" {
		if err := writef(a.output, "homepage: %s\n", plugin.Homepage); err != nil {
			return err
		}
	}
	if len(plugin.Tags) > 0 {
		if err := writef(a.output, "tags: %s\n", strings.Join(plugin.Tags, ", ")); err != nil {
			return err
		}
	}

	versions := plugin.Versions
	if version != "" {
		item, ok := plugin.VersionOption(version).Get()
		if !ok {
			return oops.In("bu1ld.plugin_registry").
				With("plugin", id).
				With("version", version).
				Errorf("plugin %q version %q was not found in the registry", id, version)
		}
		versions = list.NewList(item).Values()
	}
	return writePluginRegistryVersions(a.output, versions)
}

func (a *App) installRegistryPlugin(ctx context.Context, force bool, verb string) error {
	index, err := a.loadPluginRegistry(ctx)
	if err != nil {
		return err
	}
	options := a.loader.LoadOptions()
	root := buildplugin.LocalPluginDir(options)
	scope := buildplugin.SourceLocal
	if a.request.PluginGlobal {
		root = buildplugin.GlobalPluginDir(options.GlobalDir)
		scope = buildplugin.SourceGlobal
	}
	result, err := pluginregistry.Install(ctx, index, pluginregistry.InstallOptions{
		Ref:    a.request.PluginRef,
		Root:   root,
		Force:  force,
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	})
	if err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("plugin", a.request.PluginRef).
			With("root", root).
			Wrapf(err, "install registry plugin")
	}
	return writef(a.output, "%s %s@%s to %s (%s)\n", verb, result.ID, result.Version, result.Path, scope)
}

func (a *App) validatePluginRegistry(ctx context.Context) error {
	index, err := a.loadPluginRegistrySource(ctx, a.request.RegistrySource)
	if err != nil {
		return err
	}
	report, err := pluginregistry.ValidateIndex(index)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "validate plugin registry")
	}
	if err := writef(
		a.output,
		"registry ok: %d plugins, %d versions, %d approved, %d rejected, %d assets\n",
		report.Plugins,
		report.Versions,
		report.ApprovedVersions,
		report.RejectedVersions,
		report.Assets,
	); err != nil {
		return err
	}
	for _, warning := range report.Warnings {
		if err := writef(a.output, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) printPluginPublishSnippet(context.Context) error {
	manifestPath := strings.TrimSpace(a.request.PluginManifestPath)
	if manifestPath == "" {
		return oops.In("bu1ld.plugin_registry").New("plugin manifest path is required")
	}
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(a.cfg.WorkDir, manifestPath)
	}
	manifest, err := buildplugin.ReadManifest(manifestPath)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "read plugin manifest")
	}
	if strings.TrimSpace(manifest.Namespace) == "" {
		return oops.In("bu1ld.plugin_registry").
			With("file", manifestPath).
			New("plugin manifest namespace is required to publish registry metadata")
	}
	status := strings.TrimSpace(a.request.PluginStatus)
	if status == "" {
		status = "approved"
	}
	version := pluginregistry.PluginVersion{
		Version:  manifest.Version,
		Bu1ld:    strings.TrimSpace(a.request.PluginBu1ld),
		Status:   status,
		Manifest: buildplugin.ManifestFileName,
	}
	if assetURL := strings.TrimSpace(a.request.PluginAssetURL); assetURL != "" {
		version.Assets = []pluginregistry.PluginAsset{
			{
				OS:     strings.TrimSpace(a.request.PluginOS),
				Arch:   strings.TrimSpace(a.request.PluginArch),
				URL:    assetURL,
				SHA256: strings.TrimSpace(a.request.PluginSHA256),
				Format: strings.TrimSpace(a.request.PluginFormat),
			},
		}
	}
	entry := pluginregistry.Plugin{
		ID:        manifest.ID,
		Namespace: manifest.Namespace,
		Versions:  []pluginregistry.PluginVersion{version},
	}
	data, err := toml.Marshal(entry)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "encode plugin registry metadata")
	}
	_, err = a.output.Write(data)
	return err
}

func (a *App) loadPluginRegistry(ctx context.Context) (*pluginregistry.Index, error) {
	return a.loadPluginRegistrySource(ctx, "")
}

func (a *App) loadPluginRegistrySource(ctx context.Context, override string) (*pluginregistry.Index, error) {
	source := strings.TrimSpace(a.cfg.PluginRegistrySource)
	if strings.TrimSpace(override) != "" {
		source = strings.TrimSpace(override)
	}
	index, err := pluginregistry.Load(ctx, pluginregistry.LoadOptions{
		Source:   source,
		BaseDir:  a.cfg.WorkDir,
		CacheDir: filepath.Join(a.cfg.StateDir(), "registries"),
	})
	if err != nil {
		return nil, oops.In("bu1ld.plugin_registry").
			With("source", emptyDash(source)).
			Wrapf(err, "load plugin registry")
	}
	return index, nil
}

func writePluginRegistryVersions(output io.Writer, versions []pluginregistry.PluginVersion) error {
	if len(versions) == 0 {
		return writeLine(output, "versions: -")
	}
	if err := writeLine(output, "versions:"); err != nil {
		return err
	}
	for _, version := range versions {
		line := "  " + version.Version
		if version.Bu1ld != "" {
			line += " (bu1ld " + version.Bu1ld + ")"
		}
		if version.Status != "" {
			line += " [" + version.Status + "]"
		}
		if err := writeLine(output, line); err != nil {
			return err
		}
		for _, asset := range version.Assets {
			target := lo.Ternary(asset.OS != "" || asset.Arch != "", asset.OS+"/"+asset.Arch, "any")
			detail := "    asset " + target
			if asset.Format != "" {
				detail += " " + asset.Format
			}
			if asset.SHA256 != "" {
				detail += " sha256"
			}
			detail += " " + asset.URL
			if err := writeLine(output, detail); err != nil {
				return err
			}
		}
	}
	return nil
}
