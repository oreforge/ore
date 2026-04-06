# ore

Infrastructure-as-code for game server networks. Define servers in YAML, and ore builds, deploys, and manages them.

## Install

```sh
# ore cli for local desktop (enough to get started and try ore)
GONOSUMCHECK=github.com/oreforge/ore GOFLAGS=-mod=mod GONOSUMDB=github.com/oreforge/ore GOPROXY=direct go install github.com/oreforge/ore/cmd/ore@latest
# ored for the server (optional, only needed for real production use)
GONOSUMCHECK=github.com/oreforge/ore GOFLAGS=-mod=mod GONOSUMDB=github.com/oreforge/ore GOPROXY=direct go install github.com/oreforge/ore/cmd/ored@latest
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
    ports:
      - "25565:25565"
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

services:
  - name: database
    image: postgres:16
    ports:
      - "5432:5432"
    env:
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: mynetwork
    volumes:
      - name: data
        target: /var/lib/postgresql/data
```

```sh
ore up             # build and start all servers
ore status         # show server status
ore console lobby  # attach to server console (ctrl+c to detach)
ore down           # stop and remove all servers
```

## Commands

```
ore up [--no-cache] [--force]  Build and start all servers
ore down                      Stop and remove all servers
ore build [--no-cache]        Build all server images
ore status                    Show server status
ore console <server>          Attach to a server console
ore clean all                 Remove all servers, images, data, and build cache
ore clean cache               Remove cached binaries
ore clean builds              Remove build artifacts
ore clean servers             Stop and remove all running servers
ore clean images              Remove unused server images
ore clean data                Remove server data volumes
ore nodes list                List configured nodes
ore nodes add <name>          Add a remote node (--addr, --token required, --project optional)
ore nodes remove <name>       Remove a remote node
ore nodes use <name>          Set the active node
ore nodes active              Show the active node
ore nodes show <name>         Show node details
ore projects list             List remote projects
ore projects add <url>        Clone a project from a git repository
ore projects remove <name>    Remove a project
ore projects update <name>    Pull latest changes and redeploy
ore projects use <name>       Set the active project
ore projects active           Show the active project
ore version                   Print version info
```

Global flags: `-f <path>` spec file (default `ore.yaml`), `-v` verbose logging.

## Spec Reference

```yaml
network: string             # network name

gitops:                     # GitOps configuration (optional)
  poll:
    enabled: bool           # enable periodic git polling (default: false)
    interval: duration      # poll interval, e.g. 5m, 1h (default: 5m)

servers:
  - name: string            # server name
    dir: string             # path to server data directory
    software: string        # name:version (e.g. paper:1.21.11, gate:0.62.4)
    ports:                  # host:server port mappings (optional)
      - "host:server"
    memory: string          # memory limit, e.g. 512M, 2G (optional)
    cpu: string             # CPU limit, e.g. 1.5 (optional)
    jvmFlags: [ string ]    # JVM flags for Java servers (optional)
    env:                    # environment variables (optional)
      KEY: value
    volumes:                # named volumes (optional)
      - name: string
        target: string      # mount path inside the server

services:
  - name: string            # service name
    image: string           # image (e.g. postgres:16, redis:7)
    ports:                  # host:service port mappings (optional)
      - "host:service"
    env:                    # environment variables (optional)
      KEY: value
    volumes:                # named volumes (optional)
      - name: string
        target: string      # mount path inside the service
```

All servers and services join the same network and can reach each other by name. Services start before servers and stop after them.

Ports are only needed for external access from the host. Internal communication works on any port using the server or service name (e.g. `database:5432`).

Values support `${VAR}` environment variable expansion.

### Supported Software

| Name       | Runtime | Source          |
|------------|---------|-----------------|
| `paper`    | Java    | PaperMC API     |
| `velocity` | Java 21 | PaperMC API     |
| `gate`     | Native  | GitHub Releases |

## Remote Mode

`ored` is a daemon that exposes the ore engine as a REST API. The CLI auto-detects the mode: if `ore.yaml` exists
locally it runs locally, otherwise it connects to the configured remote node.

All commands work identically in both modes. The console uses WebSocket for real-time bidirectional I/O.

### Daemon Setup

```sh
ored
```

On first run, ored creates its config with defaults and an auto-generated auth token.

Projects are managed via `ore projects` commands and stored as subdirectories in the projects directory:

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

Add the remote node and set it as active:

```sh
ore nodes add prod --addr 123.321.187.123:9090 --token <token from ored config>
ore nodes use prod
```

Then manage projects and run commands against the remote node:

```sh
ore projects list
ore projects use mynetwork
ore up
ore status
ore console lobby
```

Multiple nodes can be configured and switched between. Each node remembers its active project independently.

## GitOps

ore supports automatic deployments via polling, configured per project in `ore.yaml`.

Enable polling to have ored periodically check for new commits:

```yaml
network: mynetwork
gitops:
  poll:
    enabled: true
    interval: 5m
servers:
  - ...
```

On startup, ored scans all projects and starts a polling goroutine for each project with polling enabled. When new commits are detected, it pulls and redeploys automatically.

## Configuration

Configs follow the [XDG Base Directory](https://specifications.freedesktop.org/basedir-spec/latest/) spec. Auto-created
on first run.

| Path          | Linux                          | macOS                                            | Windows                        |
|---------------|--------------------------------|--------------------------------------------------|--------------------------------|
| ore config    | `~/.config/ore/config.yaml`    | `~/Library/Application Support/ore/config.yaml`  | `%AppData%\ore\config.yaml`    |
| ored config   | `~/.config/ored/config.yaml`   | `~/Library/Application Support/ored/config.yaml` | `%AppData%\ored\config.yaml`   |
| ored projects | `~/.local/share/ored/projects` | `~/Library/Application Support/ored/projects`    | `%LocalAppData%\ored\projects` |

### ore

```yaml
log_level: info
verbose: false
context: prod

nodes:
  prod:
    addr: myserver.example.com:9090
    token: <token>
    project: mynetwork
```

| Key                    | Env             | Default | Description            |
|------------------------|-----------------|---------|------------------------|
| `log_level`            | `ORE_LOG_LEVEL` | `info`  | Log level              |
| `verbose`              | `ORE_VERBOSE`   | `false` | Debug logging          |
| `context`              | `ORE_CONTEXT`   |         | Active node name       |
| `nodes.<name>.addr`    |                 |         | Node address           |
| `nodes.<name>.token`   |                 |         | Auth token             |
| `nodes.<name>.project` |                 |         | Active project on node |

### ored

```yaml
addr: ":9090"
log_level: info
token: <auto-generated>
projects: ~/.local/share/ored/projects
bind_mounts: false
```

| Key                  | Env                       | Default                        | Description                 |
|----------------------|---------------------------|--------------------------------|-----------------------------|
| `addr`               | `ORED_ADDR`               | `:9090`                        | Listen address              |
| `log_level`          | `ORED_LOG_LEVEL`          | `info`                         | Log level                   |
| `token`              | `ORED_TOKEN`              | *(auto-generated)*             | Bearer token for API auth   |
| `projects`           | `ORED_PROJECTS`           | `$XDG_DATA_HOME/ored/projects` | Projects directory          |
| `bind_mounts`        | `ORED_BIND_MOUNTS`        | `false`                        | Bind-mount server data dirs |
