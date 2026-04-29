package dsl

import (
	"go/token"
	"path/filepath"
	"slices"
	"strings"

	"bu1ld/internal/fsx"

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

func (l *Loader) scanImportGraph() ([]string, []importDependency, error) {
	root, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return nil, nil, err
	}
	collector := newLoadCollector()
	if err := scanImports(l.fs, root, map[string]bool{}, map[string]bool{}, collector); err != nil {
		return nil, nil, oops.In("bu1ld.dsl").
			With("file", root).
			Wrapf(err, "scan imported build files")
	}
	return collector.filePaths(), collector.imports.Values(), nil
}

func scanImports(
	fs afero.Fs,
	path string,
	stack map[string]bool,
	seen map[string]bool,
	collector *loadCollector,
) error {
	absPath, err := cleanAbsPath(path)
	if err != nil {
		return err
	}
	if stack[absPath] {
		return oops.In("bu1ld.dsl").
			With("file", absPath).
			New("import cycle detected")
	}
	if seen[absPath] {
		return nil
	}
	stack[absPath] = true
	defer delete(stack, absPath)

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
	seen[absPath] = true

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
