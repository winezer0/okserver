# okserver

浏览器文件工作台与命令行传输工具。Go 单二进制包含 Web 服务端、CLI 和嵌入式前端；运行时无需 Node.js 或数据库。

## 能力

- 浏览器目录导航、分页、搜索、排序，中英切换、明暗主题。
- 同一文件串行分片上传，多个文件有界并发；暂停、恢复、取消、自动重试。
- 分片和完整文件统一使用 SHA-256。恢复前验证已上传前缀，正常上传维护增量摘要。
- curl/wget 原始 POST、单文件 multipart；支持 curl 显式偏移 PUT 续传。
- 下载支持 HEAD、Range、If-Range、强 ETag 和完整文件摘要响应头。
- CLI 默认四路 Range 下载，落盘后验证 SHA-256，再原子替换目标。
- 支持文件读写 API、OPFS 与 Web Locks 的浏览器通过下载按钮四路下载，分片独立持久暂存，完整 SHA-256 校验通过后才提交目标文件；其他浏览器使用原生下载，网页不自动校验原生下载结果。
- 文件系统和 JSON 保存任务及摘要清单；有界校验 Worker、断电恢复、空间预算和过期清理。
- Basic Auth、URL 参数认证、HTTPS、slogs 结构化日志、SSE、健康检查和指标。

前端：React、严格 TypeScript、Vite、Tailwind、Base UI、React Router、TanStack Query/Table、Lucide、Zod、hash-wasm。后端：Go、Gin HTTP 接入、标准库 embed、winezer0/slogs。无 GORM、Redis、数据库或多套上传协议。

## 构建与启动

需要 Go（版本见 go.mod）和 Node.js（版本见 web/package.json）。Windows 可从任意工作目录运行：

```bat
start-webui-dev.bat
start-webui-build.bat
start-webui-build.bat --build-only

rem 开发：前端端口 后端端口 配置文件 存储目录
start-webui-dev.bat 28081 28080 server.yaml D:\uploads
rem 生产构建并启动：后端端口 配置文件 存储目录
start-webui-build.bat 28080 server.yaml D:\uploads
```

开发脚本分别启动 Vite 和 Go，通过同源代理转发 `/upload`、`/download`、管理 API 和 SSE；支持前端热更新。`--no-open` 禁止自动打开浏览器，Ctrl+C 停止脚本启动的进程。

手动构建：

```sh
npm --prefix web ci
npm --prefix web test
npm --prefix web run build
go build -o bin/okserver ./cmd/okserver
bin/okserver -c server.yaml
```

Windows 二进制使用 `.exe` 后缀。`internal/webui/assets` 是需提交的构建产物，不应手动编辑。

## 配置

默认只允许同源网页访问数据接口。独立跨域前端可配置 `server.allowed_origins: ["https://files.example.com"]`，只接受完整来源，不接受通配符；预检请求不返回数据，实际请求仍须认证。TLS 在反向代理终止时，也应将浏览器访问的 HTTPS 来源加入白名单。响应暴露 SHA-256、ETag、Range 及上传状态字段。

```sh
okserver -g
okserver -g -c custom.yaml
okserver -c custom.yaml -s 127.0.0.1:28080 -p ./uploads
okserver --version
```

`-g` 生成可运行配置后退出，拒绝覆盖已有文件。缺少默认配置时使用内置默认值；显式 `-c` 指定的文件不存在则报错。只支持 [server.yaml](server.yaml) 中的新结构，未知字段、旧字段、重复字段和多个 YAML 文档直接报错。

| 设置 | 默认值 |
|---|---|
| server.listen / path | :28080 / ./uploads |
| upload.chunk_size | 8 MiB |
| upload.max_concurrent | 4 |
| upload.max_per_identity | 32 个活动任务 |
| upload.buffer_budget_bytes | 64 MiB，限制可同时预留的上传缓冲 |
| upload.max_sessions / max_pending_bytes | 128 / 100 GiB |
| upload.max_file_size | 0，已知长度文件不额外限大小 |
| upload.min_free_disk_bytes | 1 GiB |
| upload.idle_timeout / session_ttl | 2 分钟 / 24 小时 |
| upload.terminal_ttl / verify_workers | 7 天 / 2 |
| download.max_concurrent | 16 |

资源上限中的 0 表示关闭该项限制；分片大小 0 使用 8 MiB。缓冲预算启用时至少 8 MiB，按每个上传请求预留 8 MiB 控制并发。未知长度和 multipart 暂存始终有有限上限，默认 100 GiB，并受文件大小和任务配额的较小值约束。

