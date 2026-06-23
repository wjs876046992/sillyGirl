# sillyGirl Project Summary

> Auto-generated for agent readability. Last updated: 2026-06-23

## 1. Repository Overview

- **Module**: `github.com/cdle/sillyplus` (Go)
- **Version**: Current branch `v2.1`, latest stable tag `v2.1.5`
- **Go version**: `1.18` (declared in go.mod), CI uses `1.20.2`
- **Purpose**: Cross-platform chatbot framework focused on message relay. Supports Telegram/QQ/Web/etc. adapters, dual JS engine (goja + Node.js), plugin system with comment-driven annotations, built-in cron, bucket storage, and REST API.

## 2. Top-Level Layout

```
sillyGirl/
  main.go                 # Entry point
  go.mod / go.sum         # Go dependencies (Go 1.18, Gin, gRPC, MongoDB, goja, boltdb, Clash proxy)
  Dockerfile              # Multi-arch Docker image (linux/amd64, linux/arm64)
  .dockerfile             # Alternative Dockerfile (unconfirmed difference)
  Makefile                # Local build shortcuts
  install.sh              # One-click install script (curl from cdle/binary repo)
  release-dryrun.sh       # Pre-release dry-run: version bump, changelog generation, plan display
  release.js              # Release helper (unconfirmed content)
  workflow.yaml           # Legacy CI (outdated, replaced by .github/workflows/build.yml)
  README.md               # Comprehensive docs + API reference

  core/                   # Core framework (~30 Go files)
    plugin_parse.go       # Parses @rule/@http/@on_start/etc. annotations from plugin scripts
    transport.go          # Message routing pipeline
    grpc_sender.go        # gRPC-based message sending
    grpc_bucket.go        # gRPC bucket (distributed storage)
    web.go                # Gin-based HTTP server (port 8080, admin dashboard)
    bucket.go             # BoltDB-backed key-value store
    node_bucket.go        # Node.js plugin bucket integration
    cron.go               # Cron scheduler wrapper
    adapter.go            # Adapter interface definition
    plugin_impl.go        # Plugin implementation loader
    plugin_module.go      # Module system for plugins
    plugin_message.go     # Message abstraction for plugins
    plugin_utils.go       # Plugin utility helpers
    platform.go           # Platform type definitions
    masters.go            # Admin management
    carry.go              # Message relay/carry logic
    task.go               # Task scheduling
    queue.go              # Message queue
    bucket.go             # Persistent storage (BoltDB)
    encrypt.go            # Plugin encryption/decryption
    node_xml.go           # XML parser for CQ codes
    node_temp.go          # Temporary data management
    node_debug.go         # Debug node utilities
    node_request.go       # HTTP request node
    machine_id.go         # Machine fingerprinting
    time.go               # Time utilities
    base_sender.go        # Base sender abstraction
    utils/                # Shared utilities (init, public IP, goroutine monitor)
    common/               # Shared types (Function, Http, Filter, Message, Reply)

  proto3/                 # Protocol Buffers + multi-language stubs
    srpc.proto            # gRPC service definition
    srpc_pb2.py / srpc_pb2_grpc.py   # Python gRPC stubs
    srpc.ts / srpc.d.ts            # TypeScript types
    sillygirl.ts / sillygirl.d.ts   # TypeScript client types
    sillygirl.js          # Webpack-bundled JS (output)
    sillygirl.py          # Python client binding
    sillygirl.d.ts        # TypeScript declarations
    package.json          # Node deps for proto3 build (webpack, etc.)
    webpack.config.js     # Webpack config for sillygirl.js
    tsconfig.json         # TypeScript config
    list.json             # ??? (unconfirmed)
    note.sh               # ??? (unconfirmed)
    vm.js                 # VM-related script
    set_up.py             # Python setup helper
    node_modules/         # (gitignored — not tracked)
    dist/                 # Webpack output (generated)

  adapters/               # Platform adapters
    web/main.go           # Web/HTTP adapter (WebSocket admin interface)
    qq/main.go            # QQ adapter (currently commented out in main.go)
    pagermaid/sillyplus.py # PagerMaid Python adapter

  emoji/                  # Emoji handling
    raw.go                # Raw emoji data
    my.go                 # Custom emoji
    data.go               # Emoji data registry

  mongodb/                # MongoDB integration
    main.go               # Connection pool setup
    pool.go               # Pool management

  utils/                  # Shared utilities
    init.go               # Global initialization helpers
```

## 3. Architecture

### 3.1 Core Message Pipeline

