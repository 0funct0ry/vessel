package api

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type portView struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}

type mountView struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

type containerNetworkView struct {
	NetworkID string `json:"network_id"`
	IPAddress string `json:"ip_address"`
}

type containerView struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	ImageID string            `json:"image_id"`
	Command string            `json:"command"`
	Created int64             `json:"created"`
	State   string            `json:"state"`
	Status  string            `json:"status"`
	Health  string            `json:"health"`
	Ports   []portView        `json:"ports"`
	Labels  map[string]string `json:"labels"`
}

type containerDetailView struct {
	ID            string                          `json:"id"`
	Name          string                          `json:"name"`
	Image         string                          `json:"image"`
	Command       []string                        `json:"command"`
	Created       string                          `json:"created"`
	State         string                          `json:"state"`
	Status        string                          `json:"status"`
	ExitCode      int                             `json:"exit_code"`
	Health        string                          `json:"health"`
	RestartPolicy string                          `json:"restart_policy"`
	Mounts        []mountView                     `json:"mounts"`
	Networks      map[string]containerNetworkView `json:"networks"`
	Env           []string                        `json:"env"`
	Labels        map[string]string               `json:"labels"`
	Raw           json.RawMessage                 `json:"raw"`
}

type imageView struct {
	ID          string            `json:"id"`
	RepoTags    []string          `json:"repo_tags"`
	RepoDigests []string          `json:"repo_digests"`
	Created     int64             `json:"created"`
	Size        int64             `json:"size"`
	Labels      map[string]string `json:"labels"`
	UsedByCount int               `json:"used_by_count"`
	Dangling    bool              `json:"dangling"`
}

type imageDetailView struct {
	ID           string            `json:"id"`
	RepoTags     []string          `json:"repo_tags"`
	RepoDigests  []string          `json:"repo_digests"`
	Created      string            `json:"created"`
	Size         int64             `json:"size"`
	Architecture string            `json:"architecture"`
	OS           string            `json:"os"`
	Env          []string          `json:"env"`
	Entrypoint   []string          `json:"entrypoint"`
	Cmd          []string          `json:"cmd"`
	Labels       map[string]string `json:"labels"`
	UsedByCount  int               `json:"used_by_count"`
	UsedBy       []imageUseView    `json:"used_by"`
	Dangling     bool              `json:"dangling"`
	Raw          json.RawMessage   `json:"raw"`
}

type imageUseView struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	State         string `json:"state"`
}

type historyLayerView struct {
	ID        string   `json:"id"`
	Created   int64    `json:"created"`
	CreatedBy string   `json:"created_by"`
	Size      int64    `json:"size"`
	Comment   string   `json:"comment"`
	Tags      []string `json:"tags"`
}

type volumeUseView struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	MountPath     string `json:"mount_path"`
	RW            bool   `json:"rw"`
}

type volumeView struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	SizeBytes  *int64            `json:"size_bytes,omitempty"`
	CreatedAt  string            `json:"created_at"`
	Labels     map[string]string `json:"labels"`
	Scope      string            `json:"scope"`
	UsedBy     []volumeUseView   `json:"used_by"`
	Raw        json.RawMessage   `json:"raw,omitempty"`
}

type ipamConfigView struct {
	Subnet       string            `json:"subnet,omitempty"`
	Gateway      string            `json:"gateway,omitempty"`
	IPRange      string            `json:"ip_range,omitempty"`
	AuxAddresses map[string]string `json:"aux_addresses,omitempty"`
}

type networkConnectionView struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	IPv4Address   string `json:"ipv4_address"`
	IPv6Address   string `json:"ipv6_address"`
}

type networkView struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Driver      string                  `json:"driver"`
	Scope       string                  `json:"scope"`
	Internal    bool                    `json:"internal"`
	Attachable  bool                    `json:"attachable"`
	EnableIPv6  bool                    `json:"enable_ipv6"`
	IPAMDriver  string                  `json:"ipam_driver"`
	IPAM        []ipamConfigView        `json:"ipam"`
	IPAMOptions map[string]string       `json:"ipam_options"`
	DriverOpts  map[string]string       `json:"driver_opts"`
	Labels      map[string]string       `json:"labels"`
	Containers  []networkConnectionView `json:"containers"`
	Raw         json.RawMessage         `json:"raw,omitempty"`
}

type hostContainersView struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Stopped int `json:"stopped"`
}

