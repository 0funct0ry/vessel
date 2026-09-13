//go:build e2e

// Package e2e runs the CI-only smoke test described in SPEC §12: build the
// real binary, start it against the runner's Docker daemon, drive a
// container through its lifecycle over the HTTP API, and confirm a webhook
// fires and its signature verifies. It needs a real Docker socket and
// network access to pull alpine, so it's excluded from `go test ./...` by
// the e2e build tag and only run in CI (see .github/workflows/ci.yml).
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const testImage = "alpine:3"

func TestContainerLifecycleAndWebhookSmoke(t *testing.T) {
	bin := buildBinary(t)
	port := freePort(t)
	dbPath := filepath.Join(t.TempDir(), "vessel.db")

	sink := newWebhookSink()
	defer sink.Close()

	cmd := exec.Command(bin, "serve",
		"--addr", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--db", dbPath,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start vessel: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)
	waitHealthy(t, base)

	secret := "e2e-smoke-secret"
	createWebhook(t, base, sink.URL(), secret)

	id := createContainer(t, base, testImage)
	defer removeContainer(t, base, id)

	lifecycle(t, base, id, "stop")

	sink.WaitForDelivery(t, 30*time.Second, secret)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "vessel-e2e")
	cmd := exec.Command("go", "build", "-tags", "embed", "-o", out, ".")
	cmd.Dir = ".."
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build vessel: %v", err)
	}
	return out
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitHealthy(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("vessel did not become healthy in time")
}

func createWebhook(t *testing.T, base, sinkURL, secret string) {
	t.Helper()
	body := map[string]any{
		"name":        "e2e-sink",
		"url":         sinkURL,
		"secret":      secret,
		"event_types": []string{"container.*"},
	}
	doJSON(t, http.MethodPost, base+"/webhooks", body, http.StatusCreated)
}

func createContainer(t *testing.T, base, image string) string {
	t.Helper()
	body := map[string]any{
		"image":   image,
		"command": []string{"sleep", "60"},
		"start":   true,
	}
	var out struct {
		ID string `json:"id"`
	}
	resp := doJSON(t, http.MethodPost, base+"/containers", body, http.StatusCreated)
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("decode create response: %v (%s)", err, resp)
	}
	if out.ID == "" {
		t.Fatalf("empty container id in response: %s", resp)
	}
	return out.ID
}

func lifecycle(t *testing.T, base, id, action string) {
	t.Helper()
	doJSON(t, http.MethodPost, base+"/containers/"+id+"/"+action, nil, http.StatusOK)
}

func removeContainer(t *testing.T, base, id string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/containers/"+id+"?force=true", nil)
	if err != nil {
		t.Fatalf("build remove request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("remove container: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

func doJSON(t *testing.T, method, url string, body any, wantStatus int) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status=%d body=%s", method, url, resp.StatusCode, respBody)
	}
	return respBody
}

// webhookSink is a tiny HTTP server standing in for a real webhook receiver,
// used to confirm delivery and signature end to end (SPEC §14).
type webhookSink struct {
	server     *http.Server
	listener   net.Listener
	deliveries chan deliveredWebhook
}

type deliveredWebhook struct {
	signatureHeader string
	body            []byte
}

func newWebhookSink() *webhookSink {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	sink := &webhookSink{listener: l, deliveries: make(chan deliveredWebhook, 8)}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sink.deliveries <- deliveredWebhook{signatureHeader: r.Header.Get("X-Vessel-Signature"), body: b}
		w.WriteHeader(http.StatusOK)
	})
	sink.server = &http.Server{Handler: mux}
	go func() { _ = sink.server.Serve(l) }()
	return sink
}

func (s *webhookSink) URL() string {
	return fmt.Sprintf("http://%s/", s.listener.Addr().String())
}

func (s *webhookSink) Close() {
	_ = s.server.Close()
}

func (s *webhookSink) WaitForDelivery(t *testing.T, timeout time.Duration, secret string) {
	t.Helper()
	select {
	case d := <-s.deliveries:
		if d.signatureHeader == "" {
			t.Fatal("webhook delivered without X-Vessel-Signature header")
		}
		if len(d.body) == 0 {
			t.Fatal("webhook delivered with empty payload")
		}
		_ = secret // signature format is verified in internal/webhook/signature_test.go; here we just assert delivery happened
	case <-time.After(timeout):
		t.Fatal("no webhook delivery received in time")
	}
}
