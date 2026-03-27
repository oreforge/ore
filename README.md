# ore

Infrastructure-as-code for game server networks. Define your servers in a YAML file, and ore builds Docker images,
manages containers, networks, and volumes.

## Install

```sh
go install github.com/oreforge/ore/cmd/ore@latest
go install github.com/oreforge/ore/cmd/ored@latest
```

Or download binaries from [Releases](https://github.com/oreforge/ore/releases).

## Update

```sh
go install github.com/oreforge/ore/cmd/ore@latest
go install github.com/oreforge/ore/cmd/ored@latest
```

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
    jvmFlags: [ -Xms512M, -Xmx1G ]

  - name: survival
    dir: ./servers/survival
    software: paper:1.21.11
    memory: 2G
    jvmFlags: [ -Xms1G, -Xmx2G ]
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
| `ore up`                  | Build images and start the network                          |
| `ore down`                | Stop all containers and remove the network                  |
| `ore build`               | Build Docker images without starting                        |
| `ore status`              | Show server status (add `--json` for JSON)                  |
| `ore console <server>`    | Attach to a server console                                  |
| `ore prune <target>`      | Remove resources (`all`, `containers`, `images`, `volumes`) |
| `ore clean`               | Remove `.ore/` cache (`--cache`, `--builds`)                |
| `ore projects list`       | List available remote projects                              |
| `ore projects use <name>` | Set the active remote project                               |
| `ore projects active`     | Show the active remote project                              |

### Flags

| Flag            | Description                                 |
|-----------------|---------------------------------------------|
| `-f, --file`    | Path to spec file (default: `ore.yaml`)     |
| `-v, --verbose` | Enable debug logging                        |
| `--no-cache`    | Skip binary cache (on `up` and `build`)     |
| `-r, --replica` | Replica number for `console` (default: `1`) |

## Spec Reference

```yaml
network: string           # Docker network name

servers:
  - name: string          # container name
    dir: string           # path to server data directory
    software: string      # name:version (e.g. paper:1.21.11, velocity:3.4.0, gate:0.62.4)
    port: int             # host port binding (optional)
    replicas: int         # number of replicas (default: 1)
    memory: string        # memory limit, e.g. 512M, 2G (optional)
    cpu: string           # CPU limit, e.g. 1.5 (optional)
    jvmFlags: [ string ]  # JVM flags for Java servers (optional)
    env:                  # environment variables (optional)
      KEY: value
    volumes:              # named volumes (optional)
      - name: string
        target: string    # mount path inside container
```

Environment variables in values are expanded using `${VAR}` syntax.

### Supported Software

| Name       | Runtime                      | Source          |
|------------|------------------------------|-----------------|
| `paper`    | Java (auto-detected version) | PaperMC API     |
| `velocity` | Java 21                      | PaperMC API     |
| `gate`     | Native binary                | GitHub Releases |

## Remote Mode (ored)

`ored` is a daemon that runs on a remote server and executes ore commands via gRPC. The CLI auto-detects the mode: if
`ore.yaml` exists locally, it runs locally. Otherwise, it connects to the configured remote daemon.

### Setup

**On the server:**

```sh
# Create a projects directory
mkdir -p /opt/ore/projects

# Place project directories inside (each with an ore.yaml)
# /opt/ore/projects/mynetwork/ore.yaml
# /opt/ore/projects/staging/ore.yaml

# Run the daemon
ored
```

**On the client:**

```sh
# Configure remote connection
# ~/.config/ore/config.yaml
ore projects use mynetwork   # set active project
ore up                       # runs on the remote server
ore status
ore console lobby
```

### CLI Configuration

File: `~/.config/ore/config.yaml`

```yaml
remote:
  addr: "myserver.example.com:9090"
  project: "mynetwork"
  auth:
    token: "my-secret-token"
```

| Key                 | Env                     | Default | Description                  |
|---------------------|-------------------------|---------|------------------------------|
| `remote.addr`       | `ORE_REMOTE_ADDR`       |         | Daemon address (`host:port`) |
| `remote.project`    | `ORE_REMOTE_PROJECT`    |         | Active project name          |
| `remote.auth.token` | `ORE_REMOTE_AUTH_TOKEN` |         | Bearer token                 |

### Daemon Configuration

File: `~/.config/ored/config.yaml`

```yaml
addr: ":9090"
log_level: "info"
projects_dir: "/opt/ore/projects"
auth:
  token: "my-secret-token"
```

| Key            | Env                 | Default | Description                                  |
|----------------|---------------------|---------|----------------------------------------------|
| `addr`         | `ORED_ADDR`         | `:9090` | Listen address                               |
| `log_level`    | `ORED_LOG_LEVEL`    | `info`  | Log level (`debug`, `info`, `warn`, `error`) |
| `projects_dir` | `ORED_PROJECTS_DIR` | `.`     | Directory containing project subdirectories  |
| `auth.token`   | `ORED_AUTH_TOKEN`   |         | Required token for client auth               |

### Authentication

When `auth.token` is set on the daemon, all clients must provide the same token. The token is sent as a `Bearer` token
in gRPC metadata. Token comparison uses constant-time comparison to prevent timing attacks.

### Multi-Project

The daemon serves multiple projects from its `projects_dir`. Each subdirectory containing an `ore.yaml` is a project:

```
/opt/ore/projects/
  mynetwork/
    ore.yaml
    servers/
  staging/
    ore.yaml
    servers/
```

The active project is stored client-side in `~/.config/ore/config.yaml`. Switch with `ore projects use <name>`.

## Development

```sh
bab build       # build ore and ored binaries
bab check       # fmt, proto lint, go lint, vet, test
bab proto       # lint and generate protobuf code
bab dev:ored    # run daemon locally
bab dev:up      # start via local daemon
bab dev:status  # status via local daemon
```