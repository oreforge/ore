# ore

Infrastructure-as-code for game server networks. Define servers in YAML, and ore builds, deploys, and manages them.

## Install ore

### Homebrew (macOS / Linux)

```sh
brew install --cask oreforge/tap/ore
```

### Scoop (Windows)

```sh
scoop bucket add oreforge https://github.com/oreforge/scoop-bucket
scoop install ore
```

### Debian / Ubuntu

```sh
curl -LO "https://github.com/oreforge/ore/releases/latest/download/ore_$(curl -s https://api.github.com/repos/oreforge/ore/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_linux_amd64.deb"
sudo dpkg -i ore_*.deb
```

### RHEL / Fedora

```sh
curl -LO "https://github.com/oreforge/ore/releases/latest/download/ore_$(curl -s https://api.github.com/repos/oreforge/ore/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_linux_amd64.rpm"
sudo rpm -i ore_*.rpm
```

### Alpine

```sh
curl -LO "https://github.com/oreforge/ore/releases/latest/download/ore_$(curl -s https://api.github.com/repos/oreforge/ore/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_linux_amd64.apk"
sudo apk add --allow-untrusted ore_*.apk
```

### Arch Linux

```sh
curl -LO "https://github.com/oreforge/ore/releases/latest/download/ore_$(curl -s https://api.github.com/repos/oreforge/ore/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_linux_amd64.pkg.tar.zst"
sudo pacman -U ore_*.pkg.tar.zst
```

### Go

```sh
go install github.com/oreforge/ore/cmd/ore@latest
```

### Binary

