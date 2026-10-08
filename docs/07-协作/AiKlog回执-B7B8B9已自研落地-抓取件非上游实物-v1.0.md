# AiKlog 回执 · B7「IM 绑定」+ B8「媒体底座」+ B9「技能包」已自研落地

> 提交：2026-09-21 · 我方基线 `sync/ag-base@90185a9`（C1 `e23ca42` server + C2 `90185a9` web，本回执为其后文档提交）
> 关联：本回执结论已**双投**「协作资料/」与空间根目录。**未新建工单**——本批无上游交付依赖（理由见下）。
> 前置：B6 回执（REQ-006 收口）· 本批为《上游内容线与工作流集成方案-20260921》的**收尾批次，B0–B9 至此全部完成**。

## 一、结论先行

**B7 / B8 / B9 已全部落地并上线 aiklog.cn。B0–B9 十二个批次至此收口。**

关键更正：这三个批次在方案文档里都被写成「移植项」，但开工前逐一回上游真实仓库核对，
**三个批次的上游物件全部不存在**。所以它们不是移植，是**自研**：

| 批次 | 原方案假设（"移植"） | 上游实物（实测） | 我方处理 |
|---|---|---|---|
| **B7** | `im_bindings` / `im_binding_codes` 表 + `im_tools.go` / `wechat_im.go` | **全不存在**（0 命中） | **自研**：绑定码 + 身份映射 |
| **B8** | `file_media` 表 + `media.go` / `media_worker.go` / `transcriber.go` + `files/{id}/thumbnail\|media\|transcoded` | **全不存在**；服务器亦无 ffmpeg | **自研底座**（收窄）：只落表 + 图片 header 级探测 |
| **B9** | `skill_packages` / `skill_grants` 表 + `skill.go` + `skills/export-market` + `market_export.go` | 两张表**只有模型、零实现**；导出市场**不存在** | **自研**（收窄）：目录 + 免费包自助领取 + 管理员上架/发放；**不做支付** |

**因此本批同样不需要上游交付任何东西。** 请勿按我方旧清单去准备并不存在的交付物。

## 二、⚠️ 一条需要上游自查的事（本批最重要的副产品）

**我们在本仓发现：`_tmp` / `*_ref` 这类"抓取件"里的表名与上游实物严重不一致。**

具体：我曾把 `aikmap_cn_ref/_tmp/schema.go`（抓取来的资料）当作上游 DDL 依据，
据此把 B7/B8/B9 都列为"移植项"。回上游真实 `server/internal/repo/schema.go` 核对后发现：
里面写的 `im_bindings` / `im_binding_codes` / `skill_packages` / `skill_grants` 等表，**上游一个都没有**。

👉 **建议上游自查**：如果上游也存在"用抓取件/中间产物当权威依据"的流程，请确认权威源只有真实仓库路径。
我方已把这条固化为铁律（见 §五）。

## 三、我方交付（代码 `sync/ag-base@90185a9`）

### B7 IM 绑定
- `im_bindings`（`UNIQUE(platform, platform_user_id)` → 改绑天然成立）+ `im_binding_codes`（PK=`code`，一次性 + 10 min TTL）。
- 流程：**网页端生成码 → IM 端发 `/bind <码>` 消费**（IM 里不适合输账号密码）。
  码集 `ABCDEFGHJKMNPQRSTUVWXYZ23456789`（去 I/L/O/0/1）、长 6；`platform=''` 的码任意平台可用，
  限定了平台的码**只在那个平台**可消费。
- **身份注入**：`imActor` 按 `(platform, platform_user_id)` 反查站内用户，贯穿
  `imDispatch` / `imPost` / 审计；**查不到就回退 `homeOwnerID()`** —— 既有单用户部署行为完全不变。
- **顺手回收一个错误接线**：`/bind` 前缀原本指向 `IntentStatus`（占位），发 `/bind X` 只会回一句状态；
  现改 `IntentBind`，并新增 `/unbind`。

### B8 媒体底座（范围收窄）
- `file_media` 表**字段按上游设想留足**（`duration_ms/width/height/codec/bitrate/thumbnail_ref/transcoded_ref/probe_status`）。
- `GET /api/v1/files/{id}/media`：**懒探测**。图片走 `image.DecodeConfig` **只读 header**（不解全图、不吃内存），
  `svg` 是活性文档明确跳过；音视频标 `probe_status='unsupported'`（**不是报错**）。
- 结论：**外部二进制就位后由 worker 补数即可，表结构无需再迁。**

