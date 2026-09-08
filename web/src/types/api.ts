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
