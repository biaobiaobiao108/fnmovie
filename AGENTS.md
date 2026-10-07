# AGENTS.md

## 项目

FnMovie 是 Windows x64 飞牛影视桌面客户端。界面和应用逻辑使用 Go 与 MyGo 原生 UI，不使用 HTML、JavaScript 或 WebView；视频使用 Go 动态加载的 libmpv，在 MyGoSurface HWND 中渲染。

## 开发规范

- UI 状态保留在 Go 应用层；应用实现和测试位于 `internal/app`，飞牛接口统一放在 `internal/app/server.go`，播放器实现放在 `internal/app/player.go` / 平台文件中。根目录 `main.go` 只负责启动应用。
- 影视库媒体请求必须以 `ancestor_guid` 传服务器库 ID，并保留按库隔离；不要退回全库查询来掩盖 API 错误。
- 不在源码、测试默认值或提交中保存 NAS 密码、会话令牌和个人配置。真实 NAS 测试通过环境变量提供凭据。
- 媒体卡片列表使用 `ui.GridView` 虚拟化。派生列表只在目录数据、导航范围、搜索或筛选条件变化时重建；海报统一经过 `PosterLoader`，最多 4 个 worker、64 个待处理任务和 256 MiB LRU。不要在卡片构建函数中启动无界 goroutine，也不要恢复逐卡片进入动画。不要给 UI 刷新率硬编码 30/60 FPS 上限；滚动帧交给 MyGo/Windows 显示刷新调度。
- 飞牛媒体列表会混合返回 `Movie`、`TV`、`Season`、`Episode` 节点；一级卡片只显示电影与剧集根节点，季度通过 `GET /season/list/{TV.guid}` 加载，集通过 `GET /episode/list/{Season.guid}` 加载。演员通过 `POST /person/list/{item.guid}` 读取。媒体层级变化时先核对服务端真实结构，不按标题文本猜测分组。
- 海报原图按服务器图片 URL 哈希后存于用户缓存目录，最多 512 MiB、30 天过期；UI 位图仍受内存 LRU 上限控制。目录和详情元数据缓存在用户配置目录，不存登录令牌或密码。
- Windows 使用 `resources/windows-amd64/player/libmpv-2.dll`，不得启动或重新加入 `mpv.exe`。libmpv 绑定通过 Windows DLL 导出和 purego 调用；在创建实例时固定 `config=no`、应用配置目录、`wid=MyGoSurface`、`vo=gpu-next`、`gpu-context=d3d11`、`hwdec=auto-safe`、`ao=wasapi`、`audio-channels=auto-safe`。不要启用压缩音频直通，不强制音轨语言，保留 libmpv 默认网络缓存。控件用 MyGo 原生 UI 附属透明窗，必须设置 `Parent` 和 `SkipTaskbar`，在主窗口客户区顶部放标题/返回、底部放控制，禁止全屏控件面板和紫色色键残留；显示淡入、2.5 秒闲置淡出，退出/最小化/缩放/全屏时同步覆盖层及 libmpv 生命周期。
- 修改 Go 后运行 `gofmt`、`go test ./...` 和 `go vet ./...`；Windows 发布前再执行固定目录构建。每次代码改动完成后创建一次 Git 提交。

## Git 与交付

- 远程公开仓库：`https://github.com/biaobiaobiao108/fnmovie`。修改完成后提交到当前分支；用户要求发布时推送远程。
- 所有 Windows x64 包固定输出到本次约定目录 `out\fnmovie-20261007\windows-amd64`，不要另建带时间戳的目录。
- `out/` 与 `build/` 不提交。`libmpv-2.dll` 由 Git LFS 管理；克隆仓库后安装 Git LFS 并运行 `git lfs pull`。

## Windows 构建

- 从仓库根目录构建 MyGo Windows x64 包：

  ```powershell
  go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
  ```
