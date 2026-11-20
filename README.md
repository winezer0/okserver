# okserver

浏览器文件工作台与命令行传输工具。Go 单一可执行文件提供 HTTP 服务、CLI，并嵌入构建后的 Web 前端。运行时不需要 Node.js 或数据库。

[English](README.en.md)

## 功能

- 浏览器文件工作台：目录浏览、分页、搜索和排序；中英文界面与明暗主题。
- 页面地址：`/files`、`/files/<目录>` 和 `/tasks`；根路径 `/` 默认进入文件页。
- 浏览器上传：多文件有界并发、单文件串行分片、暂停/恢复/取消、自动重试；使用 SHA-256 校验分片及完整文件，恢复前校验已确认前缀。
- 浏览器下载：支持原生下载；支持 File System Access API 的浏览器可使用 OPFS 持久暂存、分片恢复和完整 SHA-256 校验后导出。传输中心可查看、恢复和清理暂存任务。
- HTTP 上传：原始 POST、单文件 multipart POST，以及带 `Content-Range` 的串行 PUT 分片/续传，适用于 curl、wget 等客户端。
- HTTP 下载：HEAD、Range、If-Range、ETag 和完整文件 SHA-256 响应头。
- CLI：上传自动续传；下载默认四路 Range 并发，校验 SHA-256 后原子替换目标文件。
- 服务管理：Basic Auth 或 URL 参数认证、TLS、同源/CORS 控制、SSE 任务事件、健康检查、指标及结构化日志。
- 可选文件管理：通过 `server.allow_create_dir` 与 `server.allow_delete_file` 分别启用单级目录创建和文件/目录递归删除；浏览器支持本页多选、批量删除与多文件下载。
- 可靠性与资源控制：JSON 持久化上传任务和摘要清单、启动恢复、磁盘空间预算、并发/任务/暂存上限、过期清理和原子发布。

前端使用 React、严格 TypeScript、Vite、Tailwind、React Router、TanStack Query/Table、Base UI 和 hash-wasm；后端使用 Go、Gin 和标准库。服务不依赖数据库或 Redis。

## 构建与启动

需要 Go（版本见 `go.mod`）和 Node.js（版本见 `web/package.json`）。Windows 启动脚本可从任意工作目录运行：

```bat
start-webui-dev.bat
start-webui-build.bat
start-webui-build.bat --build-only

rem 开发：前端端口、后端端口、配置文件、存储目录
start-webui-dev.bat 28081 28080 server.yaml D:\uploads
rem 构建并启动：后端端口、配置文件、存储目录
start-webui-build.bat 28080 server.yaml D:\uploads
```

开发脚本启动 Vite 和 Go 服务，并代理 `/upload`、`/download`、管理 API 与 SSE。可用 `--no-open` 禁止自动打开浏览器，按 Ctrl+C 停止脚本启动的进程。

手动构建：

```sh
npm --prefix web ci
npm --prefix web test
npm --prefix web run build
go build -o bin/okserver ./cmd/okserver
bin/okserver -c server.yaml
```

Windows 可执行文件使用 `.exe` 后缀。`internal/webui/assets` 是构建生成并纳入版本控制的嵌入资源，不要手动编辑。

## 配置与安全

```sh
okserver -g                         # 生成默认配置，不覆盖已有文件
okserver -g -c custom.yaml
okserver -c custom.yaml -s 127.0.0.1:28080 -p ./uploads
okserver --version
```

没有显式指定配置时，缺少默认配置文件会使用内置默认值；显式 `-c` 指定的文件不存在会报错。配置只接受 `server.yaml` 的当前结构，未知、重复或旧字段及多个 YAML 文档会被拒绝。

默认仅允许同源页面访问数据 API。跨域前端可在 `server.allowed_origins` 配置完整来源（不支持通配符）；预检请求不会绕过实际请求的认证。反向代理终止 TLS 时，应把浏览器实际使用的 HTTPS 来源加入白名单。启用认证时建议使用 HTTPS。

