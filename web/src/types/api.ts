export type Role = "admin" | "operator" | "viewer";

export interface User {
  id: string;
  username: string;
  role: Role;
}

export type Capabilities = Record<string, boolean>;

export interface MeResponse {
  auth: boolean;
  user: Partial<User> & { role: Role };
  capabilities: Capabilities;
}

export interface ApiError {
  code: string;
  message: string;
  docker_status?: number;
}

export class ApiRequestError extends Error {
  code: string;
  status: number;
  dockerStatus?: number;

  constructor(status: number, err: ApiError) {
    super(err.message);
    this.name = "ApiRequestError";
    this.code = err.code;
    this.status = status;
    this.dockerStatus = err.docker_status;
  }
}

export interface ContainerPort { ip?: string; private_port: number; public_port?: number; type: string }
export interface Container { id: string; name: string; image: string; image_id: string; command: string; created: number; state: string; status: string; health: string; ports: ContainerPort[]; labels: Record<string, string> }
export interface ContainerMount { type: string; name?: string; source: string; destination: string; rw: boolean }
export interface ContainerNetwork { network_id: string; ip_address: string }
export interface ContainerSecurity { privileged: boolean; readonly_rootfs: boolean; user: string; userns_mode: string; apparmor_profile: string }
export interface ContainerResources { cpu_shares: number; cpus: number; memory: number; memory_swap: number; memory_reservation: number; pids_limit: number; oom_kill_disable: boolean; cpu_period: number; cpu_quota: number; cgroup_parent: string; cgroupns_mode: string }
export interface ContainerDetail { id: string; name: string; image: string; command: string[]; created: string; state: string; status: string; exit_code: number; health: string; restart_policy: string; mounts: ContainerMount[]; networks: Record<string, ContainerNetwork>; env: string[]; labels: Record<string, string>; ports: ContainerPort[]; security: ContainerSecurity; resources: ContainerResources; raw: unknown }
export interface HostDisk { images: number; containers: number; volumes: number; build_cache: number; reclaimable: number; images_reclaimable: number; containers_reclaimable: number; volumes_reclaimable: number; build_cache_reclaimable: number }
export interface HostTopEntry { id: string; name: string; cpu_pct?: number; mem_used?: number; mem_limit?: number }
export interface Host { server_version: string; api_version: string; operating_system: string; os_type: string; architecture: string; kernel_version: string; cpus: number; memory_bytes: number; cpu_pct: number; memory: { used: number; limit: number }; containers: { total: number; running: number; paused: number; stopped: number }; disk: HostDisk; top_cpu: HostTopEntry[]; top_mem: HostTopEntry[] }
export interface Image { id: string; repo_tags: string[]; repo_digests: string[]; created: number; size: number; labels: Record<string, string>; used_by_count: number; dangling: boolean }
export interface ImageUse { container_id: string; container_name: string; state: string }
export interface ImageDetail { id: string; repo_tags: string[]; repo_digests: string[]; created: string; size: number; architecture: string; os: string; env: string[]; entrypoint: string[]; cmd: string[]; labels: Record<string, string>; used_by_count: number; used_by: ImageUse[]; dangling: boolean; raw: unknown }
export interface HistoryLayer { id: string; created: number; created_by: string; size: number; comment: string; tags: string[] }
export interface DockerfileReconstruction { dockerfile: string; approximate: true }
export interface VolumeUse { container_id: string; container_name: string; mount_path: string; rw: boolean }
export interface Volume { name: string; driver: string; mountpoint: string; size_bytes?: number; created_at: string; labels: Record<string, string>; scope: string; used_by: VolumeUse[]; raw?: unknown }
export interface NetworkIPAM { subnet?: string; gateway?: string; ip_range?: string; aux_addresses?: Record<string, string> }
export interface NetworkConnection { container_id: string; container_name: string; ipv4_address: string; ipv6_address: string }
export interface Network { id: string; name: string; driver: string; scope: string; internal: boolean; attachable: boolean; enable_ipv6: boolean; ipam_driver: string; ipam: NetworkIPAM[]; ipam_options: Record<string, string>; driver_opts: Record<string, string>; labels: Record<string, string>; containers: NetworkConnection[]; raw?: unknown }
export interface CreateContainerResponse { id: string; name: string; warnings: string[]; start_error?: string }
export interface CommitContainerResponse { image_id: string }
export interface Top { titles: string[]; processes: string[][] }
export interface PullEvent { id: string; status: string; current?: number; total?: number; error?: string }
export interface Stats { ts: string; cpu_pct: number | null; mem: { used: number; limit: number }; net: { rx: number; tx: number }; blk: { read: number; write: number } }
export interface LogLine { ts?: string; stream: "stdout" | "stderr" | "vessel"; line: string }
export interface DockerEvent { event_id?: string; type: "container" | "image" | "volume" | "network" | string; action: string; id: string; name?: string; attrs: Record<string, string>; timestamp: string }
export interface ContainerFileEntry { name: string; path: string; type: "file" | "dir" | "symlink"; size: number; mode: string; modified_at: string }
export interface ContainerFileView { name: string; size: number; mime: string; kind: "text" | "image" | "binary"; content?: string }
export interface StackWarning { kind: string; message: string }
export interface StackService { name: string; image: string; container_id?: string; container_name?: string; state: string; status?: string }
export type StackStatus = "running" | "stopped" | "partial" | "not_deployed";
export interface Stack { id: string; name: string; source: string; compose_yaml: string; env_content: string; status: StackStatus; service_count: number; container_count: number; services: StackService[]; warnings: StackWarning[]; parse_error?: string; created_at: string; updated_at: string }
export interface StackEvent { kind: string; name: string; phase: string; detail: string }
export interface StackLogLine { service: string; stream: string; ts?: string; line: string }
