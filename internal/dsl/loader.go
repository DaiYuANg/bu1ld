package dsl

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type Loader struct {
	cfg    config.Config
	fs     afero.Fs
	parser *Parser
}

func NewLoader(cfg config.Config, fs afero.Fs, parser *Parser) *Loader {
	return &Loader{
		cfg:    cfg,
		fs:     fs,
		parser: parser,
	}
}

func (l *Loader) Load() (build.Project, error) {
	return l.LoadContext(context.Background())
}

func (l *Loader) LoadContext(ctx context.Context) (build.Project, error) {
	if !l.cfg.NoCache {
		if project, ok := l.loadConfigCache(); ok {
			return project, nil
		}
	}

	filePaths, imports, err := l.scanImportGraph()
	if err != nil {
		return build.Project{}, err
	}
	file, registry, envs, err := l.loadCompiledFile(ctx)
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "parse build file")
	}
	defer registry.Close()
	project, err := lowerProject(ctx, file, registry)
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "evaluate build file")
	}
	compiledFiles := []*File{file}
	packageDiscoveries, err := l.loadWorkspacePackages(ctx, &project, file, &compiledFiles, &filePaths, &imports, &envs)
	if err != nil {
		return build.Project{}, err
	}
	project = applyPackageDependencies(project)
	if !l.cfg.NoCache {
		if err := l.saveConfigCache(project, compiledFiles, filePaths, imports, packageDiscoveries, envs); err != nil {
			return build.Project{}, oops.In("bu1ld.dsl").
				With("file", l.cfg.BuildFilePath()).
				Wrapf(err, "write configuration cache")
		}
	}
	return project, nil
}

func (l *Loader) loadWorkspacePackages(
	ctx context.Context,
	project *build.Project,
	file *File,
	compiledFiles *[]*File,
	filePaths *[]string,
	imports *[]importDependency,
	envs *[]envDependency,
) ([]packageDiscovery, error) {
	patterns, err := WorkspacePackagePatterns(file)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	packageFiles, packageDiscoveries, err := discoverPackageBuildFiles(l.fs, l.cfg.WorkDir, patterns)
	if err != nil {
		return nil, err
	}
	filePathItems := list.NewList(*filePaths...)
	importItems := list.NewList(*imports...)
	compiledFileItems := list.NewList(*compiledFiles...)
	envItems := list.NewList(*envs...)
	for _, path := range packageFiles {
		packageFilePaths, packageImports, scanErr := scanPackageImportGraph(l.fs, path)
		if scanErr != nil {
			return nil, scanErr
		}
		filePathItems.Add(packageFilePaths...)
		importItems.Add(packageImports...)

		result, loadErr := l.loadPackageProject(ctx, path)
		if loadErr != nil {
			return nil, loadErr
		}
		compiledFileItems.Add(result.file)
		mergePackageProject(project, result.project, result.pkg)
		envItems.Add(result.envs...)
	}
	*filePaths = sortedUniqueStrings(filePathItems.Values())
	*imports = importItems.Values()
	*compiledFiles = compiledFileItems.Values()
	*envs = envItems.Values()
	return packageDiscoveries, nil
}

type packageLoadResult struct {
	file    *File
	project build.Project
	pkg     build.Package
	envs    []envDependency
}

func (l *Loader) loadPackageProject(ctx context.Context, path string) (packageLoadResult, error) {
	packageFile, packageRegistry, packageEnvs, err := l.loadCompiledPath(ctx, path)
	if err != nil {
		return packageLoadResult{}, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "parse package build file")
	}
	defer packageRegistry.Close()

	packageProject, err := lowerProject(ctx, packageFile, packageRegistry)
	if err != nil {
		return packageLoadResult{}, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "evaluate package build file")
	}
	pkg, err := l.packageMetadata(packageFile, path)
	if err != nil {
		return packageLoadResult{}, err
	}
	return packageLoadResult{file: packageFile, project: packageProject, pkg: pkg, envs: packageEnvs}, nil
}

func (l *Loader) LoadFile() (*File, error) {
	file, registry, _, err := l.loadCompiledFile(context.Background())
	if registry != nil {
		registry.Close()
	}
	return file, err
}

func (l *Loader) LoadOptions() buildplugin.LoadOptions {
	return buildplugin.LoadOptions{
		ProjectDir: l.cfg.WorkDir,
		Env:        l.cfg.ChildEnv(),
	}
}

