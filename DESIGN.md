# DESIGN.md

## Direction

- Concept: 私人胶片放映厅 (Private Cinema Archive) —— 现代高阶桌面影音工作台的精密秩序与经典电影放映厅的温润光影沉浸感。
- Target qualities:
  1. 暗曜沉浸 (Deep Obsidian): 温润黑曜石与低漫反射深石板色，彻底衬托海报与剧照的高饱和色彩。
  2. 胶片琥珀 (Filmic Amber): 提取经典胶卷放映机光芒，以琥珀金作为评分与高光标识，搭配电影灰蓝作为交互基调。
  3. 精密几何 (Precision Geometry): 严格统一的 4px/8px 比例阶梯，海报严格 2:3 比例，文字基线与图标严密左对齐。
- Anti-qualities:
  1. 拒绝办公浅色面板 (No Office Tool Aesthetic): 告别浅灰记事本配色与昼夜割裂的穿梭体验。
  2. 拒绝胶囊标签与居中堆砌 (No Pill Slop & Centered Text): 严禁无意义分类药丸，卡片信息严谨左对齐。
  3. 拒绝弹窗打断 (No Dialog Interruption): 简介与状态自适应展示，保持选片心流不被弹窗割裂。

## Color System

```
Canvas / Background:    #0d0f12 (极深黑曜底色)
Sidebar Background:     #13161b (侧边栏深沉表面)
Surface:                #1a1e24 (卡片、列表项、悬浮容器表面)
Surface Hover:          rgba(255, 255, 255, 0.06)
Surface Pressed:        rgba(255, 255, 255, 0.10)
Border Subtle:          rgba(255, 255, 255, 0.08)
Border Strong:          rgba(255, 255, 255, 0.16)
Text Primary:           #f0f2f5 (主要标题、影片名称)
Text Secondary:         #a2a8b4 (副标题、年份、季度集数)
Text Muted:             rgba(240, 242, 245, 0.60) (辅助说明、空状态)
Accent Primary:         #5e85a8 (电影灰蓝，主操作与激活态)
Accent Hover:           #6f97bc
Accent Pressed:         #4f7394
Accent Selected:        rgba(94, 133, 168, 0.18)
Rating Amber:           #e5a93c (胶片琥珀金，有效评分与星标)
Rating Background:      rgba(229, 169, 60, 0.16)
Favorite Rose:          #e0526b (收藏高亮)
Danger:                 #e05656 (报错、异常提示)
Placeholder:            #1a1e24
Placeholder Text:       #4e5666
Immersive Detail Tint:  #0b0c10 (详情页底层融合墨色)
```

## Typography

- Font family: Segoe UI Variable, Segoe UI, "Microsoft YaHei", sans-serif. 数字与时间使用等宽特性 (tnum)。
- Display / Hero Title: 32px / 40px, Bold (大轮播与详情页标题)
- Section Heading:      20px / 28px, Bold (栏目标题：继续观看、影视库名称)
- Card Title:           14px / 20px, Bold, Left-Aligned (海报下方标题)
- Body / Metadata:      12px / 18px, Regular, Left-Aligned (年份、分类、集数)
- Micro / Badge:        11px / 14px, Medium, tnum (播放时间轴、角标)

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

- Navigation transition: 160ms, Y 位移 6 DIP + 淡入。
- Button transition: 120ms 颜色过渡与按压微压缩。
- Poster 3D tilt: 360ms 阻尼弹簧，悬停视差倾斜角度不超过 5 度。
- Player controls fade: 180ms 淡入，闲置 2500ms 淡出。
- Respect ReduceMotion: 开启时禁用 3D 倾斜与位移，保留瞬时透明度过渡。
