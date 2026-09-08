package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

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
	c.JSON(http.StatusCreated, volumeToView(*volume, nil, true))
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
	cpu, memory, err := s.hostAggregates(c.Request.Context(), running)
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
	})
}

func (s *server) hostAggregates(ctx context.Context, containers []dockerapi.Container) (float64, hostMemoryView, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var cpu float64
	var memory hostMemoryView
	var firstErr error
	for _, container := range containers {
		if container.State != "running" {
			continue
		}
		id := container.ID
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
			if sample.CPUPercent != nil {
				cpu += *sample.CPUPercent
			}
			memory.Used += sample.MemUsage
			memory.Limit += sample.MemLimit
		}()
	}
	wg.Wait()
	return cpu, memory, firstErr
}

func diskToHostView(disk *dockerapi.DiskUsageInfo) hostDiskView {
	var view hostDiskView
	for _, image := range disk.Images {
		view.Images += image.Size
		if image.Containers == 0 {
			view.Reclaimable += image.Size
		}
	}
	for _, container := range disk.Containers {
		view.Containers += container.SizeRW
		if container.State != "running" {
			view.Reclaimable += container.SizeRW
		}
	}
	for _, volume := range disk.Volumes {
		view.Volumes += volume.UsageData.Size
		if volume.UsageData.RefCount == 0 {
			view.Reclaimable += volume.UsageData.Size
		}
	}
	for _, cache := range disk.BuildCache {
		view.BuildCache += cache.Size
		if cache.UsageCount == 0 {
			view.Reclaimable += cache.Size
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
	views := make([]volumeView, 0, len(volumes))
	for _, volume := range volumes {
		views = append(views, volumeToView(volume, containers, false))
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
	c.JSON(http.StatusOK, volumeToView(*volume, containers, true))
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
