package dsl

import (
	"go/token"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lyonbrown4d/bu1ld/internal/fsx"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/plano/ast"
	planofrontend "github.com/arcgolabs/plano/frontend/plano"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type importDependency struct {
	Importer string
	Path     string
	Matches  []string
}

type packageDiscovery struct {
	Pattern string
	Matches []string
}

type loadCollector struct {
	files   *set.OrderedSet[string]
	imports *list.List[importDependency]
}

func newLoadCollector() *loadCollector {
	return &loadCollector{
		files:   set.NewOrderedSet[string](),
		imports: list.NewList[importDependency](),
	}
}

func (c *loadCollector) addFile(path string) {
	if c == nil {
		return
	}
	c.files.Add(path)
}

func (c *loadCollector) addImport(importer, path string, matches []string) {
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

func (l *Loader) scanImportGraph() ([]string, []importDependency, error) {
	root, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return nil, nil, err
	}
	collector := newLoadCollector()
	if err := scanImports(l.fs, root, set.NewSet[string](), set.NewSet[string](), collector); err != nil {
		return nil, nil, oops.In("bu1ld.dsl").
			With("file", root).
			Wrapf(err, "scan imported build files")
	}
	return collector.filePaths(), collector.imports.Values(), nil
}

func scanImports(
	fs afero.Fs,
	path string,
	stack *set.Set[string],
	seen *set.Set[string],
	collector *loadCollector,
) error {
	absPath, err := cleanAbsPath(path)
	if err != nil {
		return err
	}
	if stack.Contains(absPath) {
		return oops.In("bu1ld.dsl").
			With("file", absPath).
			New("import cycle detected")
	}
	if seen.Contains(absPath) {
		return nil
	}
	stack.Add(absPath)
	defer stack.Remove(absPath)

	data, err := afero.ReadFile(fs, absPath)
	if err != nil {
		return oops.In("bu1ld.dsl").
			With("file", absPath).
			Wrapf(err, "read build file")
	}
	collector.addFile(absPath)

	fset := token.NewFileSet()
	file, diagnostics := planofrontend.ParseFile(fset, absPath, data)
	if err := diagnosticsError(fset, diagnostics); err != nil {
		return err
	}
	seen.Add(absPath)

	for _, stmt := range file.Statements {
		importDecl, ok := stmt.(*ast.ImportDecl)
		if !ok || importDecl.Path == nil {
			continue
		}
		importPath := importDecl.Path.Value
		matches, err := resolveImportPaths(fs, absPath, importPath)
		if err != nil {
			location := fset.Position(importDecl.Pos())
			return oops.In("bu1ld.dsl").
				With("file", absPath).
				With("import", importPath).
				With("line", location.Line).
				With("column", location.Column).
				Wrapf(err, "resolve import")
		}
		collector.addImport(absPath, importPath, matches)
		for _, match := range matches {
			if err := scanImports(fs, match, stack, seen, collector); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveImportPaths(fs afero.Fs, importerPath, importPath string) ([]string, error) {
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

func discoverPackageBuildFiles(fs afero.Fs, workDir string, patterns []string) ([]string, []packageDiscovery, error) {
	files := set.NewOrderedSet[string]()
	discoveries := list.NewList[packageDiscovery]()
	for _, pattern := range patterns {
		matches, err := resolvePackageBuildFiles(fs, workDir, pattern)
		if err != nil {
			return nil, nil, err
		}
		discoveries.Add(packageDiscovery{
			Pattern: pattern,
			Matches: slices.Clone(matches),
		})
		files.Add(matches...)
	}
	return files.Values(), discoveries.Values(), nil
}

func resolvePackageBuildFiles(fs afero.Fs, workDir, pattern string) ([]string, error) {
	if pattern == "" {
		return nil, oops.In("bu1ld.dsl").New("workspace package pattern is required")
	}
	candidate := pattern
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workDir, candidate)
	}
	candidate = filepath.Clean(candidate)

	var matches []string
	var err error
	if strings.ContainsAny(candidate, "*?[") {
		matches, err = fsx.Glob(fs, candidate, doublestar.WithNoFollow())
		if err != nil {
			return nil, oops.In("bu1ld.dsl").
				With("package_pattern", pattern).
				Wrapf(err, "resolve workspace package glob")
		}
	} else {
		matches = []string{candidate}
	}

	files := list.NewList[string]()
	for _, match := range matches {
		path, ok := packageBuildFile(fs, match)
		if ok {
			files.Add(path)
		}
	}
	values := files.Values()
	slices.Sort(values)
	if len(values) == 0 {
		return nil, oops.In("bu1ld.dsl").
			With("package_pattern", pattern).
			Errorf("workspace package pattern %q matched no build files", pattern)
	}
	return values, nil
}

func packageBuildFile(fs afero.Fs, path string) (string, bool) {
	info, err := fs.Stat(path)
	if err != nil {
		return "", false
	}
	if !info.IsDir() {
		if filepath.Base(path) == "build.bu1ld" {
			return filepath.Clean(path), true
		}
		return "", false
	}
	buildFile := filepath.Join(path, "build.bu1ld")
	if _, err := fs.Stat(buildFile); err != nil {
		return "", false
	}
	return filepath.Clean(buildFile), true
}
