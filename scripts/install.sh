#!/usr/bin/env bash
# =============================================================================
# AiKlog 爱库录 · VPS 一键部署脚本
#
# 用法（在仓库目录内，或任意目录传 --source git）：
#   sudo bash scripts/install.sh                          # 本地源码构建部署
#   sudo bash scripts/install.sh --source git             # 从 GitHub 克隆构建部署
#   sudo bash scripts/install.sh --domain blog.example.com
#   sudo bash scripts/install.sh --port 9000 --no-nginx   # 自管入口（如已有反代/Caddy）
#
# 脚本做的事：装依赖 → 构建（前端嵌入 → go build）→ 装到 /opt/aiklog
#             → systemd 托管 → （可选）nginx 反代 → 健康检查 → 打印管理员密码
# 幂等：重复执行安全（自动备份旧二进制与数据库后热替换）。
# =============================================================================
set -euo pipefail

# ---------- 参数 ----------
DOMAIN=""
PORT="8780"
INSTALL_DIR="/opt/aiklog"
SOURCE="local"          # local | git
GIT_URL="https://github.com/AiKdex/kuaibo.git"
NO_NGINX="0"
GO_VERSION="1.27.1"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)  DOMAIN="$2";  shift 2 ;;
    --port)    PORT="$2";    shift 2 ;;
    --dir)     INSTALL_DIR="$2"; shift 2 ;;
    --source)  SOURCE="$2";  shift 2 ;;
    --git-url) GIT_URL="$2"; shift 2 ;;
    --no-nginx) NO_NGINX="1"; shift ;;
    -h|--help) sed -n '3,14p' "$0"; exit 0 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done

# ---------- 前置检查 ----------
SUDO=""
if [[ "$(id -u)" != "0" ]]; then
  command -v sudo >/dev/null || { echo "错误：非 root 且无 sudo"; exit 1; }
  SUDO="sudo"
fi

echo "==> [0/6] 环境探测"
OS_ID=""; OS_ARCH="$(uname -m)"
if [[ -f /etc/os-release ]]; then . /etc/os-release; OS_ID="$ID"; fi
[[ "$OS_ARCH" == "x86_64" ]] && GO_ARCH="amd64" || GO_ARCH="arm64"
command -v curl >/dev/null || { $SUDO apt-get update -qq && $SUDO apt-get install -y -qq curl; }
echo "    系统: ${PRETTY_NAME:-unknown}  架构: $OS_ARCH  入口端口: $PORT"

# ---------- 依赖：Go ----------
have_go() {
  command -v go >/dev/null || return 1
  local v
  v="$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | head -1)"
  [[ -n "$v" && "$v" > "go1.26" ]]
}
if ! have_go; then
  echo "==> [1/6] 安装 Go ${GO_VERSION}"
  curl -fsSL -o /tmp/go.tgz "https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
    || curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf /tmp/go.tgz
  $SUDO ln -sf /usr/local/go/bin/go /usr/local/bin/go
  $SUDO ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
else
  echo "==> [1/6] Go 已就绪: $(go version)"
fi
export PATH="/usr/local/go/bin:$PATH"

# ---------- 依赖：Node ----------
have_node() { command -v node >/dev/null && [[ "$(node -v | tr -d v | cut -d. -f1)" -ge 18 ]]; }
if ! have_node; then
  echo "==> [1/6] 安装 Node.js 20"
  if curl -fsSL https://deb.nodesource.com/setup_20.x | $SUDO -E bash - 2>/dev/null; then
    $SUDO apt-get install -y -qq nodejs
  else
    $SUDO apt-get update -qq && $SUDO apt-get install -y -qq nodejs npm   # 兜底发行版包
  fi
else
  echo "==> [1/6] Node 已就绪: $(node -v)"
fi
npm config set registry https://registry.npmmirror.com 2>/dev/null || true

# ---------- 获取源码 ----------
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "$SOURCE" == "git" ]]; then
  echo "==> [2/6] 克隆源码: $GIT_URL"
  REPO_DIR="/tmp/aiklog-src"
  rm -rf "$REPO_DIR"
  # 强制 HTTP/1.1：GitHub 匿名 clone 在部分网络下走 HTTP/2 会被中间设备掐断，
  # 报 "curl 16 Error in the HTTP2 framing layer / expected flush after ref listing"。
  # 局部 -c 覆盖，不污染用户全局 git 配置。
  git -c http.version=HTTP/1.1 -c http.postBuffer=524288000 clone --depth 1 "$GIT_URL" "$REPO_DIR"
else
  [[ -f "$REPO_DIR/server/go.mod" ]] || { echo "错误：未找到源码（$REPO_DIR）。请用 --source git 或在仓库目录内运行。"; exit 1; }
  echo "==> [2/6] 使用本地源码: $REPO_DIR"
