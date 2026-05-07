package cache

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"bu1ld/internal/cachefile"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/httpx"
	httpadapter "github.com/arcgolabs/httpx/adapter"
	"github.com/arcgolabs/httpx/adapter/std"
	"github.com/go-chi/chi/v5"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type cacheKeyInput struct {
	Key string `path:"key"`
}

type cachePutInput struct {
	Key     string `path:"key"`
	RawBody []byte `contentType:"application/octet-stream"`
}

type cacheDataOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

type cacheHeadOutput struct {
	Status        int
	ContentLength int64 `header:"Content-Length"`
}

type cacheStatusOutput struct {
	Status int
}

func NewHTTPXServer(store *Store, logger *slog.Logger) (httpx.ServerRuntime, error) {
	server, _, err := newHTTPXServer(store, logger, chi.NewMux())
	return server, err
}

func NewHTTPHandler(store *Store) http.Handler {
	_, router, err := newHTTPXServer(store, nil, chi.NewMux())
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "create cache server", http.StatusInternalServerError)
		})
	}
	return router
}

func newHTTPXServer(store *Store, logger *slog.Logger, router *chi.Mux) (httpx.ServerRuntime, http.Handler, error) {
	if store == nil {
		return nil, nil, oops.In("bu1ld.cache.server").New("cache store is nil")
	}
	if router == nil {
		router = chi.NewMux()
	}

	adapter := std.New(router, httpadapter.HumaOptions{
		Title:             "bu1ld remote cache",
		Version:           "0.1.0",
		Description:       "Remote action cache and content-addressable blob store.",
		DisableDocsRoutes: true,
	})
	server := httpx.New(
		httpx.WithAdapter(adapter),
		httpx.WithLogger(logger),
		httpx.WithPanicRecover(true),
	)

	for _, register := range []func(httpx.ServerRuntime) error{
		store.registerActionRoutes,
		store.registerBlobRoutes,
	} {
		if err := register(server); err != nil {
			return nil, nil, err
		}
	}
	return server, router, nil
}

func (s *Store) registerActionRoutes(server httpx.ServerRuntime) error {
	if err := httpx.Get(server, "/v1/actions/{key}", s.getAction); err != nil {
		return err
	}
	if err := httpx.Head(server, "/v1/actions/{key}", s.headAction); err != nil {
		return err
	}
	return httpx.Put(server, "/v1/actions/{key}", s.putAction)
}

func (s *Store) registerBlobRoutes(server httpx.ServerRuntime) error {
	if err := httpx.Get(server, "/v1/blobs/{key}", s.getBlob); err != nil {
		return err
	}
	if err := httpx.Head(server, "/v1/blobs/{key}", s.headBlob); err != nil {
		return err
	}
	return httpx.Put(server, "/v1/blobs/{key}", s.putBlob)
}

func (s *Store) getAction(_ context.Context, input *cacheKeyInput) (*cacheDataOutput, error) {
	if err := validateCacheKey(input.Key, "action key"); err != nil {
		return nil, err
	}
	return s.readCacheData(s.recordPath(input.Key))
}

func (s *Store) headAction(_ context.Context, input *cacheKeyInput) (*cacheHeadOutput, error) {
	if err := validateCacheKey(input.Key, "action key"); err != nil {
		return nil, err
	}
	return s.statCacheData(s.recordPath(input.Key))
}

func (s *Store) putAction(_ context.Context, input *cachePutInput) (*cacheStatusOutput, error) {
	if err := validateCacheKey(input.Key, "action key"); err != nil {
		return nil, err
	}

	var record Record
	if err := cachefile.Unmarshal(input.RawBody, &record); err != nil {
		return nil, httpx.NewError(http.StatusBadRequest, "decode action record", err)
	}
	if record.ActionKey != "" && record.ActionKey != input.Key {
		return nil, httpx.NewError(http.StatusBadRequest, "action key mismatch")
	}
	for _, digest := range recordDigests(record) {
		if _, err := s.fs.Stat(s.blobPath(digest)); err != nil {
			if isNotExist(err) {
				return nil, httpx.NewError(http.StatusBadRequest, "action record references missing blob")
			}
			return nil, oops.In("bu1ld.cache.server").
				With("digest", digest).
				Wrapf(err, "stat cache blob")
		}
	}
	if err := s.writeRecordBytes(input.Key, input.RawBody); err != nil {
		return nil, err
	}
	return &cacheStatusOutput{Status: http.StatusCreated}, nil
}

func (s *Store) getBlob(_ context.Context, input *cacheKeyInput) (*cacheDataOutput, error) {
	if err := validateCacheKey(input.Key, "blob digest"); err != nil {
		return nil, err
	}
	return s.readCacheData(s.blobPath(input.Key))
}

func (s *Store) headBlob(_ context.Context, input *cacheKeyInput) (*cacheHeadOutput, error) {
	if err := validateCacheKey(input.Key, "blob digest"); err != nil {
		return nil, err
	}
	return s.statCacheData(s.blobPath(input.Key))
}

func (s *Store) putBlob(_ context.Context, input *cachePutInput) (*cacheStatusOutput, error) {
	if err := validateCacheKey(input.Key, "blob digest"); err != nil {
		return nil, err
	}
	if snapshot.HashBytes(input.RawBody) != input.Key {
		return nil, httpx.NewError(http.StatusBadRequest, "blob digest mismatch")
	}
	if err := s.writeBlobBytes(input.Key, input.RawBody); err != nil {
		return nil, err
	}
	return &cacheStatusOutput{Status: http.StatusCreated}, nil
}

func (s *Store) readCacheData(path string) (*cacheDataOutput, error) {
	data, err := afero.ReadFile(s.fs, path)
	if err != nil {
		if isNotExist(err) {
			return nil, httpx.NewError(http.StatusNotFound, "cache object not found")
		}
		return nil, oops.In("bu1ld.cache.server").
			With("path", path).
			Wrapf(err, "read cache object")
	}
	return &cacheDataOutput{
		ContentType: "application/octet-stream",
		Body:        data,
	}, nil
}

func (s *Store) statCacheData(path string) (*cacheHeadOutput, error) {
	info, err := s.fs.Stat(path)
	if err != nil {
		if isNotExist(err) {
			return nil, httpx.NewError(http.StatusNotFound, "cache object not found")
		}
		return nil, oops.In("bu1ld.cache.server").
			With("path", path).
			Wrapf(err, "stat cache object")
	}
	return &cacheHeadOutput{
		Status:        http.StatusNoContent,
		ContentLength: info.Size(),
	}, nil
}

func validateCacheKey(value, label string) error {
	if len(value) != 64 || strings.Contains(value, "/") {
		return httpx.NewError(http.StatusBadRequest, "invalid "+label)
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return httpx.NewError(http.StatusBadRequest, "invalid "+label)
	}
	return nil
}
