package dsl

import (
	"fmt"
	"path/filepath"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"

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

	filePaths, imports, err := l.scanImportGraph()
	if err != nil {
		return build.Project{}, err
	}
	file, registry, envs, err := l.loadCompiledFile()
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "parse build file")
	}
	defer registry.Close()
	project, err := lowerProject(file, registry)
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", l.cfg.BuildFilePath()).
			Wrapf(err, "evaluate build file")
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
	file, registry, _, err := l.loadCompiledFile()
	if registry != nil {
		registry.Close()
	}
	return file, err
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

func (l *Loader) loadCompiledFile() (*File, *buildplugin.Registry, []envDependency, error) {
	buildFile, err := cleanAbsPath(l.cfg.BuildFilePath())
	if err != nil {
		return nil, nil, nil, err
	}
	readFile := func(path string) ([]byte, error) {
		data, readErr := afero.ReadFile(l.fs, path)
		if readErr != nil {
			return nil, fmt.Errorf("read %q: %w", path, readErr)
		}
		return data, nil
	}
	return l.parser.compilePathDetailed(buildFile, l.LoadOptions(), readFile)
}
