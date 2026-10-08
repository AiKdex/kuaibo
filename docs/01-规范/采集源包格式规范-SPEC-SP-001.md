# 采集源包格式规范 v1（Source Pack Spec v1）

> 编号：SPEC-SP-001｜版本：v1.0｜状态：生效
> 发起：上游（AiKmap 主系统）｜适用：AiKlog / AiKBox / AiKdex 各壳采集源包生产与上架
> 关联：《能力插件协议 v2 草案》§6（配置包）｜《上游回执-AiKlog需求清单v3.0》R3
> 2026-09-20

> **【规范地位】** 本文档于 2026-09-20 由协作底稿 `docs/07-协作/上游交付-采集源包格式规范-v1.md` 提升为 AiKlog 权威规范，归入 `docs/01-规范/`。内容以源交付稿为准，此处为权威归档与交叉引用入口；后续修订只在此处进行。

---

## 1. 定位与边界

**源包（Source Pack）= 一个可导入的 sources 模板集合的配置包，纯配置、无代码、无任意执行。**

| 属性 | 值 |
|---|---|
| 形态 | zip（L3 市场包，与主题/插件同渠道分发） |
| 内容 | `manifest.json` + 0..N 个源模板条目（内嵌于 manifest 或外置 `sources/*.json`） |
| 可执行代码 | **禁止**（协议 §3：L3 不允许任意代码执行） |
| 落地效果 | 导入后写入 `sources` 表（每源一条），由内核 Collector 引擎执行抓取 |
| 商品属性 | 可上架可售卖（计费/分成见结算 v0.3，本规范不涉及） |

**三层归属（沿用 v3.0 回执 §1.1 结论）**：引擎 = 内核 `kind=capacity`（编译期内置 + 能力开关）；源包 = 本规范（L3 配置包）；AI 面 = `ai.Registry` 工具（probe/web_clip）。

---

## 2. 包结构

```
source-pack-<id>-<version>.zip
├── manifest.json              # 必选：源包声明（见 §3）
├── sources/
│   ├── <source-1>.json        # 可选：源模板条目（每文件一条 Source，见 §4）
│   └── <source-2>.json
└── README.md                  # 可选：作者说明/合规声明补充
```

约束：
- 源模板可内嵌于 manifest（`sources` 数组）或外置 `sources/*.json`，二选一或并存（并存时外置优先覆盖同名 id）；
- zip 内禁止任何 `.so / .dll / .exe / .sh / .py / .js（运行时）` 可执行物；违规包上架审核直接拒；
- 单包可含多个源模板（推荐 1 源=1 文件，便于审阅与差异更新）。

---

## 3. manifest.json（源包声明）

```json
{
  "id": "source-pack-qiuzhi-bengbu",
  "name": "求职源包-蚌埠官办招聘",
  "version": "1.0.0",
  "min_core_version": "1.0.0",
  "min_schema": 1,
  "max_schema": 1,
  "author": "AiKmap 官方",
  "description": "蚌埠市官办招聘公告/岗位栏目采集模板集",
  "kind": "source-pack",
  "target": ["aiklog", "aikmap", "aikbox"],
  "license": "MIT",
  "compliance": {
    "target_site": "安徽省公共招聘网（蚌埠站）",
    "usage": "岗位/公告信息聚合展示",
    "robots_allowed": true,
    "fetch_frequency": "6h",
    "copyright_note": "仅聚合链接与标题摘要，正文跳转原站"
  },
  "sources": [
    {
      "id": "job-bengbu-ahgz",
      "name": "安徽公共招聘网蚌埠站",
      "kind": "job",
      "channel_type": "job_position",
      "fetch_mode": "http",
      "target_dir": "采集/求职/蚌埠",
      "url": "https://example.gov.cn/list/",
      "template": { "...": "见 §4" }
    }
  ]
}
```

### 3.1 字段说明

| 字段 | 必填 | 类型 | 说明 |
|---|---|---|---|
| `id` | ✅ | string | 源包唯一 id（`source-pack-` 前缀）；应用中心安装主键 |
| `name` / `description` | ✅ | string | 展示名/一句话说明 |
| `version` | ✅ | semver | 语义化版本（`x.y.z`） |
| `min_core_version` | ✅ | semver | 要求宿主最低核心版本（下限比较，复用 v2 `versionAtLeast`） |
| `min_schema` / `max_schema` | 可选 | int | 源包 schema 版本区间（当前恒 1；`max_schema=nil`=不设上限） |
| `kind` | ✅ | string | **恒为 `source-pack`**（与插件 kind=capacity 区分；供应用中心归一到「源包」类别） |
| `target` | ✅ | string[] | 适用壳标识（`aiklog`/`aikmap`/`aikbox`/`aikdex` 等）；不匹配 → `422 MARKET_TARGET_MISMATCH`（沿用 B3 口径） |
| `compliance` | ✅ | object | 合规声明（见 §5；**必填**——上架审核硬门槛） |
| `sources` | ✅ | array | 源模板数组（每项见 §4） |

### 3.2 版本校验口径（回应催办问题 3）

- **宿主兼容**：`min_core_version` 用 v2 `versionAtLeast(hostVersion, min_core_version)` 下限比较（与插件一致，不另立体系）；
- **schema 区间**：`min_schema ≤ 当前 schema 版本 ≤ max_schema`（当前 schema=1），超区间 → `422 MARKET_SCHEMA_MISMATCH`；
- **目标壳**：`target` 含当前壳 → 通过，否则 `422 MARKET_TARGET_MISMATCH`。