认证配置为 `server.auth: "user:password"` 或 `server.param: "token:secret"`。同时启用时，合法 Basic 优先，否则校验参数认证。协议参数名不能用作认证参数名，重复认证参数被拒绝。日志和启动示例不输出真实凭证。前端静态代码公开，数据接口均经过认证。

`log.console`、`log.file` 分别支持 text/json 格式及日志级别。`log.file.localtime` 控制轮转备份文件名的时区。TLS 配置使用 `server.certificate.cert` 和 `key`；启用认证时应使用 HTTPS。反向代理需允许长时间流式请求，关闭上传请求缓冲及 SSE 响应缓冲。

## 简单上传与下载

上传 URL 必须包含完整目标文件名，不接受 `/upload/目录/`。

```sh
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/dir/file.zip"
curl --fail-with-body -F "file=@file.zip" "http://localhost:28080/upload/dir/file.zip"
wget -O - --post-file="file.zip" "http://localhost:28080/upload/dir/file.zip"

# 可选完整摘要：参数优先于 X-File-SHA256 请求头
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/file.zip?sha256=<64位十六进制>"

# 查询上传偏移，再显式指定续传位置
curl -I "http://localhost:28080/upload/file.zip"
curl --fail-with-body -C 8388608 -T "file.zip" "http://localhost:28080/upload/file.zip"

# 断点下载与获取完整摘要
curl --fail-with-body -C - -o file.zip "http://localhost:28080/download/file.zip"
wget -c -O file.zip "http://localhost:28080/download/file.zip"
curl -I "http://localhost:28080/download/file.zip"
```

Windows 使用 `curl.exe` 避免 PowerShell 命令别名。无需指定 application/octet-stream。原始流和 multipart 共用接口，multipart 仅接受一个 `file` 部分，保存名称由 URL 决定。

服务端可返回 202，表示字节已接收、仍在后台校验；查询同一上传地址的 HEAD，直到 `Upload-State: completed` 才是发布成功。返回 failed 时文件不会发布。下载 HEAD/GET/206 的 `X-File-SHA256` 始终是**完整文件**摘要，`X-File-Size` 是完整大小。

curl/wget 不自动验证文件摘要；可使用 PowerShell `Get-FileHash file.zip -Algorithm SHA256`、Linux `sha256sum file.zip` 或 macOS `shasum -a 256 file.zip` 与响应头比对。未提供客户端摘要时，服务端摘要仅证明收到的内容。

## CLI

```sh
okserver cli -c -d @file.zip "http://localhost:28080/upload/dir/file.zip"
okserver cli -c -parallel 4 -o file.zip "http://localhost:28080/download/dir/file.zip"
okserver cli -auth "user:password" -ca ca.crt -c -d @file.zip "https://server/upload/file.zip"
```

`-parallel` 为 1 到 16，默认 4。CLI 下载先写目标目录内的 `.okserver-downloads`，断点与目标版本一起持久化；校验失败不会替换已有目标。浏览器需 HTTPS 或 localhost 才能使用文件读写 API；文件名与下载图标使用同一下载流程，支持时启用托管校验下载。

浏览器托管下载将数据放在当前站点 OPFS 的 `okserver-downloads-v1` 中，与用户选择的目标文件隔离。每块写入并关闭后才更新 JSON 记录；刷新后重新选择保存位置，会先检查服务端版本并校验已有分片，仅下载缺失或损坏的块。完整校验使用 Worker 分块读取磁盘文件，导出也使用固定大小缓冲，失败或取消时中止目标写入流，保留原目标。浏览器可能回收站点存储，因此恢复不承诺永久保留。

传输中心页“下载暂存”展示未完成任务及实际暂存字节，支持恢复、单项“取消并清理”和批量清理。清理会先取消任务并等待请求及写入结束，通过跨标签页消息和 Web Locks 排除并行写入，再删除专用暂存目录；不会删除用户目标文件。成功后自动清理，失败暂存由用户清理。暂存与目标导出期间需要额外磁盘空间；空间不足会停止并保留可清理状态。

浏览器与 CLI 的初始状态查询、恢复查询、校验轮询及传输重试覆盖网络错误、408/423/429/5xx，使用有上限的指数退避、抖动及 `Retry-After`；认证、参数、版本或最终摘要错误直接失败。失败/取消的旧上传任务可以重新开始，不按旧偏移恢复。