fi

# ---------- 构建 ----------
echo "==> [3/6] 构建前端 + 嵌入 + go build（约 2~5 分钟）"
cd "$REPO_DIR/web"
npm install --no-fund --no-audit >/dev/null
npm run build >/dev/null
cd "$REPO_DIR"
rm -rf server/internal/handler/webdist
cp -r web/dist server/internal/handler/webdist
mkdir -p "$REPO_DIR/dist"
cd "$REPO_DIR/server"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" GOTOOLCHAIN=local CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o "$REPO_DIR/dist/aiklog" ./cmd/aikmap
echo "    产物: $(ls -lh "$REPO_DIR/dist/aiklog" | awk '{print $5, $9}')"

# ---------- 安装 ----------
echo "==> [4/6] 安装到 $INSTALL_DIR（已有实例自动备份）"
TS="$(date +%Y%m%d%H%M%S)"
$SUDO mkdir -p "$INSTALL_DIR/bin" "$INSTALL_DIR/data" "$INSTALL_DIR/log"
if [[ -f "$INSTALL_DIR/bin/aiklog" ]]; then
  $SUDO cp -a "$INSTALL_DIR/bin/aiklog" "$INSTALL_DIR/bin/aiklog.bak_$TS"
  [[ -f "$INSTALL_DIR/data/aikmap.db" ]] && $SUDO cp -a "$INSTALL_DIR/data/aikmap.db" "$INSTALL_DIR/data/aikmap.db.bak_$TS"
  echo "    已备份: bin/aiklog.bak_$TS"
fi
$SUDO cp -f "$REPO_DIR/dist/aiklog" "$INSTALL_DIR/bin/aiklog"
$SUDO chmod +x "$INSTALL_DIR/bin/aiklog"
[[ "$(id -u)" == "0" ]] || $SUDO chown -R "$(whoami)":"$(whoami)" "$INSTALL_DIR"

# ---------- 应用中心目录：回源 vs 本地自托管 ----------
# 为什么在这里提示（B48）：
#   应用中心按产品约定「恒指向官方站点」。取目录有两条路，**优先级是本地优先**：
#     1) $INSTALL_DIR/data/market/index.json 存在 → 直接用它（**且不验签**）
#     2) 否则 → 回源官方 plugin_market.index_url（远程索引强制 ed25519 验签）
#   所以一个**无签名**的本地索引会静默阻断回源：应用中心看起来正常（用的是本地目录），
#   但官方侧收不到任何请求 → 装机统计恒为 0，而且**没有任何报错**。
#   这正是升级老实例最容易踩的坑（老版本发行包或手工投放过本地目录）。
#
# 已签名的本地索引 = 有意自托管（离线/内网场景），**不是问题**，不打扰。
MKT="$INSTALL_DIR/data/market/index.json"
if [[ -f "$MKT" ]]; then
  if grep -qE '"signature"[[:space:]]*:' "$MKT" 2>/dev/null; then
    echo "    应用中心: 本地自托管目录（已签名，不回源官方）"
  else
    echo "    ⚠️  应用中心: 检测到**未签名**的本地目录 data/market/index.json"
    echo "        它会静默阻断官方目录回源 —— 应用中心看起来正常，但官方侧统计不到本机。"
    echo "        想用官方目录（推荐，可收到官方更新）："
    echo "            sudo mv $MKT $MKT.localbak && sudo systemctl restart aiklog"
    echo "        想保留离线自托管：把该文件用官方签发方重新签名即可（保持现状即可）。"
  fi
else
  echo "    应用中心: 无本地目录，将回源官方（默认 aikmap.cn/market/index.json）"
fi

# ---------- systemd ----------
echo "==> [5/6] systemd 服务"
UNIT=/etc/systemd/system/aiklog.service
$SUDO tee "$UNIT" > /dev/null <<EOF
[Unit]
Description=AiKlog 爱库录（自部署博客/内容引擎）
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bin/aiklog -addr 127.0.0.1:$PORT -db data/aikmap.db
Restart=always
RestartSec=3
LimitNOFILE=65535
StandardOutput=append:$INSTALL_DIR/log/aiklog.log
StandardError=append:$INSTALL_DIR/log/aiklog.log

[Install]
WantedBy=multi-user.target
EOF
$SUDO systemctl daemon-reload
if systemctl is-active --quiet aiklog 2>/dev/null; then
  $SUDO systemctl restart aiklog
else
  $SUDO systemctl enable --now aiklog
fi
sleep 3
HEALTH="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/v1/health" || true)"
[[ "$HEALTH" == "200" ]] || { echo "错误：health=$HEALTH，看日志: $INSTALL_DIR/log/aiklog.log"; exit 1; }
echo "    服务运行中，health=200"

