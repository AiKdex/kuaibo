# AiKlog致上游 —— REQ-010 家族传承记录模块：锚点不可达，请给源码包与设计稿

- 提出方：AiKlog（AiKdex/AiKlog fork，基于上游 v0.1.29）
- 日期：2026-09-30
- 关联：REQ-010（已 closed，上游答复已收悉，感谢！）
- 性质：**缺件单**（先例：BUG-001 同类——锚点不在可追踪分支时请求源码包）

## 1. 已收悉并认可上游答复

- 家族传承记录模块已实现并部署（CoreModules.Family 独立内核模块，一期 `73a5017` / 二期 AI 参谋 `fd1460b` / IM 语音归档 `cf779ec`）。
- 付费分层口径（基础 M0 免费、增强 tier=pro、scope=feature:family）与 REQ-008 一致，照单全收。
- 时间轴复用 md 文件 `content_state` 打标（node_type=life_event）不建表，设计认可。

## 2. 缺件内容（阻塞移植）

上游答复中的三个锚点 commit 在 AiKlog 仓库的任何可追踪分支（main / sync/ag-base / tags 至 v0.1.29）中均不可达（`cat-file` 全部 Not a valid object name，Org 锚点 `7bea3b9` 同）：

- `73a5017`（一期：三表 + 14 端点 + 四 Tab 前端 + 提醒 ticker）
- `fd1460b`（AI 人生参谋）
- `cf779ec`（IM 语音归档）

**请上游任选其一**：

1. **推送锚点到可追踪分支**：将 `73a5017`、`fd1460b`、`cf779ec`（以及 Org 锚点 `7bea3b9`）推到 AiKlog 仓库 `sync/ag-base` 或独立分支，我方按 SHA 移植（禁 merge，逐文件映射）；
2. **投递源码包**：按既有源码包模式（先例：R1 blog_plugins.go / R3 采集源包规范），提供——
   - 后端：`family` 模块全部 Go 文件（repo/migration/handler/service + CoreModules 装配段）；
   - 前端：四 Tab 视图组件与路由/入口接线段；
   - 文档：`docs/家族传承记录模块-需求设计与实施-v1.md`（含 v2 实施记录）；
   - DDL：三表建表与索引（含 `family.enabled` 白名单开关的注册段）。

## 3. 移植计划（收到缺件后）

- 按 REQ-008/009 既有模式：最小 diff + `[family]` 标 + 协作区预告 → 单测（三表 CRUD/门禁 402/时间轴打标）→ 124 构建 → 两实例部署复验。
- 付费门禁直接复用本 fork 已上线的站点级 `orgPaidGate` 模型（feature:family scope）。
- 若上游建议「先同步主干至最新再评估」，请一并说明主干同步的权威方式（当前 sync/ag-base 不可达，且我方禁 merge 上游 main）。
