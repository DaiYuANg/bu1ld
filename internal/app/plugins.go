package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"bu1ld/internal/dsl"
	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type pluginEntry struct {
	Source    buildplugin.Source
	Namespace string
	ID        string
	Version   string
	Path      string
	Checksum  string
	Rules     []string
	Status    string
	Err       error
	Declared  bool
	Installed bool
}

func (a *App) printPlugins(ctx context.Context, failOnIssue bool) error {
	entries, err := a.pluginEntries(ctx)
	if err != nil {
		return err
	}
	return a.writePluginReport(entries, failOnIssue)
}

func (a *App) printPluginsDoctor(ctx context.Context) error {
	options := a.loader.LoadOptions()
	localDir := buildplugin.LocalPluginDir(options)
	globalDir := buildplugin.GlobalPluginDir(options.GlobalDir)
	lockPath := a.loader.LockFilePath()
	if err := writef(a.output, "local plugins: %s (%s)\n", localDir, pathStatus(localDir)); err != nil {
		return err
	}
	if err := writef(a.output, "global plugins: %s (%s)\n\n", globalDir, pathStatus(globalDir)); err != nil {
		return err
	}

	entries, err := a.pluginEntries(ctx)
	if err != nil {
		return err
	}
	lock, found, err := buildplugin.ReadLockFile(lockPath)
	if err != nil {
		return oops.In("bu1ld.plugins").
			With("file", lockPath).
			Wrapf(err, "read plugin lockfile")
	}
	if found {
		entries = applyLockDiagnostics(entries, lock)
	} else if err := writef(a.output, "lockfile: %s (missing)\n\n", lockPath); err != nil {
		return err
	}
	return a.writePluginReport(entries, true)
}

func (a *App) pluginEntries(ctx context.Context) ([]pluginEntry, error) {
	entries := list.NewList[pluginEntry]()
	seen := set.NewSet[string]()

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

	declarations, err := dsl.RawPluginDeclarationsFromPath(a.loader.FS(), a.loader.BuildFilePath())
	if err != nil {
		return nil, oops.In("bu1ld.plugins").Wrapf(err, "read plugin declarations")
	}

	for _, item := range declarations {
		entry := inspectPlugin(ctx, a.registry, a.loader.LoadOptions(), item.Declaration)
		entry.Declared = true
		key := pluginEntryKey(entry)
		if seen.Contains(key) {
			continue
		}
		seen.Add(key)
		entries.Add(entry)
	}

	installed := a.installedPluginEntries()
	for i := range installed {
		entry := installed[i]
		key := pluginEntryKey(entry)
		if seen.Contains(key) {
			continue
		}
		seen.Add(key)
		entries.Add(entry)
	}

	values := entries.Values()
	return sortPluginEntries(values), nil
}

func (a *App) declaredPluginEntries(ctx context.Context) ([]pluginEntry, error) {
	declarations, err := dsl.RawPluginDeclarationsFromPath(a.loader.FS(), a.loader.BuildFilePath())
	if err != nil {
		return nil, oops.In("bu1ld.plugins").Wrapf(err, "read plugin declarations")
	}
	entries := list.NewList[pluginEntry]()
	for _, item := range declarations {
		entry := inspectPlugin(ctx, a.registry, a.loader.LoadOptions(), item.Declaration)
		entry.Declared = true
		entries.Add(entry)
	}
	return entries.Values(), nil
}