---

## 4. 源模板条目（sources 条目 / sources/*.json）

每条对应 `sources` 表一行，字段与表结构一一映射：

```json
{
  "id": "job-bengbu-ahgz",
  "city": "bengbu",
  "name": "安徽公共招聘网蚌埠站",
  "kind": "job",
  "channel_type": "job_position",
  "fetch_mode": "http",
  "target_dir": "采集/求职/蚌埠",
  "url": "https://example.gov.cn/list/",
  "template": {
    "list": {
      "urls": ["https://example.gov.cn/list/?page={n}"],
      "item_selector": "div.job-item",
      "fields": {
        "title": "h3 a",
        "link": "h3 a@href",
        "date": "span.date",
        "company": "span.company"
      },
      "page": {"start": 1, "step": 1, "max": 10}
    },
    "status_rules": {
      "title_empty": "reject",
      "link_external": "reject",
      "date_stale_days": 90
    }
  }
}
```

### 4.1 字段与 sources 表映射

| 条目字段 | sources 表列 | 说明 |
|---|---|---|
| `id` | `id` | 源唯一 id（安装时生成或沿用包内） |
| `city` | `city` | 城市标识（bengbu/hefei/…）；`kind=job` 且空 → 校验拒 |
| `name` | `name` | 源名称 |
| `kind` | `kind` | `job\|tender\|policy`（频道类型） |
| `channel_type` | `channel_type` | `job_position\|job_notice\|generic`（把关 L0） |
| `fetch_mode` | `fetch_mode` | `http\|tls\|trawl\|skip`（分级 fetch；`browser/trawl` 预留） |
| `target_dir` | `target_dir` | 入库目录（空=按 kind 默认 `采集/就业情报\|采集/培训补贴`） |
| `url` | `url` | 列表页 URL（必填；SSRF 白名单校验对象） |
| `template` | `template` | 站点模板 JSON（`list.urls` / `item_selector` / `fields` / `status_rules`） |
| `enabled` | `enabled` | 安装默认 1 |

### 4.2 校验规则（安装时）

1. `url` 必须命中宿主 SSRF 白名单（`ssrf.go` 同款防护；默认仅 http/https 且非内网段）；
2. `template.list.urls` 非空数组；`fields` 至少含 `title`/`link`；
3. `fetch_mode` 取值合法（`NormalizeFetchMode`）；`channel_type` 取值合法（`NormalizeChannelType`）；
4. 不合规条目 → 整包安装失败（原子回滚，不写半包）。

---

## 5. compliance（合规声明，必填）

| 字段 | 必填 | 说明 |
|---|---|---|
| `target_site` | ✅ | 采集目标站点名（上架审核展示） |
| `usage` | ✅ | 用途声明（如"岗位/公告聚合展示"） |
| `robots_allowed` | ✅ | 是否遵守 robots（false 的包上架从严） |
| `fetch_frequency` | 可选 | 建议抓取频率（如 `6h`/`1d`）；平台审核抓取频率上限 |
| `copyright_note` | 可选 | 版权/来源声明（转载边界说明） |

责任边界（沿用 v3.0 回执 R8 结论）：**内容合规由源包作者承担，平台负审核与下架责任**；平台代收分成的商品，审核不通过不上架。

---

## 6. 落地位置（回应催办问题 2）

| 对象 | 落地 |
|---|---|
| 安装产物 | **写 `sources` 表**（每源一行，表=索引+配置的权威源） |
| 包本体 | zip 归档于 `market/official/`（或租户市场目录），表内 `template` 为解析后的 JSON 载荷 |
| 更新 | 同 id 重装 = 覆盖式更新（`INSERT OR REPLACE` 语义），`enabled` 保留原状态 |

> 表=索引、目录=载荷：运行期 Collector 只读 `sources` 表，不读 zip；zip 仅作为可分发/可审计的交付物。

---

## 7. 上架与安装流程（与能力应用衔接）

```
第三方/官方 → 打包 zip（本规范）→ 应用中心上架（审核：manifest 校验 + compliance 审查）
→ 用户安装 → 校验（版本/target/schema/SSRF/字段）→ 写 sources 表 → 能力开闸（capability.collector）
→ 采集管理页可见 → POST /collect/run 触发
```

配套端点（内核已有）：
- `POST /api/v1/collect/run`（触发 run）
- `GET /api/v1/collect/runs`（查询状态）
- 事件 `collector.finished {run_id, status, created, failed}`（bus 冻结 topic）

---

## 8. 示例与校验样例

- 官方源包样例：`source-pack-qiuzhi-*.zip`（一源多城：同模板多 city 条目，kind 分流）——后续在应用中心官方区提供；
- 校验失败样例：`422 MARKET_SCHEMA_MISMATCH`（schema 超区间）、`422 MARKET_TARGET_MISMATCH`（target 不含本壳）、`422 MARKET_SHA_MISMATCH`（zip 哈希不符，沿用插件包）。

---

*上游 · AiKmap 主系统 · 2026-09-20｜2026-09-20 提升为 AiKlog 权威规范（SPEC-SP-001）*
