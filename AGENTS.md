# AGENTS.md

## 项目

FnMovie 是 Windows x64 飞牛影视桌面客户端。界面和应用逻辑使用 Go 与 MyGo 原生 UI，不使用 HTML、JavaScript 或 WebView；视频使用 Go 动态加载的 libmpv，在 MyGoSurface HWND 中渲染。

## 开发规范

- UI 状态保留在 Go 应用层；应用实现和测试位于 `internal/app`，飞牛接口统一放在 `internal/app/server.go`，播放器实现放在 `internal/app/player.go` / 平台文件中。根目录 `main.go` 只负责启动应用。
- 影视库媒体请求必须以 `ancestor_guid` 传服务器库 ID，并保留按库隔离；不要退回全库查询来掩盖 API 错误。
- 不在源码、测试默认值或提交中保存 NAS 密码、会话令牌和个人配置。真实 NAS 测试通过环境变量提供凭据。
- 媒体卡片列表使用 `ui.GridView` 虚拟化。派生列表只在目录数据、导航范围、搜索或筛选条件变化时重建；海报统一经过 `PosterLoader`，最多 4 个 worker、64 个待处理任务和 256 MiB LRU。不要在卡片构建函数中启动无界 goroutine，也不要恢复逐卡片进入动画。不要给 UI 刷新率硬编码 30/60 FPS 上限；滚动帧交给 MyGo/Windows 显示刷新调度。
- 纵向滚动统一复用 `advanceSmoothScroll` / `bindSmoothScroll`，每个区域独立保存位置与动画状态；普通滚轮平滑推进，精确输入和减少动态效果模式直接跟手，边界保留原生冒泡。长列表用 `ui.List` / `ui.GridView` 只构建可见行，详情头部与分集共用一个滚动容器；禁止滚动时全量重建长列表、触发进场动画或设置固定帧率。
- 全项目禁止 Tooltip、悬停说明气泡和自动弹出的帮助浮窗；图标按钮用 `Label` 提供辅助技术语义，功能菜单只在用户明确点击时展开。播放器播放/快退/快进组必须相对播放器区域严格居中，不受两侧控件宽度影响。
- 详情页准备播放只使用触发按钮内的加载动画，不再叠加加载弹窗；保留取消操作。播放覆盖层尺寸必须包含控制栏完整圆角边框与上下留白，避免窗口裁剪。
- 页面导航（目录、电影/剧集详情、演员详情和返回）使用单个稳定 Key 的内容容器，统一约 160 ms、最多 6 DIP 的轻微位移与淡入；按钮与筛选使用约 120 ms 的颜色过渡。优先使用 MyGo `ElementTransition`，遵循系统减少动态效果偏好；只在动画存续期间请求显示调度帧，不启动每帧 goroutine，不做逐卡片/逐集进场、不复制大列表做退出动画、不通过动画改变布局尺寸。异步元数据更新不得重新触发页面进场。
- 剧集演职人员先查询真实 TV GUID；根节点为空时沿服务器返回的当前 Season GUID、首集 Episode GUID 依次补查，限定请求数量并拒绝过期季度结果。空数据或错误要有可见状态，不得隐藏整个演职人员区域。剧集列表使用无边框背景的播放图标与清晰的整行点击区；演员页头像和身份信息直接融入页面，避免拉满宽度的白色介绍卡片。
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
