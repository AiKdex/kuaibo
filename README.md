# 快博 kuaibo

> 基于 爱库录（AiKlog）引擎的**自部署 AI 知识库博客 / 建站系统**。

**目录即站点，文件即文章。** 拖拽文件即发布，语义检索与双链互文开箱可用；Go 单二进制 + SQLite，数据完全在自己手里。

## 特性

- **前台即后台**：拖拽文件发成博客，无需单独 CMS
- **语义检索 + 双链互文**：知识自动成网，引用即关联
- **应用中心**：主题与插件从官方目录一键安装（目录强制 ed25519 验签）
- **商城**：商品 / 购物车 / 收银台 / 订单 / 优惠券 / 退款 / 退货全链路
- **数字交付**：付款后凭条目级下载令牌取件，带次数上限
- **集成**：IM 绑定、Webhook 事件、字幕/转码、缩略图派生、离线备份

## 快速开始

```bash
git clone --depth 1 https://github.com/AiKdex/kuaibo.git
cd kuaibo
sudo bash scripts/install.sh --domain blog.example.com
```

脚本会自动：装依赖（Go / Node / nginx）→ 构建前后端 → 装到 `/opt/aiklog` → systemd 托管 → nginx 反代 → 健康检查。**幂等**，重复执行即升级（自动备份旧二进制与数据库）。

- 没有域名先试跑：`sudo bash scripts/install.sh`，会给你一个 `http://<公网IP>.nip.io` 入口
- 已有反代（Cloudflare Tunnel / Caddy）：加 `--no-nginx`，把入口指向 `127.0.0.1:8780`

部署后务必记下管理员初始密码（脚本与启动日志各打印一次）：

```bash
grep '初始管理员密码' /opt/aiklog/log/aiklog.log | tail -1
```

## 站点名称可自定义

这是一套**建站系统**：站点名称可在后台「设置」里自行修改，出厂默认是引擎名，部署后改成你自己的站名即可，不影响引擎本身。

## 安全组

放行 22 / 80（用 HTTPS 则加 443）。**不要放行 8780** —— 服务只监听本机，由 nginx 对外。

## 数据与备份

数据都在 `/opt/aiklog/data`：`aikmap.db`（SQLite）+ `files/`（上传的文件）。备份这两个即可；schema 迁移在启动时自动执行。

## 文档

- 一键安装脚本（含详细注释）：`scripts/install.sh`
- 文档索引：`docs/README.md`

## 许可

MIT，见 `LICENSE`。
