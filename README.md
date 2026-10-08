# 哞哩影院

**把家里的飞牛影视片库，带到 Windows 桌面。**

哞哩影院让找片和继续观看更顺手：登录自己的飞牛影视，按影视库浏览电影与剧集，查看详情，再从上次的进度接着播放。它是一款 Windows x64 原生桌面客户端，界面贴合 Windows 使用习惯，视频由 libmpv 播放。

> **准备开始？** 前往 [GitHub Releases 下载 Windows 版本](https://github.com/biaobiaobiao108/fnmovie/releases/latest)。应用需要连接到你有权访问的飞牛影视服务器。

## 从片库到播放

- **打开应用就能接着看。** 首页展示片库推荐和最近的观看记录，继续播放时从飞牛影视读取实际进度。
- **按熟悉的方式找片。** 电影、电视剧和影视库各自浏览；海报网格显示片名、年份与评分，也可以按片名搜索。
- **先了解影片，再开始播放。** 详情页展示简介、评分和演职人员。剧集可以切换季度、查看分集信息并直接播放。
- **播放时常用控件都在手边。** 调整进度、音量和倍速，切换可用的字幕与音轨，或进入全屏。观看进度会同步回飞牛影视。

## 界面截图

以下均为哞哩影院原生客户端的实际截图，展示了测试片库中的真实海报和影视数据。连接自己的账户后，应用显示该账户有权访问的内容。

| 首页：推荐与继续观看 | 电影库：按海报找片 |
|:---:|:---:|
| ![哞哩影院首页，展示推荐和继续观看区域](docs/assets/native-home.png) | ![电影库网格，展示真实海报、评分和年份](docs/assets/native-library.png) |

| 电视剧库：独立浏览剧集 | 搜索：用片名缩小范围 |
|:---:|:---:|
| ![电视剧库，展示真实剧集卡片](docs/assets/native-tv.png) | ![搜索真实片库影片后的结果](docs/assets/native-search.png) |

| 影片详情：简介与演职人员 | 剧集详情：季度与分集 |
|:---:|:---:|
| ![真实电影详情页，展示影片简介和演职人员](docs/assets/native-detail.png) | ![真实剧集详情页，展示季度和分集列表](docs/assets/native-series.png) |

## 第一次使用

1. 从 [Releases](https://github.com/biaobiaobiao108/fnmovie/releases/latest) 下载 Windows x64 安装包，或使用便携版。
2. 启动哞哩影院，输入自己的飞牛影视服务器地址、用户名和密码。
3. 选择有权限访问的影视库，浏览内容并开始播放。

请确保电脑能连接到飞牛影视服务器。首次连接所需的地址和账户由你在应用内填写；项目不提供或预置测试账户。

## 账户与数据

登录密码和会话凭据由 Windows 凭据管理器保护。服务器地址与用户名保存在本机用户配置目录中。媒体信息按所选影视库读取；应用不会在项目文件中保存你的 NAS 密码或会话令牌。

## 适用平台

- Windows x64
- 可访问的飞牛影视服务与个人账户

客户端界面由 Go 与 MyGo 原生绘制，视频播放通过动态加载的 libmpv 完成，不使用浏览器页面或 WebView。

## 开发

需要 Go 1.27.1 或更新版本。克隆仓库后先安装 Git LFS 并运行 `git lfs pull`，再执行：

```powershell
go test ./...
go vet ./...
pwsh -File scripts/build-windows.ps1
```

Windows x64 构建输出到 `out\fnmovie-20261007\windows-amd64`。

### 发布版本

在 PowerShell 中运行一次 `pwsh -File scripts/install-command.ps1`，然后在仓库目录输入 `fnmovie patch`、`fnmovie minor` 或 `fnmovie major`。命令会更新 `mygo.json` 版本、提交并创建对应的 `vX.Y.Z` tag，再依次推送当前分支和 tag；tag 会触发 GitHub Actions 构建并发布 Windows amd64 安装包。发布要求工作区干净且当前分支配置了远程 upstream；当前分支上已提交但尚未推送的提交也会一起推送。可用 `fnmovie patch -WhatIf` 预览。