```
Adapters (web, qq, pagermaid, ...)
  -> core.Messages channel
  -> transport.go (message parsing + filtering)
  -> plugin_parse.go rules matching (@rule, @match, @regex, @pattern)
  -> plugin execution (goja JS engine or Node.js subprocess)
  -> adapter.reply() (response back to IM platform)
```

### 3.2 Plugin System

Two modes:

1. **Goja (built-in)**: Scripts parse `@rule` annotations, execute directly in Goja (ES5.1+). Zero dependency, fast.
2. **Node.js (external)**: Scripts with `@service true` launch as subprocesses. Ports auto-assigned (40000-50000). Nginx-style reverse proxy from Gin to Node processes. Full npm ecosystem.

Plugin annotations parsed by `plugin_parse.go`:

| Annotation | Example | Meaning |
|---|---|---|
| `@rule` | `raw ^你好$` | Regex trigger (supports multiple) |
| `@match` / `@regex` / `@pattern` | Same as @rule | Aliases |
| `@on_start` / `@service` | `true` | Long-running background service |
| `@http` | `GET /api/hello` | HTTP route registration |
| `@platform` / `@imType` | `telegram` | Platform whitelist |
| `@platform-` | `qq` | Platform blacklist |
| `@userId` / `@uid` | `12345` | User whitelist |
| `@groupId` / `@gid` | `67890` | Group whitelist |
| `@admin` | `true` | Admin-only plugin |
| `@priority` | `100` | Execution priority |
| `@title` | `MyPlugin` | Plugin display name |
| `@disable` | `true` | Disabled by default |
| `@public` | `true` | Public plugin |
| `@cron` | `0 */5 * * * *` | Cron schedule |
| `@carry` | `true` | Message relay enabled |
| `@form` | `{title:"Name", key:"x.y"}` | Admin form config |
| `@origin` | `custom` | Plugin source |
| `@version` | `v1.0.0` | Semantic version |
| `@author` | `cdle` | Author name |
| `@message` | `telegram *` | Platform/message filter |

### 3.3 Storage

- **BoltDB** (`bucket.go`): Persistent local key-value store. `Bucket("name")` returns a scoped namespace.
- **MongoDB** (`mongodb/`): Connection pool for large-scale deployments.
- **gRPC Bucket** (`grpc_bucket.go`): Distributed storage for multi-instance setups.

### 3.4 Node.js Plugin Lifecycle

1. `plugin_parse.go` detects `@service true` + `@http` routes
2. Core assigns random port from 40000-50000
3. Sets `HTTP_LISTEN_PORT` and `PLUGIN_ID` env vars
4. Spawns Node.js subprocess
5. Waits for port readiness (health check)
6. Registers Gin reverse proxy route(s) to Node process
7. On plugin reload: kills subprocess, re-spawns with new port

## 4. Key Entry Points

### main.go
- Calls `core.Init()` to bootstrap everything
- Reads `anti_kasi` config; if true, starts goroutine monitor (`utils.MonitorGoroutine()`)
- `-t` flag: launches terminal interactive adapter (read stdin, send to `core.Messages`)
- `-d` flag: debug mode (skips terminal launch)
- Auto-opens browser on Windows after 3s
- Listens for SIGTERM/SIGINT for graceful shutdown (critical for Docker PID 1)

### core.Init()
- Initializes logging, BoltDB storage, cron scheduler
- Loads all `.go` and `.js` plugins from `plugins/` directory
- Parses annotations, registers HTTP routes, starts cron jobs
- Starts the Web adapter (Gin server on port 8080)

### proto3/srpc.proto
- Defines gRPC service for cross-language communication
- Used by Python/TypeScript/JS bindings for programmatic interaction with sillyGirl

## 5. Dependencies (Key)

| Package | Purpose |
|---|---|
| `github.com/gin-gonic/gin` | HTTP server (admin dashboard, REST API) |
| `github.com/dop251/goja` | Go-native JS engine (ES5.1+) |
| `github.com/dop251/goja_nodejs` | Node.js compat layer for goja |
| `github.com/gorilla/websocket` | WebSocket (Web adapter) |
| `github.com/boltdb/bolt` | Local persistent storage |
| `go.mongodb.org/mongo-driver` | MongoDB integration |
| `google.golang.org/grpc` | gRPC communication |
| `github.com/robfig/cron/v3` | Cron scheduler |
| `github.com/Dreamacro/clash` | Proxy support |
| `github.com/beego/beego/v2` | ??? (unconfirmed) |
| `github.com/elastic/go-elasticsearch/v6` | Elasticsearch integration |
| `github.com/go-redis/redis/v8` | Redis integration |

## 6. CI/CD Pipeline