type hostDiskView struct {
	Images                int64 `json:"images"`
	Containers            int64 `json:"containers"`
	Volumes               int64 `json:"volumes"`
	BuildCache            int64 `json:"build_cache"`
	Reclaimable           int64 `json:"reclaimable"`
	ImagesReclaimable     int64 `json:"images_reclaimable"`
	ContainersReclaimable int64 `json:"containers_reclaimable"`
	VolumesReclaimable    int64 `json:"volumes_reclaimable"`
	BuildCacheReclaimable int64 `json:"build_cache_reclaimable"`
}

type hostMemoryView struct {
	Used  uint64 `json:"used"`
	Limit uint64 `json:"limit"`
}

type hostView struct {
	ID              string             `json:"id"`
	ServerVersion   string             `json:"server_version"`
	APIVersion      string             `json:"api_version"`
	MinAPIVersion   string             `json:"min_api_version"`
	OperatingSystem string             `json:"operating_system"`
	OSType          string             `json:"os_type"`
	Architecture    string             `json:"architecture"`
	KernelVersion   string             `json:"kernel_version"`
	CPUs            int                `json:"cpus"`
	MemoryBytes     int64              `json:"memory_bytes"`
	CPUPercent      float64            `json:"cpu_pct"`
	Memory          hostMemoryView     `json:"memory"`
	Containers      hostContainersView `json:"containers"`
	Images          int                `json:"images"`
	Disk            hostDiskView       `json:"disk"`
	TopCPU          []hostTopView      `json:"top_cpu"`
	TopMem          []hostTopView      `json:"top_mem"`
}

type hostTopView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_pct,omitempty"`
	MemUsed    uint64  `json:"mem_used,omitempty"`
	MemLimit   uint64  `json:"mem_limit,omitempty"`
}

type topView struct {
	Titles    []string   `json:"titles"`
	Processes [][]string `json:"processes"`
}

func normalizedContainerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func containerToView(v dockerapi.Container) containerView {
	ports := make([]portView, 0, len(v.Ports))
	for _, p := range v.Ports {
		ports = append(ports, portView{p.IP, p.PrivatePort, p.PublicPort, p.Type})
	}
	return containerView{
		ID: v.ID, Name: normalizedContainerName(v.Names), Image: v.Image,
		ImageID: v.ImageID, Command: v.Command, Created: v.Created, State: v.State,
		Status: v.Status, Health: v.Health, Ports: ports, Labels: nonNilMap(v.Labels),
	}
}

func containerDetailToView(v *dockerapi.ContainerDetail) containerDetailView {
	mounts := make([]mountView, 0, len(v.Mounts))
	for _, m := range v.Mounts {
		mounts = append(mounts, mountView{m.Type, m.Name, m.Source, m.Destination, m.RW})
	}
	networks := make(map[string]containerNetworkView, len(v.Networks))
	for name, n := range v.Networks {
		networks[name] = containerNetworkView{n.NetworkID, n.IPAddress}
	}
	return containerDetailView{
		ID: v.ID, Name: strings.TrimPrefix(v.Name, "/"), Image: v.Image, Command: nonNilSlice(v.Command),
		Created: v.Created, State: v.State, Status: v.Status, ExitCode: v.ExitCode, Health: v.Health,
		RestartPolicy: v.RestartPolicy, Mounts: mounts, Networks: networks, Env: nonNilSlice(v.Env),
		Labels: nonNilMap(v.Labels), Raw: v.Raw,
	}
}

func imageInUseCount(imageID string, tags []string, containers []dockerapi.Container) int {
	count := 0
	for _, c := range containers {
		used := imageID != "" && c.ImageID == imageID
		if !used && c.ImageID == "" {
			for _, tag := range tags {
				if c.Image == tag {
					used = true
					break
				}
			}
		}
		if used {
			count++
		}
	}
	return count
}

func isDangling(tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, tag := range tags {
		if tag != "" && tag != "<none>:<none>" {
			return false
		}
	}
	return true
}

func imageToView(v dockerapi.Image, containers []dockerapi.Container) imageView {
	return imageView{
		ID: v.ID, RepoTags: nonNilSlice(v.RepoTags), RepoDigests: nonNilSlice(v.RepoDigests),
		Created: v.Created, Size: v.Size, Labels: nonNilMap(v.Labels),
		UsedByCount: imageInUseCount(v.ID, v.RepoTags, containers), Dangling: isDangling(v.RepoTags),
	}
}

