# AiKlog 落地回执：源包导入器（SPEC-SP-001）+ WebDAV / Org 接线

> 编号：ACK-20260920-B｜壳：AiKlog（爱库录）｜日期：2026-09-20
> 承接：《采集源包格式规范 v1》（SPEC-SP-001，上游 09-20 交付）、上游阻塞项解除包 `handler/{webdav,org,webdavmount}.go`
> 状态：**已合入并端到端验证通过**（本地实例 12/12，单元测试全绿）；未部署生产

---

## 一、源包导入器（SPEC-SP-001 落地）

### 1.1 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/admin/apps/install-source-pack` | multipart `file=zip`，或 `application/json` 的 `{"manifest":{...}}` 直投 |
| GET | `/api/v1/admin/apps/source-packs` | 已装源包列表（含各自导入的源条数） |
| DELETE | `/api/v1/admin/apps/source-packs/{id}` | 卸载：删该包导入的 sources（按 `pack_id` 精确匹配）+ 移除应用中心登记 |

**同一链路复用**：本地 `/admin/apps/install-zip` 与在线 `/admin/apps/market/install` 均已分流——
manifest `kind=source-pack`（或包内带 `sources/` 目录）自动走源包导入，不进插件登记。
在线分流必须发生在「索引分类强制覆盖 kind」**之前**，否则源包会被市场索引归成 plugin。

### 1.2 校验（全过才落库，失败不写半包）

| 项 | 错误码 | 依据 |
|---|---|---|
| kind 恒为 `source-pack` | `SOURCE_PACK_BAD_KIND` | §3.1 |
| target 含本壳（或 `*`/空） | `MARKET_TARGET_MISMATCH` | §3.2（B3 口径） |
| `min_core_version` 下限比较 | `PLUGIN_CORE_TOO_OLD` | §3.2 问题 3（复用 v2 `VersionAtLeast`，不另立体系） |
| schema 区间（当前恒 1） | `MARKET_SCHEMA_MISMATCH` | §3.2 |
| compliance.target_site / usage 必填 | `SOURCE_PACK_COMPLIANCE_REQUIRED` | §5（上架硬门槛） |
| 包内可执行物（.so/.exe/.sh/.py/.js…） | `SOURCE_PACK_EXECUTABLE_DENIED` | §2（协议 §3 不允许任意代码执行） |
| url 命中 SSRF 白名单 | `SOURCE_PACK_SSRF_BLOCKED` | §4.2（复用采集器同源 `checkPublicURL`） |
| template.list.urls 非空 / fields 含 title+link | `SOURCE_PACK_BAD_TEMPLATE` | §4.2 |
| kind=job 必须声明 city | `SOURCE_PACK_BAD_ENTRY` | §4.1 |

### 1.3 落地与两处宿主取舍

- **写 `sources` 表**（每源一行），事务导入，任一条目失败整体回滚（规范 §6：表=索引，zip 仅作可分发交付物）。
- **同 id 重装 = 覆盖式更新，`enabled` 保留用户手动改过的状态**（规范 §6）。
- 取舍 1：**zip 本体不落盘归档**。规范 §6 提到归档 `market/official/`，但运行期 Collector 只读表不读 zip；
  本壳把 compliance 摘要写进 `blog_plugins.settings_schema` 供应用中心卡片展示，zip 留作审计交付物不入库。
  如需归档可后续加 `data/market/` 落盘，不影响协议。
- 取舍 2：**新增两列**承载溯源（规范未定义，属宿主实现细节）：
  - `sources.pack_id`（源包 id；覆盖更新与卸载按此定位，含索引）
  - `blog_plugins.pack_type`（`''`=普通应用 / `'source-pack'`=源包；应用中心据此区分卸载行为）

安装响应示例：`{"ok":true,"id":"source-pack-e2e","created":1,"updated":0,"total":1,"pack_type":"source-pack","capability":"collector"}`

---

## 二、WebDAV / Org 接线

上游 `handler/{webdav,org,webdavmount}.go` 此前因核心层分歧挂起（`_pending_upstream/`，`_` 前缀不参与编译）。
本轮补齐后已全部移入 `internal/handler/` 并注册路由。

