package api

import (
	"context"
	"io"
	"net/url"

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
	LogStream(context.Context, string, dockerapi.LogsOptions) (dockerapi.LogStream, error)
	StatsStream(context.Context, string) (dockerapi.StatsStream, error)
	Stats(context.Context, string) (dockerapi.Stats, error)
	Events(context.Context, dockerapi.EventsOptions) (*dockerapi.EventReader, error)
	Top(context.Context, string, string) (*dockerapi.TopEntry, error)
	ListImages(context.Context, bool) ([]dockerapi.Image, error)
	InspectImage(context.Context, string) (*dockerapi.ImageDetail, error)
	History(context.Context, string) ([]dockerapi.HistoryLayer, error)
	ExportImages(context.Context, []string) (io.ReadCloser, error)
	ImportImages(context.Context, io.Reader) (dockerapi.ImportStream, error)
	BuildImage(context.Context, io.Reader, []string) (dockerapi.BuildStream, error)
	ListVolumes(context.Context) ([]dockerapi.Volume, error)
	InspectVolume(context.Context, string) (*dockerapi.Volume, error)
	ListNetworks(context.Context) ([]dockerapi.Network, error)
	InspectNetwork(context.Context, string) (*dockerapi.Network, error)
	Lifecycle(context.Context, string, string, url.Values) error
	RenameContainer(context.Context, string, string) error
	RemoveContainer(context.Context, string, dockerapi.RemoveContainerOptions) error
	CreateContainer(context.Context, dockerapi.Spec) (dockerapi.CreateResult, error)
	CommitContainer(context.Context, string, dockerapi.CommitOptions) (dockerapi.CommitResult, error)
	PullImage(context.Context, string) (dockerapi.PullStream, error)
	TagImage(context.Context, string, string, string) error
	RemoveImage(context.Context, string, dockerapi.RemoveImageOptions) error
	CreateVolume(context.Context, dockerapi.CreateVolumeOptions) (*dockerapi.Volume, error)
	RemoveVolume(context.Context, string, bool) error
	WithVolumeMount(context.Context, string, func(string) error) error
	CloneVolume(context.Context, string, string) error
	CreateNetwork(context.Context, dockerapi.CreateNetworkOptions) (*dockerapi.Network, error)
	RemoveNetwork(context.Context, string) error
	NetworkConnect(context.Context, string, string, bool) error
	Prune(context.Context, string) (*dockerapi.PruneReport, error)
	CreateExec(context.Context, string, dockerapi.ExecOptions) (string, error)
	StartExec(context.Context, string, bool) (dockerapi.ExecSession, error)
	ResizeExec(context.Context, string, int, int) error
	ListDirectory(context.Context, string, string) ([]dockerapi.FileEntry, error)
	UploadFiles(context.Context, string, string, io.Reader) error
	CreateDirectory(context.Context, string, string) error
	DownloadPath(context.Context, string, string) (io.ReadCloser, error)
	RemovePath(context.Context, string, string) error
	RenamePath(context.Context, string, string, string) error
	ReadFile(context.Context, string, string) (string, []byte, error)
	WriteFile(context.Context, string, string, []byte) error
}
