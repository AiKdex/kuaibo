# 安装与部署 · 爱库录 AiKlog

单二进制 + SQLite。无需 PHP / MySQL。

## 0. 一键部署（推荐）

```bash
git clone https://github.com/AiKdex/AiKlog.git
cd AiKlog
sudo bash scripts/install.sh --domain blog.example.com
```

脚本会：装依赖 → 构建（前端嵌入 → go build）→ 装到 `/opt/aiklog` → systemd 托管
→ （可选）nginx 反代 → 健康检查 → 打印管理员密码。**幂等**，重复执行即升级
（自动备份旧二进制与数据库后热替换）。

常用参数：

| 参数 | 说明 |
|---|---|
| `--domain` | 绑域名（不传则用 `公网IP.nip.io` 试跑） |
| `--port` | 监听端口（默认 8780，只监听本机，由反代对外） |
| `--dir` | 安装目录（默认 `/opt/aiklog`） |
| `--source git` | 从 GitHub 克隆源码（脚本不在本地时用） |
| `--no-nginx` | 自管入口（已有反代 / Caddy / Cloudflare Tunnel） |

装完想换绑域名，重跑一遍并带新 `--domain` 即可。

## 1. 获取二进制

### 源码构建

```bash
git clone https://github.com/AiKdex/AiKlog.git
cd AiKlog
chmod +x scripts/build-release.sh
./scripts/build-release.sh 0.1.1
# 产物：dist/aiklog
```

脚本会：`npm build` → 复制 `webdist` → `go build` 并注入版本号。

### 直接运行构建产物

```bash
chmod +x aiklog
```

## 2. 运行

```bash
mkdir -p data
AIKMAP_ADMIN_PASSWORD='请改成强密码' \
  ./aiklog -addr 127.0.0.1:8780 -db ./data/aiklog.db
```

| 参数 | 说明 |
|---|---|
| `-addr` | 监听地址（建议仅本机，由反代暴露） |
| `-db` | SQLite 路径（文件即全部数据） |

首次启动会在日志/设置中初始化管理员（默认用户名 `admin`）。

## 3. 反向代理（Nginx 示例）

```nginx
server {
    listen 443 ssl http2;
    server_name blog.example.com;
    ssl_certificate     /etc/letsencrypt/live/blog.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/blog.example.com/privkey.pem;

    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:8780;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

`X-Forwarded-Proto: https` 会让登录 Cookie 自动带 `Secure`。

## 4. systemd

```ini
[Unit]
Description=AiKlog 爱库录
After=network.target

[Service]
Type=simple
User=aiklog
WorkingDirectory=/opt/aiklog
Environment=AIKMAP_ADMIN_PASSWORD=请改成强密码
ExecStart=/opt/aiklog/bin/aiklog -addr 127.0.0.1:8780 -db /opt/aiklog/data/aiklog.db
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/aiklog/data

[Install]
WantedBy=multi-user.target
```

```bash
systemctl enable --now aiklog
```

## 5. 验收

| URL | 期望 |
|---|---|
| `/` | SPA 或你的落地页 |
| `/app#/blog?view=public` | 公开博客 |
| `/app#/desk` | 控制台（登录） |
| `/api/v1/health` | `{"ok":true}` |
| `/api/v1/version` | 含真实 `version`（非 `dev`） |
| `/api/v1/blog/feed.xml` | RSS |
| `/api/v1/blog/sitemap.xml` | sitemap |

## 6. 备份与升级

```bash
# 备份（整库 + 文件树，含「博客」目录）
cp data/aiklog.db /backup/aiklog-$(date +%F).db
rsync -a data/files/ /backup/files/

# 升级：替换二进制 → 重启（schema 自动 migrate，配置在 DB）
systemctl stop aiklog
cp dist/aiklog /opt/aiklog/bin/aiklog
systemctl start aiklog
```

## 7. 安全清单

- [ ] 修改默认管理员密码  
- [ ] 仅监听 `127.0.0.1`，经 HTTPS 反代  
- [ ] 防火墙不暴露 8780  
- [ ] 定期备份 `db` + `files`  

## 8. 应用中心与官方目录

应用中心按产品约定**恒指向官方站点**（不是某次安装的快照）。取目录有两条路，**本地优先**：

| 路径 | 触发条件 | 是否验签 | 是否回源官方 |
|---|---|---|---|
| 本地自托管 | `data/market/index.json` 存在 | **不验签** | 否 |
| 官方回源 | 无本地目录 | 强制 ed25519 验签 | 是 |

**为什么首次安装不要自带本地目录**：回源官方是更新目录的唯一途径。自带了本地目录，
就收不到官方的新应用；而**本地目录不验签**，也意味着它不受官方签名保护。

`install.sh` 会在两种情况下提示：

- 检测到**未签名**的本地目录 → 警告它会静默阻断回源，并给出一键切换命令：
  ```bash
  sudo mv data/market/index.json data/market/index.json.localbak
  sudo systemctl restart aiklog
  ```
- 检测到**已签名**的本地目录 → 认为是**有意的离线自托管**（内网场景），不打扰。

### 离线 / 内网自托管

完全离线的环境请自备目录并用官方签发方签名（`iss` 须在二进制的内嵌公钥表里，
当前为 `aiklog-license`）。签名必须由**与验签同一份代码**产出，否则字节对不上。

### 匿名装机统计

回源官方目录时，实例会附带一个**由本机密钥派生的匿名标识**（32 位 hex，不可反推密钥）
与版本号，官方侧据此把回源去重成「装机数」并统计版本分布。

- **不发送任何个人信息**：没有域名、文件、笔记、账号，也不记录 IP 与 User-Agent。
- **可关闭**：设置 → 运维 → `plugin_market.report_identity`（默认开）。关闭后不再附带标识，
  应用中心功能完全不受影响。
- 数字读作**下界**：内网与全离线安装不回源。形状校验挡得住垃圾数据，**挡不住故意伪造**。

### 排查

```bash
# 目录能否取到（200 = 可以）
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8780/market/index.json

# 回源失败时的具体原因
grep -i market /opt/aiklog/log/aiklog.log | tail -5
```

常见报错：

| 报错 | 原因 |
|---|---|
| `市场索引未签名` | 本地目录是旧的无签名文件（见上）或官方目录未签名 |
| `市场索引验签失败：签名缺失或验签不通过` | 签名与内容不匹配（常见于手工编辑过索引） |
| `市场索引不可达` | 连不上 `aikmap.cn`（内网/离线，属正常） |

---

## 9. 常见问题

| 现象 | 处理 |
|---|---|
| `no matching files found`（go build） | 先执行前端 build 并生成 `webdist`，或用 `build-release.sh` |
| version 仍为 `dev` | 使用发布脚本注入 `-X main.version=` |
| 公开页 404 `BLOG_CLOSED` | 博客管理中开启博客（`blog.open`） |
| 图片不显示 | 静态 `/media` 需在反代侧配置，或文章内用绝对 URL |

---

**爱库录 · 目录即站点，文件即文章**
