package api

import (
	"archive/tar"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestContainerFilesRunningUploadAndDownload(t *testing.T) {
	fake := newFakeDockerClient()
	fake.container.State = "running"
	fake.files = []dockerapi.FileEntry{{Name: "config", Path: "/config", Type: "dir"}}
	fake.download = io.NopCloser(strings.NewReader("tar-data"))
	router := NewRouter(Config{Docker: fake, AllowExec: true})
	list := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files?path=/")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"config"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("path", "/tmp")
	part, _ := writer.CreateFormFile("files", "hello.txt")
	_, _ = part.Write([]byte("hello"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/containers/c1/files", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || fake.uploadPath != "/tmp" {
		t.Fatalf("upload = %d path=%q", response.Code, fake.uploadPath)
	}
	tr := tar.NewReader(bytes.NewReader(fake.uploadBody))
	header, err := tr.Next()
	if err != nil || header.Name != "hello.txt" {
		t.Fatalf("tar = %#v %v", header, err)
	}
	download := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files/download?path=/tmp/hello.txt")
	if download.Code != http.StatusOK || download.Body.String() != "tar-data" || !strings.Contains(download.Header().Get("Content-Disposition"), "hello.txt.tar") {
		t.Fatalf("download = %d %q %q", download.Code, download.Body.String(), download.Header().Get("Content-Disposition"))
	}
}

func TestContainerFilesStoppedAndTraversal(t *testing.T) {
	fake := newFakeDockerClient()
	fake.container.State = "exited"
	router := NewRouter(Config{Docker: fake, AllowExec: true})
	stopped := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files")
	if stopped.Code != http.StatusConflict || !strings.Contains(stopped.Body.String(), "container_not_running") {
		t.Fatalf("stopped = %d %s", stopped.Code, stopped.Body.String())
	}
	fake.container.State = "running"
	for _, target := range []string{"/api/v1/containers/c1/files/download?path=/tmp/../secret", "/api/v1/containers/c1/folders"} {
		var req *http.Request
		if strings.HasSuffix(target, "folders") {
			req = httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"path":"/tmp/../secret"}`))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(http.MethodGet, target, nil)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_path") {
			t.Fatalf("%s = %d %s", target, rec.Code, rec.Body.String())
		}
	}
	noExec := NewRouter(Config{Docker: fake})
	if response := performRequest(noExec, http.MethodGet, "/api/v1/containers/c1/files"); response.Code != http.StatusNotFound {
		t.Fatalf("listing with exec disabled = %d", response.Code)
	}
	if response := performRequest(noExec, http.MethodPost, "/api/v1/containers/c1/folders"); response.Code != http.StatusNotFound {
		t.Fatalf("mkdir with exec disabled = %d", response.Code)
	}
	if response := performRequest(noExec, http.MethodDelete, "/api/v1/containers/c1/files?path=/tmp/a"); response.Code != http.StatusNotFound {
		t.Fatalf("delete with exec disabled = %d", response.Code)
	}
	renameReq := httptest.NewRequest(http.MethodPost, "/api/v1/containers/c1/files/rename", strings.NewReader(`{"path":"/tmp/a","name":"b"}`))
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp := httptest.NewRecorder()
	noExec.ServeHTTP(renameResp, renameReq)
	if renameResp.Code != http.StatusNotFound {
		t.Fatalf("rename with exec disabled = %d", renameResp.Code)
	}
	fake.download = io.NopCloser(strings.NewReader("tar-data"))
	if response := performRequest(noExec, http.MethodGet, "/api/v1/containers/c1/files/download?path=/tmp/file"); response.Code != http.StatusOK {
		t.Fatalf("download with exec disabled = %d", response.Code)
	}
	fake.readFileName, fake.readFileData = "file.txt", []byte("hi")
	if response := performRequest(noExec, http.MethodGet, "/api/v1/containers/c1/files/view?path=/tmp/file.txt"); response.Code != http.StatusOK {
		t.Fatalf("view with exec disabled = %d", response.Code)
	}
	writeReq := httptest.NewRequest(http.MethodPut, "/api/v1/containers/c1/files/content", strings.NewReader(`{"path":"/tmp/file.txt","content":"hi"}`))
	writeReq.Header.Set("Content-Type", "application/json")
	writeResp := httptest.NewRecorder()
	noExec.ServeHTTP(writeResp, writeReq)
	if writeResp.Code != http.StatusNoContent {
		t.Fatalf("edit with exec disabled = %d", writeResp.Code)
	}

	deleteTraversal := performRequest(router, http.MethodDelete, "/api/v1/containers/c1/files?path=/tmp/../secret")
	if deleteTraversal.Code != http.StatusBadRequest || !strings.Contains(deleteTraversal.Body.String(), "invalid_path") {
		t.Fatalf("delete traversal = %d %s", deleteTraversal.Code, deleteTraversal.Body.String())
	}
	viewTraversal := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files/view?path=/tmp/../secret")
	if viewTraversal.Code != http.StatusBadRequest || !strings.Contains(viewTraversal.Body.String(), "invalid_path") {
		t.Fatalf("view traversal = %d %s", viewTraversal.Code, viewTraversal.Body.String())
	}
}

func TestContainerFileDeleteRenameViewEdit(t *testing.T) {
	fake := newFakeDockerClient()
	fake.container.State = "running"
	router := NewRouter(Config{Docker: fake, AllowExec: true})

	del := performRequest(router, http.MethodDelete, "/api/v1/containers/c1/files?path=/tmp/a")
	if del.Code != http.StatusNoContent || len(fake.removeCalls) != 1 || fake.removeCalls[0] != "/tmp/a" {
		t.Fatalf("delete = %d calls=%v", del.Code, fake.removeCalls)
	}

	renameReq := httptest.NewRequest(http.MethodPost, "/api/v1/containers/c1/files/rename", strings.NewReader(`{"path":"/tmp/a","name":"b"}`))
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp := httptest.NewRecorder()
	router.ServeHTTP(renameResp, renameReq)
	if renameResp.Code != http.StatusOK || len(fake.renameCalls) != 1 || fake.renameCalls[0] != [2]string{"/tmp/a", "/tmp/b"} {
		t.Fatalf("rename = %d calls=%v body=%s", renameResp.Code, fake.renameCalls, renameResp.Body.String())
	}
	badName := httptest.NewRequest(http.MethodPost, "/api/v1/containers/c1/files/rename", strings.NewReader(`{"path":"/tmp/a","name":"b/c"}`))
	badName.Header.Set("Content-Type", "application/json")
	badNameResp := httptest.NewRecorder()
	router.ServeHTTP(badNameResp, badName)
	if badNameResp.Code != http.StatusBadRequest {
		t.Fatalf("rename with slash in name = %d", badNameResp.Code)
	}

	fake.readFileName, fake.readFileData = "hello.txt", []byte("hello world")
	view := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files/view?path=/tmp/hello.txt")
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), `"kind":"text"`) || !strings.Contains(view.Body.String(), `"content":"hello world"`) {
		t.Fatalf("view text = %d %s", view.Code, view.Body.String())
	}

	fake.readFileName, fake.readFileData = "logo.png", []byte("\x89PNG\r\n\x1a\n0000000000000")
	viewImage := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files/view?path=/tmp/logo.png")
	if viewImage.Code != http.StatusOK || !strings.Contains(viewImage.Body.String(), `"kind":"image"`) {
		t.Fatalf("view image = %d %s", viewImage.Code, viewImage.Body.String())
	}

	fake.readFileName, fake.readFileData = "app", []byte{0x7f, 'E', 'L', 'F', 0x00, 0x01, 0x02}
	viewBinary := performRequest(router, http.MethodGet, "/api/v1/containers/c1/files/view?path=/tmp/app")
	if viewBinary.Code != http.StatusOK || !strings.Contains(viewBinary.Body.String(), `"kind":"binary"`) || strings.Contains(viewBinary.Body.String(), `"content"`) {
		t.Fatalf("view binary = %d %s", viewBinary.Code, viewBinary.Body.String())
	}

	writeReq := httptest.NewRequest(http.MethodPut, "/api/v1/containers/c1/files/content", strings.NewReader(`{"path":"/tmp/hello.txt","content":"updated"}`))
	writeReq.Header.Set("Content-Type", "application/json")
	writeResp := httptest.NewRecorder()
	router.ServeHTTP(writeResp, writeReq)
	if writeResp.Code != http.StatusNoContent || fake.writeFilePath != "/tmp/hello.txt" || string(fake.writeFileData) != "updated" {
		t.Fatalf("edit = %d path=%q data=%q", writeResp.Code, fake.writeFilePath, fake.writeFileData)
	}

	fake.container.State = "exited"
	stoppedDelete := performRequest(router, http.MethodDelete, "/api/v1/containers/c1/files?path=/tmp/a")
	if stoppedDelete.Code != http.StatusConflict {
		t.Fatalf("delete on stopped container = %d", stoppedDelete.Code)
	}
}
