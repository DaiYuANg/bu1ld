package dsl

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"

	"bu1ld/internal/build"
	"bu1ld/internal/cachefile"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

const configCacheVersion = 2

type configCacheRecord struct {
	Version   int                 `json:"version"`
	BuildFile string              `json:"buildFile"`
	Files     []configCacheFile   `json:"files"`
	Imports   []configCacheImport `json:"imports,omitempty"`
	Envs      []configCacheEnv    `json:"envs,omitempty"`
	Plugins   []configCachePlugin `json:"plugins,omitempty"`
	Project   configCacheProject  `json:"project"`
}

type configCacheFile struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

type configCacheImport struct {
	Importer string   `json:"importer"`
	Path     string   `json:"path"`
	Matches  []string `json:"matches"`
}

type configCacheEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type configCachePlugin struct {
	Source       buildplugin.Source `json:"source"`
	Namespace    string             `json:"namespace,omitempty"`
	ID           string             `json:"id"`
	Version      string             `json:"version,omitempty"`
	DeclaredPath string             `json:"declaredPath,omitempty"`
	Path         string             `json:"path"`
	Checksum     string             `json:"checksum"`
}

type configCacheProject struct {
	Tasks []configCacheTask `json:"tasks"`
}

type configCacheTask struct {
	Name    string   `json:"name"`
	Deps    []string `json:"deps,omitempty"`
	Inputs  []string `json:"inputs,omitempty"`
	Outputs []string `json:"outputs,omitempty"`
	Command []string `json:"command,omitempty"`
}

func (l *Loader) loadConfigCache() (build.Project, bool) {
	var record configCacheRecord
	if err := cachefile.Read(l.fs, l.configCachePath(), &record); err != nil {
		return build.Project{}, false
	}
	if !l.configCacheRecordValid(record) {
		return build.Project{}, false
	}
	return projectFromConfigCache(record.Project), true
}

func (l *Loader) saveConfigCache(project build.Project, file *File, filePaths []string, imports []importDependency, envs []envDependency) error {
	record, err := l.newConfigCacheRecord(project, file, filePaths, imports, envs)
	if err != nil {
		return err
	}
	if err := cachefile.Write(l.fs, l.configCachePath(), record); err != nil {
		return oops.In("bu1ld.dsl").
			With("path", l.configCachePath()).
			Wrapf(err, "write configuration cache")
	}
	return nil
}

func (l *Loader) newConfigCacheRecord(project build.Project, file *File, filePaths []string, imports []importDependency, envs []envDependency) (configCacheRecord, error) {
	buildFile, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return configCacheRecord{}, err
	}
	files, err := configCacheFiles(l.fs, filePaths)
	if err != nil {
		return configCacheRecord{}, err
	}
	plugins, err := l.configCachePlugins(file)
	if err != nil {
		return configCacheRecord{}, err
	}
	return configCacheRecord{
		Version:   configCacheVersion,
		BuildFile: buildFile,
		Files:     files,
		Imports:   configCacheImports(imports),
		Envs:      configCacheEnvs(envs),
		Plugins:   plugins,
		Project:   configCacheProjectFromBuild(project),
	}, nil
}

func (l *Loader) configCacheRecordValid(record configCacheRecord) bool {
	if record.Version != configCacheVersion || len(record.Files) == 0 {
		return false
	}
	buildFile, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil || record.BuildFile != buildFile {
		return false
	}
	for _, file := range record.Files {
		if !configCacheFileValid(l.fs, file) {
			return false
		}
	}
	for _, item := range record.Imports {
		if !configCacheImportValid(l.fs, item) {
			return false
		}
	}
	for _, item := range record.Envs {
		if !configCacheEnvValid(item) {
			return false
		}
	}
	for _, plugin := range record.Plugins {
		if !l.configCachePluginValid(plugin) {
			return false
		}
	}
	return true
}

func (l *Loader) configCachePath() string {
	return filepath.Join(l.cfg.CachePath(), "config", "project.bin")
}

func configCacheFiles(fs afero.Fs, paths []string) ([]configCacheFile, error) {
	items := collectionx.NewList[configCacheFile]()
	for _, path := range paths {
		checksum, err := snapshot.DigestFile(fs, path)
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("file", path).
				Wrapf(err, "hash configuration file")
		}
		items.Add(configCacheFile{
			Path:     path,
			Checksum: checksum,
		})
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCacheFile) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return values, nil
}

