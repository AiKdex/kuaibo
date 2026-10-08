# AiKlog 回执 · B6「订阅与通知」已自研落地（REQ-006 收口）

> 提交：2026-09-21 · 我方基线 `sync/ag-base@dbbab7d`（C1 `afb2c82` + C2 `dbbab7d`）
> 关联工单：**REQ-006**（本回执为其结论；结论已同步投「协作资料/」与空间根目录）

## 一、结论先行

**B6 已全部落地并上线 aiklog.cn，工单可收口。** 我方按「未实现 ≠ 不能做」的既有约定，
在上游未回口径的情况下先按现状实现，并把**事件名**与**通知类型**收敛为集中常量表，
上游日后若给出口径，改一处即可对齐。

## 二、上游假设订正（已在文档与代码中固化）

| 项 | 我方原方案假设 | 上游实物（实测） | 我方处理 |
|---|---|---|---|
| `doc_subscriptions` | 列为「移植项」 | **表不存在** | **自研**（本壳内容模型） |
| `mentions` | 列为「移植项」 | **表不存在**（上游按 `notifications.type='mention'` 落） | **自研**独立提及表 |
| `webhook_subscriptions` | 「归并为一套」 | 仅 DDL、零实现 | 沿用已上线的 `blog_webhooks`，**不另起一套** |

所以 B6 的净工作量 = **自研订阅 + 自研提及 + 修通知中心两个真实缺陷**；
**不需要上游交付任何东西**——这正是本回执要说明的：请勿按我方旧清单去准备并不存在的交付物。

## 三、我方交付（代码 `sync/ag-base@dbbab7d`）

- `doc_subscriptions` 表 + `service/subscription.go`：文件/目录订阅。匹配口径 =
  「文件 id + 全部祖先目录 id」连成一条链，`target_id IN (链)` **一次查出**订阅者，
  file 型与 dir 型共一条 SQL；投递**跳过编辑者本人**、同 (用户,文件) 600s 窗口去重、
  **永不返回错误**（订阅是增值能力，不阻断正文写入）。
- `mentions` 表 + `service/mention.go`：`@[显示名](user_id)` 显式写法 + `@用户名` 纯写法；
  解析是**纯函数**（不查库）；落库 `INSERT OR IGNORE` 幂等，**只有真插入成功才发通知**；
  评论删除与文件 `Purge` 级联清理提及行。
- 新增 7 条 API：`GET|POST /subscriptions`、`GET /subscriptions/status`、
  `DELETE /subscriptions/{target_type}/{target_id}`、`GET /mentions`、
  `GET /mentions/unread-count`、`POST /mentions/read`。
- 前端：阅读页订阅按钮、「@我」页与侧栏入口、通知图标补 `subscription` / `mention` / `comment`。

## 四、修掉的两个真实缺陷（**与上游同构，建议上游同步自查**）

1. 🔴 **通知中心读接口无用户隔离（越权）**：`notify` 的 `List / UnreadCount / MarkRead / MarkAllRead`
   原本**完全不带 user 条件** —— 任何登录用户都能看到**所有人的**通知，含 `org.transfer` 这类
   带组织与移交信息的条目。已改为按 `user_id` 隔离，管理员额外可见站点级（`SystemOwnerID`）。
   👉 **若上游 `service/notify.go` 仍是"全表可见"，这是个货真价实的越权，建议一并修。**
2. **`notifications.payload` 被写成 BLOB**：`AddUser` 直接绑 `json.Marshal` 的 `[]byte`，
   驱动按 BLOB 落库而列声明是 TEXT → `LIKE` / `GLOB` 在 BLOB 上**恒不匹配**，
   任何"按 payload 内容查重/检索"的逻辑都会静默失效。
   修法：写入端绑 `string`；查询端用 `instr(col, ?) > 0`（对 BLOB / TEXT 都成立，**兼容历史行**，不必洗数据）。

## 五、验证（可复现）

- 服务器 `go build / vet / test ./internal/...` 全绿。
- **隔离端到端 57/57 PASS**：沙箱独立端口 + 生产库副本 + 两个临时用户，**生产零改动**
  （未鉴权 5 端点全 401；订阅 CRUD 与幂等；编辑者不给自己发通知；600s 去重；提及四种边界；
  评论提及与删除级联；**通知按用户逐条 id 比对**（可见集合 = 库内本人行、两用户零交集、管理员不越视）；
  payload 落 TEXT；退订后不再通知；`Purge` 级联）。
- 生产残留核对：`doc_subscriptions` / `mentions` / `notifications` 均 = 0，文件数与用户数未变。

## 六、仍待上游口径（**不阻塞**，我方已留单点可改）

- **Q1** 出站 webhook 的**权威事件名枚举**，或明确「各壳自定、不要求互通」。
- **Q2** `notifications.type` 是**开放字符串**还是**权威枚举**（枚举请给全集）。
- **Q3/Q4** `doc_subscriptions` / `mentions` 上游**做不做**（我方可按上游口径平移字段，
  现有实现把差异全部收在常量表与 `extra` JSON 里）。

以上若有答复，直接在 **REQ-006** 下回复即可（本回执已作为关联文档投递）。若上游答复口径与
我方现状不同，我方按新口径**单点调整**后再回执一版。

—— AiKlog（爱库录）· 2026-09-21
