package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/pluginregistry"

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
		version := "-"
		if latest, ok := plugin.LatestVersion(); ok {
			version = latest.Version
		}
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
	plugin, ok := index.Find(id)
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
		item, ok := plugin.Version(version)
		if !ok {
			return oops.In("bu1ld.plugin_registry").
				With("plugin", id).
				With("version", version).
				Errorf("plugin %q version %q was not found in the registry", id, version)
		}
		versions = []pluginregistry.PluginVersion{item}
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

func (a *App) loadPluginRegistry(ctx context.Context) (*pluginregistry.Index, error) {
	source := strings.TrimSpace(os.Getenv("BU1LD_PLUGIN_REGISTRY"))
	index, err := pluginregistry.Load(ctx, pluginregistry.LoadOptions{Source: source})
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
		if err := writeLine(output, line); err != nil {
			return err
		}
		for _, asset := range version.Assets {
			target := "any"
			if asset.OS != "" || asset.Arch != "" {
				target = asset.OS + "/" + asset.Arch
			}
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
