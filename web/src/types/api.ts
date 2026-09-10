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
export interface Container { id: string; name: string; image: string; image_id: string; command: string; created: number; state: string; status: string; ports: ContainerPort[]; labels: Record<string, string> }
export interface ContainerMount { type: string; name?: string; source: string; destination: string; rw: boolean }
export interface ContainerNetwork { network_id: string; ip_address: string }
export interface ContainerDetail { id: string; name: string; image: string; command: string[]; created: string; state: string; status: string; exit_code: number; health: string; restart_policy: string; mounts: ContainerMount[]; networks: Record<string, ContainerNetwork>; env: string[]; labels: Record<string, string>; raw: unknown }
export interface HostDisk { images: number; containers: number; volumes: number; build_cache: number; reclaimable: number; images_reclaimable: number; containers_reclaimable: number; volumes_reclaimable: number; build_cache_reclaimable: number }
export interface Host { server_version: string; api_version: string; containers: { total: number; running: number; paused: number; stopped: number }; disk: HostDisk }
export interface Image { id: string; repo_tags: string[]; repo_digests: string[]; created: number; size: number; labels: Record<string, string>; used_by_count: number; dangling: boolean }
export interface ImageUse { container_id: string; container_name: string; state: string }
export interface ImageDetail { id: string; repo_tags: string[]; repo_digests: string[]; created: string; size: number; architecture: string; os: string; env: string[]; entrypoint: string[]; cmd: string[]; labels: Record<string, string>; used_by_count: number; used_by: ImageUse[]; dangling: boolean; raw: unknown }
export interface HistoryLayer { id: string; created: number; created_by: string; size: number; comment: string; tags: string[] }
export interface DockerfileReconstruction { dockerfile: string; approximate: true }
export interface Volume { name: string; driver: string }
export interface Network { id: string; name: string; driver: string }
export interface CreateContainerResponse { id: string; name: string; warnings: string[]; start_error?: string }
export interface CommitContainerResponse { image_id: string }
export interface Top { titles: string[]; processes: string[][] }
export interface PullEvent { id: string; status: string; current?: number; total?: number; error?: string }
export interface Stats { ts: string; cpu_pct: number | null; mem: { used: number; limit: number }; net: { rx: number; tx: number }; blk: { read: number; write: number } }
export interface LogLine { ts?: string; stream: "stdout" | "stderr" | "vessel"; line: string }
export interface ContainerFileEntry { name: string; path: string; type: "file" | "dir" | "symlink"; size: number; mode: string; modified_at: string }
export interface ContainerFileView { name: string; size: number; mime: string; kind: "text" | "image" | "binary"; content?: string }
