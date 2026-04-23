package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"

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
	file, err := l.LoadFile()
	if err != nil {
		return build.Project{}, err
	}
	project, err := EvaluateWithRegistry(file, l.parser.registry.CloneWithOptions(buildplugin.LoadOptions{ProjectDir: l.cfg.WorkDir}))
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "parse build file")
	}
	return project, nil
}

func (l *Loader) LoadFile() (*File, error) {
	path := l.cfg.BuildFilePath()
	file, err := l.loadFile(path, map[string]bool{}, map[string]bool{})
	if err != nil {
		return nil, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "load build file")
	}
	return file, nil
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

func (l *Loader) loadFile(path string, stack map[string]bool, seen map[string]bool) (*File, error) {
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
		for _, importedPath := range importedPaths {
			imported, err := l.loadFile(importedPath, stack, seen)
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
		return matches, nil
	}
	return []string{pattern}, nil
}
