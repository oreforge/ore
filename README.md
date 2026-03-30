# ore

Infrastructure-as-code for game server networks. Define servers in YAML, and ore builds Docker images, manages containers, networks, and volumes.

## Install

```sh
go install github.com/oreforge/ore/cmd/ore@latest
go install github.com/oreforge/ore/cmd/ored@latest
```

Or download binaries from [Releases](https://github.com/oreforge/ore/releases).

## Quick Start

Create an `ore.yaml`:

```yaml
network: mynetwork

servers:
  - name: proxy
    dir: ./servers/proxy
    software: gate:0.62.4
    port: 25565
    memory: 512M

  - name: lobby
    dir: ./servers/lobby
    software: paper:1.21.11
    memory: 1G
    jvmFlags: [-Xms512M, -Xmx1G]

  - name: survival
    dir: ./servers/survival
    software: paper:1.21.11
    memory: 2G
    jvmFlags: [-Xms1G, -Xmx2G]
    volumes:
      - name: world
        target: /data/world
```

```sh
ore up             # build images and start all servers
ore status         # show container states
ore console lobby  # attach to server console (ctrl+c to detach)
ore down           # stop everything
```

## Commands

| Command                   | Description                                                 |
|---------------------------|-------------------------------------------------------------|
| `ore up [--no-cache]`     | Build images and start the network                          |
| `ore down`                | Stop all containers and remove the network                  |
| `ore build [--no-cache]`  | Build Docker images without starting                        |
| `ore status [--json]`     | Show server status                                          |
| `ore console <server>`    | Attach to a server console (`-r` for replica number)        |
| `ore prune <target>`      | Remove resources (`all`, `containers`, `images`, `volumes`) |
| `ore clean`               | Remove `.ore/` cache (`--cache`, `--builds`)                |
| `ore projects list`       | List available remote projects                              |
| `ore projects use <name>` | Set the active remote project                               |
| `ore projects active`     | Show the active remote project                              |
| `ore version`             | Print version info                                          |

Global flags: `-f <path>` spec file (default `ore.yaml`), `-v` verbose logging.

## Spec Reference

```yaml
network: string           # Docker network name

servers:
  - name: string          # container name
    dir: string           # path to server data directory
    software: string      # name:version (e.g. paper:1.21.11, gate:0.62.4)
    port: int             # host port binding (optional)
    replicas: int         # number of replicas (default: 1)
    memory: string        # memory limit, e.g. 512M, 2G (optional)
    cpu: string           # CPU limit, e.g. 1.5 (optional)
    jvmFlags: [string]    # JVM flags for Java servers (optional)
    env:                  # environment variables (optional)
      KEY: value
    volumes:              # named volumes (optional)
      - name: string
        target: string    # mount path inside container
```

Values support `${VAR}` environment variable expansion.

### Supported Software

| Name       | Runtime | Source          |
|------------|---------|-----------------|
| `paper`    | Java    | PaperMC API     |
| `velocity` | Java 21 | PaperMC API     |
| `gate`     | Native  | GitHub Releases |

## Remote Mode

`ored` is a daemon that exposes the ore engine as a REST API. The CLI auto-detects the mode: if `ore.yaml` exists locally it runs locally, otherwise it connects to the configured remote daemon.

All commands work identically in both modes. The console uses WebSocket for real-time bidirectional I/O.

### Server Setup

```sh
# Run the daemon
ored
```

On first run, ored creates a config file with all defaults and an auto-generated auth token. Projects are stored as subdirectories (each containing an `ore.yaml`) in the projects directory.

```
~/.local/share/ored/projects/
  mynetwork/
    ore.yaml
    servers/
  staging/
    ore.yaml
    servers/
```

### Client Setup

Configure the CLI to connect to the remote daemon. The ore config is auto-created on first run.

```yaml
# ore config file
remote:
  addr: "myserver.example.com:9090"
  auth:
    token: "<token from ored config>"
```

```sh
ore projects list            # list available projects
ore projects use mynetwork   # set active project
ore up                       # runs on the remote server
ore status
ore console lobby
```

### Configuration

Configs follow the [XDG Base Directory](https://specifications.freedesktop.org/basedir-spec/latest/) spec via the `adrg/xdg` library.

**ore** (CLI):

| Key                 | Env                     | Default | Description          |
|---------------------|-------------------------|---------|----------------------|
| `log_level`         | `ORE_LOG_LEVEL`         | `info`  | Log level            |
| `remote.addr`       | `ORE_REMOTE_ADDR`       |         | Daemon address       |
| `remote.project`    | `ORE_REMOTE_PROJECT`    |         | Active project name  |
| `remote.auth.token` | `ORE_REMOTE_AUTH_TOKEN` |         | Bearer token         |

**ored** (daemon):

| Key           | Env                | Default                     | Description                |
|---------------|--------------------|-----------------------------|----------------------------|
| `addr`        | `ORED_ADDR`        | `:9090`                     | Listen address             |
| `log_level`   | `ORED_LOG_LEVEL`   | `info`                      | Log level                  |
| `projects`    | `ORED_PROJECTS`    | `$XDG_DATA_HOME/ored/projects` | Projects directory      |
| `bind_mounts` | `ORED_BIND_MOUNTS` | `false`                     | Bind-mount data dirs       |
| `auth.token`  | `ORED_AUTH_TOKEN`  | *(auto-generated)*          | Bearer token for API auth  |

### Authentication

The ored daemon auto-generates a bearer token on first run and saves it to the config file. All API requests must include this token. The token is sent as `Authorization: Bearer <token>` and validated using constant-time comparison.

### API Documentation

When ored is running, interactive API docs are available at:

```
http://localhost:9090/api/docs
```

The OpenAPI 3.1 spec is auto-generated from the Go types and served at `/api/openapi.yaml`.

### Graceful Shutdown

When ored receives SIGINT or SIGTERM, it stops accepting new requests, waits for in-flight operations, then stops all running containers across all projects.

## Development

```sh
bab build        # build ore and ored binaries
bab test         # run tests
bab lint         # run golangci-lint
bab check        # fmt, lint, vet, test
bab dev:ored     # run daemon locally
bab dev:up       # start via local daemon
bab dev:status   # status via local daemon
```
