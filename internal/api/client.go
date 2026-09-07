package api

import (
	"context"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

// DockerClient is the subset of the Engine client used by the read-only HTTP
// surface. It lives in api so handlers can be tested without a Docker daemon.
type DockerClient interface {
	Info(context.Context) (*dockerapi.Info, error)
	Version(context.Context) (*dockerapi.VersionInfo, error)
	DiskUsage(context.Context) (*dockerapi.DiskUsageInfo, error)
	ListContainers(context.Context, dockerapi.ListContainersOptions) ([]dockerapi.Container, error)
	InspectContainer(context.Context, string) (*dockerapi.ContainerDetail, error)
	Top(context.Context, string, string) (*dockerapi.TopEntry, error)
	ListImages(context.Context, bool) ([]dockerapi.Image, error)
	InspectImage(context.Context, string) (*dockerapi.ImageDetail, error)
	ListVolumes(context.Context) ([]dockerapi.Volume, error)
	InspectVolume(context.Context, string) (*dockerapi.Volume, error)
	ListNetworks(context.Context) ([]dockerapi.Network, error)
	InspectNetwork(context.Context, string) (*dockerapi.Network, error)
}
