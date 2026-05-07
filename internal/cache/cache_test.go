package cache

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	"bu1ld/internal/snapshot"

	"github.com/spf13/afero"
)

func TestStoreSaveLoadRestoreBinaryRecord(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	workDir := "/workspace"
	outputDir := filepath.Join(workDir, "dist")
	if err := fs.MkdirAll(outputDir, 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := afero.WriteFile(fs, filepath.Join(outputDir, "artifact.txt"), []byte("artifact"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewStore(config.Config{WorkDir: workDir}, fs)
	task := build.NewTask("build")
	task.Outputs.Add("dist")

	if err := store.Save(task, "abc123"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	recordPath := store.recordPath("abc123")
	if got, want := filepath.Ext(recordPath), ".bin"; got != want {
		t.Fatalf("record extension = %q, want %q", got, want)
	}

	data, err := afero.ReadFile(fs, recordPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if bytes.HasPrefix(data, []byte("{")) {
		t.Fatalf("record still looks like JSON")
	}

	record, hit, err := store.Load("abc123")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !hit {
		t.Fatalf("Load() hit = false, want true")
	}
	if got, want := len(record.Outputs), 1; got != want {
		t.Fatalf("len(record.Outputs) = %d, want %d", got, want)
	}

	if removeErr := fs.RemoveAll(outputDir); removeErr != nil {
		t.Fatalf("RemoveAll() error = %v", removeErr)
	}
	if restoreErr := store.Restore(record); restoreErr != nil {
		t.Fatalf("Restore() error = %v", restoreErr)
	}

	restored, err := afero.ReadFile(fs, filepath.Join(outputDir, "artifact.txt"))
	if err != nil {
		t.Fatalf("ReadFile() restored error = %v", err)
	}
	if got, want := string(restored), "artifact"; got != want {
		t.Fatalf("restored file = %q, want %q", got, want)
	}
}

func TestStoreRemotePushPullRestore(t *testing.T) {
	t.Parallel()

	serverStore := NewStore(config.Config{WorkDir: "/server"}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(serverStore))
	defer server.Close()

	pushFS := afero.NewMemMapFs()
	pushWorkDir := "/push"
	outputDir := filepath.Join(pushWorkDir, "dist")
	if err := pushFS.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := afero.WriteFile(pushFS, filepath.Join(outputDir, "artifact.txt"), []byte("remote artifact"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	actionKey := strings.Repeat("a", 64)
	task := build.NewTask("build")
	task.Outputs.Add("dist")

	pushStore := NewStore(config.Config{
		WorkDir:         pushWorkDir,
		RemoteCacheURL:  server.URL,
		RemoteCachePush: true,
	}, pushFS)
	if err := pushStore.Save(task, actionKey); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	pullFS := afero.NewMemMapFs()
	pullWorkDir := "/pull"
	pullStore := NewStore(config.Config{
		WorkDir:         pullWorkDir,
		RemoteCacheURL:  server.URL,
		RemoteCachePull: true,
	}, pullFS)

	record, hit, err := pullStore.Load(actionKey)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !hit {
		t.Fatalf("Load() hit = false, want true")
	}
	if err := pullStore.Restore(record); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restored, err := afero.ReadFile(pullFS, filepath.Join(pullWorkDir, "dist", "artifact.txt"))
	if err != nil {
		t.Fatalf("ReadFile() restored error = %v", err)
	}
	if got, want := string(restored), "remote artifact"; got != want {
		t.Fatalf("restored file = %q, want %q", got, want)
	}
}

func TestRemoteCacheRejectsBlobDigestMismatch(t *testing.T) {
	t.Parallel()

	store := NewStore(config.Config{WorkDir: "/server"}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(store))
	defer server.Close()

	digest := snapshot.HashBytes([]byte("expected"))
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/blobs/"+digest, strings.NewReader("actual"))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
