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
export interface Host { server_version: string; api_version: string; containers: { total: number; running: number; paused: number; stopped: number } }
export interface Stats { ts: string; cpu_pct: number | null; mem: { used: number; limit: number }; net: { rx: number; tx: number }; blk: { read: number; write: number } }
export interface LogLine { ts?: string; stream: "stdout" | "stderr" | "vessel"; line: string }