认证使用 `server.auth: "user:password"`（HTTP Basic）或 `server.param: "token:secret"`（URL 参数）。两者同时启用时，合法 Basic 凭证优先，否则验证参数凭证。协议保留参数名不能用作认证参数名，重复认证参数会被拒绝。静态前端资源可公开访问，数据路由需要认证；日志和启动提示不会输出配置中的凭证。

`server.disable_front_ui: true` 只关闭 `/`、`/files`、`/files/...`、`/tasks` 和前端静态资源，不关闭 API。API 仍按配置执行认证；若要禁止目录管理，也应保持下面两个管理开关为 `false`。

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `server.listen` / `server.files_dir` | `:28080` / `./files` | 监听地址与对外文件根目录 |
| `server.states_dir` | `./okserver_states` | 上传会话元数据、锁、磁盘预算和文件摘要清单 |
| `server.cache_dir` | `./okserver_cache` | 上传分块和流式上传临时数据 |
| `server.allow_create_dir` | `false` | 启用浏览器单级目录创建 |
| `server.allow_delete_file` | `false` | 启用浏览器文件及目录递归删除 |
| `server.disable_front_ui` | `false` | 禁止访问浏览器前端页面和静态资源 |
| `upload.chunk_size` | `8MiB` | 上传分片大小；配置为 0 时使用默认值 |
| `upload.max_concurrent` | 4 | 全局上传并发 |
| `upload.max_per_identity` | 32 | 每个身份的活动任务上限 |
| `upload.buffer_budget_bytes` | `64MiB` | 上传请求预留缓冲总预算 |
| `upload.max_sessions` / `max_pending_bytes` | 128 / `100GiB` | 任务数和暂存数据上限 |
| `upload.max_file_size` | 0 | 0 表示不额外限制已知长度文件 |
| `upload.min_free_disk_bytes` | `1GiB` | 最低可用空间 |
| `upload.idle_timeout` / `session_ttl` | 2 分钟 / 24 小时 | 请求空闲与活动会话时限 |
| `upload.terminal_ttl` / `verify_workers` | 7 天 / 2 | 终态保留时间与校验并发 |
| `download.max_concurrent` | 16 | 全局下载并发 |

上传容量配置支持 `B`、`KiB`、`MiB`、`GiB`、`TiB`、`PiB`、`EiB` 后缀，按 1024 进制换算；未带单位的整数仍按字节解析，兼容现有配置。

资源限制值为 0 表示关闭相应限制；未知长度和 multipart 上传仍受暂存上限约束。`log.console`、`log.file` 可分别配置格式和级别。TLS 证书配置在 `server.certificate`。反向代理应允许长时间流式请求，关闭上传请求缓冲和 SSE 响应缓冲。

## HTTP 上传与下载

上传 URL 必须包含目标文件名，不能只指定目录：

```sh
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/dir/file.zip"
curl --fail-with-body -F "file=@file.zip" "http://localhost:28080/upload/dir/file.zip"
wget -O - --post-file="file.zip" "http://localhost:28080/upload/dir/file.zip"

# 可选的完整文件 SHA-256
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/file.zip?sha256=<64位十六进制>"

# 查询上传状态并从明确偏移续传
curl -I "http://localhost:28080/upload/file.zip"
curl --fail-with-body -C 8388608 -T "file.zip" "http://localhost:28080/upload/file.zip"

# 断点下载
curl --fail-with-body -C - -o file.zip "http://localhost:28080/download/dir/file.zip"
wget -c -O file.zip "http://localhost:28080/download/dir/file.zip"
curl -I "http://localhost:28080/download/dir/file.zip"
```

Windows PowerShell 使用 `curl.exe` 以避开命令别名。原始流和 multipart 共用上传接口；multipart 只接受一个名为 `file` 的部分，保存路径由 URL 决定。服务端可能返回 202，表示数据已接收、后台校验尚未完成；应查询上传 URL 的 HEAD，直到 `Upload-State: completed`。失败任务不会发布文件。下载 HEAD、GET、206 响应中的 `X-File-SHA256` 始终表示完整文件摘要，`X-File-Size` 表示完整文件大小。curl/wget 不会自动校验摘要，可将响应头与本地 SHA-256 计算结果比较。

