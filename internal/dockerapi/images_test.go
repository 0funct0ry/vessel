package dockerapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.43/images/img/history" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[
			{"Id":"sha256:top","Created":200,"CreatedBy":"CMD [\"sh\"]","Size":10,"Comment":"","Tags":["acme/app:1"]},
			{"Id":"<missing>","Created":100,"CreatedBy":"ADD file:abc in /","Size":5,"Comment":"","Tags":null}
		]`)
	}))
	defer srv.Close()

	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	layers, err := c.History(context.Background(), "img")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Fatalf("len(layers) = %d, want 2", len(layers))
	}
	if layers[0].ID != "sha256:top" || layers[0].Size != 10 || layers[0].CreatedBy != `CMD ["sh"]` || len(layers[0].Tags) != 1 {
		t.Fatalf("layers[0] = %+v", layers[0])
	}
	if layers[1].ID != "<missing>" || layers[1].Tags != nil {
		t.Fatalf("layers[1] = %+v", layers[1])
	}
}