func (l *Loader) PluginSchemas() ([]buildplugin.Metadata, error) {
	return l.parser.Schemas()
}

func (l *Loader) LockFilePath() string {
	return filepath.Join(l.cfg.WorkDir, buildplugin.LockFileName)
}

func (l *Loader) BuildFilePath() string {
	return l.cfg.BuildFilePath()
}

func (l *Loader) FS() afero.Fs {
	return l.fs
}

func (l *Loader) ReadBuildFile() ([]byte, string, error) {
	path, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return nil, "", err
	}
	data, err := afero.ReadFile(l.fs, path)
	if err != nil {
		return nil, "", fmt.Errorf("read %q: %w", path, err)
	}
	return data, path, nil
}

func (l *Loader) loadCompiledFile(ctx context.Context) (*File, *buildplugin.Registry, []envDependency, error) {
	buildFile, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return nil, nil, nil, err
	}
	return l.loadCompiledPath(ctx, buildFile)
}

func (l *Loader) loadCompiledPath(ctx context.Context, path string) (*File, *buildplugin.Registry, []envDependency, error) {
	readFile := func(path string) ([]byte, error) {
		data, readErr := afero.ReadFile(l.fs, path)
		if readErr != nil {
			return nil, fmt.Errorf("read %q: %w", path, readErr)
		}
		return data, nil
	}
	return l.parser.compilePathDetailed(ctx, path, l.LoadOptions(), readFile)
}

func (l *Loader) packageMetadata(file *File, buildFilePath string) (build.Package, error) {
	dir := filepath.Dir(buildFilePath)
	rel, err := filepath.Rel(l.cfg.WorkDir, dir)
	if err != nil {
		return build.Package{}, oops.In("bu1ld.dsl").
			With("file", buildFilePath).
			Wrapf(err, "resolve package directory")
	}
	rel = filepath.ToSlash(rel)
	return PackageMetadata(file, rel, rel)
}

func scanPackageImportGraph(fs afero.Fs, path string) ([]string, []importDependency, error) {
	collector := newLoadCollector()
	if err := scanImports(fs, path, set.NewSet[string](), set.NewSet[string](), collector); err != nil {
		return nil, nil, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "scan package imported build files")
	}
	return collector.filePaths(), collector.imports.Values(), nil
}

func mergePackageProject(project *build.Project, packageProject build.Project, pkg build.Package) {
	if project.Packages == nil {
		project.Packages = list.NewList[build.Package]()
	}
	project.Packages.Add(pkg)
	if project.Tasks == nil {
		project.Tasks = list.NewList[build.Task]()
	}
	if packageProject.Tasks == nil {
		return
	}
	packageProject.Tasks.Range(func(_ int, task build.Task) bool {
		project.Tasks.Add(qualifyPackageTask(task, pkg))
		return true
	})
}

func qualifyPackageTask(task build.Task, pkg build.Package) build.Task {
	localName := task.LocalName
	if localName == "" {
		localName = task.Name
	}
	task.Package = pkg.Name
	task.LocalName = localName
	task.WorkDir = pkg.Dir
	task.Name = build.QualifyTaskName(pkg.Name, localName)
	task.Deps = qualifyPackageDeps(task.Deps, pkg.Name)
	return task
}

func qualifyPackageDeps(deps *list.List[string], packageName string) *list.List[string] {
	values := list.NewList[string]()
	for _, dep := range build.Values(deps) {
		values.Add(build.QualifyTaskName(packageName, dep))
	}
	return values
}

func applyPackageDependencies(project build.Project) build.Project {
	tasks := mapping.NewMap[string, build.Task]()
	if project.Tasks != nil {
		project.Tasks.Range(func(_ int, task build.Task) bool {
			tasks.Set(task.Name, task)
			return true
		})
	}
	if project.Tasks == nil || project.Packages == nil {
		return project
	}
	packageDeps := mapping.NewMultiMap[string, string]()
	project.Packages.Range(func(_ int, pkg build.Package) bool {
		packageDeps.Set(pkg.Name, build.Values(pkg.Deps)...)
		return true
	})
	project.Tasks.Range(func(index int, task build.Task) bool {
		for _, depPackage := range packageDeps.Get(task.Package) {
			depTask := build.QualifyTaskName(depPackage, task.LocalName)
			if _, ok := tasks.Get(depTask); ok {
				task.Deps.Add(depTask)
			}
		}
		_ = project.Tasks.Set(index, task)
		return true
	})
	return project
}

func sortedUniqueStrings(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}
