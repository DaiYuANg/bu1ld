package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"bu1ld/internal/gocacheprog"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	options := gocacheprog.Options{
		RemoteCacheURL: firstEnv("BU1LD_GO_CACHEPROG_REMOTE_CACHE_URL", "BU1LD_REMOTE_CACHE__URL", "BU1LD_REMOTE_CACHE_URL"),
		CacheDir:       firstEnv("BU1LD_GO_CACHEPROG_CACHE_DIR", "BU1LD_GO__CACHEPROG_CACHE_DIR"),
		RemotePull:     envBool(true, "BU1LD_REMOTE_CACHE__PULL", "BU1LD_REMOTE_CACHE_PULL"),
		RemotePush:     envBool(false, "BU1LD_REMOTE_CACHE__PUSH", "BU1LD_REMOTE_CACHE_PUSH"),
	}

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&options.RemoteCacheURL, "remote-cache-url", options.RemoteCacheURL, "bu1ld coordinator remote cache URL")
	flags.StringVar(&options.RemoteCacheURL, "url", options.RemoteCacheURL, "bu1ld coordinator remote cache URL")
	flags.StringVar(&options.CacheDir, "cache-dir", options.CacheDir, "local GOCACHEPROG disk path")
	flags.BoolVar(&options.RemotePull, "remote-cache-pull", options.RemotePull, "pull Go cache entries from the coordinator")
	flags.BoolVar(&options.RemotePush, "remote-cache-push", options.RemotePush, "push Go cache entries to the coordinator")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}

	return gocacheprog.Serve(context.Background(), os.Stdin, os.Stdout, options)
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
