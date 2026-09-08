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

// versionMismatchRE matches Docker's 400 body when the client is pinned to
// an API version outside the daemon's supported range, e.g.:
// "client version 1.50 is too new. Maximum supported API version is 1.43"
// "client version 1.43 is too old. Minimum supported API version is 1.44"
// (the latter shows up once a daemon raises its minimum past vessel's
// compiled-in defaultAPIVersion).
var versionMismatchRE = regexp.MustCompile(`client version .* is too (new|old)`)

// looksLikeVersionMismatch reports whether a 400 body signals an API-version
// problem worth renegotiating for. Every one of Docker's own validation
// errors carries a {"message": "..."} envelope; the one known exception is
// some daemons' /info endpoint, which — when pinned below the daemon's
// declared minimum API version — returns 400 with a body that decodes as a
// (mostly empty) Info struct and no "message" field at all, rather than the
// usual error text. Treating "valid JSON object, no message field" as a
// version issue too catches that case without needing an endpoint-specific
// special case.
func looksLikeVersionMismatch(body []byte) bool {
	if versionMismatchRE.Match(body) {
		return true
	}
	var parsed dockerErrorBody
	return json.Unmarshal(body, &parsed) == nil && parsed.Message == ""
}

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
		if rerr == nil && looksLikeVersionMismatch(peek) {
			if negErr := c.negotiateVersion(ctx); negErr != nil {
				return nil, negErr
			}
			resp, err = c.doOnce(ctx, method, path, body)
			if err != nil {
				return nil, err
			}
		} else {
			// Not a version issue — reconstruct the body so mapError can read it.
			resp.Body = io.NopCloser(bytes.NewReader(peek))
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	return resp, nil
}

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	return c.doOnceWithClient(ctx, method, path, body, c.httpClient)
}

// doStream uses the same error and API-version negotiation semantics as do,
// without the normal request deadline. Streams are governed by their context;
// a large image pull must not be cut off by the non-streaming 30s timeout.
func (c *Client) doStream(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	client := *c.httpClient
	client.Timeout = 0
	resp, err := c.doOnceWithClient(ctx, method, path, body, &client)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusBadRequest {
		peek, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr == nil && looksLikeVersionMismatch(peek) {
			if err := c.negotiateVersion(ctx); err != nil {
				return nil, err
			}
			resp, err = c.doOnceWithClient(ctx, method, path, body, &client)
			if err != nil {
				return nil, err
			}
		} else {
			resp.Body = io.NopCloser(bytes.NewReader(peek))
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}
	return resp, nil
}

func (c *Client) doOnceWithClient(ctx context.Context, method, path string, body []byte, client *http.Client) (*http.Response, error) {
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

	resp, err := client.Do(req)
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
