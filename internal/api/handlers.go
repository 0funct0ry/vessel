package api

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

// maxFileViewBytes bounds the file-browser's view/edit round trip (both are a
// preview and a lightweight editor per M15.4, not a full editor for large or
// binary files) and matches decodeBody's JSON request-body limit.
const maxFileViewBytes = 1 << 20

var loadedImageRE = regexp.MustCompile(`(?m)^Loaded image: ([^\s]+)\s*$`)
var buildStepRE = regexp.MustCompile(`(?m)Step ([0-9]+)/([0-9]+)\s*:`)
var buildContextLimit int64 = dockerapi.MaxBuildContextBytes

var resourceNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
var imageReferenceRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/-]*(?::[a-zA-Z0-9][a-zA-Z0-9_.-]*|@[A-Za-z0-9_+.-]+:[A-Fa-f0-9]+)?$`)

func decodeBody(c *gin.Context, into any) error {
	dec := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return invalidInput("invalid JSON request body")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return invalidInput("invalid JSON request body")
	}
	return nil
}
func validResourceName(name string) error {
	if !resourceNameRE.MatchString(name) {
		return invalidName("name must match ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$")
	}
	return nil
}
func required(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return invalidInput("%s is required", field)
	}
	return nil
}
func queryBool(c *gin.Context, name string) (bool, error) {
	raw, ok := c.GetQuery(name)
	if !ok || raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, invalidInput("%s must be a boolean", name)
	}
	return value, nil
}

func safeContainerPath(value string) (string, error) {
	if value == "" {
		return "/", nil
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", invalidPath("path must not contain ..")
		}
	}
	return value, nil
}

func (s *server) requireRunningContainer(c *gin.Context) error {
	detail, err := s.docker.InspectContainer(c.Request.Context(), c.Param("id"))
	if err != nil {
		return forResource("container", c.Param("id"), err)
	}
	if detail.State != "running" {
		return containerNotRunningError{}
	}
	return nil
}

func (s *server) handleContainerFiles(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	dir := c.Query("path")
	if dir == "" {
		dir = "/"
	}
	entries, err := s.docker.ListDirectory(c.Request.Context(), c.Param("id"), dir)
	if err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (s *server) handleContainerUpload(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		Fail(c, invalidInput("invalid multipart upload"))
		return
	}
	dir, err := safeContainerPath(c.Request.FormValue("path"))
	if err != nil {
		Fail(c, err)
		return
	}
	headers := c.Request.MultipartForm.File["files"]
	if len(headers) == 0 {
		Fail(c, invalidInput("at least one file is required"))
		return
	}
	tmp, err := os.CreateTemp("", "vessel-files-*.tar")
	if err != nil {
		Fail(c, err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	tw := tar.NewWriter(tmp)
	for _, header := range headers {
		file, openErr := header.Open()
		if openErr != nil {
			tw.Close()
			tmp.Close()
			Fail(c, openErr)
			return
		}
		name := path.Base(header.Filename)
		if name == "." || name == "/" {
			file.Close()
			tw.Close()
			tmp.Close()
			Fail(c, invalidPath("upload file name is invalid"))
			return
		}
		writeErr := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: header.Size})
		if writeErr == nil {
			_, writeErr = io.Copy(tw, file)
		}
		file.Close()
		if writeErr != nil {
			tw.Close()
			tmp.Close()
			Fail(c, writeErr)
			return
		}
	}
	if err := tw.Close(); err != nil {
		tmp.Close()
		Fail(c, err)
		return
	}
	if err := tmp.Close(); err != nil {
		Fail(c, err)
		return
	}
	archive, err := os.Open(tmpPath)
	if err != nil {
		Fail(c, err)
		return
	}
	defer archive.Close()
	if err := s.docker.UploadFiles(c.Request.Context(), c.Param("id"), dir, archive); err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleContainerFolder(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		Fail(c, invalidInput("path is required"))
		return
	}
	dir, err := safeContainerPath(body.Path)
	if err != nil {
		Fail(c, err)
		return
	}
	if err := s.docker.CreateDirectory(c.Request.Context(), c.Param("id"), dir); err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleContainerDownload(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	filePath, err := safeContainerPath(c.Query("path"))
	if err != nil {
		Fail(c, err)
		return
	}
	archive, err := s.docker.DownloadPath(c.Request.Context(), c.Param("id"), filePath)
	if err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	defer archive.Close()
	name := path.Base(strings.TrimSuffix(filePath, "/"))
	if name == "." || name == "/" {
		name = "container-files"
	}
	c.Header("Content-Type", "application/x-tar")
	c.Header("Content-Disposition", `attachment; filename="`+name+`.tar"`)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, archive)
}

func (s *server) handleContainerFileDelete(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	target, err := safeContainerPath(c.Query("path"))
	if err != nil {
		Fail(c, err)
		return
	}
	if target == "" || target == "/" {
		Fail(c, invalidPath("path is required"))
		return
	}
	if err := s.docker.RemovePath(c.Request.Context(), c.Param("id"), target); err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleContainerFileRename(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		Fail(c, invalidInput("path is required"))
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.ContainsRune(body.Name, '/') {
		Fail(c, invalidInput("name must be a single path segment"))
		return
	}
	from, err := safeContainerPath(body.Path)
	if err != nil {
		Fail(c, err)
		return
	}
	if from == "" || from == "/" {
		Fail(c, invalidPath("path is required"))
		return
	}
	to := path.Join(path.Dir(from), body.Name)
	if err := s.docker.RenamePath(c.Request.Context(), c.Param("id"), from, to); err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": to})
}

func (s *server) handleContainerFileView(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	target, err := safeContainerPath(c.Query("path"))
	if err != nil {
		Fail(c, err)
		return
	}
	if target == "" || target == "/" {
		Fail(c, invalidPath("path is required"))
		return
	}
	name, data, err := s.docker.ReadFile(c.Request.Context(), c.Param("id"), target)
	if err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	if len(data) > maxFileViewBytes {
		Fail(c, invalidInput("file exceeds %d bytes and cannot be previewed", maxFileViewBytes))
		return
	}
	kind, mimeType := classifyFileContent(data)
	response := gin.H{"name": name, "size": len(data), "mime": mimeType, "kind": kind}
	switch kind {
	case "text":
		response["content"] = string(data)
	case "image":
		response["content"] = base64.StdEncoding.EncodeToString(data)
	}
	c.JSON(http.StatusOK, response)
}

func (s *server) handleContainerFileWrite(c *gin.Context) {
	if err := s.requireRunningContainer(c); err != nil {
		Fail(c, err)
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		Fail(c, invalidInput("path is required"))
		return
	}
	target, err := safeContainerPath(body.Path)
	if err != nil {
		Fail(c, err)
		return
	}
	if target == "" || target == "/" {
		Fail(c, invalidPath("path is required"))
		return
	}
	if err := s.docker.WriteFile(c.Request.Context(), c.Param("id"), target, []byte(body.Content)); err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	c.Status(http.StatusNoContent)
}

// classifyFileContent sniffs data for the file-browser's view panel: images
// render inline, text is editable, anything else is reported as binary so
// the UI can hide the view/edit actions (SPEC's "lightweight file manager,
// not a full editor").
func classifyFileContent(data []byte) (kind, mimeType string) {
	mimeType = http.DetectContentType(data)
	base, _, _ := strings.Cut(mimeType, ";")
	switch {
	case strings.HasPrefix(base, "image/"):
		return "image", base
	case isLikelyText(data):
		return "text", "text/plain; charset=utf-8"
	default:
		return "binary", base
	}
}

func isLikelyText(data []byte) bool {
	sample := data
	if len(sample) > 8000 {
		sample = sample[:8000]
	}
	if !utf8.Valid(sample) {
		return false
	}
	for _, b := range sample {
		if b == 0 {
			return false
		}
	}
	return true
}

func (s *server) handleContainerLifecycle(c *gin.Context) {
	id, action := c.Param("id"), c.Param("action")
	q := url.Values{}
	switch action {
	case "start", "restart", "pause", "unpause":
	case "stop":
		if raw, ok := c.GetQuery("t"); ok {
			t, err := strconv.Atoi(raw)
			if err != nil || t < 0 {
				Fail(c, invalidInput("t must be a non-negative integer"))
				return
			}
			q.Set("t", strconv.Itoa(t))
		}
	case "kill":
		if signal := c.Query("signal"); signal != "" {
			if !regexp.MustCompile(`^SIG[A-Z0-9]+$`).MatchString(signal) {
				Fail(c, invalidInput("signal must be a POSIX signal name"))
				return
			}
			q.Set("signal", signal)
		}
	case "rename":
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeBody(c, &body); err != nil {
			Fail(c, err)
			return
		}
		if err := validResourceName(body.Name); err != nil {
			Fail(c, err)
			return
		}
		err := s.docker.RenameContainer(c.Request.Context(), id, body.Name)
		if err != nil {
			Fail(c, forResource("container", id, err))
			return
		}
		c.Status(http.StatusNoContent)
		return
	default:
		c.Status(http.StatusNotFound)
		return
	}
	if err := s.docker.Lifecycle(c.Request.Context(), id, action, q); err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleContainerRemove(c *gin.Context) {
	force, err := queryBool(c, "force")
	if err != nil {
		Fail(c, err)
		return
	}
	volumes, err := queryBool(c, "volumes")
	if err != nil {
		Fail(c, err)
		return
	}
	id := c.Param("id")
	if err = s.docker.RemoveContainer(c.Request.Context(), id, dockerapi.RemoveContainerOptions{Force: force, Volumes: volumes}); err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleContainerCreate(c *gin.Context) {
	var body struct {
		Name       string   `json:"name"`
		Image      string   `json:"image"`
		Command    []string `json:"command"`
		Entrypoint []string `json:"entrypoint"`
		Env        []string `json:"env"`
		Ports      []struct {
			Container string `json:"container"`
			Host      string `json:"host"`
			Protocol  string `json:"protocol"`
		} `json:"ports"`
		Mounts []struct {
			Source   string `json:"source"`
			Target   string `json:"target"`
			Type     string `json:"type"`
			ReadOnly bool   `json:"ro"`
		} `json:"mounts"`
		Network       string            `json:"network"`
		RestartPolicy string            `json:"restart_policy"`
		Labels        map[string]string `json:"labels"`
		Start         bool              `json:"start"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if body.Name != "" {
		if err := validResourceName(body.Name); err != nil {
			Fail(c, err)
			return
		}
	}
	if !imageReferenceRE.MatchString(body.Image) {
		Fail(c, invalidImageReference("image must be repo[:tag|@digest]"))
		return
	}
	if body.Network != "" {
		networks, err := s.docker.ListNetworks(c.Request.Context())
		if err != nil {
			Fail(c, err)
			return
		}
		found := false
		for _, network := range networks {
			// Docker's default bridge is conventionally addressed by name in
			// NetworkMode, while user-selected networks use their stable IDs.
			if network.ID == body.Network || (body.Network == "bridge" && network.Name == "bridge") {
				found = true
				break
			}
		}
		if !found {
			Fail(c, invalidInput("network must identify an existing network"))
			return
		}
	}
	spec := dockerapi.Spec{Name: body.Name, Image: body.Image, Command: body.Command, Entrypoint: body.Entrypoint, Env: body.Env, Network: body.Network, RestartPolicy: body.RestartPolicy, Labels: body.Labels, Start: body.Start}
	for _, p := range body.Ports {
		spec.Ports = append(spec.Ports, dockerapi.PortSpec{Container: p.Container, Host: p.Host, Protocol: p.Protocol})
	}
	for _, m := range body.Mounts {
		spec.Mounts = append(spec.Mounts, dockerapi.MountSpec{Source: m.Source, Target: m.Target, Type: m.Type, ReadOnly: m.ReadOnly})
	}
	result, err := s.docker.CreateContainer(c.Request.Context(), spec)
	response := gin.H{"id": result.ID, "name": body.Name, "warnings": result.Warnings}
	var startErr *dockerapi.StartError
	if errors.As(err, &startErr) {
		response["start_error"] = dockerMessage(startErr)
		c.JSON(http.StatusCreated, response)
		return
	}
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (s *server) handleContainerCommit(c *gin.Context) {
	var body struct {
		Repo    string `json:"repo"`
		Tag     string `json:"tag"`
		Comment string `json:"comment"`
		Pause   *bool  `json:"pause"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if err := required(body.Repo, "repo"); err != nil {
		Fail(c, err)
		return
	}
	reference := body.Repo
	if body.Tag != "" {
		reference += ":" + body.Tag
	}
	if !imageReferenceRE.MatchString(reference) {
		Fail(c, invalidImageReference("image must be repo[:tag|@digest]"))
		return
	}
	pause := true
	if body.Pause != nil {
		pause = *body.Pause
	}
	id := c.Param("id")
	result, err := s.docker.CommitContainer(c.Request.Context(), id, dockerapi.CommitOptions{Repo: body.Repo, Tag: body.Tag, Comment: body.Comment, Pause: pause})
	if err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"image_id": result.ImageID})
}

func (s *server) handleImagePull(c *gin.Context) {
	var body struct {
		Reference string `json:"reference"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if !imageReferenceRE.MatchString(body.Reference) {
		Fail(c, invalidName("reference must be repo[:tag|@digest]"))
		return
	}
	Stream(c, func(send func(string, any) error) error {
		reader, err := s.docker.PullImage(c.Request.Context(), body.Reference)
		if err != nil {
			return err
		}
		defer reader.Close()
		last := map[string]time.Time{}
		for {
			progress, err := reader.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			now := time.Now()
			if progress.ID != "" && now.Sub(last[progress.ID]) < time.Second/5 {
				continue
			}
			last[progress.ID] = now
			payload := map[string]any{"id": progress.ID, "status": progress.Status}
			if progress.ProgressDetail.Current > 0 {
				payload["current"] = progress.ProgressDetail.Current
			}
			if progress.ProgressDetail.Total > 0 {
				payload["total"] = progress.ProgressDetail.Total
			}
			if progress.Error != "" {
				payload["error"] = progress.Error
			}
			if err := send("pull", payload); err != nil {
				return err
			}
		}
	})
}

func (s *server) handleImageExport(c *gin.Context) {
	refs := c.QueryArray("ref")
	if len(refs) == 0 {
		Fail(c, invalidQuery("at least one ref is required"))
		return
	}
	for _, ref := range refs {
		if !imageReferenceRE.MatchString(ref) {
			Fail(c, invalidImageReference("ref must be repo[:tag|@digest] or image ID"))
			return
		}
	}
	archive, err := s.docker.ExportImages(c.Request.Context(), refs)
	if err != nil {
		Fail(c, err)
		return
	}
	defer archive.Close()
	c.Header("Content-Type", "application/x-tar")
	c.Header("Content-Disposition", `attachment; filename="vessel-images-`+strconv.FormatInt(time.Now().Unix(), 10)+`.tar"`)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, archive)
}

