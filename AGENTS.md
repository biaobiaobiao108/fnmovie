# AGENTS.md

## 项目

FnMovie 是 Windows x64 飞牛影视桌面客户端。界面和应用逻辑使用 Go 与 MyGo 原生 UI，不使用 HTML、JavaScript 或 WebView；视频由随应用分发的 mpv 播放。

## 开发规范

- UI 状态保留在 Go 应用层；飞牛接口统一放在 `server.go`，播放器实现放在 `player.go` / 平台文件中。
- 影视库媒体请求必须以 `ancestor_guid` 传服务器库 ID，并保留按库隔离；不要退回全库查询来掩盖 API 错误。
- 不在源码、测试默认值或提交中保存 NAS 密码、会话令牌和个人配置。真实 NAS 测试通过环境变量提供凭据。
- Windows 播放器嵌入 `MyGoSurface`。使用打包的 `resources/windows-amd64/player/scripts/osc.lua`；鼠标移动到视频区域会唤醒控件，空闲 2.5 秒后隐藏。避免另建覆盖视频的顶层控制窗口。
- 修改 Go 后先运行 `gofmt`、`go test ./...` 和 `go vet ./...`；检查通过后，每次代码改动都创建一次 Git 提交。

## Windows 构建

- 所有 Windows x64 产物固定输出到 `out\fnmovie-20261007\windows-amd64`：

  ```powershell
  go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
  ```

- `out/` 与 `build/` 不提交。mpv 可执行文件由 Git LFS 管理；克隆仓库后安装 Git LFS 并运行 `git lfs pull`。