### 2.1 补齐的缺口

| # | 缺口 | 处置 |
|---|---|---|
| 1 | `service.File` 缺 `ViewCount` | 结构体加字段（列已在） |
| 2 | `FileStore.ReplaceContentBinary` | 新增：原地覆盖写（WebDAV PUT 语义；Upload 遇同名会加后缀另存，不符合 PUT）。含流回绕、版本自增、文本重索引 |
| 3 | `NotifyStore.AddUser` | 新增：按 user_id 投递；`Add` 变为其 SystemOwnerID 包装 |
| 4 | handler 缺 `spaceRole` / `quotaUse` / `quotaUploadMB` | 新增 `handler/quota.go`：角色=属主→`space_members`；配额读 `storage.quota_upload_mb` / `storage.quota_total_mb`（0/空=不限） |
| 5 | handler 缺 `curHomeSpaceID` | 在 `mu.go` 补齐（当前用户 home space） |

### 2.2 接线后发现的表/列缺口（fork 侧早期裁剪遗留，**非上游缺件**）

上游包假定这些对象已存在，本 fork 没有，导致 Org 端点 500。已自建（DDL 与上游字段一致）：

| 对象 | 说明 |
|---|---|
| `tags.kind` 列 | 组织树节点 = `tags` 表中 `kind='org'` 的行（`owner_id='org'` 保留字），fork 的 tags 表无 kind 列 |
| `org_memberships` 表 | 组织任职（since/until 任期，历史可回溯） |
| `org_transfers` / `org_transfer_items` 表 | 移交流（draft→previewed→executed→archived\|cancelled）与明细 |
| `space_members` 表 | 空间成员角色；`service/org.go` 的部门空间查询一直 JOIN 它，但 fork 侧从未建表 |

> 前三项是我方自建，字段按 service 层 SQL 反推，与上游命名一致；若上游后续交付正式 DDL，
> 我方 `hasColumn` / `CREATE TABLE IF NOT EXISTS` 均为幂等，不会冲突。

### 2.3 路由清单（CoreModules 门控：模块 nil → 未注册 → 404）

- WebDAV：`/api/v1/dav/`（对外服务端，Basic 认证；另受 `dav.enabled` 开关，默认开）+ `/api/v1/webdav/mounts*` 8 条
- Org：`/api/v1/org/{settings,tree,tree/paths,departments,memberships,members,transfers,custodian-count}` 等 20 条
- 装配点：`cmd/aikmap/capabilities.go` 的 `buildCoreModules`，按 `capability.webdav` / `capability.org` 门控

---

## 三、验证

| 项 | 结果 |
|---|---|
| `go build -mod=readonly ./...` / `go vet ./...` | 全绿 |
| `go test ./...` | 全绿（含新增 `service/sourcepack_test.go` 11 用例） |
| 本地实例端到端 | **12/12 PASS**：迁移表/列 3、Org 端点开→200 / 关→404 各 2、WebDAV mounts→200、/dav/ 匿名→401、源包安装/重装/422/列表/卸载 5 |
| 源包重装语义 | 首次 `created=1`；重装 `updated=1` 且保留 `enabled=false`（用户手动停用不被安装覆盖） |
| 卸载隔离 | 只删 `pack_id` 匹配的源，用户自建源不受影响 |

---

## 四、给上游的确认点（无需立即行动）

1. **组织树承载方式**：本 fork 的 `tags` 表原本只有 path 无 kind，我加了 `kind` 列承载 `org` 节点。
   若主系统组织树用的是独立表而非 tags，请告知——我方可加适配层对齐（当前实现与上游 SQL 完全一致）。
2. **Org 三张表**：`org_memberships` / `org_transfers` / `org_transfer_items` 本 fork 从未建表，我按 service 层 SQL 反推补齐。
   如上游有权威 DDL，请提供以便校对字段（当前以我方 service 层用法为准，功能已通）。
3. **源包 zip 归档**：我选择不落盘（表即权威载荷）。若上架审核需要留存包体，请明确归档路径约定。

---

*AiKlog（爱库录）· 2026-09-20*