func (s *server) handleImageImport(c *gin.Context) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		Fail(c, invalidInput("Content-Type must be multipart/form-data"))
		return
	}
	reader, err := c.Request.MultipartReader()
	if err != nil {
		Fail(c, invalidInput("invalid multipart upload"))
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		Fail(c, invalidInput("tar file is required"))
		return
	}
	defer part.Close()
	if part.FormName() != "tar" || part.FileName() == "" || !strings.HasSuffix(strings.ToLower(part.FileName()), ".tar") {
		Fail(c, invalidInput("tar must be a .tar file upload"))
		return
	}
	Stream(c, func(send func(string, any) error) error {
		stream, err := s.docker.ImportImages(c.Request.Context(), part)
		if err != nil {
			return err
		}
		defer stream.Close()
		loaded := []string{}
		for {
			line, err := stream.Next()
			if errors.Is(err, io.EOF) {
				return send("done", gin.H{"images": loaded})
			}
			if err != nil {
				return err
			}
			if line.Error != "" {
				return errors.New(line.Error)
			}
			if line.Stream != "" {
				for _, match := range loadedImageRE.FindAllStringSubmatch(line.Stream, -1) {
					loaded = append(loaded, match[1])
				}
				if err := send("import", gin.H{"stream": line.Stream}); err != nil {
					return err
				}
			}
		}
	})
}