func configCacheImports(imports []importDependency) []configCacheImport {
	items := collectionx.NewList[configCacheImport]()
	for _, item := range imports {
		matches := slices.Clone(item.Matches)
		slices.Sort(matches)
		items.Add(configCacheImport{
			Importer: item.Importer,
			Path:     item.Path,
			Matches:  matches,
		})
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCacheImport) int {
		return cmp.Compare(left.Importer+"\x00"+left.Path, right.Importer+"\x00"+right.Path)
	})
	return values
}

func configCacheEnvs(envs []envDependency) []configCacheEnv {
	items := collectionx.NewList[configCacheEnv]()
	for _, item := range envs {
		items.Add(configCacheEnv(item))
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCacheEnv) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return values
}

func (l *Loader) configCachePlugins(file *File) ([]configCachePlugin, error) {
	declarations, err := PluginDeclarations(file)
	if err != nil {
		return nil, err
	}

	loader := buildplugin.NewProcessLoader(l.LoadOptions())
	items := collectionx.NewList[configCachePlugin]()
	for _, item := range declarations {
		declaration := buildplugin.NormalizeDeclaration(item.Declaration)
		if declaration.Source != buildplugin.SourceLocal && declaration.Source != buildplugin.SourceGlobal {
			continue
		}
		path, err := loader.ResolvePath(declaration)
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("plugin", declaration.Namespace).
				With("source", declaration.Source).
				Wrapf(err, "resolve plugin path")
		}
		checksum, err := buildplugin.ChecksumFile(path)
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("plugin", declaration.Namespace).
				With("path", path).
				Wrapf(err, "checksum plugin binary")
		}
		items.Add(configCachePlugin{
			Source:       declaration.Source,
			Namespace:    declaration.Namespace,
			ID:           declaration.ID,
			Version:      declaration.Version,
			DeclaredPath: declaration.Path,
			Path:         filepath.Clean(path),
			Checksum:     checksum,
		})
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCachePlugin) int {
		return cmp.Compare(configCachePluginKey(left), configCachePluginKey(right))
	})
	return values, nil
}

func configCacheFileValid(fs afero.Fs, file configCacheFile) bool {
	if file.Path == "" || file.Checksum == "" {
		return false
	}
	checksum, err := snapshot.DigestFile(fs, file.Path)
	return err == nil && checksum == file.Checksum
}

func configCacheImportValid(fs afero.Fs, item configCacheImport) bool {
	if item.Importer == "" || item.Path == "" {
		return false
	}
	matches, err := resolveImportPaths(fs, item.Importer, item.Path)
	if err != nil {
		return false
	}
	return slices.Equal(matches, item.Matches)
}

func configCacheEnvValid(item configCacheEnv) bool {
	return item.Name != "" && os.Getenv(item.Name) == item.Value
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

func configCacheProjectFromBuild(project build.Project) configCacheProject {
	tasks := collectionx.NewList[configCacheTask]()
	if project.Tasks != nil {
		project.Tasks.Range(func(_ int, task build.Task) bool {
			tasks.Add(configCacheTask{
				Name:    task.Name,
				Deps:    slices.Clone(build.Values(task.Deps)),
				Inputs:  slices.Clone(build.Values(task.Inputs)),
				Outputs: slices.Clone(build.Values(task.Outputs)),
				Command: slices.Clone(build.Values(task.Command)),
			})
			return true
		})
	}
	return configCacheProject{Tasks: tasks.Values()}
}

func projectFromConfigCache(cached configCacheProject) build.Project {
	tasks := collectionx.NewList[build.Task]()
	for _, item := range cached.Tasks {
		task := build.NewTask(item.Name)
		task.Deps = collectionx.NewList[string](item.Deps...)
		task.Inputs = collectionx.NewList[string](item.Inputs...)
		task.Outputs = collectionx.NewList[string](item.Outputs...)
		task.Command = collectionx.NewList[string](item.Command...)
		tasks.Add(task)
	}
	return build.Project{Tasks: tasks}
}

func cleanAbsPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", oops.In("bu1ld.dsl").
			With("path", path).
			Wrapf(err, "resolve absolute path")
	}
	return filepath.Clean(absPath), nil
}

func configCachePluginKey(item configCachePlugin) string {
	return string(item.Source) + "\x00" + item.Namespace + "\x00" + item.ID + "\x00" + item.Version + "\x00" + item.Path
}
