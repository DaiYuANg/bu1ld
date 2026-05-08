package gocacheprog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bu1ld/internal/cache"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/mo"
)

type Options struct {
	RemoteCacheURL   string
	RemoteCacheToken string
	CacheDir         string
	RemotePull       bool
	RemotePush       bool
}

type Server struct {
	options Options
	client  *cache.RemoteClient
	mu      sync.Mutex
	entries *mapping.Map[string, localEntry]
}

type localEntry struct {
	entry    cache.GoCacheEntry
	diskPath string
}

func Serve(ctx context.Context, input io.Reader, output io.Writer, options Options) error {
	server, cleanup, err := NewServer(options)
	if err != nil {
		return err
	}
	defer cleanup()
	return server.Serve(ctx, input, output)
}

func NewServer(options Options) (*Server, func(), error) {
	cleanup := func() {}
	if options.CacheDir == "" {
		dir, err := os.MkdirTemp("", "bu1ld-go-plugin-cacheprog-*")
		if err != nil {
			return nil, cleanup, fmt.Errorf("create go cacheprog temp dir: %w", err)
		}
		options.CacheDir = dir
		cleanup = func() { _ = os.RemoveAll(dir) }
	}
	if err := os.MkdirAll(options.CacheDir, 0o755); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("create go cacheprog cache dir: %w", err)
	}

	var client *cache.RemoteClient
	if options.RemoteCacheURL != "" {
		client = cache.NewRemoteClientWithToken(options.RemoteCacheURL, options.RemoteCacheToken)
	}
	return &Server{
		options: options,
		client:  client,
		entries: mapping.NewMap[string, localEntry](),
	}, cleanup, nil
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	encoder := json.NewEncoder(output)

	if err := encoder.Encode(Response{
		ID:            0,
		KnownCommands: []Cmd{CmdGet, CmdPut, CmdClose},
	}); err != nil {
		return fmt.Errorf("write go cacheprog capabilities: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var request Request
		if err := decoder.Decode(&request); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode go cacheprog request: %w", err)
		}

		body, err := decodeBody(decoder, request)
		if err != nil {
			if encodeErr := encoder.Encode(Response{ID: request.ID, Err: err.Error()}); encodeErr != nil {
				return fmt.Errorf("write go cacheprog error response: %w", encodeErr)
			}
			continue
		}

		response := s.handle(ctx, request, body)
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("write go cacheprog response: %w", err)
		}
		if request.Command == CmdClose {
			return nil
		}
	}
}

func decodeBody(decoder *json.Decoder, request Request) ([]byte, error) {
	if request.BodySize == 0 {
		return nil, nil
	}
	var body []byte
	if err := decoder.Decode(&body); err != nil {
		return nil, fmt.Errorf("decode go cacheprog request body: %w", err)
	}
	if int64(len(body)) != request.BodySize {
		return nil, fmt.Errorf("go cacheprog body size mismatch: got %d, want %d", len(body), request.BodySize)
	}
	return body, nil
}

func (s *Server) handle(ctx context.Context, request Request, body []byte) Response {
	switch request.Command {
	case CmdGet:
		return s.handleGet(ctx, request)
	case CmdPut:
		return s.handlePut(request, body)
	case CmdClose:
		return Response{ID: request.ID}
	default:
		return Response{ID: request.ID, Err: fmt.Sprintf("unsupported go cacheprog command %q", request.Command)}
	}
}