# 应用中心自检：确认目录真的拿得到（200）。失败不阻断安装 —— 目录拿不到不影响
# 站点本身可用（用户仍可写博客、传文件），只提示怎么查。
MKT_HTTP="$(curl -s -o /dev/null -m 20 -w '%{http_code}' "http://127.0.0.1:$PORT/market/index.json" || true)"
if [[ "$MKT_HTTP" == "200" ]]; then
  echo "    应用中心自检: 目录可访问（HTTP 200）"
else
  echo "    ⚠️  应用中心自检: 目录 HTTP=$MKT_HTTP"
  echo "        常见原因：① 本地 data/market/index.json 无签名（见上方提示）"
  echo "                  ② 连不上官方 aikmap.cn（内网/离线环境 —— 属正常，自托管本地目录即可）"
  echo "        排查：curl -s -o /dev/null -w '%{http_code}\\n' http://127.0.0.1:$PORT/market/index.json"
  echo "        日志：grep -i market $INSTALL_DIR/log/aiklog.log | tail -5"
fi

# ---------- nginx（可选）----------
PUBLIC_IP="$(curl -s -m 5 ifconfig.me || hostname -I | awk '{print $1}')"
ENTRY_HOST="${DOMAIN:-${PUBLIC_IP}.nip.io}"
# B48：nginx 没装就**先装**。此前只在检测不到时「跳过反代」，而服务只听 127.0.0.1:$PORT
# —— 新机器上这等于装了个外网打不开的站点，提示语却只是轻描淡写一句「跳过 nginx」。
# 这是新机部署最容易踩且最难自己发现的坑：服务在跑、health 200，但外面访问不了。
if [[ "$NO_NGINX" != "1" ]] && ! command -v nginx >/dev/null; then
  echo "==> [6/6] 未检测到 nginx，尝试安装"
  if $SUDO apt-get update -qq && $SUDO apt-get install -y -qq nginx; then
    echo "    nginx 安装完成"
  else
    echo "    ⚠️  nginx 安装失败（换源或网络问题）。服务已启动但**仅监听 127.0.0.1:$PORT，外网打不开**。"
    echo "        手动补装后重跑本脚本即可："
    echo "            sudo apt-get update && sudo apt-get install -y nginx"
    echo "        或自管入口：加 --no-nginx 跳过本脚本反代，再用 Cloudflare Tunnel / Caddy 指向 127.0.0.1:$PORT"
  fi
fi
if [[ "$NO_NGINX" != "1" ]] && command -v nginx >/dev/null; then
  echo "==> [6/6] nginx 反代入口: $ENTRY_HOST"
  if [[ -d /etc/nginx/sites-available ]]; then
    NG_CONF=/etc/nginx/sites-available/aiklog
    NG_EN=/etc/nginx/sites-enabled/aiklog
  else
    NG_CONF=/etc/nginx/conf.d/aiklog.conf
    NG_EN="$NG_CONF"
  fi
  $SUDO tee "$NG_CONF" > /dev/null <<EOF
server {
    listen 80;
    server_name $ENTRY_HOST;
    client_max_body_size 200M;
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml text/javascript image/svg+xml;
    location / {
        proxy_pass http://127.0.0.1:$PORT;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;
        proxy_buffering off;
    }
}
EOF
  [[ "$NG_CONF" != "$NG_EN" ]] && $SUDO ln -sf "$NG_CONF" "$NG_EN"
  $SUDO nginx -t && $SUDO systemctl reload nginx
else
  echo "==> [6/6] 跳过 nginx（--no-nginx 或未安装）；服务监听 127.0.0.1:$PORT"
fi

# ---------- 管理员密码 & 汇总 ----------
echo
echo "============================================================"
PW_HINT="$(grep -a '初始管理员密码' "$INSTALL_DIR/log/aiklog.log" | tail -1 || true)"
[[ -n "$PW_HINT" ]] && echo "  初始管理员（仅首次启动打印，请立即抄走）: $PW_HINT" \
                   || echo "  初始管理员密码：首次启动日志已打印过，若遗失请重置数据库后重启"
echo "  入口地址 : http://$ENTRY_HOST  （无域名时 nip.io/sslip.io 临时可用）"
echo "  数据目录 : $INSTALL_DIR/data  （备份这两个：aikmap.db + files/）"
echo "  日志     : $INSTALL_DIR/log/aiklog.log"
echo "  常用命令 : systemctl {status|restart|stop} aiklog"
echo "  HTTPS    : 有域名后建议 certbot --nginx -d $DOMAIN"
echo "============================================================"
