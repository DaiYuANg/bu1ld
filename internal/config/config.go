package config

import (
	"os"
	"path/filepath"

	"github.com/arcgolabs/collectionx"
	"github.com/arcgolabs/configx"
	"github.com/samber/oops"
)

type Config struct {
	WorkDir   string
	BuildFile string
	CacheDir  string
	NoCache   bool
	LogLevel  string
}

func New(workDir string, buildFile string, cacheDir string, noCache bool) (Config, error) {
	if workDir == "" {
		workDir = "."
	}
	if buildFile == "" {
		buildFile = "build.bu1ld"
	}
	if cacheDir == "" {
		cacheDir = ".bu1ld/cache"
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return Config{}, oops.In("bu1ld.config").
			With("work_dir", workDir).
			Wrapf(err, "resolve project directory")
	}

	loaded, err := configx.LoadConfig(
		configx.WithDefaults(map[string]any{
			"build_file": buildFile,
			"cache_dir":  cacheDir,
			"no_cache":   noCache,
			"log_level":  "info",
		}),
		configx.WithFiles(existingConfigFiles(absWorkDir).Values()...),
		configx.WithEnvPrefix("BU1LD"),
		configx.WithEnvSeparator("__"),
		configx.WithPriority(configx.SourceFile, configx.SourceEnv),
	)
	if err != nil {
		return Config{}, oops.In("bu1ld.config").
			With("work_dir", absWorkDir).
			Wrapf(err, "load configx sources")
	}

	return Config{
		WorkDir:   absWorkDir,
		BuildFile: loaded.GetString("build_file"),
		CacheDir:  loaded.GetString("cache_dir"),
		NoCache:   loaded.GetBool("no_cache"),
		LogLevel:  loaded.GetString("log_level"),
	}, nil
}

func (c Config) BuildFilePath() string {
	buildFile := c.BuildFile
	if buildFile == "" {
		buildFile = "build.bu1ld"
	}
	if filepath.IsAbs(buildFile) {
		return buildFile
	}
	return filepath.Join(c.WorkDir, buildFile)
}

func (c Config) CachePath() string {
	cacheDir := c.CacheDir
	if cacheDir == "" {
		cacheDir = ".bu1ld/cache"
	}
	if filepath.IsAbs(cacheDir) {
		return cacheDir
	}
	return filepath.Join(c.WorkDir, cacheDir)
}

func (c Config) StateDir() string {
	return filepath.Join(c.WorkDir, ".bu1ld")
}

func (c Config) LogPath() string {
	return filepath.Join(c.StateDir(), "bu1ld.log")
}

func existingConfigFiles(workDir string) collectionx.List[string] {
	candidates := collectionx.NewList[string](
		"bu1ld.yaml",
		"bu1ld.yml",
		"bu1ld.toml",
		"bu1ld.json",
		".bu1ld.yaml",
		".bu1ld.yml",
		".bu1ld.toml",
		".bu1ld.json",
	)
	files := collectionx.NewList[string]()
	candidates.Range(func(_ int, candidate string) bool {
		path := filepath.Join(workDir, candidate)
		if _, err := os.Stat(path); err == nil {
			files.Add(path)
		}
		return true
	})
	return files
}
