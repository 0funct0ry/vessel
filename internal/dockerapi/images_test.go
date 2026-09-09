package dockerapi

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestBuildImageAndContext(t *testing.T) {
	var contextTar bytes.Buffer
	if err := WriteBuildContext(&contextTar, BuildContextFile{Reader: strings.NewReader("FROM scratch\n"), Size: 13}, []BuildContextFile{{Path: "app/main.go", Reader: strings.NewReader("package main"), Size: 12}}, 1024); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(contextTar.Bytes()))
	for _, want := range []string{"Dockerfile", "app/main.go"} {
		h, err := tr.Next()
		if err != nil || h.Name != want {
			t.Fatalf("entry = %#v, %v; want %q", h, err, want)
		}
	}
	if err := ValidateBuildContextPath("../secret"); !errors.Is(err, ErrInvalidBuildPath) {
		t.Fatalf("path error = %v", err)
	}
	if err := WriteBuildContext(io.Discard, BuildContextFile{Reader: strings.NewReader("1234"), Size: 4}, nil, 3); !errors.Is(err, ErrBuildContextTooLarge) {
		t.Fatalf("size error = %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.43/build" || r.URL.Query().Get("dockerfile") != "Dockerfile" || !reflect.DeepEqual(r.URL.Query()["t"], []string{"acme/api:1", "acme/api:latest"}) || r.Header.Get("Content-Type") != "application/x-tar" {
			t.Fatalf("request = %s %q", r.URL.String(), r.Header.Get("Content-Type"))
		}
		_, _ = io.WriteString(w, "{\"stream\":\"Step 1/1 : FROM scratch\\n\"}\n{\"aux\":{\"ID\":\"sha256:built\"}}\n")
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := c.BuildImage(context.Background(), bytes.NewReader(contextTar.Bytes()), []string{"acme/api:1", "acme/api:latest"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	first, err := stream.Next()
	if err != nil || first.Stream == "" {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := stream.Next()
	if err != nil || second.Aux.ID != "sha256:built" {
		t.Fatalf("second = %#v, %v", second, err)
	}
}

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
