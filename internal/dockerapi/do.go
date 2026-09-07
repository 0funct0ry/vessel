package dockerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// tooNewVersionRE matches Docker's 400 body when the client is pinned to an
// API version newer than the daemon supports, e.g.:
// "client version 1.50 is too new. Maximum supported API version is 1.43".
var tooNewVersionRE = regexp.MustCompile(`client version .* is too new`)

// do issues a request against path (which must include the leading slash,
// not the API version prefix) and returns the raw response for the caller
// to decode. On success (2xx) the response is returned as-is; the caller
// must close resp.Body. On failure, do consumes the body and returns a
// mapped error.
//
// On a "client version too new" 400, do negotiates down to the daemon's
// reported API version by querying /version and retries exactly once.
func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	resp, err := c.doOnce(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusBadRequest {
		peek, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr == nil && tooNewVersionRE.Match(peek) {
			if negErr := c.negotiateVersion(ctx); negErr != nil {
				return nil, negErr
			}
			return c.doOnce(ctx, method, path, body)
		}
		// Not a version issue — reconstruct the body so mapError can read it.
		resp.Body = io.NopCloser(bytes.NewReader(peek))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	return resp, nil
}

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url(path), reqBody)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	return resp, nil
}

// negotiateVersion queries the un-versioned /version endpoint and pins the
// client to the daemon's reported ApiVersion.
func (c *Client) negotiateVersion(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return fmt.Errorf("dockerapi: building version request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapError(resp)
	}

	var v versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return fmt.Errorf("dockerapi: decoding /version response: %w", err)
	}
	if v.APIVersion == "" {
		return fmt.Errorf("dockerapi: /version response missing ApiVersion")
	}

	c.setAPIVersion("v" + v.APIVersion)
	return nil
}