Download pre-built binaries from [Releases](https://github.com/oreforge/ore/releases).

## Install ored

ored is the server daemon for production deployments. It is available as a binary or Docker image.

### Binary

Download from [Releases](https://github.com/oreforge/ore/releases).

### Docker

```sh
docker run -d \
  --name ored \
  -p 9090:9090 \
  -v ored-data:/var/lib/ored \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/oreforge/ore/ored:latest
```

The volume persists config (including the auth token), projects, and build artifacts across restarts.

Retrieve the auto-generated auth token:

```sh
docker exec ored grep token /var/lib/ored/config/ored/config.yaml | awk '{print $2}'
```

## Production Deploy with Docker Compose

Deploy ored and the dashboard together using Docker Compose. Create a `docker-compose.yml`:

```yaml
services:
  ored:
    image: ghcr.io/oreforge/ore/ored:latest
    pull_policy: always
    restart: unless-stopped
    volumes:
      - ored-data:/var/lib/ored
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - ORED_ADDR=:9090
      - ORED_TOKEN=your-secret-token

  dashboard:
    image: ghcr.io/oreforge/ore-dashboard:latest
    pull_policy: always
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - ORE_API_URL=http://ored:9090
      - ORE_TOKEN=your-secret-token
    depends_on:
      - ored

volumes:
  ored-data:
```

Set the same token for both `ORED_TOKEN` and `ORE_TOKEN`, then start the stack:

```sh
docker compose up -d
```

The dashboard is available at `http://localhost:3000`.

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

  - name: survival
    dir: ./servers/survival
    software: paper:1.21.11
    memory: 2G
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
ore status         # show server and service status
ore console lobby  # attach to server console (ctrl+c to detach)
ore down           # stop and remove all servers
```

## Commands

```
ore up [--no-cache] [--force]       Build and start all servers
ore down                            Stop and remove all containers
ore status                          Show server and service status
ore build [--no-cache]              Build all server images
ore console <server>                Attach to a running server console
ore clean all                       Remove all containers, images, volumes, and build artifacts
ore clean containers                Stop and remove all containers and the network
ore clean images                    Remove Docker images built by ore
ore clean volumes                   Remove Docker volumes (persistent server data)
ore clean cache                     Remove cached software binaries
ore clean builds                    Remove build artifacts
ore volumes list                            List ore-managed Docker volumes
ore volumes show <name>                     Show volume metadata and current usage
ore volumes size <name>                     Measure the on-disk size of a volume
ore volumes remove <name> [--force]         Remove a volume (--force stops containers using it)
ore volumes prune [--dry-run]               Remove volumes no longer declared in ore.yaml
ore backups create <volume> [--tag t]         Snapshot a volume (tar+zstd, sha256-verified)
ore backups list [--volume v]                 List backups
ore backups show <id>                         Show backup metadata
ore backups restore <id> [--keep-pre-restore] Restore a backup into its source volume
ore backups verify <id>                       Recompute the backup's sha256 and compare
ore backups remove <id>                       Remove a backup archive and sidecar
ore nodes list                      List configured nodes
ore nodes add <name>                Add a remote node (--addr, --token required; --project, --force optional)
ore nodes show <name>               Show node details
ore nodes use <name>                Set the active node
ore nodes remove <name>             Remove a configured node
ore nodes active                    Show the active node
ore projects list                   List remote projects
ore projects add <url>              Clone a project from a git repository (--name optional)
ore projects use <name>             Set the active project
ore projects update <name>          Pull latest changes and redeploy
ore projects remove <name>          Stop servers and remove a project
ore projects active                 Show the active project
ore projects webhook <name>         Show the webhook URL for a project (--force, --no-cache optional)
ore version                         Print the ore version
```

Global flags: `-f <path>` spec file (default `ore.yaml`), `-v` verbose logging.

## Spec Reference

```yaml
network: string             # network name

gitops:                     # GitOps configuration (optional)
  poll:
    enabled: bool           # enable periodic git polling (default: false)
    interval: duration      # poll interval, e.g. 5m, 1h (default: 5m)
  webhook:
    enabled: bool           # enable webhook endpoint (default: false)
    force: bool             # default force restart on webhook trigger (default: false)
    noCache: bool           # default no-cache on webhook trigger (default: false)

servers:
  - name: string            # server name
    dir: string             # path to server data directory
    software: string        # name:version (e.g. paper:1.21.11, gate:0.62.4)
    ports:                  # host:server port mappings (optional)
      - "host:server"
    memory: string          # memory limit, e.g. 512M, 2G (optional)
    cpu: string             # CPU limit, e.g. 1.5 (optional)
    env:                    # environment variables (optional)
      KEY: value
    volumes:                # named volumes (optional)
      - name: string
        target: string      # mount path inside the server
    healthcheck:            # Docker HEALTHCHECK (optional, defaults from software provider)
      cmd: string           # check command (e.g. "nc -z localhost 25565")
      interval: duration    # time between checks (default: 2s)
      timeout: duration     # check timeout (default: 2s)
      startPeriod: duration # grace period on startup (default: 5s)
      retries: int          # consecutive failures before unhealthy (default from provider)

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
    healthcheck:            # Docker HEALTHCHECK (optional, no default for services)
      cmd: string           # check command (e.g. "pg_isready -U postgres")
      interval: duration    # time between checks (default: 10s)
      timeout: duration     # check timeout (default: 5s)
      startPeriod: duration # grace period on startup (default: 30s)
      retries: int          # consecutive failures before unhealthy (default: 3)
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

## Volumes

ore tags every Docker volume it creates with a set of labels (`ore.managed`, `ore.project`, `ore.owner`, `ore.owner.kind`, `ore.volume`, `ore.created.at`). These labels make volumes discoverable and scopable per project without any external state. Volumes follow the deterministic name `{network}_{container}_{logical}`, so they survive container recreation and `ore up` reattaches the same data.

```sh
ore volumes list                   # list volumes in the current project
ore volumes show <name>            # show metadata and which containers use it
ore volumes size <name>            # measure on-disk usage via a helper container
ore volumes remove <name>          # remove a volume (add --force if it is in use)
ore volumes prune                  # remove volumes no longer declared in ore.yaml
ore volumes prune --dry-run        # preview without removing
```

`size` runs `du -sb` on demand in a short-lived `alpine:3` helper with the volume mounted read-only; the number is always fresh, never cached. `remove --force` stops any container currently mounting the volume before removing it. `prune` compares the labeled volumes against what is declared in `ore.yaml` and removes the delta; volumes still in use are skipped and reported. Use `--dry-run` to preview the delta without removing.

Volumes created before these labels existed will not appear in these views until they are recreated by `ore up`.

## Backups

Backups snapshot a Docker volume into a local, compressed, checksummed archive. They are stored under `$XDG_DATA_HOME/ored/backups` (overridable via the `backups` key in the ored config, or `ORED_BACKUPS`) as:

```
{backups}/{network}/{volume}/{ulid}.tar.zst   # archive
{backups}/{network}/{volume}/{ulid}.json      # sidecar with metadata
```

The JSON sidecar is the source of truth — it records the backup's status, timestamps, raw and compressed sizes, sha256 checksum, tags, and storage refs. ored rebuilds its in-memory index by scanning the backups directory at startup, so there is no separate database to manage.

```sh
ore backups create <volume>                   # snapshot a volume
ore backups create <volume> --tag weekly      # attach tags
ore backups list                              # list all backups for the current project
ore backups list --volume <name>              # filter by source volume
ore backups show <id>                         # full metadata including checksum and storage refs
ore backups verify <id>                       # recompute and compare the sha256
ore backups restore <id>                      # extract the archive back into the source volume
ore backups restore <id> --keep-pre-restore   # take a safety snapshot first
ore backups remove <id>                       # remove the archive and sidecar
```

Snapshots run through a short-lived `alpine:3` helper that `tar`s the volume's contents, piped directly through zstd compression and a sha256 hasher into an atomically-committed archive file — the volume's bytes are never buffered on the host outside the final archive. Restores stream the archive back through zstd, wipe the target volume (`find -mindepth 1 -delete`), and `tar -x` into it; `--keep-pre-restore` takes a safety snapshot first so a bad restore can be rolled back. `verify` re-hashes the archive and stamps the sidecar with a `verified` timestamp on match.

Create, restore, and verify are submitted as asynchronous operations — the CLI streams their logs until completion. Backups accumulate indefinitely; there is no automatic retention policy yet.

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

ore supports automatic deployments via polling and webhooks, configured per project in `ore.yaml`.

### Polling

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

### Webhooks

Enable webhooks to trigger deployments via HTTP POST from CI/CD systems (GitHub Actions, GitLab CI, etc.):

```yaml
network: mynetwork
gitops:
  webhook:
    enabled: true
    force: false
    noCache: false
servers:
  - ...
```

Get the webhook URL for a project:

```sh
ore projects webhook mynetwork
```

This prints the full URL including the HMAC-derived secret. The secret is computed as `HMAC-SHA256(ored_token, project_name)` so it never needs to be stored separately.

The webhook endpoint accepts optional query parameters `force=true` and `no_cache=true` to override the spec defaults. Use the `--force` and `--no-cache` flags on the webhook command to include them in the URL.

Example GitHub Actions step:

```yaml
- name: Deploy
  run: curl -X POST "${{ secrets.ORE_WEBHOOK_URL }}"
```

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
| `backups`            | `ORED_BACKUPS`            | `$XDG_DATA_HOME/ored/backups`  | Backups directory           |
| `bind_mounts`        | `ORED_BIND_MOUNTS`        | `false`                        | Bind-mount server data dirs |
