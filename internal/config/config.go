package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/configx"
	"github.com/samber/oops"
)

type Config struct {
	WorkDir                     string
	Env                         string
	BuildFile                   string
	CacheDir                    string
	NoCache                     bool
	LogLevel                    string
	RemoteCacheURL              string
	RemoteCachePull             bool
	RemoteCachePush             bool
	RemoteCacheToken            string
	RemoteCacheMaxBytes         int64
	RemoteCacheMaxObjectBytes   int64
	RemoteCacheMaxAge           time.Duration
	ServerCoordinatorListenAddr string
	PluginRegistrySource        string
}

func New(
	workDir string,
	buildFile string,
	cacheDir string,
	noCache bool,
	remoteCacheURL string,
	remoteCachePull bool,
	remoteCachePush bool,
) (Config, error) {
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

	configFiles := existingConfigFiles(absWorkDir).Values()
	envName, err := selectedEnv(configFiles)
	if err != nil {
		return Config{}, oops.In("bu1ld.config").
			With("work_dir", absWorkDir).
			Wrapf(err, "resolve selected environment")
	}

	loaded, err := configx.LoadConfig(
		configx.WithDefaults(map[string]any{
			"env":                            envName,
			"build_file":                     buildFile,
			"cache_dir":                      cacheDir,
			"no_cache":                       noCache,
			"log_level":                      "info",
			"remote_cache_url":               remoteCacheURL,
			"remote_cache_pull":              remoteCachePull,
			"remote_cache_push":              remoteCachePush,
			"remote_cache.token":             "",
			"remote_cache.max_bytes":         0,
			"remote_cache.max_object_bytes":  0,
			"remote_cache.max_age":           "",
			"server.coordinator.listen_addr": "127.0.0.1:19876",
			"plugin_registry":                "",
		}),
		configx.WithDotenv(dotenvFiles(absWorkDir, envName).Values()...),
		configx.WithFiles(configFiles...),
		configx.WithEnvPrefix("BU1LD"),
		configx.WithEnvSeparator("__"),
		configx.WithPriority(configx.SourceDotenv, configx.SourceFile, configx.SourceEnv),
	)
	if err != nil {
		return Config{}, oops.In("bu1ld.config").
			With("work_dir", absWorkDir).
			Wrapf(err, "load configx sources")
	}

	return Config{
		WorkDir:                     absWorkDir,
		Env:                         loaded.GetString("env"),
		BuildFile:                   loaded.GetString("build_file"),
		CacheDir:                    loaded.GetString("cache_dir"),
		NoCache:                     loaded.GetBool("no_cache"),
		LogLevel:                    loaded.GetString("log_level"),
		RemoteCacheURL:              configString(loaded, "remote_cache.url", "remote_cache_url"),
		RemoteCachePull:             configBool(loaded, "remote_cache.pull", "remote_cache_pull"),
		RemoteCachePush:             configBool(loaded, "remote_cache.push", "remote_cache_push"),
		RemoteCacheToken:            configString(loaded, "remote_cache.token", "remote_cache_token"),
		RemoteCacheMaxBytes:         configInt64(loaded, "remote_cache.max_bytes", "remote_cache_max_bytes"),
		RemoteCacheMaxObjectBytes:   configInt64(loaded, "remote_cache.max_object_bytes", "remote_cache_max_object_bytes"),
		RemoteCacheMaxAge:           configDuration(loaded, "remote_cache.max_age", "remote_cache_max_age"),
		ServerCoordinatorListenAddr: loaded.GetString("server.coordinator.listen_addr"),
		PluginRegistrySource:        configString(loaded, "plugin_registry.source", "plugin_registry", "plugin_registry_source"),
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

func (c Config) ChildEnv() []string {
	return []string{
		"BU1LD_ENV=" + c.Env,
		"BU1LD_REMOTE_CACHE__URL=" + c.RemoteCacheURL,
		"BU1LD_REMOTE_CACHE_URL=" + c.RemoteCacheURL,
		"BU1LD_REMOTE_CACHE__PULL=" + strconv.FormatBool(c.RemoteCachePull),
		"BU1LD_REMOTE_CACHE_PULL=" + strconv.FormatBool(c.RemoteCachePull),
		"BU1LD_REMOTE_CACHE__PUSH=" + strconv.FormatBool(c.RemoteCachePush),
		"BU1LD_REMOTE_CACHE_PUSH=" + strconv.FormatBool(c.RemoteCachePush),
		"BU1LD_REMOTE_CACHE__TOKEN=" + c.RemoteCacheToken,
		"BU1LD_REMOTE_CACHE_TOKEN=" + c.RemoteCacheToken,
		"BU1LD_SERVER__COORDINATOR__LISTEN_ADDR=" + c.ServerCoordinatorListenAddr,
	}
}

func existingConfigFiles(workDir string) *list.List[string] {
	candidates := list.NewList[string](
		"bu1ld.yaml",
		"bu1ld.yml",
		"bu1ld.toml",
		"bu1ld.json",
		".bu1ld.yaml",
		".bu1ld.yml",
		".bu1ld.toml",
		".bu1ld.json",
	)
	files := list.NewList[string]()
	candidates.Range(func(_ int, candidate string) bool {
		path := filepath.Join(workDir, candidate)
		if _, err := os.Stat(path); err == nil {
			files.Add(path)
		}
		return true
	})
	return files
}

func selectedEnv(configFiles []string) (string, error) {
	loaded, err := configx.LoadConfig(
		configx.WithDefaults(map[string]any{"env": ""}),
		configx.WithFiles(configFiles...),
		configx.WithEnvPrefix("BU1LD"),
		configx.WithEnvSeparator("__"),
		configx.WithPriority(configx.SourceFile, configx.SourceEnv),
	)
	if err != nil {
		return "", err
	}
	return loaded.GetString("env"), nil
}

func dotenvFiles(workDir, envName string) *list.List[string] {
	files := list.NewList[string]()
	if envName != "" {
		files.Add(
			filepath.Join(workDir, ".env."+envName+".local"),
			filepath.Join(workDir, ".env."+envName),
		)
	}
	files.Add(
		filepath.Join(workDir, ".env.local"),
		filepath.Join(workDir, ".env"),
	)
	return files
}

func configString(loaded *configx.Config, paths ...string) string {
	for _, path := range paths {
		if loaded.Exists(path) {
			return loaded.GetString(path)
		}
	}
	return ""
}

func configBool(loaded *configx.Config, paths ...string) bool {
	for _, path := range paths {
		if loaded.Exists(path) {
			return loaded.GetBool(path)
		}
	}
	return false
}

func configInt64(loaded *configx.Config, paths ...string) int64 {
	for _, path := range paths {
		if !loaded.Exists(path) {
			continue
		}
		value := strings.TrimSpace(loaded.GetString(path))
		if value == "" {
			return 0
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
		return 0
	}
	return 0
}

func configDuration(loaded *configx.Config, paths ...string) time.Duration {
	for _, path := range paths {
		if !loaded.Exists(path) {
			continue
		}
		value := strings.TrimSpace(loaded.GetString(path))
		if value == "" {
			return 0
		}
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
		return 0
	}
	return 0
}
