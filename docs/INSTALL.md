# 安装与部署 · 爱库录 AiKlog

单二进制 + SQLite。无需 PHP / MySQL。

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

## 8. 常见问题

| 现象 | 处理 |
|---|---|
| `no matching files found`（go build） | 先执行前端 build 并生成 `webdist`，或用 `build-release.sh` |
| version 仍为 `dev` | 使用发布脚本注入 `-X main.version=` |
| 公开页 404 `BLOG_CLOSED` | 博客管理中开启博客（`blog.open`） |
| 图片不显示 | 静态 `/media` 需在反代侧配置，或文章内用绝对 URL |

---

**爱库录 · 目录即站点，文件即文章**
