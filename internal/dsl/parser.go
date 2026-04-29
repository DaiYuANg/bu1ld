package dsl

import (
	"context"
	"fmt"
	"io"

	"bu1ld/internal/build"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/golang"

	planocomp "github.com/arcgolabs/plano/compiler"
)

type Parser struct {
	registry *buildplugin.Registry
}

func NewParser() *Parser {
	registry, err := buildplugin.NewRegistry(buildplugin.LoadOptions{}, golang.New())
	if err != nil {
		panic(err)
	}
	return NewParserWithRegistry(registry)
}

func NewParserWithRegistry(registry *buildplugin.Registry) *Parser {
	return &Parser{registry: registry}
}

func (p *Parser) Schemas() ([]buildplugin.Metadata, error) {
	schemas, err := p.registry.Schemas()
	if err != nil {
		return nil, fmt.Errorf("read plugin schemas: %w", err)
	}
	return schemas, nil
}

func (p *Parser) Parse(reader io.Reader) (build.Project, error) {
	return p.ParseWithOptions(reader, buildplugin.LoadOptions{})
}

func (p *Parser) ParseWithOptions(reader io.Reader, options buildplugin.LoadOptions) (build.Project, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return build.Project{}, fmt.Errorf("read DSL source: %w", err)
	}

	file, registry, _, err := p.compileSourceDetailed("dsl", data, options)
	if err != nil {
		return build.Project{}, err
	}
	defer registry.Close()

	project, err := lowerProject(file, registry)
	if err != nil {
		return build.Project{}, err
	}
	return project, nil
}

func (p *Parser) ParseFile(source string) (*File, error) {
	file, registry, _, err := p.compileSourceDetailed("dsl", []byte(source), buildplugin.LoadOptions{})
	if registry != nil {
		registry.Close()
	}
	return file, err
}

func (p *Parser) compileSourceDetailed(
	filename string,
	src []byte,
	options buildplugin.LoadOptions,
) (*File, *buildplugin.Registry, []envDependency, error) {
	return p.compileDetailed(filename, options, nil, func(compiler *planocomp.Compiler) planocomp.Result {
		return compiler.CompileSourceDetailed(context.Background(), filename, src)
	})
}

func (p *Parser) compilePathDetailed(
	path string,
	options buildplugin.LoadOptions,
	readFile func(string) ([]byte, error),
) (*File, *buildplugin.Registry, []envDependency, error) {
	return p.compileDetailed(path, options, readFile, func(compiler *planocomp.Compiler) planocomp.Result {
		return compiler.CompileFileDetailed(context.Background(), path)
	})
}

func (p *Parser) compileDetailed(
	path string,
	options buildplugin.LoadOptions,
	readFile func(string) ([]byte, error),
	run func(*planocomp.Compiler) planocomp.Result,
) (*File, *buildplugin.Registry, []envDependency, error) {
	tracker := newEnvTracker()
	registry := p.registry.CloneWithOptions(options)

	firstCompiler, err := newCompiler(registry, readFile, tracker.Lookup)
	if err != nil {
		registry.Close()
		return nil, nil, nil, err
	}
	firstResult := run(firstCompiler)
	if firstErr := firstPassDiagnosticsError(firstResult.FileSet, firstResult.Diagnostics); firstErr != nil {
		registry.Close()
		return nil, nil, nil, firstErr
	}

	firstFile := &File{Path: path, Result: firstResult}
	declarations, err := PluginDeclarations(firstFile)
	if err != nil {
		registry.Close()
		return nil, nil, nil, err
	}
	for _, item := range declarations {
		if declareErr := registry.Declare(context.Background(), item.Declaration); declareErr != nil {
			registry.Close()
			return nil, nil, nil, dslErrorAt(firstResult.FileSet, item.Pos, "declare plugin %q: %v", item.Declaration.Namespace, declareErr)
		}
	}

	fullCompiler, err := newCompiler(registry, readFile, tracker.Lookup)
	if err != nil {
		registry.Close()
		return nil, nil, nil, err
	}
	fullResult := run(fullCompiler)
	if err := diagnosticsError(fullResult.FileSet, fullResult.Diagnostics); err != nil {
		registry.Close()
		return nil, nil, nil, err
	}

	return &File{Path: path, Result: fullResult}, registry, tracker.Values(), nil
}
