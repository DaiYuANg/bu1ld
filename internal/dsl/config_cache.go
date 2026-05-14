package dsl

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/lyonbrown4d/bu1ld/internal/build"
	"github.com/lyonbrown4d/bu1ld/internal/cachefile"
	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"
	"github.com/lyonbrown4d/bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

const configCacheVersion = 4

type configCacheRecord struct {
	Version   int                  `json:"version"`
	BuildFile string               `json:"buildFile"`
	Files     []configCacheFile    `json:"files"`
	Imports   []configCacheImport  `json:"imports,omitempty"`
	Packages  []configCachePackage `json:"packages,omitempty"`
	Envs      []configCacheEnv     `json:"envs,omitempty"`
	Plugins   []configCachePlugin  `json:"plugins,omitempty"`
	Project   configCacheProject   `json:"project"`
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

type configCachePackage struct {
	Pattern string   `json:"pattern"`
	Matches []string `json:"matches"`
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
	Packages []configCacheProjectPackage `json:"packages,omitempty"`
	Tasks    []configCacheTask           `json:"tasks"`
}

type configCacheProjectPackage struct {
	Name string   `json:"name"`
	Dir  string   `json:"dir"`
	Deps []string `json:"deps,omitempty"`
}

type configCacheTask struct {
	Name    string            `json:"name"`
	Deps    []string          `json:"deps,omitempty"`
	Inputs  []string          `json:"inputs,omitempty"`
	Outputs []string          `json:"outputs,omitempty"`
	Command []string          `json:"command,omitempty"`
	Action  configCacheAction `json:"action,omitzero"`
	Local   string            `json:"local,omitempty"`
	Package string            `json:"package,omitempty"`
	WorkDir string            `json:"workDir,omitempty"`
}

type configCacheAction struct {
	Kind   string         `json:"kind,omitempty"`
	Params map[string]any `json:"params,omitempty"`
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

func (l *Loader) saveConfigCache(
	project build.Project,
	files []*File,
	filePaths []string,
	imports []importDependency,
	packages []packageDiscovery,
	envs []envDependency,
) error {
	record, err := l.newConfigCacheRecord(project, files, filePaths, imports, packages, envs)
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

func (l *Loader) newConfigCacheRecord(
	project build.Project,
	files []*File,
	filePaths []string,
	imports []importDependency,
	packages []packageDiscovery,
	envs []envDependency,
) (configCacheRecord, error) {
	buildFile, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return configCacheRecord{}, err
	}
	cacheFiles, err := configCacheFiles(l.fs, filePaths)
	if err != nil {
		return configCacheRecord{}, err
	}
	plugins, err := l.configCachePlugins(files)
	if err != nil {
		return configCacheRecord{}, err
	}
	return configCacheRecord{
		Version:   configCacheVersion,
		BuildFile: buildFile,
		Files:     cacheFiles,
		Imports:   configCacheImports(imports),
		Packages:  configCachePackages(packages),
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
	return l.configCacheFilesValid(record.Files) &&
		l.configCacheImportsValid(record.Imports) &&
		l.configCachePackagesValid(record.Packages) &&
		configCacheEnvsValid(record.Envs) &&
		l.configCachePluginsValid(record.Plugins)
}

func (l *Loader) configCachePath() string {
	return filepath.Join(l.cfg.CachePath(), "config", "project.bin")
}

func configCacheFiles(fs afero.Fs, paths []string) ([]configCacheFile, error) {
	items := list.NewList[configCacheFile]()
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
	items := list.NewList[configCacheImport]()
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

func configCachePackages(packages []packageDiscovery) []configCachePackage {
	items := list.NewList[configCachePackage]()
	for _, item := range packages {
		matches := slices.Clone(item.Matches)
		slices.Sort(matches)
		items.Add(configCachePackage{
			Pattern: item.Pattern,
			Matches: matches,
		})
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCachePackage) int {
		return cmp.Compare(left.Pattern, right.Pattern)
	})
	return values
}

func configCacheEnvs(envs []envDependency) []configCacheEnv {
	items := list.NewList[configCacheEnv]()
	for _, item := range envs {
		items.Add(configCacheEnv(item))
	}
	values := items.Values()
	slices.SortFunc(values, func(left, right configCacheEnv) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return values
}
