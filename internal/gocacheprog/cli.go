package gocacheprog

import (
	"os"
	"strconv"
	"strings"
)

func OptionsFromEnv() Options {
	return Options{
		RemoteCacheURL: firstEnv("BU1LD_GO_CACHEPROG_REMOTE_CACHE_URL", "BU1LD_REMOTE_CACHE__URL", "BU1LD_REMOTE_CACHE_URL"),
		RemoteCacheToken: firstEnv(
			"BU1LD_GO_CACHEPROG_REMOTE_CACHE_TOKEN",
			"BU1LD_REMOTE_CACHE__TOKEN",
			"BU1LD_REMOTE_CACHE_TOKEN",
		),
		CacheDir:   firstEnv("BU1LD_GO_CACHEPROG_CACHE_DIR", "BU1LD_GO__CACHEPROG_CACHE_DIR"),
		RemotePull: envBool(true, "BU1LD_REMOTE_CACHE__PULL", "BU1LD_REMOTE_CACHE_PULL"),
		RemotePush: envBool(false, "BU1LD_REMOTE_CACHE__PUSH", "BU1LD_REMOTE_CACHE_PUSH"),
	}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func envBool(fallback bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
		switch strings.ToLower(value) {
		case "1", "yes", "y", "on":
			return true
		case "0", "no", "n", "off":
			return false
		}
	}
	return fallback
}
