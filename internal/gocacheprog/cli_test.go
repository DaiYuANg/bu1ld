package gocacheprog

import "testing"

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv("BU1LD_REMOTE_CACHE__URL", "http://127.0.0.1:19876")
	t.Setenv("BU1LD_GO__CACHEPROG_CACHE_DIR", "local-cache")
	t.Setenv("BU1LD_REMOTE_CACHE__PULL", "off")
	t.Setenv("BU1LD_REMOTE_CACHE__PUSH", "yes")

	options := OptionsFromEnv()
	if got, want := options.RemoteCacheURL, "http://127.0.0.1:19876"; got != want {
		t.Fatalf("RemoteCacheURL = %q, want %q", got, want)
	}
	if got, want := options.CacheDir, "local-cache"; got != want {
		t.Fatalf("CacheDir = %q, want %q", got, want)
	}
	if options.RemotePull {
		t.Fatalf("RemotePull = true, want false")
	}
	if !options.RemotePush {
		t.Fatalf("RemotePush = false, want true")
	}
}
