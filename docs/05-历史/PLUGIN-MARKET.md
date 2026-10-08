# 爱库录应用市场协议（v0.1）

> 对齐上游记档：emlog / Z-Blog 应用中心形态；**v0.2 先免费应用**。  
> 本仓库当前：协议草案 + 内置插件；远程安装 API 待阶段 D 实现。

---

## 1. 应用类型

| kind | 说明 |
|---|---|
| `plugin` | 博客功能插件（挂载点组件） |
| `theme` | 公开博客主题 |

## 2. 包结构

```
aiklog-app-<id>-<version>.zip
├── manifest.json
├── dist/                 # 编译后的前端组件（或可直接 import 的模块）
├── README.md             # 可选
└── LICENSE               # 可选
```

### manifest.json

```json
{
  "id": "blog-related-posts",
  "kind": "plugin",
  "name": "相关推荐",
  "version": "0.1.0",
  "description": "文章页同分类相关推荐",
  "author": "爱库录",
  "min_core_version": "0.1.0",
  "mount_points": ["post_bottom"],
  "api_permissions": ["blog.read"],
  "frontend_entry": "blog-related-posts",
  "price": 0
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `id` | 是 | 全局唯一，kebab-case |
| `kind` | 是 | `plugin` \| `theme` |
| `version` | 是 | semver |
| `min_core_version` | 是 | 低于核心版本 → 409 |
| `mount_points` | plugin | post_bottom / sidebar / list_item / head |
| `frontend_entry` | plugin | 必须命中主系统已注册组件，或安装时注入 |
| `price` | 是 | `0` = 免费（当前阶段仅允许 0） |

## 3. 安装流程（目标）

```
上传 zip / 从目录安装
  → 解压校验 manifest
  → min_core_version 检查
  → frontend_entry 一致性（内置清单或允许同包组件）
  → 写入 blog_plugins（enabled=true）
  → 前端 registerBlogPlugin
```

## 4. 与内置插件关系

| | 内置 | 市场应用 |
|---|---|---|
| 分发 | 随二进制源码 | zip / 目录 |
| 示例 | meta-footer, cover, archives… | 第三方 |
| 卸载 | 可禁用；核心不依赖 | 删除登记 + 可选清 KV |

## 5. 安全

- 安装需登录；公开 GET 仅列表已启用插件  
- 不执行远程 JS eval；组件随主系统构建或经受控加载  
- KV 数据按 `plugin_id` 隔离  

## 6. 阶段

| 阶段 | 内容 |
|---|---|
| **v0.1（当前）** | 协议文档 + 内置插件证明挂载点 |
| **v0.2** | 目录/zip 安装 API、启用列表 UI、免费应用 |
| **v0.3** | 主题包、校验签名、应用索引 |

---

实现进度跟踪见 `CHANGELOG.md` 与白皮书 §8。
