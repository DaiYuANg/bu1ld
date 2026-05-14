package gocacheprog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/lyonbrown4d/bu1ld/internal/cache"
	"github.com/lyonbrown4d/bu1ld/internal/config"

	"github.com/spf13/afero"
)

func TestServePutGetAgainstRemoteCoordinator(t *testing.T) {
	t.Parallel()

	store := cache.NewStore(config.Config{WorkDir: "/server"}, afero.NewMemMapFs())
	coordinator := httptest.NewServer(cache.NewHTTPHandler(store))
	defer coordinator.Close()

	actionID := bytes.Repeat([]byte{1}, 32)
	body := []byte("go object")
	outputID := outputHash(body)

	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	if err := encoder.Encode(Request{
		ID:       1,
		Command:  CmdPut,
		ActionID: actionID,
		OutputID: outputID,
		BodySize: int64(len(body)),
	}); err != nil {
		t.Fatalf("Encode(put) error = %v", err)
	}
	if err := encoder.Encode(body); err != nil {
		t.Fatalf("Encode(body) error = %v", err)
	}
	if err := encoder.Encode(Request{ID: 2, Command: CmdGet, ActionID: actionID}); err != nil {
		t.Fatalf("Encode(get) error = %v", err)
	}
	if err := encoder.Encode(Request{ID: 3, Command: CmdClose}); err != nil {
		t.Fatalf("Encode(close) error = %v", err)
	}

	var output bytes.Buffer
	err := Serve(context.Background(), &input, &output, Options{
		RemoteCacheURL: coordinator.URL,
		CacheDir:       t.TempDir(),
		RemotePull:     true,
		RemotePush:     true,
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	decoder := json.NewDecoder(&output)
	var capabilities Response
	if err := decoder.Decode(&capabilities); err != nil {
		t.Fatalf("Decode(capabilities) error = %v", err)
	}
	if got, want := capabilities.ID, int64(0); got != want {
		t.Fatalf("capabilities ID = %d, want %d", got, want)
	}
	if len(capabilities.KnownCommands) != 3 {
		t.Fatalf("KnownCommands = %#v, want 3 commands", capabilities.KnownCommands)
	}

	var put Response
	if err := decoder.Decode(&put); err != nil {
		t.Fatalf("Decode(put) error = %v", err)
	}
	if put.Err != "" {
		t.Fatalf("put Err = %q", put.Err)
	}
	if put.DiskPath == "" {
		t.Fatalf("put DiskPath is empty")
	}

	var get Response
	if err := decoder.Decode(&get); err != nil {
		t.Fatalf("Decode(get) error = %v", err)
	}
	if get.Err != "" {
		t.Fatalf("get Err = %q", get.Err)
	}
	if get.Miss {
		t.Fatalf("get Miss = true, want false")
	}
	if !bytes.Equal(get.OutputID, outputID) {
		t.Fatalf("get OutputID = %x, want %x", get.OutputID, outputID)
	}
	if get.Size != int64(len(body)) {
		t.Fatalf("get Size = %d, want %d", get.Size, len(body))
	}
	if get.DiskPath == "" {
		t.Fatalf("get DiskPath is empty")
	}

	var close Response
	if err := decoder.Decode(&close); err != nil {
		t.Fatalf("Decode(close) error = %v", err)
	}
	if close.ID != 3 || close.Err != "" {
		t.Fatalf("close response = %#v", close)
	}

	remote := cache.NewRemoteClient(coordinator.URL)
	entry, hit, err := remote.GetGoCacheEntry(hex.EncodeToString(actionID))
	if err != nil {
		t.Fatalf("GetGoCacheEntry() error = %v", err)
	}
	if !hit {
		t.Fatalf("GetGoCacheEntry() hit = false, want true")
	}
	if got, want := entry.OutputID, hex.EncodeToString(outputID); got != want {
		t.Fatalf("remote OutputID = %q, want %q", got, want)
	}
}

func outputHash(body []byte) []byte {
	digest := sha256.Sum256(body)
	return digest[:]
}