默认拒绝覆盖已有文件。显式覆盖必须同时使用 `?overwrite=true` 和目标当前强 ETag 的 `If-Match`；新内容完整校验成功后才替换旧文件。

## CLI

```sh
okserver cli -c -d @file.zip "http://localhost:28080/upload/dir/file.zip"
okserver cli -c --parallel 4 -o file.zip "http://localhost:28080/download/dir/file.zip"
okserver cli --auth "user:password" --ca ca.crt -c -d @file.zip "https://server/upload/file.zip"
```

`--parallel` 范围为 1 到 16，默认 4。`-c` 启用下载续传（上传自动恢复）；下载暂存放在目标目录的 `.okserver-downloads` 中，续传状态与目标文件版本绑定。完整摘要校验失败时不会替换现有目标。常用选项还有 `-o`（下载目标）、`--auth`、`--ca`、`-k`（跳过 TLS 证书校验）和 `-t`（超时）。

浏览器文件读写 API 需要 HTTPS 或 localhost。支持的浏览器把托管下载暂存到当前站点 OPFS 的 `okserver-downloads-v1`，与用户目标文件隔离；刷新后重新选择目标可校验并复用有效分片。完整 SHA-256 校验通过后才导出。站点存储可能被浏览器回收，因此暂存恢复不保证永久保留。传输中心支持恢复、取消并清理单项任务及批量清理；清理会等待请求和写入结束，并通过 Web Locks 协调多标签页，不删除用户目标文件。原生下载由浏览器管理，页面无法跟踪其完成状态或自动验证结果。

浏览器与 CLI 对网络错误、408、423、429 和 5xx 使用有上限的退避与 `Retry-After`；认证、参数、版本和最终摘要错误直接失败。失败或取消的旧上传可重新开始，不会从不可信旧偏移继续。

## HTTP API

| 路径 | 方法 | 功能 |
|---|---|---|
| `/upload/<filename>` | POST / PUT | 创建或继续上传；原始请求体或 `Content-Range` 串行分片 |
| `/upload/<filename>` | HEAD | 上传偏移、大小、任务 ID、状态；`prefix=sha256` 查询前缀摘要 |
| `/upload/<filename>` | GET | 以 `Content-Type: application/offset` 查询偏移 |
| `/upload/<filename>` | DELETE | 取消未发布的上传 |
| `/download/<filename>` | GET / HEAD | 文件下载、Range、版本与完整文件摘要 |
| `/api/v1/files` | GET | 文件列表、分页、搜索和排序 |
| `/api/v1/capabilities` | GET | 当前实例启用的目录创建和删除能力 |
| `/api/v1/create` | POST | 在指定父目录下创建单级目录（需 `allow_create_dir`） |
| `/api/v1/delete` | POST | 批量删除文件或递归删除目录（需 `allow_delete_file`） |
| `/api/v1/uploads` | GET | 活动上传任务列表 |
| `/api/v1/uploads/<id>` | GET / DELETE | 查询或取消任务 |
| `/api/v1/uploads/cleanup` | POST | 清理过期记录 |
| `/api/v1/events` | GET | SSE 任务快照与事件 |
| `/healthz`, `/readyz`, `/metrics` | GET / HEAD | 健康、就绪和 Prometheus 指标 |

目录管理默认关闭。启用后，管理 API 仍需要通过服务器认证，并始终将相对路径限制在 `server.files_dir` 根目录内；绝对路径、目录穿越和内部 `.okserver` 路径会被拒绝。目录创建只创建单级目录；删除接口最多接受 500 个路径，目录会递归删除且不可恢复。删除列表先整体校验，随后逐项删除。

```yaml
server:
  allow_create_dir: true
  allow_delete_file: true
```