type stagedBuildFile struct {
	name, path string
	size       int64
}

func (s *server) handleImageBuild(c *gin.Context) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		Fail(c, invalidInput("Content-Type must be multipart/form-data"))
		return
	}
	reader, err := c.Request.MultipartReader()
	if err != nil {
		Fail(c, invalidInput("invalid multipart upload"))
		return
	}
	var dockerfile *stagedBuildFile
	var files []stagedBuildFile
	var contextPaths []string
	var tags []string
	var total int64
	cleanup := func() {
		for _, file := range files {
			_ = os.Remove(file.path)
		}
		if dockerfile != nil {
			_ = os.Remove(dockerfile.path)
		}
	}
	defer cleanup()
	stage := func(name string, source io.Reader) (stagedBuildFile, error) {
		if total >= buildContextLimit {
			return stagedBuildFile{}, dockerapi.ErrBuildContextTooLarge
		}
		f, err := os.CreateTemp("", "vessel-build-*")
		if err != nil {
			return stagedBuildFile{}, err
		}
		n, copyErr := io.Copy(f, io.LimitReader(source, buildContextLimit-total+1))
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(f.Name())
			if copyErr != nil {
				return stagedBuildFile{}, copyErr
			}
			return stagedBuildFile{}, closeErr
		}
		if n > buildContextLimit-total {
			_ = os.Remove(f.Name())
			return stagedBuildFile{}, dockerapi.ErrBuildContextTooLarge
		}
		total += n
		return stagedBuildFile{name: name, path: f.Name(), size: n}, nil
	}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			Fail(c, invalidInput("invalid multipart upload"))
			return
		}
		formName, filename := part.FormName(), part.FileName()
		switch formName {
		case "tags[]":
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			part.Close()
			if readErr != nil || len(value) > 4096 {
				Fail(c, invalidInput("invalid build tag"))
				return
			}
			tags = append(tags, strings.TrimSpace(string(value)))
		case "dockerfile":
			if dockerfile != nil {
				part.Close()
				Fail(c, invalidInput("exactly one dockerfile is required"))
				return
			}
			file, stageErr := stage("Dockerfile", part)
			part.Close()
			if stageErr != nil {
				if errors.Is(stageErr, dockerapi.ErrBuildContextTooLarge) {
					Fail(c, buildContextTooLarge())
				} else {
					Fail(c, stageErr)
				}
				return
			}
			dockerfile = &file
		case "context_path[]":
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			part.Close()
			if readErr != nil || len(value) > 4096 {
				Fail(c, invalidPath("invalid context path"))
				return
			}
			contextPaths = append(contextPaths, strings.TrimSpace(string(value)))
		case "context":
			if filename == "" {
				part.Close()
				Fail(c, invalidPath("context files require a relative path"))
				return
			}
			file, stageErr := stage(filename, part)
			part.Close()
			if stageErr != nil {
				if errors.Is(stageErr, dockerapi.ErrBuildContextTooLarge) {
					Fail(c, buildContextTooLarge())
				} else {
					Fail(c, stageErr)
				}
				return
			}
			files = append(files, file)
		default:
			part.Close()
			Fail(c, invalidInput("unknown multipart field %q", formName))
			return
		}
	}
	if len(contextPaths) > 0 && len(contextPaths) != len(files) {
		Fail(c, invalidPath("each context file requires one relative path"))
		return
	}
	for i := range files {
		if len(contextPaths) > 0 {
			files[i].name = contextPaths[i]
		}
		if err := dockerapi.ValidateBuildContextPath(files[i].name); err != nil {
			Fail(c, invalidPath("context path must not contain .."))
			return
		}
	}
	if dockerfile == nil || dockerfile.size == 0 {
		Fail(c, invalidInput("dockerfile is required"))
		return
	}
	if len(tags) == 0 {
		Fail(c, invalidImageReference("at least one tag is required"))
		return
	}
	for _, tag := range tags {
		if !imageReferenceRE.MatchString(tag) {
			Fail(c, invalidImageReference("tag must be repo[:tag]"))
			return
		}
	}
	tarFile, err := os.CreateTemp("", "vessel-build-context-*.tar")
	if err != nil {
		Fail(c, err)
		return
	}
	tarPath := tarFile.Name()
	defer os.Remove(tarPath)
	dockerSource, err := os.Open(dockerfile.path)
	if err != nil {
		tarFile.Close()
		Fail(c, err)
		return
	}
	defer dockerSource.Close()
	context := make([]dockerapi.BuildContextFile, 0, len(files))
	opened := make([]*os.File, 0, len(files))
	defer func() {
		for _, f := range opened {
			_ = f.Close()
		}
	}()
	for _, file := range files {
		f, openErr := os.Open(file.path)
		if openErr != nil {
			tarFile.Close()
			Fail(c, openErr)
			return
		}
		opened = append(opened, f)
		context = append(context, dockerapi.BuildContextFile{Path: file.name, Reader: f, Size: file.size})
	}
	if err := dockerapi.WriteBuildContext(tarFile, dockerapi.BuildContextFile{Reader: dockerSource, Size: dockerfile.size}, context, buildContextLimit); err != nil {
		tarFile.Close()
		if errors.Is(err, dockerapi.ErrInvalidBuildPath) {
			Fail(c, invalidPath("context path must not contain .."))
		} else if errors.Is(err, dockerapi.ErrBuildContextTooLarge) {
			Fail(c, buildContextTooLarge())
		} else {
			Fail(c, err)
		}
		return
	}
	if err := tarFile.Close(); err != nil {
		Fail(c, err)
		return
	}
	Stream(c, func(send func(string, any) error) error {
		archive, err := os.Open(tarPath)
		if err != nil {
			return err
		}
		defer archive.Close()
		stream, err := s.docker.BuildImage(c.Request.Context(), archive, tags)
		if err != nil {
			return err
		}
		defer stream.Close()
		imageID := ""
		for {
			line, readErr := stream.Next()
			if errors.Is(readErr, io.EOF) {
				return send("done", gin.H{"image_id": imageID})
			}
			if readErr != nil {
				return readErr
			}
			if line.Error != "" {
				return errors.New(line.Error)
			}
			if line.Aux.ID != "" {
				imageID = line.Aux.ID
			}
			if line.Stream != "" {
				payload := gin.H{"line": line.Stream}
				if match := buildStepRE.FindStringSubmatch(line.Stream); len(match) == 3 {
					payload["step"], _ = strconv.Atoi(match[1])
					payload["total_steps"], _ = strconv.Atoi(match[2])
				}
				if err := send("build", payload); err != nil {
					return err
				}
			}
		}
	})
}
func (s *server) handleImageTag(c *gin.Context) {
	var body struct {
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if err := required(body.Repo, "repo"); err != nil {
		Fail(c, err)
		return
	}
	id := c.Param("id")
	if err := s.docker.TagImage(c.Request.Context(), id, body.Repo, body.Tag); err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}
func (s *server) handleImageRemove(c *gin.Context) {
	force, err := queryBool(c, "force")
	if err != nil {
		Fail(c, err)
		return
	}
	noprune, err := queryBool(c, "noprune")
	if err != nil {
		Fail(c, err)
		return
	}
	id := c.Param("id")
	if err = s.docker.RemoveImage(c.Request.Context(), id, dockerapi.RemoveImageOptions{Force: force, NoPrune: noprune}); err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleVolumeCreate(c *gin.Context) {
	var body struct {
		Name   string            `json:"name"`
		Driver string            `json:"driver"`
		Labels map[string]string `json:"labels"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if err := validResourceName(body.Name); err != nil {
		Fail(c, err)
		return
	}
	volume, err := s.docker.CreateVolume(c.Request.Context(), dockerapi.CreateVolumeOptions{Name: body.Name, Driver: body.Driver, Labels: body.Labels})
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, volumeToView(*volume, nil, nil, true))
}
func (s *server) handleVolumeRemove(c *gin.Context) {
	force, err := queryBool(c, "force")
	if err != nil {
		Fail(c, err)
		return
	}
	name := c.Param("name")
	if err = s.docker.RemoveVolume(c.Request.Context(), name, force); err != nil {
		Fail(c, forResource("volume", name, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleNetworkCreate(c *gin.Context) {
	var body struct {
		Name    string            `json:"name"`
		Driver  string            `json:"driver"`
		Subnet  string            `json:"subnet"`
		Gateway string            `json:"gateway"`
		Labels  map[string]string `json:"labels"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if err := validResourceName(body.Name); err != nil {
		Fail(c, err)
		return
	}
	network, err := s.docker.CreateNetwork(c.Request.Context(), dockerapi.CreateNetworkOptions{Name: body.Name, Driver: body.Driver, Subnet: body.Subnet, Gateway: body.Gateway, Labels: body.Labels})
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, networkToView(*network, false))
}
func (s *server) handleNetworkRemove(c *gin.Context) {
	id := c.Param("id")
	if err := s.docker.RemoveNetwork(c.Request.Context(), id); err != nil {
		Fail(c, forResource("network", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}
func (s *server) handleNetworkConnect(c *gin.Context)    { s.handleNetworkConnection(c, false) }
func (s *server) handleNetworkDisconnect(c *gin.Context) { s.handleNetworkConnection(c, true) }
func (s *server) handleNetworkConnection(c *gin.Context, disconnect bool) {
	var body struct {
		Container string `json:"container"`
	}
	if err := decodeBody(c, &body); err != nil {
		Fail(c, err)
		return
	}
	if err := required(body.Container, "container"); err != nil {
		Fail(c, err)
		return
	}
	id := c.Param("id")
	if err := s.docker.NetworkConnect(c.Request.Context(), id, body.Container, disconnect); err != nil {
		Fail(c, forResource("network", id, err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handlePrune(c *gin.Context) {
	kind := c.Param("kind")
	switch kind {
	case "containers", "images", "volumes", "networks", "builder":
	default:
		Fail(c, invalidInput("kind must be containers, images, volumes, networks, or builder"))
		return
	}
	report, err := s.docker.Prune(c.Request.Context(), kind)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (s *server) handleHost(c *gin.Context) {
	info, err := s.docker.Info(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	engineVersion, err := s.docker.Version(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	disk, err := s.docker.DiskUsage(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}

	running, err := s.docker.ListContainers(c.Request.Context(), dockerapi.ListContainersOptions{All: true})
	if err != nil {
		Fail(c, err)
		return
	}
	cpu, memory, top, err := s.hostAggregates(c.Request.Context(), running)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, hostView{
		ID:              info.ID,
		ServerVersion:   firstNonEmpty(info.ServerVersion, engineVersion.Version),
		APIVersion:      engineVersion.APIVersion,
		MinAPIVersion:   engineVersion.MinAPI,
		OperatingSystem: info.OperatingSystem,
		OSType:          firstNonEmpty(info.OSType, engineVersion.Os),
		Architecture:    firstNonEmpty(info.Architecture, engineVersion.Arch),
		KernelVersion:   engineVersion.KernelVer,
		CPUs:            info.NCPU,
		MemoryBytes:     info.MemTotal,
		CPUPercent:      cpu,
		Memory:          memory,
		Containers: hostContainersView{
			Total: info.Containers, Running: info.ContainersRunning,
			Paused: info.ContainersPaused, Stopped: info.ContainersStopped,
		},
		Images: info.Images,
		Disk:   diskToHostView(disk),
		TopCPU: topByCPU(top),
		TopMem: topByMem(top),
	})
}

type hostTopEntry struct {
	ID, Name          string
	CPUPercent        float64
	MemUsed, MemLimit uint64
}

func (s *server) hostAggregates(ctx context.Context, containers []dockerapi.Container) (float64, hostMemoryView, []hostTopEntry, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var cpu float64
	var memory hostMemoryView
	var top []hostTopEntry
	var firstErr error
	for _, container := range containers {
		if container.State != "running" {
			continue
		}
		id, name := container.ID, normalizedContainerName(container.Names)
		wg.Add(1)
		go func() {
			defer wg.Done()
			sample, err := s.docker.Stats(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if errors.Is(err, dockerapi.ErrNotFound) || errors.Is(err, dockerapi.ErrConflict) {
					return
				}
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			var cpuPct float64
			if sample.CPUPercent != nil {
				cpuPct = *sample.CPUPercent
			}
			cpu += cpuPct
			memory.Used += sample.MemUsage
			memory.Limit += sample.MemLimit
			top = append(top, hostTopEntry{ID: id, Name: name, CPUPercent: cpuPct, MemUsed: sample.MemUsage, MemLimit: sample.MemLimit})
		}()
	}
	wg.Wait()
	return cpu, memory, top, firstErr
}

func topByCPU(entries []hostTopEntry) []hostTopView {
	sorted := append([]hostTopEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CPUPercent > sorted[j].CPUPercent })
	return topEntriesToView(sorted)
}

func topByMem(entries []hostTopEntry) []hostTopView {
	sorted := append([]hostTopEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MemUsed > sorted[j].MemUsed })
	return topEntriesToView(sorted)
}

func topEntriesToView(sorted []hostTopEntry) []hostTopView {
	if len(sorted) > 5 {
		sorted = sorted[:5]
	}
	views := make([]hostTopView, 0, len(sorted))
	for _, e := range sorted {
		views = append(views, hostTopView{ID: e.ID, Name: e.Name, CPUPercent: e.CPUPercent, MemUsed: e.MemUsed, MemLimit: e.MemLimit})
	}
	return views
}

func diskToHostView(disk *dockerapi.DiskUsageInfo) hostDiskView {
	var view hostDiskView
	for _, image := range disk.Images {
		view.Images += image.Size
		if image.Containers == 0 {
			view.Reclaimable += image.Size
			view.ImagesReclaimable += image.Size
		}
	}
	for _, container := range disk.Containers {
		view.Containers += container.SizeRW
		if container.State != "running" {
			view.Reclaimable += container.SizeRW
			view.ContainersReclaimable += container.SizeRW
		}
	}
	for _, volume := range disk.Volumes {
		view.Volumes += volume.UsageData.Size
		if volume.UsageData.RefCount == 0 {
			view.Reclaimable += volume.UsageData.Size
			view.VolumesReclaimable += volume.UsageData.Size
		}
	}
	for _, cache := range disk.BuildCache {
		view.BuildCache += cache.Size
		if cache.UsageCount == 0 {
			view.Reclaimable += cache.Size
			view.BuildCacheReclaimable += cache.Size
		}
	}
	return view
}

func (s *server) handleContainers(c *gin.Context) {
	query, err := parseCollectionQuery(c.Request.URL.Query(), "containers")
	if err != nil {
		Fail(c, err)
		return
	}
	containers, err := s.docker.ListContainers(c.Request.Context(), dockerapi.ListContainersOptions{All: query.all})
	if err != nil {
		Fail(c, err)
		return
	}
	views := make([]containerView, 0, len(containers))
	for _, container := range containers {
		views = append(views, containerToView(container))
	}
	c.JSON(http.StatusOK, filterSortContainers(views, query))
}

func (s *server) handleContainer(c *gin.Context) {
	id := c.Param("id")
	container, err := s.docker.InspectContainer(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	c.JSON(http.StatusOK, containerDetailToView(container))
}

func (s *server) handleContainerTop(c *gin.Context) {
	id := c.Param("id")
	top, err := s.docker.Top(c.Request.Context(), id, c.Query("ps_args"))
	if err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	c.JSON(http.StatusOK, topView{Titles: nonNilSlice(top.Titles), Processes: nonNilSlice(top.Processes)})
}

func (s *server) handleImages(c *gin.Context) {
	query, err := parseCollectionQuery(c.Request.URL.Query(), "images")
	if err != nil {
		Fail(c, err)
		return
	}
	images, err := s.docker.ListImages(c.Request.Context(), false)
	if err != nil {
		Fail(c, err)
		return
	}
	containers, err := s.allContainers(c)
	if err != nil {
		Fail(c, err)
		return
	}
	views := make([]imageView, 0, len(images))
	for _, image := range images {
		views = append(views, imageToView(image, containers))
	}
	c.JSON(http.StatusOK, filterSortImages(views, query))
}

func (s *server) handleImage(c *gin.Context) {
	id := c.Param("id")
	image, err := s.docker.InspectImage(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	containers, err := s.allContainers(c)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, imageDetailToView(image, containers))
}

func (s *server) handleImageHistory(c *gin.Context) {
	id := c.Param("id")
	layers, err := s.docker.History(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	c.JSON(http.StatusOK, historyToView(layers))
}

func (s *server) handleImageDockerfile(c *gin.Context) {
	id := c.Param("id")
	image, err := s.docker.InspectImage(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	history, err := s.docker.History(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("image", id, err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"dockerfile": dockerapi.ReconstructDockerfile(history, image.Config), "approximate": true})
}

func (s *server) handleVolumes(c *gin.Context) {
	query, err := parseCollectionQuery(c.Request.URL.Query(), "volumes")
	if err != nil {
		Fail(c, err)
		return
	}
	volumes, err := s.docker.ListVolumes(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	containers, err := s.allContainers(c)
	if err != nil {
		Fail(c, err)
		return
	}
	// Disk usage is an optional Docker capability. A volume list remains useful
	// when an older engine omits UsageData or /system/df cannot be read.
	sizes := map[string]*int64{}
	if disk, diskErr := s.docker.DiskUsage(c.Request.Context()); diskErr == nil {
		sizes = volumeSizes(disk)
	}
	views := make([]volumeView, 0, len(volumes))
	for _, volume := range volumes {
		views = append(views, volumeToView(volume, containers, sizes[volume.Name], false))
	}
	c.JSON(http.StatusOK, filterSortVolumes(views, query))
}

func (s *server) handleVolume(c *gin.Context) {
	name := c.Param("name")
	volume, err := s.docker.InspectVolume(c.Request.Context(), name)
	if err != nil {
		Fail(c, forResource("volume", name, err))
		return
	}
	containers, err := s.allContainers(c)
	if err != nil {
		Fail(c, err)
		return
	}
	sizes := map[string]*int64{}
	if disk, diskErr := s.docker.DiskUsage(c.Request.Context()); diskErr == nil {
		sizes = volumeSizes(disk)
	}
	c.JSON(http.StatusOK, volumeToView(*volume, containers, sizes[volume.Name], true))
}

func (s *server) handleNetworks(c *gin.Context) {
	query, err := parseCollectionQuery(c.Request.URL.Query(), "networks")
	if err != nil {
		Fail(c, err)
		return
	}
	networks, err := s.docker.ListNetworks(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	views := make([]networkView, 0, len(networks))
	for _, network := range networks {
		views = append(views, networkToView(network, false))
	}
	c.JSON(http.StatusOK, filterSortNetworks(views, query))
}

func (s *server) handleNetwork(c *gin.Context) {
	id := c.Param("id")
	network, err := s.docker.InspectNetwork(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("network", id, err))
		return
	}
	c.JSON(http.StatusOK, networkToView(*network, true))
}

func (s *server) allContainers(c *gin.Context) ([]dockerapi.Container, error) {
	return s.docker.ListContainers(c.Request.Context(), dockerapi.ListContainersOptions{All: true})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
