package dockerapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestExportAndImportImages(t *testing.T) {
	archive := []byte("tar fixture")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.43/images/get":
			if got, want := r.URL.Query()["names"], []string{"acme/api:1", "acme/web:2"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("names = %#v, want %#v", got, want)
			}
			_, _ = w.Write(archive)
		case "/v1.43/images/load":
			if r.URL.Query().Get("quiet") != "false" || r.Header.Get("Content-Type") != "application/x-tar" {
				t.Fatalf("request = %s %s", r.URL.String(), r.Header.Get("Content-Type"))
			}
			got, _ := io.ReadAll(r.Body)
			if !bytes.Equal(got, archive) {
				t.Fatalf("body = %q", got)
			}
			_, _ = io.WriteString(w, "{\"stream\":\"Loading image\\n\"}\n{\"stream\":\"Loaded image: acme/api:1\\n\"}\n{\"error\":\"bad archive\"}\n")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	exported, err := c.ExportImages(context.Background(), []string{"acme/api:1", "acme/web:2"})
	if err != nil {
		t.Fatal(err)
	}
	defer exported.Close()
	got, _ := io.ReadAll(exported)
	if !bytes.Equal(got, archive) {
		t.Fatalf("archive = %q", got)
	}
	stream, err := c.ImportImages(context.Background(), bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	first, err := stream.Next()
	if err != nil || first.Stream != "Loading image\n" {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := stream.Next()
	if err != nil || second.Stream != "Loaded image: acme/api:1\n" {
		t.Fatalf("second = %#v, %v", second, err)
	}
	third, err := stream.Next()
	if err != nil || third.Error != "bad archive" {
		t.Fatalf("third = %#v, %v", third, err)
	}
	if _, err := stream.Next(); err != io.EOF {
		t.Fatalf("EOF = %v", err)
	}
}

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
