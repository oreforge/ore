# ore

Infrastructure-as-code for game server networks. Define servers in YAML, and ore builds Docker images, manages containers, networks, and volumes.

## Install

```sh
# ore cli for local desktop (enough to get started and try ore)
go install github.com/oreforge/ore/cmd/ore@latest
# ored for the server (optional, only needed for real production use)
go clean -modcache && go install github.com/oreforge/ore/cmd/ored@latest
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

```
ore up [--no-cache]           Build images and start the network
ore down                      Stop all containers and remove the network
ore build [--no-cache]        Build Docker images without starting
ore status [--json]           Show server status
ore console <server>          Attach to a server console
ore prune <target>            Remove resources (all, containers, images, volumes)
ore clean [--cache|--builds]  Remove .ore/ cache and build artifacts
ore servers list              List configured servers
ore servers add <name>        Add a remote server (--addr, --token required)
ore servers remove <name>     Remove a remote server
ore servers use <name>        Set the active server
ore servers active            Show the active server
ore servers show <name>       Show server details
ore projects list             List remote projects
ore projects add <url>        Clone a project from a git repository
ore projects remove <name>    Remove a project
ore projects update <name>    Pull latest changes
ore projects use <name>       Set the active project
ore projects active           Show the active project
ore version                   Print version info
```

Global flags: `-f <path>` spec file (default `ore.yaml`), `-v` verbose logging.

## Spec Reference

```yaml
network: string             # Docker network name

servers:
  - name: string            # container name
    dir: string             # path to server data directory
    software: string        # name:version (e.g. paper:1.21.11, gate:0.62.4)
    port: int               # host port binding (optional)
    memory: string          # memory limit, e.g. 512M, 2G (optional)
    cpu: string             # CPU limit, e.g. 1.5 (optional)
    jvmFlags: [ string ]    # JVM flags for Java servers (optional)
    env:                    # environment variables (optional)
      KEY: value
    volumes:                # named volumes (optional)
      - name: string
        target: string      # mount path inside container
```

Values support `${VAR}` environment variable expansion.

### Supported Software

| Name       | Runtime | Source          |
|------------|---------|-----------------|
| `paper`    | Java    | PaperMC API     |
| `velocity` | Java 21 | PaperMC API     |
| `gate`     | Native  | GitHub Releases |

## Remote Mode

`ored` is a daemon that exposes the ore engine as a REST API. The CLI auto-detects the mode: if `ore.yaml` exists
locally it runs locally, otherwise it connects to the configured remote server.

All commands work identically in both modes. The console uses WebSocket for real-time bidirectional I/O.

### Server Setup

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

Add the remote server and set it as active:

```sh
ore servers add prod --addr 123.321.187.123:9090 --token <token from ored config>
ore servers use prod
```

Then manage projects and run commands against the remote server:

```sh
ore projects list
ore projects use mynetwork
ore up
ore status
ore console lobby
```

Multiple servers can be configured and switched between. Each server remembers its active project independently.

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

servers:
  prod:
    addr: myserver.example.com:9090
    token: <token>
    project: mynetwork
```

| Key                      | Env             | Default | Description              |
|--------------------------|-----------------|---------|--------------------------|
| `log_level`              | `ORE_LOG_LEVEL` | `info`  | Log level                |
| `verbose`                | `ORE_VERBOSE`   | `false` | Debug logging            |
| `context`                | `ORE_CONTEXT`   |         | Active server name       |
| `servers.<name>.addr`    |                 |         | Server address           |
| `servers.<name>.token`   |                 |         | Auth token               |
| `servers.<name>.project` |                 |         | Active project on server |

### ored

```yaml
addr: ":9090"
log_level: info
token: <auto-generated>
projects: ~/.local/share/ored/projects
bind_mounts: false

git_poll:
  enabled: false
  interval: 5m
  on_update: deploy
```

| Key                  | Env                       | Default                        | Description                 |
|----------------------|---------------------------|--------------------------------|-----------------------------|
| `addr`               | `ORED_ADDR`               | `:9090`                        | Listen address              |
| `log_level`          | `ORED_LOG_LEVEL`          | `info`                         | Log level                   |
| `token`              | `ORED_TOKEN`              | *(auto-generated)*             | Bearer token for API auth   |
| `projects`           | `ORED_PROJECTS`           | `$XDG_DATA_HOME/ored/projects` | Projects directory          |
| `bind_mounts`        | `ORED_BIND_MOUNTS`        | `false`                        | Bind-mount server data dirs |
| `git_poll.enabled`   | `ORED_GIT_POLL_ENABLED`   | `false`                        | Enable git polling          |
| `git_poll.interval`  | `ORED_GIT_POLL_INTERVAL`  | `5m`                           | Poll interval               |
| `git_poll.on_update` | `ORED_GIT_POLL_ON_UPDATE` | `deploy`                       | Action on update (deploy)   |