func (a *App) installedPluginEntries() []pluginEntry {
	options := a.loader.LoadOptions()
	entries := list.NewList[pluginEntry]()
	for _, scope := range []struct {
		source buildplugin.Source
		root   string
	}{
		{source: buildplugin.SourceLocal, root: buildplugin.LocalPluginDir(options)},
		{source: buildplugin.SourceGlobal, root: buildplugin.GlobalPluginDir(options.GlobalDir)},
	} {
		manifests, err := buildplugin.DiscoverManifests(scope.root)
		if err != nil {
			entries.Add(pluginEntry{
				Source: scope.source,
				Path:   scope.root,
				Status: "error",
				Err:    err,
			})
			continue
		}
		for i := range manifests {
			manifest := manifests[i]
			entries.Add(pluginEntryFromManifest(scope.source, manifest))
		}
	}
	return entries.Values()
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

func pluginEntryFromManifest(source buildplugin.Source, file buildplugin.ManifestFile) pluginEntry {
	entry := pluginEntry{
		Source:    source,
		Path:      file.Path,
		Status:    "installed",
		Installed: true,
	}
	if file.Err != nil {
		entry.Status = "error"
		entry.Err = file.Err
		return entry
	}
	entry.Namespace = file.Manifest.Namespace
	entry.ID = file.Manifest.ID
	entry.Version = file.Manifest.Version
	entry.Path = file.Manifest.BinaryPath(file.Path)
	entry.Rules = manifestRuleNames(file.Manifest.Rules)
	if err := executableFileError(entry.Path); err != nil {
		entry.Status = "missing"
		entry.Err = err
	}
	return entry
}

func (a *App) writePluginsLock(ctx context.Context) error {
	entries, err := a.declaredPluginEntries(ctx)
	if err != nil {
		return err
	}
	locked := list.NewList[buildplugin.LockedPlugin]()
	for i := range entries {
		entry := entries[i]
		if entry.Err != nil {
			return oops.In("bu1ld.plugins").
				With("plugin", entry.ID).
				With("namespace", entry.Namespace).
				Wrapf(entry.Err, "lock plugin")
		}
		lockedPlugin, lockErr := lockedPluginFromEntry(entry)
		if lockErr != nil {
			return oops.In("bu1ld.plugins").
				With("plugin", entry.ID).
				With("namespace", entry.Namespace).
				Wrapf(lockErr, "lock plugin")
		}
		locked.Add(lockedPlugin)
	}
	lock := buildplugin.NewLockFile(locked.Values())
	path := a.loader.LockFilePath()
	if err := buildplugin.WriteLockFile(path, lock); err != nil {
		return oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "write plugin lockfile")
	}
	return writef(a.output, "wrote %s (%d plugins)\n", path, len(lock.Plugins))
}

func lockedPluginFromEntry(entry pluginEntry) (buildplugin.LockedPlugin, error) {
	locked := buildplugin.LockedPlugin{
		Source:    entry.Source,
		Namespace: entry.Namespace,
		ID:        entry.ID,
		Version:   entry.Version,
		Path:      entry.Path,
	}
	if entry.Path == "" {
		return locked, nil
	}
	checksum, err := buildplugin.ChecksumFile(entry.Path)
	if err != nil {
		return buildplugin.LockedPlugin{}, oops.In("bu1ld.plugins").
			With("plugin", entry.ID).
			With("path", entry.Path).
			Wrapf(err, "checksum locked plugin")
	}
	locked.Checksum = checksum
	return locked, nil
}

func applyLockDiagnostics(entries []pluginEntry, lock buildplugin.LockFile) []pluginEntry {
	values := append([]pluginEntry(nil), entries...)
	matched := set.NewSet[string]()
	for index := range values {
		entry := values[index]
		locked, ok := lock.Find(entry.Source, entry.Namespace, entry.ID)
		if !ok {
			if entry.Declared && isProcessPluginSource(entry.Source) {
				values[index] = withPluginIssue(entry, "unlocked", fmt.Errorf("plugin is not in %s", buildplugin.LockFileName))
			}
			continue
		}
		matched.Add(lockDiagnosticKey(locked.Source, locked.Namespace, locked.ID))
		values[index] = withLockDiagnostic(entry, locked)
	}
	for i := range lock.Plugins {
		locked := lock.Plugins[i]
		if matched.Contains(lockDiagnosticKey(locked.Source, locked.Namespace, locked.ID)) {
			continue
		}
		values = append(values, pluginEntry{
			Source:    locked.Source,
			Namespace: locked.Namespace,
			ID:        locked.ID,
			Version:   locked.Version,
			Path:      locked.Path,
			Checksum:  locked.Checksum,
			Status:    "stale-lock",
			Err:       errors.New("locked plugin is not present in current plugin graph"),
		})
	}
	return sortPluginEntries(values)
}

