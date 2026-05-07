package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLoadsDistributedCacheFromEnvironment(t *testing.T) {
	restoreEnv(t,
		"BU1LD_REMOTE_CACHE__URL",
		"BU1LD_REMOTE_CACHE__PUSH",
		"BU1LD_SERVER__COORDINATOR__LISTEN_ADDR",
	)

	workDir := t.TempDir()
	t.Setenv("BU1LD_REMOTE_CACHE__URL", "http://192.168.1.10:19876")
	t.Setenv("BU1LD_REMOTE_CACHE__PUSH", "true")
	t.Setenv("BU1LD_SERVER__COORDINATOR__LISTEN_ADDR", "0.0.0.0:19876")

	cfg, err := New(workDir, "", "", false, "", true, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got, want := cfg.RemoteCacheURL, "http://192.168.1.10:19876"; got != want {
		t.Fatalf("RemoteCacheURL = %q, want %q", got, want)
	}
	if !cfg.RemoteCachePush {
		t.Fatalf("RemoteCachePush = false, want true")
	}
	if got, want := cfg.ServerCoordinatorListenAddr, "0.0.0.0:19876"; got != want {
		t.Fatalf("ServerCoordinatorListenAddr = %q, want %q", got, want)
	}
}

func TestNewUsesConfiguredEnvDotenv(t *testing.T) {
	restoreEnv(t, "BU1LD_REMOTE_CACHE__URL")

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "bu1ld.toml"), []byte(`
env = "lan"
`), 0o644); err != nil {
		t.Fatalf("WriteFile(bu1ld.toml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".env.lan"), []byte(`
BU1LD_REMOTE_CACHE__URL=http://10.0.0.5:19876
`), 0o644); err != nil {
		t.Fatalf("WriteFile(.env.lan) error = %v", err)
	}

	cfg, err := New(workDir, "", "", false, "", true, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got, want := cfg.RemoteCacheURL, "http://10.0.0.5:19876"; got != want {
		t.Fatalf("RemoteCacheURL = %q, want %q", got, want)
	}
	if got, want := cfg.Env, "lan"; got != want {
		t.Fatalf("Env = %q, want %q", got, want)
	}
}

func restoreEnv(t *testing.T, keys ...string) {
	t.Helper()

	values := map[string]string{}
	present := map[string]bool{}
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		values[key] = value
		present[key] = ok
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%s) error = %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, values[key])
				continue
			}
			_ = os.Unsetenv(key)
		}
	})
}