func (s *Server) handleGet(ctx context.Context, request Request) Response {
	actionID, err := cacheKey(request.ActionID, "action id")
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}

	if entry, ok := s.localEntry(actionID); ok {
		return entryResponse(request.ID, entry.entry, entry.diskPath)
	}
	if s.client == nil || !s.options.RemotePull {
		return Response{ID: request.ID, Miss: true}
	}

	entry, hit, err := s.client.GetGoCacheEntry(actionID)
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	if !hit {
		return Response{ID: request.ID, Miss: true}
	}
	if ctx.Err() != nil {
		return Response{ID: request.ID, Err: ctx.Err().Error()}
	}
	body, hit, err := s.client.GetGoCacheOutput(entry.OutputID)
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	if !hit {
		return Response{ID: request.ID, Err: "remote go cache output is missing"}
	}
	if snapshot.HashBytes(body) != entry.OutputID {
		return Response{ID: request.ID, Err: "remote go cache output digest mismatch"}
	}

	diskPath, err := s.writeLocalOutput(entry.OutputID, body)
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	entry.ActionID = actionID
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	if entry.Size == 0 {
		entry.Size = int64(len(body))
	}
	s.setLocalEntry(actionID, entry, diskPath)
	return entryResponse(request.ID, entry, diskPath)
}

func (s *Server) handlePut(request Request, body []byte) Response {
	actionID, err := cacheKey(request.ActionID, "action id")
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	outputID, err := cacheKey(request.OutputID, "output id")
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	if request.BodySize != int64(len(body)) {
		return Response{ID: request.ID, Err: "go cacheprog put body size mismatch"}
	}
	if snapshot.HashBytes(body) != outputID {
		return Response{ID: request.ID, Err: "go cacheprog put output id mismatch"}
	}

	diskPath, err := s.writeLocalOutput(outputID, body)
	if err != nil {
		return Response{ID: request.ID, Err: err.Error()}
	}
	entry := cache.GoCacheEntry{
		ActionID: actionID,
		OutputID: outputID,
		Size:     int64(len(body)),
		Time:     time.Now().UTC(),
	}
	s.setLocalEntry(actionID, entry, diskPath)

	if s.client != nil && s.options.RemotePush {
		if err := s.client.PutGoCacheOutput(outputID, body); err != nil {
			return Response{ID: request.ID, Err: err.Error()}
		}
		if err := s.client.PutGoCacheEntry(actionID, entry); err != nil {
			return Response{ID: request.ID, Err: err.Error()}
		}
	}
	return Response{ID: request.ID, OutputID: request.OutputID, Size: entry.Size, Time: &entry.Time, DiskPath: diskPath}
}

func (s *Server) localEntry(actionID string) (localEntry, bool) {
	return s.localEntryOption(actionID).Get()
}

func (s *Server) localEntryOption(actionID string) mo.Option[localEntry] {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries.Get(actionID)
	if !ok || entry.diskPath == "" {
		return mo.None[localEntry]()
	}
	if _, err := os.Stat(entry.diskPath); err != nil {
		s.entries.Delete(actionID)
		return mo.None[localEntry]()
	}
	return mo.Some(entry)
}

func (s *Server) setLocalEntry(actionID string, entry cache.GoCacheEntry, diskPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries.Set(actionID, localEntry{entry: entry, diskPath: diskPath})
}

func (s *Server) writeLocalOutput(outputID string, body []byte) (string, error) {
	path := s.localOutputPath(outputID)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat local go cache output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create local go cache output dir: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("write local go cache output: %w", err)
	}
	return path, nil
}

func (s *Server) localOutputPath(outputID string) string {
	prefix := "00"
	if len(outputID) >= 2 {
		prefix = outputID[:2]
	}
	return filepath.Join(s.options.CacheDir, prefix, outputID+"-d")
}

func entryResponse(id int64, entry cache.GoCacheEntry, diskPath string) Response {
	outputID, err := hex.DecodeString(entry.OutputID)
	if err != nil {
		return Response{ID: id, Err: err.Error()}
	}
	return Response{
		ID:       id,
		OutputID: outputID,
		Size:     entry.Size,
		Time:     &entry.Time,
		DiskPath: diskPath,
	}
}

func cacheKey(data []byte, label string) (string, error) {
	if len(data) != sha256.Size {
		return "", fmt.Errorf("go cacheprog %s must be %d bytes", label, sha256.Size)
	}
	return hex.EncodeToString(data), nil
}