### GitHub Actions (`.github/workflows/build.yml`)
- **Trigger**: Push to `v2.*` branches, PRs, or manual `workflow_dispatch`
- **Jobs**:
  1. `timestamp`: Generate build timestamp + version base
  2. `build`: Cross-compile for 5 platforms (Linux amd64/arm64, macOS amd64/arm64, Windows amd64)
  3. `docker`: Build multi-arch Docker image (`ntwck/sillygirl:dev` or `:latest` on release)
  4. `release`: Create/update GitHub Release with all binaries

- **Release modes**:
  - Auto (push to v2.*): Creates pre-release with `-dev.{timestamp}` tag
  - Manual (`workflow_dispatch`): Update existing release with provided `release_tag`
  - On manual release: auto-deletes old pre-releases

### release-dryrun.sh
Run from `sillyGirl/` directory:
```bash
bash release-dryrun.sh
```
Phases:
1. Pre-flight: gh CLI, remote, branch, clean status check
2. Version bump: finds last non-dev tag (v2.1.x), increments patch
3. Changelog: groups commits by feat/fix/chore/other, outputs to `/tmp/release_notes_V.md`
4. Dry-run summary: prints all commands that would be executed (no actual changes)

### deploy-test.sh
Remote deployment to PM2-managed instance:
```bash
scp deploy-test.sh pagermaid@192.168.1.12:/tmp/
ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh /home/pagermaid/docker/sillyplus v2.1.6"
```
Flow:
1. Validate ELF binary
2. Backup current binary
3. Stop pm2 process
4. Replace binary
5. Restart pm2
6. Health check (online status)
7. Auto-rollback on failure

### install.sh
One-click install (curl-based):
```bash
bash install.sh
# Downloads from raw.githubusercontent.com, installs to /usr/local/sillyGirl
```

## 7. Docker Deployment

```bash
docker run -d \
  --name sillygirl \
  -p 8080:8080 \
  -v $(pwd)/sillygirl-data:/data \
  --restart unless-stopped \
  ntwck/sillygirl:latest
```

- Binary at `/sillyGirl` (outside volume — immutable)
- Data at `/data` (volume-mounted — plugins, DB, config)
- Entrypoint script symlinks `/sillyGirl` into `/data/sillyGirl`
- Sets `SILLYGIRL_DATA_PATH=/data`
- Supports `linux/amd64` and `linux/arm64`

## 8. Recent Changelog (since v2.1.5)

| Commit | Description |
|---|---|
| 1a8bb8b | feat: 分佣系统后端修复 — 商品图片/订单时间/登录token |
| cfbe0af | feat: API 支持通过 PUT /api/storage 设置 "reload" 值直接触发插件重载 |
| ef99b31 | fix(cors): 动态 Origin 头替代硬编码 *，支持 credentials include 跨域请求 |
| 4210a1c | fix: 升级下载支持 GitHub 加速代理 (ghproxy) |
| f1ebf98 | feat: 支持私聊消息采集搬运（监听指定用户私聊并转发到群） |
| bc718ac | fix: KillPeer 跳过自身 PID 避免 Docker 重启后自杀 |
| 45bd5c4 | fix: 将sillyGirl.pid写入临时目录而非持久化data目录 |

## 9. Important Paths & Conventions

- **Plugin directory**: `plugins/` (created at runtime in working directory or `/data/plugins/` in Docker)
- **Data directory**: `.sillyplus/` (config, DB, cache) or `/data/.sillyplus/` in Docker
- **Node.js runtime**: `language/` in working directory or `/data/language/` in Docker
- **Build output**: `sillyGirl_{os}_{arch}[.exe]`
- **Docker image**: `ntwck/sillygirl` (Docker Hub)
- **Branch convention**: `v2.*` for release branches, `main` in workflow.yaml is outdated
- **Tag convention**: `v2.1.x` (stable), `v2.1.x-dev.{timestamp}` (dev builds)

## 10. Things to Know

- `workflow.yaml` is **outdated** (uses old go-version v2, references cdle/sillyplus repo). The active CI is `.github/workflows/build.yml`.
- QQ adapter exists (`adapters/qq/main.go`) but is commented out in `main.go`.
- `adapters/pagermaid/sillyplus.py` is a Python-based adapter (for PagerMaid).
- Plugin encryption is supported (`encrypt.go` + `@encrypt true` annotation).
- The `proto3/` directory generates multi-language bindings from a single `.proto` file — used by `sillyplus_plugins` and external tools.
- Web admin dashboard accessible at `http://host:8080/admin`.
- Admin user detection: `Bucket("qq")["masters"]` — concatenated with `&`.
- Group control commands: `listen`, `unlisten`, `reply`, `noreply` (sent by admin in group chat).
