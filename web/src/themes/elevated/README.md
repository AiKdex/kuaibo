# AiKlog Theme: Elevated（高端暗色）

暗色高端博客主题，玉色点缀，衬线正文，AI 侧栏。

## 特性

- 🌙 暗色调设计，高端大气
- 🟢 玉色（#2dd4a8）主题色
- 📖 衬线体正文（Noto Serif SC），阅读感高级
- 🤖 AI 问答侧栏
- 📝 AI 摘要卡片
- 🏷️ 标签云 + 归档
- 📎 相关推荐
- 💬 评论系统
- 🤖 读者 AI 悬浮按钮
- 📱 完整响应式

## 安装

将整个 `elevated` 目录复制到 AiKlog 的主题目录：

```bash
cp -r elevated /path/to/aiklog/web/src/themes/elevated
```

然后在主题注册文件（`web/src/themes/index.js`）中添加注册：

```js
import registerElevated from './elevated/index.js'

// 在 registerThemes 函数中：
registerElevated(app, { registerTheme })
```

重新构建前端即可在后台「站点设置」中切换到 Elevated 主题。

## 文件结构

```
elevated/
├── manifest.js        # 主题声明（名称、特性、挂载点）
├── index.js           # 注册入口
├── ElevatedView.vue   # 主组件（列表 + 文章视图）
├── style.css          # 样式（暗色 + 玉色 + 衬线体）
└── README.md          # 说明文档
```

## 色彩系统

| 用途 | 色值 |
|------|------|
| 主色（玉色） | `#2dd4a8` |
| 背景 | `#0a0f1a` |
| 卡片 | `#1a2236` |
| 文字 | `#e2e8f0` |
| 次要文字 | `#94a3b8` |
| 边框 | `rgba(255,255,255,0.06)` |

## 依赖

- Vue 3
- AiKlog 主题系统
- Google Fonts: Inter, Noto Sans SC, Noto Serif SC（可选，系统字体兜底）
