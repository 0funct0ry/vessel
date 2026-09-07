package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

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
		Containers: hostContainersView{
			Total: info.Containers, Running: info.ContainersRunning,
			Paused: info.ContainersPaused, Stopped: info.ContainersStopped,
		},
		Images: info.Images,
		Disk: hostDiskView{
			LayersSize: disk.LayersSize, Images: len(disk.Images), Containers: len(disk.Containers),
			Volumes: len(disk.Volumes), BuildCache: len(disk.BuildCache),
		},
	})
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
