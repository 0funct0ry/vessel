# Vessel

![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)
![Build](https://github.com/0funct0ry/vessel/actions/workflows/ci.yml/badge.svg)

Vessel is a single binary that gives you a web dashboard for the Docker
containers running on one machine. Download it, run it, and you can see and
control every container, image, volume, and network on that host from your
browser — no agent to install, no database to set up, no extra services to
run alongside it.

**Built for one host at a time.** Vessel talks to a single Docker Engine. It
does not manage clusters, Swarm, or Kubernetes.

<img src="docs/img/screenshot.png" alt="Vessel container list">

## Key features

- **See everything at a glance.** A live dashboard shows every container,
  image, volume, and network on the host, plus CPU, memory, and network
  usage updating in real time.
- **Control containers from your browser.** Start, stop, restart, pause,
  rename, or remove containers — and open an interactive terminal
  (`exec`) session inside any running one.
- **Manage images and volumes.** Pull images with live progress, build from
  an uploaded Dockerfile, export or import tarballs, and browse or edit
  files inside a running container or volume.
- **Deploy stacks.** Write, visually edit, and run multi-container Compose
  stacks — bring them up, tear them down, or redeploy with one click.
- **Get notified the moment something changes.** Subscribe to container,
  image, volume, or network events with signed (HMAC-SHA256) webhooks,
  complete with retries, a delivery log, and replay.
- **Lock it down when you need to.** Optional login with JWT sessions and
  three roles (admin, operator, viewer) — and Vessel refuses to expose
  itself to the network at all unless you turn authentication on.
- **One file, nothing else to install.** The entire web interface is baked
  into the binary. No database is required — Vessel runs fully in memory by
  default, or persists to a single SQLite file if you point it at one.

## Prerequisites

| Requirement      | Version                                         | Needed for                             |
|------------------|-------------------------------------------------|----------------------------------------|
| Docker Engine    | Any recent version, reachable via socket or TCP | Running Vessel at all                  |
| Go               | 1.23+                                           | Building from source or `go install`   |
| Node.js          | 20+                                             | Building the web interface from source |
| `curl` and `tar` | —                                               | The install script                     |

Prebuilt binaries and the Docker image need none of the above — only Docker
itself, to give Vessel something to manage.

## Getting started

### Install a prebuilt binary

```bash
curl -fsSL https://raw.githubusercontent.com/0funct0ry/vessel/main/install.sh | sh
```

This downloads the right binary for your OS and CPU, verifies its checksum,
and installs it to `/usr/local/bin` (override with `INSTALL_DIR`).

### Or run it with Docker

```bash
docker run -d --name vessel \
  -p 7373:7373 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v vessel-data:/data \
  ghcr.io/0funct0ry/vessel:latest \
  serve --addr 0.0.0.0 --auth --db /data/vessel.db
```

### Or install from source

1. **Clone the repository**

   ```bash
   git clone https://github.com/0funct0ry/vessel.git
   cd vessel
   ```

2. **Install dependencies**

   ```bash
   cd web && npm ci && cd ..
   ```

3. **Configure Vessel** (all optional — every setting has a sensible
   default, so you can skip straight to step 4)

   ```bash
   export VESSEL_ADDR=127.0.0.1
   export VESSEL_PORT=7373
   export VESSEL_DB=./vessel.db   # omit to run fully in-memory
   ```

4. **Build and run**

   ```bash
   make build
   ./bin/vessel serve
   ```

Open `http://localhost:7373` in your browser. That's it — Vessel shows the
containers already on your machine immediately.

## Usage guide

### Everyday commands

```bash
# Start the server on the default address and port
vessel serve

# Persist users, webhooks, and delivery history to disk
vessel serve --db vessel.db --auth

# Add an admin user (requires --db)
vessel user add admin --role admin --db vessel.db

# Check that Vessel can actually reach the Docker engine
vessel doctor
```

### Security by default

Vessel proxies the Docker socket, which is root-equivalent on the host:
**anything that can reach Vessel's API can control the machine.** Every
default reflects that.

`vessel serve` refuses to bind to any address other than `127.0.0.1` unless
authentication is turned on:

```text
refusing to bind 0.0.0.0 without --auth
Vessel proxies the Docker socket, which is equivalent to root on this host.
Either bind loopback and use an SSH tunnel:
  ssh -L 7373:127.0.0.1:7373 user@host
or create a user and enable auth:
  vessel user add admin --role admin --db vessel.db
  vessel serve --db vessel.db --auth --addr 0.0.0.0
Override at your own risk with --i-know-what-im-doing.
```

This is a deliberate safeguard, not a bug to work around. If you're exposing
Vessel beyond `127.0.0.1`, put a user and `--auth` in front of it, or tunnel
over SSH instead.

## Testing and quality assurance

Run the full check suite the same way CI does:

```bash
make build   # builds the web UI, then the Go binary
make test    # go test ./... && vitest run
make lint    # golangci-lint, tsc --noEmit, eslint
```

To run a single Go test:

```bash
go test ./internal/dockerapi/... -run TestName -v
```

## Deployment

Vessel ships as a single static binary — `CGO_ENABLED=0`, no runtime
dependencies beyond the Docker socket it talks to. Three ways to deploy it:

- **Binary release** — download from [Releases](https://github.com/0funct0ry/vessel/releases)
  and run it directly on the host, ideally behind an SSH tunnel or with
  `--auth` turned on.
- **Container image** — `ghcr.io/0funct0ry/vessel`, built from a
  distroless base image, with the Docker socket mounted in. See the
  [`docker run` example](#or-run-it-with-docker) above.
- **Build your own** — `make build` produces `bin/vessel`; the release
  pipeline (GoReleaser) cross-compiles for Linux and macOS on amd64/arm64,
  with Windows on amd64 as a best effort.

## Contributing

Contributions are welcome. Before opening a pull request:

```bash
make build && make test && make lint
```

Commit messages reference the milestone they belong to, in the form
`[M<n>] Title` — check recent commit history for the convention.

## License

MIT — see [LICENSE](LICENSE).
