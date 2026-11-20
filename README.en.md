# okserver

A browser file workspace and command-line transfer tool. A single Go executable provides the HTTP server and CLI and embeds the built web interface. Node.js and a database are not required at runtime.

[简体中文](README.md)

## Features

- Browser workspace with directory browsing, pagination, search, sorting, Chinese/English UI, and light/dark themes.
- Page routes are `/files`, `/files/<directory>`, and `/tasks`; `/` opens the file page.
- Browser uploads with bounded concurrency across files and serial chunks per file; pause, resume, cancel, and retry. SHA-256 checks cover chunks and complete files, and resume validates the acknowledged prefix.
- Browser downloads use native browser downloads by default. Browsers supporting the File System Access API can use OPFS staging, resumable chunks, and full SHA-256 verification before export. The transfer center can inspect, resume, and clean staged tasks.
- HTTP uploads support raw POST, single-file multipart POST, and serial PUT chunks/resume with `Content-Range`, for clients such as curl and wget.
- HTTP downloads support HEAD, Range, If-Range, ETag, and a full-file SHA-256 response header.
- CLI uploads resume automatically. Downloads use four concurrent Range requests by default, verify SHA-256, then atomically replace the target.
- Basic Auth or URL-parameter authentication, TLS, same-origin/CORS controls, SSE task events, health checks, metrics, and structured logs.
- Optional file management: `server.allow_create_dir` enables single-level directory creation and `server.allow_delete_file` enables file and recursive directory deletion; the browser supports page selection, batch deletion, and multi-file downloads.
- Reliability and resource controls include JSON-persisted upload tasks and digest manifests, startup recovery, disk-space budgeting, concurrency/session/staging limits, expiration cleanup, and atomic publication.

The frontend uses React, strict TypeScript, Vite, Tailwind, React Router, TanStack Query/Table, Base UI, and hash-wasm. The backend uses Go, Gin, and the standard library. No database or Redis is required.

## Build and run

Requires Go (see `go.mod`) and Node.js (see `web/package.json`). Windows launchers can be run from any working directory:

```bat
start-webui-dev.bat
start-webui-build.bat
start-webui-build.bat --build-only

rem Development: frontend port, backend port, config file, storage directory
start-webui-dev.bat 28081 28080 server.yaml D:\uploads
rem Build and run: backend port, config file, storage directory
start-webui-build.bat 28080 server.yaml D:\uploads
```

The development launcher runs Vite and Go and proxies `/upload`, `/download`, management APIs, and SSE. Use `--no-open` to suppress automatic browser launch; Ctrl+C stops processes started by the launcher.

Manual build:

```sh
npm --prefix web ci
npm --prefix web test
npm --prefix web run build
go build -o bin/okserver ./cmd/okserver
bin/okserver -c server.yaml
```

Windows executables use the `.exe` suffix. `internal/webui/assets` contains generated, version-controlled embedded assets; do not edit them by hand.

## Configuration and security

```sh
okserver -g                         # Generate defaults; existing files are never overwritten
okserver -g -c custom.yaml
okserver -c custom.yaml -s 127.0.0.1:28080 -p ./uploads
okserver --version
```

Without an explicit config path, a missing default config uses built-in defaults. A missing file passed with `-c` is an error. Only the current `server.yaml` structure is accepted; unknown, duplicate, or legacy fields and multiple YAML documents are rejected.

By default, data APIs accept same-origin browser requests. Configure complete origins in `server.allowed_origins` for a cross-origin frontend; wildcards are not supported. A CORS preflight does not bypass authentication on the actual request. If TLS terminates at a reverse proxy, add the HTTPS origin used by the browser to the allowlist. HTTPS is recommended whenever authentication is enabled.

Authentication uses `server.auth: "user:password"` (HTTP Basic) or `server.param: "token:secret"` (URL parameter). If both are enabled, valid Basic credentials take precedence; otherwise the URL parameter is checked. Protocol-reserved parameter names cannot be used for authentication, and duplicate authentication parameters are rejected. Static frontend assets are public, while data routes require authentication. Logs and startup examples do not print configured credentials.

`server.disable_front_ui: true` disables `/`, `/files`, `/files/...`, `/tasks`, and frontend assets. It does not disable APIs. APIs still enforce configured authentication; keep the management switches below `false` as well when directory management must be unavailable.