```sh
# 在相对父目录 reports 下创建单级目录
curl --fail-with-body -u 'user:password' \\
  -H 'Content-Type: application/json' \\
  -d '{"parent":"reports","name":"2026"}' \\
  http://localhost:28080/api/v1/create

# 批量删除文件和目录；目录会递归删除
curl --fail-with-body -u 'user:password' \\
  -H 'Content-Type: application/json' \\
  -d '{"paths":["reports/old.csv","tmp"]}' \\
  http://localhost:28080/api/v1/delete
```

创建请求使用 `{ "parent": "相对父目录", "name": "单级目录名" }`；根目录用空字符串作为 `parent`。删除请求使用 `{ "paths": ["相对路径", ...] }`。Web 界面通过 `/api/v1/capabilities` 读取开关状态并隐藏未启用的操作，服务端仍会独立拒绝被禁用的写请求。多选仅作用于当前页；批量下载会下载所选文件并跳过所选目录。

浏览器和 CLI 使用 `Upload-Token` 隔离上传会话，服务端保存凭证摘要。分片默认携带 `X-Chunk-SHA256`，最后一片携带完整摘要。会话元数据和分块数据分别保存在 `server.states_dir` 与 `server.cache_dir`。留空时，三个目录默认分别为相对于当前工作目录的 `./files`、`./okserver_states` 和 `./okserver_cache`。目录须互不重叠。服务端下载直接流式读取，不生成下载暂存文件。旧配置 `server.path` 和 `upload.chunk_dir` 不再支持。保留旧版已发布文件时，将 `server.files_dir` 指向原 `uploads` 目录；新状态和缓存目录可为空，文件摘要会按需重建。旧会话状态不迁移，未完成上传不能续传。上传任务管理、摘要缓存和存储发布由 JSON 清单、文件锁及原子替换支持；发布后的受管理文件应保持不可变，外部更新应通过替换文件完成。

## 代码结构

| 目录 | 职责 |
|---|---|
| `cmd/okserver`、`cmd/client` | 服务与 CLI 入口 |
| `internal/server` | HTTP 路由、中间件、认证、配置、SSE 与传输处理 |
| `internal/upload`、`internal/uploadstore` | 上传状态机、资源控制、会话和任务持久化 |
| `internal/storage`、`internal/diskbudget` | 文件锁、磁盘预算、持久写入和原子发布 |
| `internal/digest`、`internal/hashmanifest` | SHA-256、文件指纹及摘要清单 |
| `internal/client` | CLI 上传、Range 下载、恢复、重试和校验 |
| `web/src/features` | 文件浏览、上传队列、下载和传输中心 |
| `internal/webui/assets` | 编译后嵌入二进制的前端资源 |

## 测试

```sh
go test ./internal/... ./cmd/...
go test -race ./internal/... ./cmd/...
npm --prefix web test
npm --prefix web run build
python tests/transfer_cli.py
node --test tests/browser.test.cjs
node --test tests/cli-large.test.cjs
node --test tests/release.test.cjs tests/launchers.test.cjs
```

真实浏览器验收需要 Chrome/Edge，可通过 `CHROME_PATH` 指定。`OKSERVER_BROWSER_MB`、`OKSERVER_CLI_MB` 和 `OKSERVER_LARGE_TEST_MB` 可调整大文件验收规模；`OKSERVER_MEASURE_RESOURCES=1` 可记录采样资源数据。测试会创建并清理隔离数据。不要在 Vite 重建嵌入资源或 `npm ci` 期间并行执行 Go 构建验收。

浏览器自动化使用真实 Chromium、OPFS 和写入事务，覆盖下载中断、刷新后分片复用及保护原目标；测试以 OPFS 句柄替代操作系统保存位置选择器，因此实际磁盘位置选择、长期权限保留和用户拒绝授权仍需手动验收。完整协议设计见[串行上传下载接口设计方案](webserver串行上传下载接口设计方案.md)。
