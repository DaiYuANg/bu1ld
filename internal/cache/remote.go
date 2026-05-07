package cache

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/samber/oops"
)

type RemoteClient struct {
	baseURL string
	client  *http.Client
}

func NewRemoteClient(baseURL string) *RemoteClient {
	return &RemoteClient{
		baseURL: strings.TrimRight(baseURL, "/"),
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

	resp, err := c.client.Do(req)
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

func (c *RemoteClient) get(path string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, false, oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "create remote cache request")
	}

	resp, err := c.client.Do(req)
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
	req, err := http.NewRequest(http.MethodPut, c.url(path), bytes.NewReader(data))
	if err != nil {
		return oops.In("bu1ld.cache.remote").
			With("path", path).
			Wrapf(err, "create remote cache request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
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
