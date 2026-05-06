package dsl

import (
	"context"
	"fmt"
	"io"

	"bu1ld/internal/build"
	buildplugin "bu1ld/internal/plugin"
	"bu1ld/internal/plugins/archive"
	"bu1ld/internal/plugins/docker"
	gitplugin "bu1ld/internal/plugins/git"
	"bu1ld/internal/plugins/golang"

	planocomp "github.com/arcgolabs/plano/compiler"
)

type Parser struct {
	registry *buildplugin.Registry
}

func NewParser() *Parser {
	registry, err := buildplugin.NewRegistry(
		buildplugin.LoadOptions{},
		golang.New(),
		docker.New(),
		archive.New(),
		gitplugin.New(),
	)
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
	return p.ParseContext(context.Background(), reader)
}

func (p *Parser) ParseContext(ctx context.Context, reader io.Reader) (build.Project, error) {
	return p.ParseWithOptionsContext(ctx, reader, buildplugin.LoadOptions{})
}

func (p *Parser) ParseWithOptions(reader io.Reader, options buildplugin.LoadOptions) (build.Project, error) {
	return p.ParseWithOptionsContext(context.Background(), reader, options)
}

func (p *Parser) ParseWithOptionsContext(ctx context.Context, reader io.Reader, options buildplugin.LoadOptions) (build.Project, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return build.Project{}, fmt.Errorf("read DSL source: %w", err)
	}

	file, registry, _, err := p.compileSourceDetailed(ctx, "dsl", data, options)
	if err != nil {
		return build.Project{}, err
	}
	defer registry.Close()

	project, err := lowerProject(ctx, file, registry)
	if err != nil {
		return build.Project{}, err
	}
	return project, nil
}

func (p *Parser) ParseFile(source string) (*File, error) {
	file, registry, _, err := p.compileSourceDetailed(context.Background(), "dsl", []byte(source), buildplugin.LoadOptions{})
	if registry != nil {
		registry.Close()
	}
	return file, err
}

func (p *Parser) compileSourceDetailed(
	ctx context.Context,
	filename string,
	src []byte,
	options buildplugin.LoadOptions,
) (*File, *buildplugin.Registry, []envDependency, error) {
	return p.compileDetailed(ctx, filename, options, nil, func(compiler *planocomp.Compiler) planocomp.Result {
		return compiler.CompileSourceDetailed(ctx, filename, src)
	})
}

func (p *Parser) compilePathDetailed(
	ctx context.Context,
	path string,
	options buildplugin.LoadOptions,
	readFile func(string) ([]byte, error),
) (*File, *buildplugin.Registry, []envDependency, error) {
	return p.compileDetailed(ctx, path, options, readFile, func(compiler *planocomp.Compiler) planocomp.Result {
		return compiler.CompileFileDetailed(ctx, path)
	})
}

func (p *Parser) compileDetailed(
	ctx context.Context,
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
		if declareErr := registry.Declare(ctx, item.Declaration); declareErr != nil {
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
