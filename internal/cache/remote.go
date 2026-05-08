package cache

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/samber/oops"
)

type RemoteClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewRemoteClient(baseURL string) *RemoteClient {
	return NewRemoteClientWithToken(baseURL, "")
}

func NewRemoteClientWithToken(baseURL, token string) *RemoteClient {
	return &RemoteClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *RemoteClient) GetAction(actionKey string) ([]byte, bool, error) {
	return c.get("/v1/actions/" + actionKey)
}

func (c *RemoteClient) PutAction(actionKey string, data []byte) error {
	return c.put("/v1/actions/"+actionKey, data)
}

func (c *RemoteClient) HasBlob(digest string) (bool, error) {
	req, err := http.NewRequest(http.MethodHead, c.url("/v1/blobs/"+digest), nil)
	if err != nil {
		return false, oops.In("bu1ld.cache.remote").
			With("digest", digest).
			Wrapf(err, "create remote cache request")
	}

	c.authorize(req)
	resp, err := c.do(req)
	if err != nil {
		return false, oops.In("bu1ld.cache.remote").
			With("digest", digest).
			Wrapf(err, "check remote cache blob")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, remoteStatusError(resp, "check remote cache blob")
	}
}

func (c *RemoteClient) GetBlob(digest string) ([]byte, bool, error) {
	return c.get("/v1/blobs/" + digest)
}

func (c *RemoteClient) PutBlob(digest string, data []byte) error {
	return c.put("/v1/blobs/"+digest, data)
}

func (c *RemoteClient) GetGoCacheEntry(actionID string) (GoCacheEntry, bool, error) {
	data, hit, err := c.get("/v1/go/cache/actions/" + actionID)
	if err != nil || !hit {
		return GoCacheEntry{}, hit, err
	}
	var entry GoCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return GoCacheEntry{}, false, oops.In("bu1ld.cache.remote").
			With("action_id", actionID).
			Wrapf(err, "decode remote go cache action")
	}
	return entry, true, nil
}

func (c *RemoteClient) PutGoCacheEntry(actionID string, entry GoCacheEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return oops.In("bu1ld.cache.remote").
			With("action_id", actionID).
			Wrapf(err, "encode go cache action")
	}
	return c.putContent("/v1/go/cache/actions/"+actionID, data, "application/json")
}

func (c *RemoteClient) GetGoCacheOutput(outputID string) ([]byte, bool, error) {
	return c.get("/v1/go/cache/outputs/" + outputID)
}

func (c *RemoteClient) PutGoCacheOutput(outputID string, data []byte) error {
	return c.put("/v1/go/cache/outputs/"+outputID, data)
}

func (c *RemoteClient) get(path string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, false, oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "create remote cache request")
	}

	c.authorize(req)
	resp, err := c.do(req)
	if err != nil {
		return nil, false, oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "fetch remote cache object")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, remoteStatusError(resp, "fetch remote cache object")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "read remote cache response")
	}
	return data, true, nil
}

func (c *RemoteClient) put(path string, data []byte) error {
	return c.putContent(path, data, "application/octet-stream")
}

func (c *RemoteClient) putContent(path string, data []byte, contentType string) error {
	req, err := http.NewRequest(http.MethodPut, c.url(path), bytes.NewReader(data))
	if err != nil {
		return oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "create remote cache request")
	}
	req.Header.Set("Content-Type", contentType)
	c.authorize(req)

	resp, err := c.do(req)
	if err != nil {
		return oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "upload remote cache object")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return remoteStatusError(resp, "upload remote cache object")
	}
}

func (c *RemoteClient) url(path string) string {
	return c.baseURL + path
}

func (c *RemoteClient) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *RemoteClient) do(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}
		resp, err := c.client.Do(req)
		if err == nil && (resp.StatusCode < 500 || resp.StatusCode == http.StatusNotImplemented) {
			return resp, nil
		}
		if attempt == 2 {
			return resp, err
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return c.client.Do(req)
}

func remoteStatusError(resp *http.Response, action string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return oops.In("bu1ld.cache.remote").
		With("status", resp.StatusCode).
		Errorf("%s: %d %s", action, resp.StatusCode, message)
}
