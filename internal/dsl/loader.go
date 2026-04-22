package dsl

import (
	"os"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	buildplugin "bu1ld/internal/plugin"

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
	path := l.cfg.BuildFilePath()
	file, err := os.Open(path)
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "open build file")
	}
	defer file.Close()

	project, err := l.parser.ParseWithOptions(file, buildplugin.LoadOptions{ProjectDir: l.cfg.WorkDir})
	if err != nil {
		return build.Project{}, oops.In("bu1ld.dsl").
			With("file", path).
			Wrapf(err, "parse build file")
	}
	return project, nil
}
