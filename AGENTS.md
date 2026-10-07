# AGENTS.md

## 项目

FnMovie 是 Windows x64 飞牛影视桌面客户端。界面和应用逻辑使用 Go 与 MyGo 原生 UI，不使用 HTML、JavaScript 或 WebView；视频由随应用分发的 mpv 播放。

## 开发规范

- UI 状态保留在 Go 应用层；飞牛接口统一放在 `server.go`，播放器实现放在 `player.go` / 平台文件中。
- 影视库媒体请求必须以 `ancestor_guid` 传服务器库 ID，并保留按库隔离；不要退回全库查询来掩盖 API 错误。
- 不在源码、测试默认值或提交中保存 NAS 密码、会话令牌和个人配置。真实 NAS 测试通过环境变量提供凭据。
- 媒体卡片列表使用 `ui.GridView` 虚拟化。派生列表只在目录数据、导航范围、搜索或筛选条件变化时重建；海报统一经过 `PosterLoader`，最多 4 个 worker、64 个待处理任务和 256 MiB LRU。不要在卡片构建函数中启动无界 goroutine，也不要恢复逐卡片进入动画。
- Windows 将 mpv 子窗口嵌入 `MyGoSurface`；播放控件由 MyGo 原生 UI 绘制在透明 `mygo.Window` 上，必须设置 `Parent` 和 `SkipTaskbar`，保持主窗口内附属层级，不得启动独立播放器/任务栏窗口。播放控件首次出现和指针移动时显示，闲置 2.5 秒后隐藏；隐藏时把焦点交还主窗口，关闭菜单或拖动进度时不隐藏。退出、最小化、恢复、缩放和全屏都要同步覆盖层并清理 mpv/IPC。
- mpv 使用应用配置目录和 `--no-config`，固定参数 `--vo=gpu-next --gpu-context=d3d11 --hwdec=auto-safe --ao=wasapi --audio-channels=auto-safe`；不要启用压缩音频直通，不强制音轨语言，保留 mpv 默认网络缓存。mpv OSC/Lua 不属于应用控件，也不得成为启动依赖；播放控件经命名管道 IPC 控制进度、音量、静音、倍速及音轨/字幕。
- 修改 Go 后运行 `gofmt`、`go test ./...` 和 `go vet ./...`；Windows 发布前再执行固定目录构建。每次代码改动完成后创建一次 Git 提交。

## Git 与交付

- 远程公开仓库：`https://github.com/biaobiaobiao108/fnmovie`。修改完成后提交到当前分支；用户要求发布时推送远程。
- 所有 Windows x64 包固定输出到本次约定目录 `out\fnmovie-20261007\windows-amd64`，不要另建带时间戳的目录。
- `out/` 与 `build/` 不提交。mpv 可执行文件由 Git LFS 管理；克隆仓库后安装 Git LFS 并运行 `git lfs pull`。

## Windows 构建

- 从仓库根目录构建 MyGo Windows x64 包：

  ```powershell
  go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
  ```