## 统一协议与代码结构

| 地址 | 方法 | 用途 |
|---|---|---|
| /upload/完整文件名 | POST / PUT | 自动创建任务，原始上传或串行 Content-Range 分片 |
| /upload/完整文件名 | HEAD | 偏移、大小、任务 ID、状态；prefix=sha256 查询前缀摘要 |
| /upload/完整文件名 | GET | Content-Type: application/offset 查询偏移 |
| /upload/完整文件名 | DELETE | 取消未发布任务 |
| /download/完整文件名 | GET / HEAD | Range 下载、版本及完整摘要 |
| /api/v1/files | GET | 目录分页、搜索、排序 |
| /api/v1/uploads、/api/v1/uploads/:id | GET | 活动任务列表、单任务结果 |
| /api/v1/uploads/:id | DELETE | 管理界面取消任务，仍需会话凭证 |
| /api/v1/uploads/cleanup | POST | 清理过期记录 |
| /api/v1/events | GET | SSE 任务快照 |

浏览器和 CLI 使用 Upload-Token 隔离会话，服务端仅保存凭证摘要。恢复前校验已确认前缀，分片默认携带 X-Chunk-SHA256，最后一片携带完整摘要。无独立创建/完成上传 API，也没有旧 curl/Tus 或多摘要查询接口。

默认拒绝覆盖。显式覆盖需 `?overwrite=true` 和目标当前强 ETag 的 `If-Match`，通过完整校验后才替换旧文件。受管理的文件发布后应保持不可变；外部更新应采用替换方式。外部原地写入会使摘要缓存失效；传输期间检测到版本变化时中止响应，客户端仍须最终校验。

| 目录 | 职责 |
|---|---|
| cmd/okserver、cmd/client | 二进制入口、CLI 参数 |
| internal/server | Gin 路由、认证、HTTP 协议映射、Worker、SSE |
| internal/upload | 上传状态、资源规则、串行写入与校验 |
| internal/uploadstore、hashmanifest | JSON 任务与摘要清单 |
| internal/storage、diskbudget | 锁、空间预算、持久化及原子发布 |
| internal/digest | SHA-256、指纹和缓存 |
| internal/client | 上传与 Range 下载调度、恢复、重试、校验 |
| web/src/features | 目录、上传队列、下载交互 |

上传管理器从持久 JSON 重建路径与任务索引，查询只读取对应当前任务及活动任务，历史 JSON 不再逐分片重复解析；目录发现允许多个进程识别新任务。下载优先查询持久摘要清单，仅在打开文件的可靠指纹和大小匹配时复用 SHA-256；清单按原子替换版本缓存，外部文件修改或清单替换会使缓存失效。

内部文件位于存储根目录 `.okserver`，不会出现在列表或下载路由。任务查询不延长有效期，取消后立即清理数据并保留终态。完整设计见 [接口设计方案](webserver串行上传下载接口设计方案.md)。

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

`OKSERVER_BROWSER_MB=2050` 启用超过 2 GiB 的真实浏览器验收，`OKSERVER_MEASURE_RESOURCES=1` 记录浏览器进程集合及服务端的采样内存和暂存空间；采样最大值不等于精确峰值。`OKSERVER_CLI_MB=2050` 调整真实 CLI 上传及四路下载验收大小，`OKSERVER_LARGE_TEST_MB` 调整恢复测试大小。浏览器验收需要 Chrome/Edge，可通过 CHROME_PATH 指定。Windows 本地、Linux WSL 验收；macOS 由最后执行的 CI 作业验收。测试使用独立缓存并自动清理生成的数据和浏览器配置目录；CLI 可通过 `OKSERVER_CACHE_DIR` 指定会话缓存位置。不要在 Vite 重建嵌入资源或 npm ci 正在运行时并行执行 Go 构建验收。

浏览器自动验收使用真实 Chromium、真实 OPFS 暂存和事务写入，并通过 OPFS 目标句柄替代系统保存选择器，覆盖下载中断、页面刷新后复用分片及原目标保护。它不等同于操作系统“另存为”对话框验收；实际磁盘目标选择、长时间权限保留及用户拒绝授权仍需手工验证。

传输中心统一展示本页上传队列、服务端活动上传和浏览器下载任务，支持类型与状态筛选、恢复及清理。下载完成记录在当前标签页会话存储中保留最近 200 条；服务端未提供其他客户端的完整历史，原生下载也无法由页面跟踪完成状态。
