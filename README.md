# FnMovie 飞牛影视桌面客户端

基于 Go 与 MyGo 原生 UI 构建的 Windows x64 飞牛影视客户端。界面由 MyGo 绘制，不使用 HTML、JavaScript 或 WebView；视频播放通过 Go 直接调用 libmpv DLL 完成，不启动 `mpv.exe`。

## 当前功能

- 首次使用或登录状态失效时通过原生弹窗登录；默认地址为 `http://192.168.31.86:5666/v`，可修改部署地址和前缀。
- 左侧按当前账户权限展示服务器返回的影视库，选择影视库后仅加载该库媒体。
- 按选中影视库浏览影片网格、详情和海报；支持关键词搜索、收藏与观看记录。
- 通过飞牛播放接口获取播放信息和媒体流，使用本机随机地址代理 Range 请求，避免把凭据写进播放 URL 或 mpv 命令行。
- 播放页由 MyGo 原生控件覆盖 `MyGoSurface` 中的 libmpv 视频画面，顶部显示影片信息和返回按钮，底部控件渐入渐出，支持进度、音量、倍速、字幕/音轨切换和全屏；同步播放进度并在退出时清理 libmpv 与本地代理。
- Windows 凭据管理器保存密码和会话令牌；服务器地址与用户名保存在 `%APPDATA%\FnMovie\settings.json`。

## 开发与构建

要求 Go 1.27.1 或更新版本。应用依赖通过 Go modules 获取，MyGo CLI 作为 Go 工具依赖提供。

```powershell
go test ./...
go vet ./...
go tool mygo dev
go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
```

Windows x64 的发布产物固定在 `out\fnmovie-20261007\windows-amd64`，包括可直接启动的 `FnMovie.exe` 和安装包 `FnMovie Setup 0.1.0.exe`。播放器文件位于 `resources\windows-amd64\player`，其中 `libmpv-2.dll` 由 Git LFS 管理；克隆后需安装 Git LFS 并运行 `git lfs pull`。第三方许可文本位于 `resources`。

## 播放与服务器协议

适配层按飞牛影视当前实际接口实现签名登录、影视库列表、媒体分页、详情、收藏、播放信息、流协商和播放记录。库列表来自 `/mdb/list`，媒体 `/item/list` 必须通过 `ancestor_guid` 指定影视库。服务器提供同源直放质量链接时优先使用；否则使用服务器的 `/media/range/{media_guid}` 流，由本地代理转发授权与 Range 请求。

播放位置通过 libmpv 客户端 API 读取，并按飞牛接口返回的续播位置启动；周期性和退出时都会提交进度。libmpv 使用应用配置目录和 `config=no`，视频输出为 D3D11 `gpu-next`、硬解优先并回退软件解码，音频解码后通过 WASAPI 默认设备输出。字幕与音轨选项以当前媒体流实际可用能力为准。

使用真实 NAS 验证播放：

```powershell
$env:FNMOVIE_LIVE_TEST='1'
$env:FNMOVIE_SERVER='http://your-nas:5666/v'
$env:FNMOVIE_USER='your-user'
$env:FNMOVIE_PASSWORD='your-password'
$env:FNMOVIE_PARENT_HWND=[string](Get-Process FnMovie).MainWindowHandle
go test -run TestLiveNASPlayback -v -count=1
```

真实 NAS 测试会短暂更新当前媒体的观看进度并恢复原进度。测试账户只能看到服务器授予该账户的影视库。

首次使用时在连接页输入飞牛账户。应用不会把测试账户密码写入仓库。
