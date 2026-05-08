package cache

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestRemoteCacheRequiresBearerToken(t *testing.T) {
	t.Parallel()

	store := NewStore(config.Config{WorkDir: "/server", RemoteCacheToken: "secret"}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(store))
	defer server.Close()

	body := []byte("authorized")
	digest := snapshot.HashBytes(body)
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/blobs/"+digest, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()
	if got, want := resp.StatusCode, http.StatusUnauthorized; got != want {
		t.Fatalf("status without token = %d, want %d", got, want)
	}

	client := NewRemoteClientWithToken(server.URL, "secret")
	if err := client.PutBlob(digest, body); err != nil {
		t.Fatalf("PutBlob() error = %v", err)
	}
	got, hit, err := client.GetBlob(digest)
	if err != nil {
		t.Fatalf("GetBlob() error = %v", err)
	}
	if !hit || !bytes.Equal(got, body) {
		t.Fatalf("GetBlob() = %q, %v; want %q, true", got, hit, body)
	}
}

func TestRemoteCacheRejectsObjectsOverConfiguredLimit(t *testing.T) {
	t.Parallel()

	store := NewStore(config.Config{WorkDir: "/server", RemoteCacheMaxObjectBytes: 3}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(store))
	defer server.Close()

	body := []byte("toolong")
	digest := snapshot.HashBytes(body)
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/blobs/"+digest, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()
	if got, want := resp.StatusCode, http.StatusRequestEntityTooLarge; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestStorePruneRemovesOldestFilesToCapacity(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	store := NewStore(config.Config{WorkDir: "/workspace"}, fs)
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	oldDigest := strings.Repeat("1", 64)
	newDigest := strings.Repeat("2", 64)
	writeCacheFile(t, fs, store.blobPath(oldDigest), []byte("old cache object"), now.Add(-2*time.Hour))
	writeCacheFile(t, fs, store.blobPath(newDigest), []byte("new"), now)

	result, err := store.Prune(PruneOptions{MaxBytes: 8, Now: now})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got, want := result.FilesRemoved, 1; got != want {
		t.Fatalf("FilesRemoved = %d, want %d", got, want)
	}
	if _, err := fs.Stat(store.blobPath(oldDigest)); !os.IsNotExist(err) {
		t.Fatalf("old cache object still exists or stat failed with %v", err)
	}
	if _, err := fs.Stat(store.blobPath(newDigest)); err != nil {
		t.Fatalf("new cache object missing: %v", err)
	}
}

func TestRemoteCacheHandlesConcurrentBlobTraffic(t *testing.T) {
	t.Parallel()

	store := NewStore(config.Config{WorkDir: "/server"}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(store))
	defer server.Close()
	client := NewRemoteClient(server.URL)

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte(strings.Repeat(string(rune('a'+i%26)), i+1))
			digest := snapshot.HashBytes(body)
			if err := client.PutBlob(digest, body); err != nil {
				errs <- err
				return
			}
			got, hit, err := client.GetBlob(digest)
			if err != nil {
				errs <- err
				return
			}
			if !hit || !bytes.Equal(got, body) {
				errs <- io.ErrUnexpectedEOF
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent remote cache operation error = %v", err)
		}
	}
}

func TestGoCacheHTTPRoutesStoreActionAndOutput(t *testing.T) {
	t.Parallel()

	store := NewStore(config.Config{WorkDir: "/server"}, afero.NewMemMapFs())
	server := httptest.NewServer(NewHTTPHandler(store))
	defer server.Close()

	actionID := strings.Repeat("1", 64)
	body := []byte("compiled go package")
	outputID := snapshot.HashBytes(body)

	action := GoCacheEntry{
		ActionID: actionID,
		OutputID: outputID,
		Size:     int64(len(body)),
	}
	encodedAction, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	missingOutputReq, err := http.NewRequest(http.MethodPut, server.URL+"/v1/go/cache/actions/"+actionID, bytes.NewReader(encodedAction))
	if err != nil {
		t.Fatalf("NewRequest(action missing output) error = %v", err)
	}
	missingOutputReq.Header.Set("Content-Type", "application/json")
	missingOutputResp, err := http.DefaultClient.Do(missingOutputReq)
	if err != nil {
		t.Fatalf("Do(action missing output) error = %v", err)
	}
	missingOutputResp.Body.Close()
	if got, want := missingOutputResp.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("missing output status = %d, want %d", got, want)
	}

	outputReq, err := http.NewRequest(http.MethodPut, server.URL+"/v1/go/cache/outputs/"+outputID, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(output) error = %v", err)
	}
	outputReq.Header.Set("Content-Type", "application/octet-stream")
	outputResp, err := http.DefaultClient.Do(outputReq)
	if err != nil {
		t.Fatalf("Do(output) error = %v", err)
	}
	outputResp.Body.Close()
	if got, want := outputResp.StatusCode, http.StatusCreated; got != want {
		t.Fatalf("output status = %d, want %d", got, want)
	}

	actionReq, err := http.NewRequest(http.MethodPut, server.URL+"/v1/go/cache/actions/"+actionID, bytes.NewReader(encodedAction))
	if err != nil {
		t.Fatalf("NewRequest(action) error = %v", err)
	}
	actionReq.Header.Set("Content-Type", "application/json")
	actionResp, err := http.DefaultClient.Do(actionReq)
	if err != nil {
		t.Fatalf("Do(action) error = %v", err)
	}
	actionResp.Body.Close()
	if got, want := actionResp.StatusCode, http.StatusCreated; got != want {
		t.Fatalf("action status = %d, want %d", got, want)
	}

	getActionResp, err := http.Get(server.URL + "/v1/go/cache/actions/" + actionID)
	if err != nil {
		t.Fatalf("Get(action) error = %v", err)
	}
	defer getActionResp.Body.Close()
	if got, want := getActionResp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("get action status = %d, want %d", got, want)
	}
	var gotAction GoCacheEntry
	if err := json.NewDecoder(getActionResp.Body).Decode(&gotAction); err != nil {
		t.Fatalf("Decode(action) error = %v", err)
	}
	if got, want := gotAction.OutputID, outputID; got != want {
		t.Fatalf("OutputID = %q, want %q", got, want)
	}
	if got, want := gotAction.Size, int64(len(body)); got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}

	getOutputResp, err := http.Get(server.URL + "/v1/go/cache/outputs/" + outputID)
	if err != nil {
		t.Fatalf("Get(output) error = %v", err)
	}
	defer getOutputResp.Body.Close()
	if got, want := getOutputResp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("get output status = %d, want %d", got, want)
	}
	gotBody, err := io.ReadAll(getOutputResp.Body)
	if err != nil {
		t.Fatalf("ReadAll(output) error = %v", err)
	}
	if !bytes.Equal(gotBody, body) {
		t.Fatalf("output body = %q, want %q", gotBody, body)
	}
}

func writeCacheFile(t *testing.T, fs afero.Fs, path string, data []byte, modTime time.Time) {
	t.Helper()
	if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := afero.WriteFile(fs, path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	if err := fs.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(%s) error = %v", path, err)
	}
}
