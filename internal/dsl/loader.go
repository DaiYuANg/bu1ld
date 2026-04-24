package dsl

import (
	"path/filepath"
	"slices"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	"bu1ld/internal/fsx"
	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx"
	"github.com/bmatcuk/doublestar/v4"
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
	if !l.cfg.NoCache {
		if project, ok := l.loadConfigCache(); ok {
			return project, nil
		}
	}

	file, filePaths, imports, err := l.loadFileSet()
	if err != nil {
		return build.Project{}, err
	}
	project, envs, err := evaluateWithRegistry(file, l.parser.registry.CloneWithOptions(buildplugin.LoadOptions{ProjectDir: l.cfg.WorkDir}))
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "parse build file")
	}
	if !l.cfg.NoCache {
		if err := l.saveConfigCache(project, file, filePaths, imports, envs); err != nil {
			return build.Project{}, oops.In("bu1ld.dsl").
				With("file", l.cfg.BuildFilePath()).
				Wrapf(err, "write configuration cache")
		}
	}
	return project, nil
}

func (l *Loader) LoadFile() (*File, error) {
	file, _, _, err := l.loadFileSet()
	return file, err
}

func (l *Loader) loadFileSet() (*File, []string, []importDependency, error) {
	path := l.cfg.BuildFilePath()
	collector := newLoadCollector()
	file, err := l.loadFile(path, map[string]bool{}, map[string]bool{}, collector)
	if err != nil {
		return nil, nil, nil, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "load build file")
	}
	return file, collector.filePaths(), collector.imports.Values(), nil
}

func (l *Loader) LoadOptions() buildplugin.LoadOptions {
	return buildplugin.LoadOptions{ProjectDir: l.cfg.WorkDir}
}

func (l *Loader) PluginSchemas() ([]buildplugin.Metadata, error) {
	return l.parser.Schemas()
}

func (l *Loader) LockFilePath() string {
	return filepath.Join(l.cfg.WorkDir, buildplugin.LockFileName)
}

func (l *Loader) loadFile(path string, stack map[string]bool, seen map[string]bool, collector *loadCollector) (*File, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "resolve build file path")
	}
	absPath = filepath.Clean(absPath)
	if stack[absPath] {
		return nil, oops.In("bu1ld.dsl").
			With("file", absPath).
			New("import cycle detected")
	}
	if seen[absPath] {
		return &File{}, nil
	}

	stack[absPath] = true
	defer delete(stack, absPath)

	data, err := afero.ReadFile(l.fs, absPath)
	if err != nil {
		return nil, oops.In("bu1ld.dsl").
			With("file", absPath).
			Wrapf(err, "read build file")
	}
	collector.addFile(absPath)
	file, err := l.parser.ParseFile(string(data))
	if err != nil {
		return nil, oops.In("bu1ld.dsl").
			With("file", absPath).
			Wrapf(err, "parse build file")
	}
	seen[absPath] = true

	merged := &File{}
	for _, statement := range file.Statements {
		importNode, ok := statement.(*ImportNode)
		if !ok {
			merged.Statements = append(merged.Statements, statement)
			continue
		}
		importedPaths, err := resolveImportPaths(l.fs, absPath, importNode.Path)
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("file", absPath).
				With("import", importNode.Path).
				With("line", importNode.Position().Line).
				With("column", importNode.Position().Column).
				Wrapf(err, "resolve import")
		}
		collector.addImport(absPath, importNode.Path, importedPaths)
		for _, importedPath := range importedPaths {
			imported, err := l.loadFile(importedPath, stack, seen, collector)
			if err != nil {
				return nil, err
			}
			merged.Statements = append(merged.Statements, imported.Statements...)
		}
	}
	return merged, nil
}

func resolveImportPaths(fs afero.Fs, importerPath string, importPath string) ([]string, error) {
	if importPath == "" {
		return nil, oops.In("bu1ld.dsl").
			With("file", importerPath).
			New("import path is required")
	}
	pattern := importPath
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(importerPath), pattern)
	}
	pattern = filepath.Clean(pattern)

	if strings.ContainsAny(pattern, "*?[") {
		matches, err := fsx.Glob(fs, pattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("file", importerPath).
				With("import", importPath).
				Wrapf(err, "resolve import glob")
		}
		if len(matches) == 0 {
			return nil, oops.In("bu1ld.dsl").
				With("file", importerPath).
				With("import", importPath).
				Errorf("import %q matched no files", importPath)
		}
		for index, match := range matches {
			matches[index] = filepath.Clean(match)
		}
		slices.Sort(matches)
		return matches, nil
	}
	return []string{pattern}, nil
}

type importDependency struct {
	Importer string
	Path     string
	Matches  []string
}

type loadCollector struct {
	files   collectionx.List[string]
	seen    collectionx.Set[string]
	imports collectionx.List[importDependency]
}

func newLoadCollector() *loadCollector {
	return &loadCollector{
		files:   collectionx.NewList[string](),
		seen:    collectionx.NewSet[string](),
		imports: collectionx.NewList[importDependency](),
	}
}

func (c *loadCollector) addFile(path string) {
	if c == nil || c.seen.Contains(path) {
		return
	}
	c.seen.Add(path)
	c.files.Add(path)
}

func (c *loadCollector) addImport(importer string, path string, matches []string) {
	if c == nil {
		return
	}
	c.imports.Add(importDependency{
		Importer: importer,
		Path:     path,
		Matches:  slices.Clone(matches),
	})
}

func (c *loadCollector) filePaths() []string {
	if c == nil {
		return nil
	}
	values := c.files.Values()
	slices.Sort(values)
	return values
}
