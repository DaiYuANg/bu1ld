package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"bu1ld/internal/dsl"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/golang"

	"github.com/DaiYuANg/arcgo/collectionx"
	"github.com/samber/oops"
)

type pluginEntry struct {
	Source    buildplugin.Source
	Namespace string
	ID        string
	Version   string
	Path      string
	Rules     []string
	Status    string
	Err       error
}

func (a *App) printPlugins(ctx context.Context, failOnIssue bool) error {
	entries, err := a.pluginEntries(ctx)
	if err != nil {
		return err
	}
	hasIssue := false
	for _, entry := range entries {
		if entry.Err != nil {
			hasIssue = true
			break
		}
	}
	if err := writePluginTable(a.output, entries); err != nil {
		return err
	}
	if failOnIssue && hasIssue {
		return oops.In("bu1ld.plugins").New("plugin doctor found issues")
	}
	return nil
}

func (a *App) printPluginsDoctor(ctx context.Context) error {
	options := a.loader.LoadOptions()
	localDir := buildplugin.LocalPluginDir(options)
	globalDir := buildplugin.GlobalPluginDir(options.GlobalDir)
	if _, err := fmt.Fprintf(a.output, "local plugins: %s (%s)\n", localDir, pathStatus(localDir)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.output, "global plugins: %s (%s)\n\n", globalDir, pathStatus(globalDir)); err != nil {
		return err
	}
	return a.printPlugins(ctx, true)
}

func (a *App) pluginEntries(ctx context.Context) ([]pluginEntry, error) {
	entries := collectionx.NewList[pluginEntry]()
	seen := collectionx.NewSet[string]()

	schemas, err := a.loader.PluginSchemas()
	if err != nil {
		return nil, oops.In("bu1ld.plugins").Wrapf(err, "read builtin plugin schemas")
	}
	for _, schema := range schemas {
		entry := pluginEntry{
			Source:    buildplugin.SourceBuiltin,
			Namespace: schema.Namespace,
			ID:        schema.ID,
			Rules:     ruleNames(schema.Rules),
			Status:    "ok",
		}
		entries.Add(entry)
		seen.Add(pluginEntryKey(entry))
	}

	file, err := a.loader.LoadFile()
	if err != nil {
		return nil, err
	}
	declarations, err := dsl.PluginDeclarations(file)
	if err != nil {
		return nil, oops.In("bu1ld.plugins").Wrapf(err, "read plugin declarations")
	}

	registry, err := buildplugin.NewRegistry(a.loader.LoadOptions(), golang.New())
	if err != nil {
		return nil, oops.In("bu1ld.plugins").Wrapf(err, "create plugin registry")
	}
	defer registry.Close()

	for _, item := range declarations {
		entry := inspectPlugin(ctx, registry, a.loader.LoadOptions(), item.Declaration)
		key := pluginEntryKey(entry)
		if seen.Contains(key) {
			continue
		}
		seen.Add(key)
		entries.Add(entry)
	}

	values := entries.Values()
	sort.SliceStable(values, func(i, j int) bool {
		left := values[i]
		right := values[j]
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Namespace < right.Namespace
	})
	return values, nil
}

func inspectPlugin(ctx context.Context, registry *buildplugin.Registry, options buildplugin.LoadOptions, declaration buildplugin.Declaration) pluginEntry {
	declaration = buildplugin.NormalizeDeclaration(declaration)
	entry := pluginEntry{
		Source:    declaration.Source,
		Namespace: declaration.Namespace,
		ID:        declaration.ID,
		Version:   declaration.Version,
		Path:      declaration.Path,
		Status:    "ok",
	}

	if declaration.Source == buildplugin.SourceLocal || declaration.Source == buildplugin.SourceGlobal {
		loader := buildplugin.NewProcessLoader(options)
		path, err := loader.ResolvePath(declaration)
		if err != nil {
			entry.Status = "error"
			entry.Err = err
			return entry
		}
		entry.Path = path
		if err := executableFileError(path); err != nil {
			entry.Status = "missing"
			entry.Err = err
			return entry
		}
	}

	if err := registry.Declare(ctx, declaration); err != nil {
		entry.Status = "error"
		entry.Err = err
		return entry
	}
	metadata, err := registry.Metadata(declaration.Namespace)
	if err != nil {
		entry.Status = "error"
		entry.Err = err
		return entry
	}
	if entry.ID == "" {
		entry.ID = metadata.ID
	}
	entry.Rules = ruleNames(metadata.Rules)
	return entry
}

func writePluginTable(output io.Writer, entries []pluginEntry) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "SOURCE\tNAMESPACE\tID\tVERSION\tPATH\tRULES\tSTATUS"); err != nil {
		return err
	}
	for _, entry := range entries {
		status := entry.Status
		if entry.Err != nil {
			status += ": " + entry.Err.Error()
		}
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			emptyDash(string(entry.Source)),
			emptyDash(entry.Namespace),
			emptyDash(entry.ID),
			emptyDash(entry.Version),
			emptyDash(entry.Path),
			emptyDash(strings.Join(entry.Rules, ",")),
			status,
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func ruleNames(rules []buildplugin.RuleSchema) []string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		names = append(names, rule.Name)
	}
	sort.Strings(names)
	return names
}

func pluginEntryKey(entry pluginEntry) string {
	return strings.Join([]string{
		string(entry.Source),
		entry.Namespace,
		entry.ID,
		entry.Version,
		entry.Path,
	}, "\x00")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func pathStatus(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	if !info.IsDir() {
		return "not a directory"
	}
	return "ok"
}

func executableFileError(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}
