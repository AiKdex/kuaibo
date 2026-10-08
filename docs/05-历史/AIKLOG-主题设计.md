# 爱库录主题设计（AiKlog Theme）

> 样板房公开面 · 2026-09-13  
> 模式：Expressive（对外产品门面），阅读优先  
> 对照：求智主题是采集工作台；爱库录是**产品博客门面**

## 设计锚点

图书馆目录卡 + 知识库印章，不是 emlog 默认皮、也不是 warm-cream 网红博客。

## 令牌

| Token | 值 | 用途 |
|---|---|---|
| --th-ink | `#1A2332` | 正文墨色（冷调夜蓝） |
| --th-ink-2 | `#3D4A5C` | 次级文字 |
| --th-ink-3 | `#6B7A8D` | 辅助/meta |
| --th-paper | `#EEF1F4` | 页面底（冷纸，非米黄） |
| --th-card | `#FFFFFF` | 卡片 |
| --th-soft | `#E4E9EE` | 分割/底纹 |
| --th-jade | `#0D7A6A` | 品牌玉色（印章/脊线/链接） |
| --th-jade-d | `#085548` | 深玉 hover |
| --th-amber | `#C47B2D` | 强调（产品名/CTA 少量） |
| --th-line | `#D0D7DE` | 细线 |

## 字体

| 角色 | 栈 | 用途 |
|---|---|---|
| display | `Georgia, 'Songti SC', 'SimSun', serif` | 站名、文章标题 |
| body | `system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif` | 正文/列表 |
| mono | `ui-monospace, 'Cascadia Code', Consolas, monospace` | 日期、版本、路径 |

## 布局

- 单栏阅读柱，max-width **720px**
- 顶栏：薄工具条（站点名 + 一句话 + GitHub）
- 列表：目录卡（左侧 3px 玉色脊线 + 索引号 + 衬线标题 + mono 日期）
- 无重阴影、无大圆角；圆角 6–10px

## 签名

**目录卡左侧玉色脊线 + 两位数索引号**（像图书馆卡片盒）

## 风险

不用卡片堆叠阴影；不用暖米黄+赤陶默认组合；标题用衬线拉开与工具 UI 的气质差。

## 数据

`dataSources: ['blog']` + `dataScope: 'shared'` — 通用公开博客，安装即用。
