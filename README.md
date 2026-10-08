# 爱库录 AiKlog

**目录即站点，文件即文章**

自部署 AI 知识库博客系统。拖拽即发、语义检索、双链互文；Go 单二进制 + SQLite，数据在自己手里。

- 样板房：<https://aiklog.com>
- 公开博客：`https://aiklog.com/app#/blog?view=public`
- 控制台：`https://aiklog.com/#/desk`（路径不对外宣传）

---

## 这是什么

爱库录（AiKlog）是 **AiKmap 主系统的精简发行版**：去掉采集、IM、重可视化，只保留 **文件 → 知识库引擎 → 公开博客** 这条链。

| | |
|---|---|
| 产品全称 | 爱库录AI知识库博客系统 |
| 对标 | emlog / Typecho（多一个知识底座） |
| 体积 | Linux 二进制约 **13MB**（精简发行） |
| 依赖 | 无需 PHP / MySQL |

---

## 快速开始

### 从源码构建

```bash
# 前端
cd web
npm install
npm run build
# 产物复制到 Go embed 路径
rm -rf ../server/internal/handler/webdist
cp -r dist ../server/internal/handler/webdist

# 后端（Linux 示例）
cd ../server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w" -o aiklog ./cmd/aikmap
```

### 运行

```bash
AIKMAP_ADMIN_PASSWORD='请改成强密码' \
  ./aiklog -addr 127.0.0.1:8780 -db ./data/aiklog.db
```

| 端点 | 说明 |
|---|---|
| `GET /` | SPA（需反代时可另挂产品落地页） |
| `GET /app#/blog?view=public` | 公开博客 |
| `GET /app#/desk` | 控制台 |
| `GET /api/v1/health` | 健康检查 |
| `GET /api/v1/blog/feed.xml` | RSS |

### systemd 示例

```ini
[Service]
User=aiklog
Environment=AIKMAP_ADMIN_PASSWORD=请改成强密码
ExecStart=/opt/aiklog/bin/aiklog -addr 127.0.0.1:8780 -db /opt/aiklog/data/aiklog.db
Restart=on-failure
```

Nginx 反代到 `127.0.0.1:8780`，并传递 `X-Forwarded-Proto`（HTTPS 下 Cookie 会自动 Secure）。

---

## 核心概念

1. **目录即站点**：固定「博客」目录 = 公开站内容源；子目录 = 分类  
2. **文件即文章**：Markdown 放入即成为文章；移除即下线  
3. **稳定 slug**：发布后生成，改文件名/分类不碎链  
4. **上传策略可配**：`blog.auto_publish_on_upload`（默认关，草稿确认后再发）  
5. **插件挂载点**：`post_bottom` / `sidebar` / `list_item` / `head`  
6. **主题协议**：`web/src/themes/aiklog` 为默认门面主题  

---

## 仓库结构

```
server/           Go 服务（cmd/aikmap + internal）
web/              Vue3 前端（含 themes/aiklog 爱库录主题）
docs/             白皮书与产品文档
CHANGELOG.md      更新日志
```

---

## 与 Aikdex 的边界

```
爱库录（本仓库）= 博客系统，无采集
Aikdex           = 采集内容站（独立项目）
Aikdex → POST /api/v1/blog/posts → 爱库录（样例投递）
```

---

## 文档

| 文档 | 说明 |
|---|---|
| [docs/爱库录白皮书.md](docs/爱库录白皮书.md) | 定位、竞品、市场、应用市场、部署与运营 |
| [docs/AIKLOG-精简发行方案.md](docs/AIKLOG-精简发行方案.md) | 裁剪/保留清单 |
| [docs/AIKLOG-样板房计划.md](docs/AIKLOG-样板房计划.md) | 样板房与部署记录 |
| [docs/AIKLOG-主题设计.md](docs/AIKLOG-主题设计.md) | 爱库录主题设计令牌 |

---

## 安全

- 首次启动务必设置 **`AIKMAP_ADMIN_PASSWORD`**，上线后立即改密  
- 写接口需登录；公开面仅 health/version/公开博客相关 GET  
- 请置于反代与 HTTPS 之后，勿直接暴露管理端口  

---

## 许可与来源

基于 AiKmap 博客底座精简发行。第三方依赖许可见各目录声明；发布前请补充 LICENSE 文件（由维护者选定）。

---

**爱库录 · 目录即站点，文件即文章**