func imageDetailToView(v *dockerapi.ImageDetail, containers []dockerapi.Container) imageDetailView {
	return imageDetailView{
		ID: v.ID, RepoTags: nonNilSlice(v.RepoTags), RepoDigests: nonNilSlice(v.RepoDigests), Created: v.Created,
		Size: v.Size, Architecture: v.Architecture, OS: v.Os, Env: nonNilSlice(v.Env),
		Entrypoint: nonNilSlice(v.Entrypoint), Cmd: nonNilSlice(v.Cmd), Labels: nonNilMap(v.Labels),
		UsedByCount: imageInUseCount(v.ID, v.RepoTags, containers), UsedBy: imageUses(v.ID, v.RepoTags, containers),
		Dangling: isDangling(v.RepoTags), Raw: v.Raw,
	}
}

func imageUses(imageID string, tags []string, containers []dockerapi.Container) []imageUseView {
	uses := make([]imageUseView, 0)
	for _, c := range containers {
		used := imageID != "" && c.ImageID == imageID
		if !used && c.ImageID == "" {
			for _, tag := range tags {
				if c.Image == tag {
					used = true
					break
				}
			}
		}
		if used {
			uses = append(uses, imageUseView{c.ID, normalizedContainerName(c.Names), c.State})
		}
	}
	sort.SliceStable(uses, func(i, j int) bool {
		if uses[i].ContainerName == uses[j].ContainerName {
			return uses[i].ContainerID < uses[j].ContainerID
		}
		return uses[i].ContainerName < uses[j].ContainerName
	})
	return uses
}

func historyToView(layers []dockerapi.HistoryLayer) []historyLayerView {
	views := make([]historyLayerView, 0, len(layers))
	for _, layer := range layers {
		views = append(views, historyLayerView{
			ID: layer.ID, Created: layer.Created, CreatedBy: layer.CreatedBy,
			Size: layer.Size, Comment: layer.Comment, Tags: nonNilSlice(layer.Tags),
		})
	}
	return views
}

func volumeUses(name string, containers []dockerapi.Container) []volumeUseView {
	uses := make([]volumeUseView, 0)
	for _, c := range containers {
		for _, mount := range c.Mounts {
			if mount.Type == "volume" && mount.Name == name {
				uses = append(uses, volumeUseView{c.ID, normalizedContainerName(c.Names), mount.Destination, mount.RW})
			}
		}
	}
	sort.SliceStable(uses, func(i, j int) bool {
		if uses[i].ContainerName == uses[j].ContainerName {
			return uses[i].ContainerID < uses[j].ContainerID
		}
		return uses[i].ContainerName < uses[j].ContainerName
	})
	return uses
}

func volumeToView(v dockerapi.Volume, containers []dockerapi.Container, sizeBytes *int64, includeRaw bool) volumeView {
	view := volumeView{
		Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint, CreatedAt: v.CreatedAt,
		SizeBytes: sizeBytes, Labels: nonNilMap(v.Labels), Scope: v.Scope, UsedBy: volumeUses(v.Name, containers),
	}
	if includeRaw {
		view.Raw = v.Raw
	}
	return view
}

func volumeSizes(disk *dockerapi.DiskUsageInfo) map[string]*int64 {
	sizes := make(map[string]*int64)
	if disk == nil {
		return sizes
	}
	for _, volume := range disk.Volumes {
		if volume.Name == "" || !volume.UsageKnown {
			continue
		}
		size := volume.UsageData.Size
		sizes[volume.Name] = &size
	}
	return sizes
}

func networkToView(v dockerapi.Network, includeRaw bool) networkView {
	ipam := make([]ipamConfigView, 0, len(v.IPAM.Config))
	for _, cfg := range v.IPAM.Config {
		ipam = append(ipam, ipamConfigView{cfg.Subnet, cfg.Gateway, cfg.IPRange, nonNilMap(cfg.AuxAddress)})
	}
	connections := make([]networkConnectionView, 0, len(v.Containers))
	for id, c := range v.Containers {
		connections = append(connections, networkConnectionView{id, c.Name, c.IPv4Address, c.IPv6Address})
	}
	sort.SliceStable(connections, func(i, j int) bool {
		if connections[i].ContainerName == connections[j].ContainerName {
			return connections[i].ContainerID < connections[j].ContainerID
		}
		return connections[i].ContainerName < connections[j].ContainerName
	})
	view := networkView{
		ID: v.ID, Name: v.Name, Driver: v.Driver, Scope: v.Scope,
		Internal: v.Internal, Attachable: v.Attachable, EnableIPv6: v.EnableIPv6,
		IPAMDriver: v.IPAM.Driver, IPAM: ipam, IPAMOptions: nonNilMap(v.IPAM.Options), DriverOpts: nonNilMap(v.Options),
		Labels: nonNilMap(v.Labels), Containers: connections,
	}
	if includeRaw {
		view.Raw = v.Raw
	}
	return view
}

func nonNilMap(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}

func nonNilSlice[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
