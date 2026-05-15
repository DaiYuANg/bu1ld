package daemon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lyonbrown4d/bu1ld/internal/app"
	"github.com/lyonbrown4d/bu1ld/internal/config"
)

func TestServeCheckTryRunStop(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, cfg, io.Discard, func(_ context.Context, _ config.Config, out io.Writer, request app.CommandRequest) error {
			_, err := fmt.Fprintf(out, "proxied %s\n", request.Kind)
			return err
		})
	}()

	status := waitForRunning(t, ctx, cfg)
	if !status.Running {
		t.Fatalf("status.Running = false")
	}
	if status.State.WorkDir != cfg.WorkDir {
		t.Fatalf("status work dir = %q, want %q", status.State.WorkDir, cfg.WorkDir)
	}

	var out bytes.Buffer
	ran, err := TryRun(ctx, cfg, &out, app.CommandRequest{Kind: app.CommandTasks})
	if err != nil {
		t.Fatalf("TryRun() error = %v", err)
	}
	if !ran {
		t.Fatalf("TryRun() ran = false")
	}
	if got := out.String(); !strings.Contains(got, "proxied tasks") {
		t.Fatalf("output = %q, want proxied tasks", got)
	}

	if _, stopped, err := Stop(ctx, cfg); err != nil {
		t.Fatalf("Stop() error = %v", err)
	} else if !stopped {
		t.Fatalf("Stop() stopped = false")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not stop")
	}
}

func TestCheckStopped(t *testing.T) {
	t.Parallel()

	status, err := Check(context.Background(), testConfig(t))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if status.Running {
		t.Fatalf("status.Running = true")
	}
	if got := FormatStatus(status); !strings.Contains(got, "stopped") {
		t.Fatalf("FormatStatus() = %q, want stopped", got)
	}
}

func waitForRunning(t *testing.T, ctx context.Context, cfg config.Config) Status {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := Check(ctx, cfg)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if status.Running {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("daemon did not become ready")
	return Status{}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()

	root := t.TempDir()
	return config.Config{
		WorkDir:         root,
		BuildFile:       "build.bu1ld",
		CacheDir:        ".bu1ld/cache",
		RemoteCachePull: true,
	}
}