func withLockDiagnostic(entry pluginEntry, locked buildplugin.LockedPlugin) pluginEntry {
	entry.Checksum = locked.Checksum
	if locked.Version != "" && entry.Version != "" && entry.Version != locked.Version {
		return withPluginIssue(entry, "lock-mismatch", fmt.Errorf("version %q does not match lockfile version %q", entry.Version, locked.Version))
	}
	if locked.Path == "" {
		return entry
	}
	if entry.Path != locked.Path {
		return withPluginIssue(entry, "lock-mismatch", fmt.Errorf("path %q does not match lockfile path %q", entry.Path, locked.Path))
	}
	if locked.Checksum == "" {
		return entry
	}
	checksum, err := buildplugin.ChecksumFile(entry.Path)
	if err != nil {
		return withPluginIssue(entry, "lock-mismatch", err)
	}
	if checksum != locked.Checksum {
		return withPluginIssue(entry, "lock-mismatch", fmt.Errorf("checksum %q does not match lockfile checksum %q", checksum, locked.Checksum))
	}
	return entry
}

func withPluginIssue(entry pluginEntry, status string, err error) pluginEntry {
	if entry.Err != nil {
		return entry
	}
	entry.Status = status
	entry.Err = err
	return entry
}

func isProcessPluginSource(source buildplugin.Source) bool {
	return source == buildplugin.SourceLocal || source == buildplugin.SourceGlobal
}

func lockDiagnosticKey(source buildplugin.Source, namespace, id string) string {
	return strings.Join([]string{string(source), namespace, id}, "\x00")
}

func writePluginTable(output io.Writer, entries []pluginEntry) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "SOURCE\tNAMESPACE\tID\tVERSION\tPATH\tRULES\tSTATUS"); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "write plugin report")
	}
	for i := range entries {
		entry := entries[i]
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
			return oops.In("bu1ld.app").Wrapf(err, "write plugin report")
		}
	}
	if err := writer.Flush(); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "flush plugin report")
	}
	return nil
}

func (a *App) writePluginReport(entries []pluginEntry, failOnIssue bool) error {
	hasIssue := false
	for i := range entries {
		entry := entries[i]
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

func ruleNames(rules []buildplugin.RuleSchema) []string {
	names := lo.Map[buildplugin.RuleSchema, string](rules, func(rule buildplugin.RuleSchema, _ int) string {
		return rule.Name
	})
	slices.Sort(names)
	return names
}

func manifestRuleNames(rules []buildplugin.ManifestRule) []string {
	names := lo.Map[buildplugin.ManifestRule, string](rules, func(rule buildplugin.ManifestRule, _ int) string {
		return rule.Name
	})
	slices.Sort(names)
	return names
}

func sortPluginEntries(entries []pluginEntry) []pluginEntry {
	slices.SortStableFunc(entries, func(left, right pluginEntry) int {
		if left.Source != right.Source {
			return strings.Compare(string(left.Source), string(right.Source))
		}
		if left.Namespace != right.Namespace {
			return strings.Compare(left.Namespace, right.Namespace)
		}
		if left.ID != right.ID {
			return strings.Compare(left.ID, right.ID)
		}
		return strings.Compare(left.Path, right.Path)
	})
	return entries
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
		return oops.In("bu1ld.plugins").
			With("path", path).
			Wrapf(err, "stat plugin executable")
	}
	if info.IsDir() {
		return oops.In("bu1ld.plugins").
			With("path", path).
			Errorf("%s is a directory", path)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode()&0o111 == 0 {
		return oops.In("bu1ld.plugins").
			With("path", path).
			Errorf("%s is not executable", path)
	}
	return nil
}
