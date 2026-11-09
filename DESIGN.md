# DESIGN.md

## Direction

- Concept: 现代影视画廊与暗曜放映厅 (Daylight Gallery & Obsidian Cinema) —— 一级页面以典雅明快的艺术画廊为底色，选片与浏览舒展自然；二级页面进入深度暗调放映厅，烘托剧照原色与光影沉浸。
- Target qualities:
  1. 画廊秩序 (Gallery Structure): 一级页面采用温润纯净的浅色体系，侧边栏、背景与卡片具有明确的微明度阶差，空间轻盈且秩序严谨。
  2. 暗曜沉浸 (Obsidian Cinema): 进入电影/剧集/演员详情页与播放器，无缝切换为深黑曜石暗调，以模糊海报色调与 OkLab 光晕呈现影视艺术。
  3. 精密几何 (Precision Geometry): 4px/8px 网格基数，海报严格 2:3 纵横比，卡片文本一律左对齐，基线与网格严密对齐。
- Anti-qualities:
  1. 拒绝单调扁平灰白 (No Flat Gray Monotony): 侧边栏与主区具有清晰的 3% 层次级差，卡片采用纯白微浮，拒绝浑浊无层次。
  2. 拒绝胶囊标签与居中堆砌 (No Pill Slop & Centered Text): 严禁无意义分类药丸，卡片标题左对齐，去除 0.0 评分噪音。
  3. 拒绝弹窗打断 (No Dialog Interruption): 简介阅读与状态原地平滑适应，保持选片心流不被弹窗割裂。

## Color System

### 一级页面：现代影视画廊 (Primary / Daylight Gallery)
```
Canvas / Background:    #f7f7f6 (温润画廊底色，低眩光)
Sidebar Background:     #f0f1f3 (侧边栏微沉表面，3% 清晰明度级差)
Surface:                #ffffff (纯白卡片与输入容器浮起表面)
Surface Hover:          rgba(0, 0, 0, 0.04)
Surface Pressed:        rgba(0, 0, 0, 0.08)
Border Subtle:          rgba(0, 0, 0, 0.08) (精密发丝分割线)
Border Strong:          rgba(0, 0, 0, 0.16)
Text Primary:           #191b1f (深石墨文字，对比度 14:1)
Text Secondary:         rgba(25, 27, 31, 0.62) (副标题与元数据，对比度 5.5:1)
Text Muted:             rgba(25, 27, 31, 0.45)
Accent Primary:         #44627a (沉静青灰蓝，导航激活与主要操作)
Accent Hover:           #375267
Accent Pressed:         #2d4456
Accent Selected:        rgba(68, 98, 122, 0.12)
Rating Badge Bg:        rgba(13, 15, 18, 0.78) (海报暗底胶片角标)
Rating Amber:           #e5a93c (胶片琥珀金，有效评分与星标)
Favorite Rose:          #d94f68 (收藏高亮)
Danger:                 #c0392b (异常与错误提示)
Placeholder:            #ececee
Placeholder Text:       #8e929a
```

### 二级页面：暗曜放映厅 (Secondary / Obsidian Cinema)
```
Canvas / Background:    #0b0c10 (深邃黑曜底色)
Backdrop Blend:         海报原图模糊 (Blur 34, Opacity 0.34) 叠加 OkLab 动态渐变过渡
Surface:                rgba(255, 255, 255, 0.09) (半透明暗调表面)
Surface Hover:          rgba(255, 255, 255, 0.14)
Surface Pressed:        rgba(255, 255, 255, 0.19)
Border:                 rgba(255, 255, 255, 0.13)
Text Primary:           #ffffff (纯净高光文字)
Text Muted:             rgba(255, 255, 255, 0.68)
Accent Dynamic:         从当前海报色调动态提取并与白色以 28% 混合
Danger:                 #ff8f86
```

## Typography

- Font family: Segoe UI Variable, Segoe UI, "Microsoft YaHei", sans-serif. 数字与时间使用等宽特性 (tnum)。
- Display / Hero Title: 32px / 40px, Bold (大轮播与详情页标题)
- Section Heading:      20px / 28px, Bold (栏目标题：继续观看、影视库名称)
- Card Title:           14px / 20px, Bold, Left-Aligned (海报下方标题)
- Body / Metadata:      12px / 18px, Regular, Left-Aligned (年份、分类、集数)
- Micro / Badge:        11px / 14px, Medium, tnum (播放时间轴、评分角标)

## Spacing & Geometry

- Grid base: 4px / 8px
- Spacing:
  - xs: 4px
  - sm: 8px
  - md: 12px
  - lg: 16px
  - xl: 24px
  - 2xl: 32px
- Radii:
  - sm: 6px (徽章、次级按钮、滑块轨道)
  - md: 8px (标准操作按钮、输入框、导航项)
  - lg: 10px (海报卡片 Poster Surface)
  - xl: 16px (大轮播 Hero、弹窗浮层容器)
- Layout sizes:
  - Sidebar width: 232 DIP
  - Title bar height: 46 DIP
  - Poster card: 150 × 225 DIP (2:3 工业标准比例)

## Motion & Interaction

- Page Tier Transition: 160ms，Y 位移 6 DIP + 淡入。二级页面进入时同步切换 Win32 标题栏按钮暗黑模式。
- Button Transition: 120ms 颜色过渡与按压微压缩。
- Poster 3D Tilt: 360ms 阻尼弹簧，悬停视差倾斜角度不超过 5 度。
- Player Controls Fade: 180ms 淡入，闲置 2500ms 淡出。
- Respect ReduceMotion: 开启时禁用 3D 倾斜与位移，保留瞬时透明度过渡。
