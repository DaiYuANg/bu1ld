package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"

	"github.com/DaiYuANg/arcgo/collectionx"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/samber/oops"
)

type Loader struct {
	cfg    config.Config
	parser *Parser
}

func NewLoader(cfg config.Config, parser *Parser) *Loader {
	return &Loader{
		cfg:    cfg,
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
		_ = l.saveConfigCache(project, file, filePaths, imports, envs)
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
		return nil, err
	}
	absPath = filepath.Clean(absPath)
	if stack[absPath] {
		return nil, fmt.Errorf("import cycle detected at %s", absPath)
	}
	if seen[absPath] {
		return &File{}, nil
	}

	stack[absPath] = true
	defer delete(stack, absPath)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	collector.addFile(absPath)
	file, err := l.parser.ParseFile(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", absPath, err)
	}
	seen[absPath] = true

	merged := &File{}
	for _, statement := range file.Statements {
		importNode, ok := statement.(*ImportNode)
		if !ok {
			merged.Statements = append(merged.Statements, statement)
			continue
		}
		importedPaths, err := resolveImportPaths(absPath, importNode.Path)
		if err != nil {
			return nil, fmt.Errorf("%s:%d:%d: %w", absPath, importNode.Position().Line, importNode.Position().Column, err)
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

func resolveImportPaths(importerPath string, importPath string) ([]string, error) {
	if importPath == "" {
		return nil, fmt.Errorf("import path is required")
	}
	pattern := importPath
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(importerPath), pattern)
	}
	pattern = filepath.Clean(pattern)

	if strings.ContainsAny(pattern, "*?[") {
		matches, err := doublestar.FilepathGlob(pattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("import %q matched no files", importPath)
		}
		for index, match := range matches {
			matches[index] = filepath.Clean(match)
		}
		sort.Strings(matches)
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
		Matches:  append([]string(nil), matches...),
	})
}

func (c *loadCollector) filePaths() []string {
	if c == nil {
		return nil
	}
	values := c.files.Values()
	sort.Strings(values)
	return values
}