| Setting | Default | Description |
|---|---:|---|
| `server.listen` / `server.files_dir` | `:28080` / `./files` | Listen address and public file root |
| `server.states_dir` | `./okserver_states` | Upload metadata, locks, disk budgets, and digest manifest |
| `server.cache_dir` | `./okserver_cache` | Upload chunks and streaming upload scratch data |
| `server.allow_create_dir` | `false` | Enable single-level directory creation in the browser |
| `server.allow_delete_file` | `false` | Enable browser file and recursive directory deletion |
| `server.disable_front_ui` | `false` | Disable browser pages and frontend assets |
| `upload.chunk_size` | 8 MiB | Upload chunk size; zero selects the default |
| `upload.max_concurrent` | 4 | Global upload concurrency |
| `upload.max_per_identity` | 32 | Active tasks per identity |
| `upload.buffer_budget_bytes` | 64 MiB | Reserved upload request buffer budget |
| `upload.max_sessions` / `max_pending_bytes` | 128 / 100 GiB | Session and staged-data limits |
| `upload.max_file_size` | 0 | Zero disables the extra limit for known-length files |
| `upload.min_free_disk_bytes` | 1 GiB | Minimum free disk space |
| `upload.idle_timeout` / `session_ttl` | 2 min / 24 h | Request idle and active session timeouts |
| `upload.terminal_ttl` / `verify_workers` | 7 days / 2 | Terminal record retention and verification workers |
| `download.max_concurrent` | 16 | Global download concurrency |

A resource limit of zero disables that limit; unknown-length and multipart uploads remain bounded by staging limits. `log.console` and `log.file` have independent formats and levels. TLS certificates are configured under `server.certificate`. Reverse proxies should allow long-lived streaming requests and disable upload request buffering and SSE response buffering.

## HTTP upload and download

Upload URLs must include the complete destination filename, not only a directory:

```sh
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/dir/file.zip"
curl --fail-with-body -F "file=@file.zip" "http://localhost:28080/upload/dir/file.zip"
wget -O - --post-file="file.zip" "http://localhost:28080/upload/dir/file.zip"

# Optional full-file SHA-256
curl --fail-with-body --data-binary "@file.zip" "http://localhost:28080/upload/file.zip?sha256=<64-hex-digits>"

# Query upload state and resume at an explicit offset
curl -I "http://localhost:28080/upload/file.zip"
curl --fail-with-body -C 8388608 -T "file.zip" "http://localhost:28080/upload/file.zip"

# Resumable download
curl --fail-with-body -C - -o file.zip "http://localhost:28080/download/dir/file.zip"
wget -c -O file.zip "http://localhost:28080/download/dir/file.zip"
curl -I "http://localhost:28080/download/dir/file.zip"
```

On Windows PowerShell, use `curl.exe` to avoid the command alias. Raw and multipart uploads share an endpoint. Multipart accepts exactly one part named `file`; the URL determines the destination path. The server may return 202 when bytes are accepted but background verification is still running. Query the upload URL with HEAD until `Upload-State: completed`; failed tasks are not published. `X-File-SHA256` on download HEAD, GET, and 206 responses always describes the complete file, and `X-File-Size` is its complete size. curl and wget do not verify this digest automatically; compare it with a locally computed SHA-256.

Overwriting existing files is denied by default. To overwrite, provide both `?overwrite=true` and the target's current strong ETag in `If-Match`. The old file is replaced only after the new content passes full verification.

## CLI

```sh
okserver cli -c -d @file.zip "http://localhost:28080/upload/dir/file.zip"
okserver cli -c --parallel 4 -o file.zip "http://localhost:28080/download/dir/file.zip"
okserver cli --auth "user:password" --ca ca.crt -c -d @file.zip "https://server/upload/file.zip"
```

`--parallel` accepts 1 through 16 and defaults to 4. `-c` enables download resume (uploads resume automatically). Downloads are staged in `.okserver-downloads` inside the target directory, with resume state bound to the target version. A failed full-file digest check never replaces an existing target. Other common options are `-o` (download target), `--auth`, `--ca`, `-k` (skip TLS certificate verification), and `-t` (timeout).

Browser file read/write APIs require HTTPS or localhost. Supported browsers stage managed downloads in the current origin's OPFS directory `okserver-downloads-v1`, separate from the user-selected target. After refresh, selecting the target again allows valid chunks to be checked and reused. Export happens only after full SHA-256 verification. Browsers may evict site storage, so staged data is not guaranteed to persist indefinitely. The transfer center can resume, cancel and clean individual tasks, or clean them in batches. Cleanup waits for requests and writes and uses Web Locks to coordinate tabs; it does not delete user target files. Native downloads are managed by the browser; the page cannot track completion or verify their contents automatically.

Browser and CLI retry network errors, 408, 423, 429, and 5xx with bounded backoff and `Retry-After`. Authentication, parameter, version, and final digest errors fail directly. Failed or canceled old uploads can be restarted; they are not resumed from an untrusted old offset.

## HTTP API

