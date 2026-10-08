# AiKlog 需求清单 v5.0 —— 个人及家族记录能力（REQ-010，致上游）

- 提出方：AiKlog（AiKdex/AiKlog fork，基于上游 v0.1.29）
- 日期：2026-09-30
- 工单：REQ-010（status=open）
- 关联：REQ-008（Org 付费分层口径，已 closed）/ REQ-009（Org 同步协调，已 closed）

## 1. 背景与诉求

AiKlog 的目标用户中存在明显的「家庭/家族」场景：家庭相册、老人健康记录、家族大事记、纪念日/周年提醒、族谱式成员关系等。该类需求与现有知识库（文件+标签+语义检索）天然契合，但缺少「成员关系」与「记录时间线」两个关键抽象。

我们在上游官网六大板块（Drive / AI 知识库 / 博客 / 协作 / IM / 应用中心）、应用市场 index.json 及已发布代码（至 v0.1.29）中均未见对应能力，故提交本需求征询上游规划。

## 2. 期望的能力范围（建议最小集，供上游参考）

- **个人记录**：私有日记/记录条目（文本+图片），按时间线组织；隐私默认仅本人可见。
- **家族记录**：
  - 家族成员档案（成员卡片、关系边：夫妻/父母子女等，支持族谱式树/图展示）；
  - 家族共享空间（成员按角色协作，可复用 spaces 模型）；
  - 大事记/纪念日（日期驱动的条目 + 周年提醒）；
  - 老照片/影像归档（复用 files + 标签 + 语义索引）。
- **录入通道**：复用 IM 工具链（Telegram/企微发文字/图片/语音直接成条目），降低长辈使用门槛。

## 3. 我方观察到的可复用底座（与上游对齐）

- Org 组织树（REQ-008/009 已对齐口径）：`kind` 语义扩展即可承载家族成员树；
- spaces 多空间模型（fork 侧 schema v8 已含 `idx_space_members_space`）：家族共享空间；
- files + tags + 语义索引：记录存储与检索；
- IM Agent 工具（上游已有对话上传/发送/转写工具）：录入通道；
- 付费分层可参照 REQ-008 口径（M0 只读免费 / M1 协作付费）。

## 4. 请上游答复的问题

1. 个人及家族记录是否已在上游路线图？若有，期望的发布节奏与形态（独立模块 / 应用中心工具 / 博客插件）？
2. 是否已有数据模型/接口设计稿可供对齐？
3. 付费分层建议口径（免费/付费边界）？
4. 若上游短期内无规划，AiKlog 将**自研落地**（先例：B6 订阅通知、B7-B9），届时会按惯例在协作区预告并带 `[family]` 标提交，以便上游后续实现时双向对齐、避免分叉。

## 5. 我方状态

- 当前不阻塞任何在途批次；本需求为规划性征询。
- 若上游确认有规划，我方按 REQ-008/009 既有模式：等上游锚点/源包进入可追踪分支后按 SHA 移植。

## 6. 上游已答复与移植跟进（2026-09-30）

- **REQ-010 已 closed，上游答复**：家族传承记录模块**已实现并部署生产**（CoreModules.Family 独立内核模块，`family.enabled` 白名单开关），一期 `73a5017`（三表+14 端点+四 Tab 前端+提醒 ticker）、二期 AI 人生参谋 `fd1460b`、IM 语音归档 `cf779ec`；期望最小集 90%+ 已覆盖，**无需自研**。
- 数据模型：`family_members` / `family_relations`（parent_of|child_of|spouse_of|sibling_of|other）/ `family_anniversaries`（birthday|anniversary|custom）；接口 `/api/v1/family/{members,relations,anniversaries,tree,timeline,settings,advise}`；时间轴不建表，md 文件 `content_state` 打标 `node_type=life_event` + `fields{occurred_at,stage,people}`。
- 付费分层：与 REQ-008 对齐——基础 M0 免费（`family.enabled` 默认关、装即用）；增强 tier=pro 门控（同 orgPaidGate 模式），scope=**feature:family**。
- **移植状态**：三个锚点在 AiKlog 可追踪分支均不可达（Org 锚点 `7bea3b9` 同）→ 已发**缺件单**《AiKlog致上游-REQ010家族模块缺件单-锚点不可达-请给源码包与设计稿-v1.0.md》（先例 BUG-001），等上游推锚点或投源码包后按 SHA 移植。
