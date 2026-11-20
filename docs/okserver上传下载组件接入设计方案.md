# okserver 上传下载组件接入设计方案

## 目标与范围

将 okserver 的 HTTP 上传、下载和任务管理能力封装成可被外部 Go 模块导入的组件。接入方负责路由挂载、登录鉴权和页面开发，可参考仓库 Web UI 的续传与校验实现。首版只提供 API，现有 Web UI 继续由独立版提供。

## 公开接口

在同一 Go 模块新增 `pkg/fileserver`。公开包只暴露必要的配置和生命周期接口，复用 `internal/server` 与上传存储逻辑。独立版和嵌入版应共用服务构建逻辑。

```go
type Config struct {
	RootDir  string // 对外文件目录
	StateDir string // 会话元数据、锁和摘要清单目录
	CacheDir string // 分块和流式暂存目录
}

func New(Config) (*Server, error)
func (s *Server) ServeHTTP(http.ResponseWriter, *http.Request)
func (s *Server) Close(context.Context) error
```

以上签名为设计草案。构造函数校验路径并初始化目录及后台任务，失败时释放已创建的资源。组件不自行监听端口或读取 YAML。宿主先停止接收请求并等待正在处理的请求，再调用 `Close` 停止后台校验与清理任务。关闭操作应支持重复调用和超时。

## 接入与目录隔离

```go
transfers, err := fileserver.New(fileserver.Config{
	RootDir:  "/srv/project-a/files",
	StateDir: "/var/lib/myapp/project-a/states",
	CacheDir: "/var/cache/myapp/project-a/uploads",
})
if err != nil {
    return err
}
mux.Handle("/workspace/", http.StripPrefix("/workspace", transfers))
```

- 磁盘上层目录由宿主配置。请求中的文件名始终相对于 `RootDir`。例如 `/workspace/download/reports/a.pdf` 对应 `RootDir/reports/a.pdf`。
- 嵌入模式要求显式配置三个互不重叠的目录，建议使用绝对路径。状态和缓存目录应位于文件根目录之外，每个实例独立使用。目录校验须考虑符号链接以及根目录边界。
- 首版一个实例对应一个存储根目录。多个项目分别创建实例并挂载到不同前缀，宿主根据调用方权限决定可访问的实例。
- 宿主可以在挂载层实现鉴权。若同一实例服务多个用户，需要定义从请求上下文传递稳定用户身份的方式，并由组件用于会话所有者校验。上传令牌用于续传会话校验。
- 独立版使用 `server.files_dir`、`server.states_dir` 和 `server.cache_dir` 分离公开文件、会话状态与上传暂存。保留已有成品时，将 `server.files_dir` 指向原文件目录；旧会话状态不迁移。

## HTTP 接入约定

下面的路径相对于挂载前缀。详细协议见 [串行上传下载接口设计方案](./webserver串行上传下载接口设计方案.md)。

| 操作 | 接入方式 |
| --- | --- |
| 查询续传 | `HEAD /upload/<path>`，携带原 `Upload-Token` 并读取 `Upload-Offset`、`Upload-Length` 和 `Upload-State`。收到 404 时可尝试从零创建，409 时提示处理已有文件或会话。 |
| 上传 | 小文件可用 POST。分片续传用 PUT，并设置 `Content-Range` 和 `X-Chunk-SHA256`。最终分片带完整文件摘要。收到 202 后查询状态，直到确认发布完成。 |
| 下载 | 先通过 HEAD 获取版本、大小和摘要。分段 GET 使用 `Range` 和 `If-Range`，检查响应状态、范围和文件版本，合并后校验完整 SHA-256。 |
| 同名文件 | 默认拒绝覆盖。显式替换使用 `overwrite=true` 和当前强 ETag 的 `If-Match`。成品已删除时，旧完成记录不应阻止重新上传。 |

自写前端应从宿主配置获取 API 前缀，并按服务地址、挂载前缀、登录身份和目标路径保存续传令牌。同一地址下的多个实例可独立处理同名文件。

## 实施与验收

1. 抽离独立版的服务初始化流程，增加 API 专用路由和可关闭的服务实例。
2. 发布公共包和独立 Go 模块示例，说明鉴权中间件、挂载、路径配置和关闭顺序。
3. 用外部 Go 模块在同一个宿主服务中挂载两个实例。分别上传同名文件，暂停后续传，再下载并比对摘要。验证会话和文件彼此隔离。
4. 验证非根路径挂载、根目录边界、已有文件冲突、删除成品后的同路径重传，以及服务关闭时后台任务退出。运行独立版回归测试确认协议行为一致。