| Path | Methods | Purpose |
|---|---|---|
| `/upload/<filename>` | POST / PUT | Create or continue an upload; raw body or serial `Content-Range` chunks |
| `/upload/<filename>` | HEAD | Offset, size, task ID, state; query prefix digest with `prefix=sha256` |
| `/upload/<filename>` | GET | Query offset using `Content-Type: application/offset` |
| `/upload/<filename>` | DELETE | Cancel an unpublished upload |
| `/download/<filename>` | GET / HEAD | Download, Range, version, and full-file digest |
| `/api/v1/files` | GET | File listing, pagination, search, and sorting |
| `/api/v1/capabilities` | GET | Enabled directory-creation and deletion capabilities |
| `/api/v1/create` | POST | Create one directory under a parent (requires `allow_create_dir`) |
| `/api/v1/delete` | POST | Batch-delete files or recursively delete directories (requires `allow_delete_file`) |
| `/api/v1/uploads` | GET | Active upload tasks |
| `/api/v1/uploads/<id>` | GET / DELETE | Query or cancel a task |
| `/api/v1/uploads/cleanup` | POST | Clean expired records |
| `/api/v1/events` | GET | SSE task snapshot and events |
| `/healthz`, `/readyz`, `/metrics` | GET / HEAD | Health, readiness, and Prometheus metrics |

Directory management is disabled by default. When enabled, management APIs still require server authentication and constrain relative paths to the `server.files_dir` root. Absolute paths, traversal, and the internal `.okserver` area are rejected. Directory creation creates one level only. Deletion accepts up to 500 paths; directories are deleted recursively and permanently. The full request list is validated before deletion begins, then paths are removed one by one.

```yaml
server:
  allow_create_dir: true
  allow_delete_file: true
```

```sh
# Create one folder under the relative parent directory reports
curl --fail-with-body -u 'user:password' \\
  -H 'Content-Type: application/json' \\
  -d '{"parent":"reports","name":"2026"}' \\
  http://localhost:28080/api/v1/create

# Batch-delete files and folders; folders are deleted recursively
curl --fail-with-body -u 'user:password' \\
  -H 'Content-Type: application/json' \\
  -d '{"paths":["reports/old.csv","tmp"]}' \\
  http://localhost:28080/api/v1/delete
```

Create requests use `{ "parent": "relative parent directory", "name": "single-level directory name" }`; use an empty `parent` for the storage root. Delete requests use `{ "paths": ["relative/path", ...] }`. The web UI reads the switches through `/api/v1/capabilities` and hides disabled actions; the server independently rejects disabled writes. Multi-selection is limited to the current page. Batch download downloads selected files and skips selected directories.

The browser and CLI use `Upload-Token` to isolate upload sessions; the server stores credential digests. Chunks include `X-Chunk-SHA256` by default, and the final chunk includes the full-file digest. Session metadata and chunk data live in `server.states_dir` and `server.cache_dir`. Empty values default to `./files`, `./okserver_states`, and `./okserver_cache`, relative to the current working directory. The directories must not overlap. Server downloads stream files directly and create no download scratch files. The old `server.path` and `upload.chunk_dir` settings are no longer supported. To keep published files from an earlier installation, point `server.files_dir` at the existing `uploads` directory. The new state and cache directories can start empty; file digests are rebuilt as needed. Old session state is not migrated, so unfinished uploads cannot resume. Upload management, digest caching, and storage publication use JSON manifests, file locks, and atomic replacement. Managed files should remain immutable after publication; external updates should replace the file.

## Code map

| Directory | Responsibility |
|---|---|
| `cmd/okserver`, `cmd/client` | Server and CLI entry points |
| `internal/server` | HTTP routes, middleware, authentication, config, SSE, and transfer handlers |
| `internal/upload`, `internal/uploadstore` | Upload state machine, resource limits, sessions, and task persistence |
| `internal/storage`, `internal/diskbudget` | File locks, disk budgeting, durable writes, and atomic publication |
| `internal/digest`, `internal/hashmanifest` | SHA-256, file fingerprints, and digest manifests |
| `internal/client` | CLI uploads, Range downloads, resume, retries, and verification |
| `web/src/features` | File browsing, upload queue, downloads, and transfer center |
| `internal/webui/assets` | Frontend assets embedded in the binary |

## Tests

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

Real browser acceptance requires Chrome or Edge; set `CHROME_PATH` to choose an executable. `OKSERVER_BROWSER_MB`, `OKSERVER_CLI_MB`, and `OKSERVER_LARGE_TEST_MB` adjust large-file test sizes. Set `OKSERVER_MEASURE_RESOURCES=1` to record sampled resource usage. Tests use isolated data and clean it up. Do not run Go build acceptance in parallel with a Vite rebuild of embedded assets or `npm ci`.

Browser automation uses real Chromium, OPFS, and write transactions to cover interrupted downloads, chunk reuse after refresh, and protection of the original target. It substitutes an OPFS handle for the operating system's save-location picker, so actual disk-location selection, long-lived permission retention, and user-denied permissions still require manual acceptance. See the [serial upload/download API design](webserver串行上传下载接口设计方案.md) for the detailed protocol design.