### B9 技能包（范围收窄）
- `skill_packages` + `skill_grants`（`grantee_type ∈ user|space|instance`）；`kind ∈ tool|skill|workflow|extension`。
- 5 条 API：`GET /skills`（带"我是否已持有"）、`GET /skills/grants`、`POST /skills/{id}/claim`、
  `POST /admin/skills`、`POST /admin/skills/{id}/grant`（后两者限 owner/admin）。
- `Claim` 只放行 `status='published'` 且 `price_cents=0`；付费包返 **402 `SKILL_NOT_FREE`**（语义就位，等支付通道）。
- **与应用中心的边界**：应用中心管**站点能力**（落 `site_licenses`），技能包管**可授权能力资产**
  （落 `skill_grants`，支持 user/space/instance 三态）——**不同表、不同维度，不合并**。

### 前端
「系统 → IM 绑定」（生成码 + 倒计时 + 复制指令 + 已绑定列表 + 解绑）与
「系统 → 技能包」（kind 过滤 + 领取/已拥有/需授权 + 我的授权）。

## 四、修掉的三个真实缺陷（**与上游同构，建议上游同步自查**）

全部在 `internal/service/digest.go`，且都是**从未成功过的死路径**：

1. 🔴 **「今日入库」恒等于全部文件**：`files.created_at` 是**毫秒**，日报却用 `time.Unix(dayStart, 0)`（**秒**）
   作阈值比较 → 对所有行都成立。已改 `UnixMilli()`。
2. 🔴 **查了一张不存在的表**：日报查 `blog_comments`，而本壳（与上游一致的现行模型）是 `comments`
   → 该查询一直 `no such table`、结果恒 0。**评论/订阅统计长期为 0 就是这里**。
3. 🔴 **日报 IM 推送是死路径**：`Send` 读 `im_bindings` —— 这张表在 B7 之前**根本不存在**，
   所以日报的 IM 推送**从未成功过一次**。B7 建表并补齐三列读取后才真正可用；
   同时补企微分支（优先 `ReplyAppMessage` 定向个人，失败退回群机器人 `SendText`）与 `rows.Err()` 检查。

👉 **若上游 `digest` 类逻辑里也有 `blog_comments` 这种历史表名残留，或跨毫秒/秒比较，建议一并排查。**

## 五、验证（可复现）

- 服务器 `npm ci` → `vite build` → **覆盖两份 webdist** → `go build`；`go vet / test ./internal/...` 全绿。
- **隔离端到端 87/87 PASS**（沙箱 8781 + 生产库副本 + 两个临时用户；跑完停实例、删沙箱，**生产零改动**）：
  未鉴权 9 个新端点全 401；绑定码生成/长度字符集/TTL/落库；同平台替换作废旧码、**跨平台不互相作废**；
  `/bind` 消费成功与**二次消费被拒**；**平台错配被拒且不改绑定**；改绑（同 IM 身份只留一行）；
  `/unbind`；非法码/缺参不改状态；未绑定回退 owner 仍可用；
  图片探测 120×45/png/落库/缓存命中、非图片 `unsupported`、不存在 404；
  技能包目录只含 published、免费领取幂等、付费 402、草稿 403、非管理员 403、**按用户隔离**；
  既有 8 个端点 + 对外 feed 全 200 无回归。
  证据：`docs/07-协作/_b7-e2e-20260921/`。
- 外网复验：`/`、`/app`、`/blog`、`health`、`feed.xml`、`sitemap.xml` 全 200；新资产**逐字节哈希一致**
  （`https://aiklog.cn/assets/index-P6VkoG8w.js` = `f51218625b7d1873762580453d5b34b1` / 155598 字节）；
  新端点经 nginx 匿名 401。
- 生产残留：5 张新表全 0；文件数 77、用户数 1、space 数 1 均未变；storage 内容哈希 e2e 前后 `RESULT=MATCH`。

## 六、仍待上游口径（**不阻塞**，我方已留单点可改）

- **Q1** 上游**做不做** IM 绑定？若做，绑定码的字段语义（长度/字符集/TTL/是否限平台）我方可按你方口径平移。
- **Q2** 上游**做不做**媒体衍生？若做，请给 `file_media` 的**权威字段语义**（尤其 `probe_status` 枚举）；
  我方字段已留足，对齐成本低。
- **Q3** 上游**做不做**技能包？若做，请给 `grantee_type` 的**权威枚举**（我方现为 `user|space|instance`）
  与"付费包"的交付形态（是否复用 `access_grants` 那条链）。
- **Q4** 上游 `digest` / 统计类逻辑是否也有 §四 那三类历史残留？若需要，我方可提供具体排查 SQL。

以上若有答复，本回执即为关联文档；若上游口径与我方现状不同，我方按新口径**单点调整**后再回执一版。

—— AiKlog（爱库录）· 2026-09-21
