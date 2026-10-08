# 爱库录精简发行方案（AI 知识库博客系统）

> 2026-09-13  
> 依据：`AiKmap-Blog-设计档案.md` v0.1 + `AiKlog设想.md`  
> 定位：**爱库录 = AiKmap 主系统的精简发行版**，不是 fork，不是另一套内核  
> 品牌：AiKlog 爱库录 · 爱库录AI知识库博客系统 · `aiklog.com`

---

## 一、产品一句话

**把知识库目录直接变成可发布的 AI 博客** —— emlog 用户能做的事都能做，多出语义搜索 / AI 摘要 / 知识图谱 / 读者问答。

前台卖点是博客；**知识库是后台引擎，不是前台功能菜单**。

---

## 二、裁剪清单（发行版必须去掉）

| 模块 | 路径 | 理由 |
|---|---|---|
| 采集执行器 | `service/collector.go` + `collector_probe.go` | 爬虫，与博客无关（归 Aikdex） |
| 采集源 | `service/source.go` + `handler/source.go` | 城市级官办源 |
| 采集工具 | `ai/tools_collect.go` + `tools_probe.go` | Agent 采集工具 |
| 采集表 | `sources` / `collect_runs` / `collect_tombstones` | 运行记录 |
| IM | `im/` + `handler/im.go` | Telegram 非博客核心 |
| 情报频道 | `ChannelView.vue` + 侧栏入口 | 采集展示 |
| TLS 指纹 | `bogdanfinn/tls-client` 及间接依赖 | 抗反爬，博客不需要 |
| ECharts | `echarts` 依赖 + 图谱 tab | ~1MB，博客不需要 |
| URL 抓取导入 | `impex` 网页抓取分支 | 保留 ZIP/Obsidian |
| batch organize/rename | `ai/tools_batch.go` 部分 | 保留 batch_tag |
| 商业化预留表 | skill/workflow/prompt/memory 等 | schema 精简 |

**预计减少：** ~3k 行 Go + ~500 行 Vue + ~4MB 二进制

---

## 三、保留清单（博客核心底座）

| 层 | 模块 |
|---|---|
| 内容 | 文件引擎、标签、集合、回收站、软删 |
| 写作 | Vditor 编辑器、阅读器、双链 `[[wiki]]`、TOC |
| 发布 | 目录即博客、`shares` 公开链、RSS、slug、站点设置、评论核 |
| 插件 | 博客插件协议（四挂载点）、主题协议、内置 5 插件 |
| AI | 网关、摘要、向量检索、语义搜索、Agent 问答（读者 AI 后置） |
| 门面 | 公开页 PublicHome/PublicView、博客管理、鉴权 |
| 底座 | SQLite、事件总线、配置、存储抽象、审计 |

---

## 四、发行版形态

| 项 | 目标 |
|---|---|
| 安装包 | 单二进制 **9–10MB** + 前端已 embed；文档 tar ~1MB |
| 服务器 | 1C1G 可跑；推荐 2C2G |
| 默认 | 站点名「爱库录」、默认主题 `aiklog`、空库引导发第一篇 |
| 不出现 | 采集频道、IM 设置、情报侧栏、tls 相关配置 |
| 升级 | 替换二进制 + schema migrate；配置在 DB |

对比：emlog ~4MB（还要 PHP+MySQL）；WordPress ~15MB。

---

## 五、实施阶段

### 阶段 0 · 精简内核（先做）

1. 建分支 `aiklog-lite`（或 build tag `//go:build aiklog`）
2. 移除采集/IM/tls/频道/echarts
3. `go build` / `npm run build` / 基础冒烟
4. 默认品牌与主题

### 阶段 1 · 博客门面补齐

- 评论 / SEO 已有 → 验收  
- 相关推荐、归档、标签云、封面（P1 插件）

### 阶段 2 · 安装与运维

- README 安装 runbook  
- systemd / 反代样例  
- 备份（含 `博客/`）

### 阶段 3 · 样板房切换

- `aiklog.com` 改跑精简二进制  
- 设置 `blog.title=爱库录`

---

## 六、与 Aikdex 边界

```
爱库录发行版 = 博客系统（无采集）
Aikdex        = 采集内容站（独立仓）
Aikdex → POST /blog/posts → 爱库录
```

---

## 七、验收清单

- [ ] 二进制无采集/IM 相关端点与表  
- [ ] 侧栏无「情报频道」  
- [ ] 公开博客可发布 / RSS / 评论 / slug  
- [ ] 爱库录主题为默认  
- [ ] 体积接近 10MB 级  
- [ ] 第三方按文档能装第二套  

---

## 八、阶段 0 进展（2026-09-13）

| 项 | 结果 |
|---|---|
| 分支 | `aiklog-lite`（本地，未 push） |
| 已裁 | 采集 collector/source/API、IM、tls-client、ChannelView、求智主题、ECharts 图谱 |
| 构建 | `go build` + `npm build` **通过** |
| 体积 | Linux 二进制 **20.1MB → 13.3MB**（约 -34%） |
| KnowledgeView | 图谱 chunk 1.1MB → 32KB（占位提示） |
| 产物 | `D:\AiKdex\build\aiklog-lite-linux-amd64` |
| **样板房** | **https://aiklog.com 已跑 aiklog-lite** |
| 站点设置 | `blog.title=爱库录`，简介/页脚已改 |
| 端点 | `/sources` `/im/*` 无公开面（401）；博客/RSS/public 正常 |

**尚未做：** schema 清理、阶段 1 插件、默认主题二次验收。

## 九、下一步

1. （可选）部署 `aiklog-lite` 到 aiklog.com  
2. 默认 `blog.title=爱库录`、主题 aiklog  
3. 阶段 1：相关推荐/归档/标签云/封面插件  
4. 合并策略：是否回主仓 main 或长期 lite 分支
