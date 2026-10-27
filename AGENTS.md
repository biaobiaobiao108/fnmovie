# AGENTS.md

## 项目

哞哩影院是 Windows x64 飞牛影视桌面客户端。界面与应用逻辑使用 Go 和 MyGo 原生 UI，不使用 HTML、JavaScript 或 WebView；视频由 Go 动态加载 libmpv 并渲染到 MyGoSurface。用户可见品牌统一为“哞哩影院”；Go 模块、应用 identifier、FnMovie 配置/缓存/凭据目录、数据库与协议标识保持兼容，不随品牌更名迁移。

## 代码地图

| 位置 | 职责 |
| --- | --- |
| `main.go`、`mygo.json` | 应用入口与构建配置 |
| `internal/app/app.go`、`app_state.go`、`app_session.go` | 应用生命周期、状态及登录会话 |
| `internal/app/app_navigation_view.go`、`app_secondary_view.go`、`media_view.go` | 导航、次级页面和媒体列表界面 |
| `internal/app/home*.go`、`app_detail*.go`、`detail_loading.go` | 首页、媒体详情及其数据加载 |
| `internal/app/server.go`、`app_catalog.go`、`app_favorites.go` | 飞牛 API、影视目录与收藏 |
| `internal/app/*cache*.go`、`app_images.go` | 目录、人物和海报缓存及图片加载 |
| `internal/app/app_playback.go`、`player*.go`、`playback*.go` | 播放流程、libmpv、控件和播放进度 |
| `internal/app/config.go`、`credentials_*.go` | 本地设置与平台凭据 |
| `internal/app/theme.go`、`motion.go`、`scroll_binding.go` | 界面样式、动画和滚动绑定 |
| `internal/mygooverlay/` | 播放器原生覆盖层适配 |
| `internal/app/*_test.go` | 对应功能的单元测试和 Windows 集成测试 |
| `resources/`、`scripts/`、`.github/workflows/` | 应用资源、构建发布脚本和 CI 工作流 |

## 通用开发约束

- UI 状态保留在 Go 应用层；应用实现和测试位于 `internal/app`，飞牛接口在 `internal/app/server.go`，播放器在 `internal/app/player.go` 及平台文件，根目录 `main.go` 只负责启动。
- MyGo v0.3 的 `ui.Element` 只在当前构建中有效，不保存到应用状态或后台任务；跨构建焦点和尺寸使用 `ui.Handle`。有状态控件构造前调用 `c.Key(...)`，输入绑定持久模型字段；优先通过 `OnClick` / `OnChange` 执行动作。异步回调使用 `Services`，通过 `Window.Update` 发布模型结果。升级 MyGo 后运行 `go tool mygo vet .`。
- 全局搜索只传关键词，不沿用当前库、类型、收藏或筛选条件；搜索缓存和结果范围统一，清空关键词恢复原页面范围。普通影视库查询必须携带服务器库 ID 对应的 `ancestor_guid` 并保持按库隔离，不得用全库查询掩盖 API 错误。
- 页面离开、账号切换或取消操作时取消相关网络请求，并拒绝过期响应。不要在源码、测试默认值或提交中保存 NAS 密码、会话令牌和个人配置；真实 NAS 测试凭据通过环境变量提供。
- 页面、首页、媒体数据/缓存和播放器的具体行为约束按需查阅下列专题，不要在实现中自行改写这些产品约定。

## 专题约束

| 任务 | 阅读 |
| --- | --- |
| 页面布局、列表、滚动、导航和动画 | [docs/agent-guides/ui.md](docs/agent-guides/ui.md) |
| 首页轮播、推荐抽样和继续观看 | [docs/agent-guides/home.md](docs/agent-guides/home.md) |
| 媒体层级、飞牛接口、异步请求和缓存 | [docs/agent-guides/media-data-and-cache.md](docs/agent-guides/media-data-and-cache.md) |
| libmpv、播放控件、进度跳转和全屏 | [docs/agent-guides/player.md](docs/agent-guides/player.md) |

## 验证与交付

- 修改 Go 代码后运行 `gofmt`、`go test ./...` 和 `go vet ./...`；升级 MyGo 后运行 `go tool mygo vet .`。
- 完成代码修改后创建 Git 提交；提交后按项目约定构建 Windows x64 包。固定输出到 `out\\fnmovie-20261007\\windows-amd64`，使用 `pwsh -File scripts/build-windows.ps1`（脚本执行 `go tool mygo build -platform windows/amd64 -o out\\fnmovie-20261007`）。用户要求发布时再推送远程。
- NSIS 中文名称通过构建脚本临时设置编译器编码；不要修改全局工具或应用数据路径。
- `out/` 和 `build/` 不提交；`libmpv-2.dll` 由 Git LFS 管理。克隆后安装 Git LFS 并运行 `git lfs pull`。
