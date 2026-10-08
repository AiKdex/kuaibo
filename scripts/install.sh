#!/usr/bin/env bash
# =============================================================================
# AiKlog 爱库录（kuaibo 快博）· VPS 部署 / 管理脚本
#
# 两种用法：
#
# ① 对话式菜单（推荐新手 / 交互部署）
#   sudo bash scripts/install.sh
#   进入菜单后选 1 安装/升级、2 部署 SSL、3 备份、4 看状态、5 卸载。
#   选 1 后会逐项对话式询问：域名、内部端口、是否自动配 nginx、源码来源。
#
# ② 命令行一次性部署（保留，便于脚本/CI 调用）
#   sudo bash scripts/install.sh --domain blog.example.com
#   sudo bash scripts/install.sh --domain blog.example.com --source git
#   sudo bash scripts/install.sh --port 9000 --no-nginx
#   sudo bash scripts/install.sh --ssl                # 部署/续签 SSL
#   sudo bash scripts/install.sh --backup             # 备份
#   sudo bash scripts/install.sh --status             # 查看状态
#   sudo bash scripts/install.sh --uninstall          # 卸载
#
# 脚本做的事：装依赖 → 构建（前端嵌入 → go build）→ 装到 /opt/aiklog
#             → systemd 托管 → （可选）nginx 反代 → 健康检查 → 打印管理员密码
# 幂等：重复执行安全（自动备份旧二进制与数据库后热替换）。
# =============================================================================
set -euo pipefail

# ---------- 默认值 ----------
DOMAIN=""
PORT="8780"
INSTALL_DIR="/opt/aiklog"
SOURCE=""             # 留空=按场景推断；git | local
GIT_URL="https://github.com/AiKdex/kuaibo.git"
NO_NGINX="0"
GO_VERSION="1.27.1"
ACTION=""            # install | ssl | backup | status | uninstall | （空=菜单）

# ---------- 解析参数 ----------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)    DOMAIN="$2";  shift 2 ;;
    --port)      PORT="$2";    shift 2 ;;
    --dir)       INSTALL_DIR="$2"; shift 2 ;;
    --source)    SOURCE="$2";  shift 2 ;;
    --git-url)   GIT_URL="$2"; shift 2 ;;
    --no-nginx)  NO_NGINX="1"; shift ;;
    --ssl)       ACTION="ssl"; shift ;;
    --backup)    ACTION="backup"; shift ;;
    --status)    ACTION="status"; shift ;;
    --uninstall) ACTION="uninstall"; shift ;;
    --interactive|-i) ACTION="menu"; shift ;;
    -h|--help)   sed -n '3,28p' "$0"; exit 0 ;;
    *) echo "未知参数: $1（输入 -h 看用法）"; exit 1 ;;
  esac
done

# ---------- 权限 ----------
SUDO=""
if [[ "$(id -u)" != "0" ]]; then
  command -v sudo >/dev/null || { echo "错误：非 root 且无 sudo"; exit 1; }
  SUDO="sudo"
fi

# ---------- 对话式小工具 ----------
# prompt <变量名> <提示> <默认值>
prompt() {
  local __var="$1" __q="$2" __def="$3" __ans
  read -r -p "$__q [$__def]: " __ans || __ans=""
  __ans="${__ans:-$__def}"
  printf -v "$__var" '%s' "$__ans"
}
# prompt_yn <变量名> <提示> <默认值(1/0)>
prompt_yn() {
  local __var="$1" __q="$2" __def="$3" __ans
  read -r -p "$__q [$__def]: " __ans || __ans=""
  __ans="${__ans:-$__def}"
  case "$__ans" in y|Y|yes|YES|是|1|true|TRUE) printf -v "$__var" '1';; *) printf -v "$__var" '0';; esac
}

# ---------- 环境探测 ----------
probe_env() {
  echo "==> [0/6] 环境探测"
  OS_ARCH="$(uname -m)"
  if [[ -f /etc/os-release ]]; then . /etc/os-release; fi
  [[ "$OS_ARCH" == "x86_64" ]] && GO_ARCH="amd64" || GO_ARCH="arm64"
  command -v curl >/dev/null || { $SUDO apt-get update -qq && $SUDO apt-get install -y -qq curl; }
  echo "    系统: ${PRETTY_NAME:-unknown}  架构: $OS_ARCH  入口端口: $PORT"
}

# ---------- 依赖：Go ----------
have_go() {
  command -v go >/dev/null || return 1
  local v
  v="$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | head -1)"
  [[ -n "$v" && "$v" > "go1.26" ]]
}
ensure_go() {
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
}

# ---------- 依赖：Node ----------
have_node() { command -v node >/dev/null && [[ "$(node -v | tr -d v | cut -d. -f1)" -ge 18 ]]; }
ensure_node() {
  if ! have_node; then
    echo "==> [1/6] 安装 Node.js 20"
    if curl -fsSL https://deb.nodesource.com/setup_20.x | $SUDO -E bash - 2>/dev/null; then
      $SUDO apt-get install -y -qq nodejs
    else
      $SUDO apt-get update -qq && $SUDO apt-get install -y -qq nodejs npm
    fi
  else
    echo "==> [1/6] Node 已就绪: $(node -v)"
  fi
  npm config set registry https://registry.npmmirror.com 2>/dev/null || true
}

# ---------- 获取源码 ----------
fetch_source() {
  if [[ "$SOURCE" == "git" ]]; then
    echo "==> [2/6] 克隆源码: $GIT_URL"
    REPO_DIR="/tmp/aiklog-src"
    rm -rf "$REPO_DIR"
    # 强制 HTTP/1.1：GitHub 匿名 clone 在部分网络下走 HTTP/2 会被中间设备掐断。
    # 若 clone 仍失败（如境内直连 GitHub 被 reset/超时），自动回退到经 ghproxy 下载源码包。
    if git -c http.version=HTTP/1.1 -c http.postBuffer=524288000 clone --depth 1 "$GIT_URL" "$REPO_DIR" 2>/dev/null; then
      echo "    源码克隆完成"
    else
      echo "    ⚠️  git clone 失败，尝试经 ghproxy 下载源码包（绕开 GitHub 直连限制）"
      if [[ "$GIT_URL" == *"github.com"* ]]; then
        TB_URL="https://ghproxy.com/${GIT_URL%.git}/archive/refs/heads/main.tar.gz"
      else
        TB_URL="${GIT_URL%.git}/archive/refs/heads/main.tar.gz"
      fi
      TGZ="/tmp/kuaibo-src-$$.tar.gz"
      if curl -fsSL --retry 3 --retry-delay 2 -o "$TGZ" "$TB_URL"; then
        mkdir -p "$REPO_DIR"
        tar xzf "$TGZ" -C "$REPO_DIR" --strip-components=1
        rm -f "$TGZ"
        echo "    源码包解压完成（经 ghproxy 代理）"
      else
        echo "错误：源码获取失败（git 与 ghproxy 代理均不可达）。"
        echo "        请在有 GitHub 访问能力的网络下，手动下载源码包并解压到 $REPO_DIR 后重跑本脚本；"
        echo "        或换网络（如手机热点）后重试原命令。"
        exit 1
      fi
    fi
  else
    REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
    [[ -f "$REPO_DIR/server/go.mod" ]] || { echo "错误：未找到源码（$REPO_DIR）。请用 --source git 或在仓库目录内运行。"; exit 1; }
    echo "==> [2/6] 使用本地源码: $REPO_DIR"
  fi
}

# ---------- 构建 ----------
build() {
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
}

# ---------- 安装二进制（含自动备份） ----------
install_binary() {
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
  MKT="$INSTALL_DIR/data/market/index.json"
  if [[ -f "$MKT" ]]; then
    if grep -qE '"signature"[[:space:]]*:' "$MKT" 2>/dev/null; then
      echo "    应用中心: 本地自托管目录（已签名，不回源官方）"
    else
      echo "    ⚠️  应用中心: 检测到**未签名**的本地目录 data/market/index.json"
      echo "        它会静默阻断官方目录回源。想用官方目录（推荐）："
      echo "            sudo mv $MKT $MKT.localbak && sudo systemctl restart aiklog"
    fi
  else
    echo "    应用中心: 无本地目录，将回源官方（默认 aikmap.cn/market/index.json）"
  fi
}

# ---------- systemd ----------
setup_systemd() {
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

  MKT_HTTP="$(curl -s -o /dev/null -m 20 -w '%{http_code}' "http://127.0.0.1:$PORT/market/index.json" || true)"
  if [[ "$MKT_HTTP" == "200" ]]; then
    echo "    应用中心自检: 目录可访问（HTTP 200）"
  else
    echo "    ⚠️  应用中心自检: 目录 HTTP=$MKT_HTTP（不影响站点使用，详见上方提示）"
  fi
}

# ---------- nginx（可选）----------
setup_nginx() {
  PUBLIC_IP="$(curl -s -m 5 ifconfig.me || hostname -I | awk '{print $1}')"
  ENTRY_HOST="${DOMAIN:-${PUBLIC_IP}.nip.io}"
  if [[ "$NO_NGINX" != "1" ]] && ! command -v nginx >/dev/null; then
    echo "==> [6/6] 未检测到 nginx，尝试安装"
    if $SUDO apt-get update -qq && $SUDO apt-get install -y -qq nginx; then
      echo "    nginx 安装完成"
    else
      echo "    ⚠️  nginx 安装失败。服务已启动但**仅监听 127.0.0.1:$PORT，外网打不开**。"
      echo "        手动补装后重跑本脚本；或加 --no-nginx 用 Cloudflare Tunnel / Caddy 指向 127.0.0.1:$PORT"
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
}

# ---------- 汇总 ----------
summarize() {
  PUBLIC_IP="$(curl -s -m 5 ifconfig.me || hostname -I | awk '{print $1}')"
  ENTRY_HOST="${DOMAIN:-${PUBLIC_IP}.nip.io}"
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
}

# ---------- 核心：安装 / 升级 ----------
do_install() {
  [[ -z "$SOURCE" ]] && SOURCE="git"
  probe_env
  ensure_go
  ensure_node
  fetch_source
  build
  install_binary
  setup_systemd
  setup_nginx
  summarize
}

# ---------- 对话式安装 ----------
interactive_install() {
  local existing=0
  [[ -f "$INSTALL_DIR/bin/aiklog" ]] && existing=1
  if [[ "$existing" == "1" ]]; then
    echo "检测到已有实例（$INSTALL_DIR），本次将升级（自动备份后替换二进制）。"
  fi
  probe_env

  local pub_ip ng_dom def_dom
  pub_ip="$(curl -s -m 5 ifconfig.me || hostname -I | awk '{print $1}')"
  ng_dom="$(grep -rhoE 'server_name[[:space:]]+[^;]+;' /etc/nginx/sites-available/aiklog /etc/nginx/conf.d/aiklog.conf 2>/dev/null | head -1 | sed -E 's/server_name[[:space:]]+//; s/;//' | tr -d ' ')"
  def_dom="${ng_dom:-${pub_ip}.nip.io}"

  echo
  echo "---- 安装配置（直接回车采用方括号里的默认值）----"
  prompt DOMAIN "站点域名（无域名可留空，自动用魔法域名 ${pub_ip}.nip.io）" "$def_dom"
  [[ -z "$DOMAIN" ]] && DOMAIN="${pub_ip}.nip.io"

  prompt PORT "内部服务端口（仅本地监听，外网走 nginx 80 端口）" "$PORT"

  local yn=1
  command -v nginx >/dev/null && yn=1 || yn=1
  prompt_yn yn "是否自动配置 nginx 反代（80 端口）？" "$yn"
  NO_NGINX="$((1-yn))"

  local repo_local=0
  [[ -f "$(cd "$(dirname "$0")/.." && pwd)/server/go.mod" ]] && repo_local=1
  local def_src="git"; [[ "$repo_local" == "1" ]] && def_src="local"
  local src
  prompt src "源码来源（git=从 GitHub 克隆 / local=当前目录）" "$def_src"
  SOURCE="$src"

  echo
  echo "配置确认：域名=$DOMAIN  端口=$PORT  nginx=$([[ "$NO_NGINX" == "1" ]] && echo 否 || echo 是)  源码=$SOURCE"
  local go
  prompt_yn go "确认开始安装/升级？" "1"
  [[ "$go" == "1" ]] || { echo "已取消。"; return; }

  ensure_go
  ensure_node
  fetch_source
  build
  install_binary
  setup_systemd
  setup_nginx
  summarize
}

# ---------- 部署 / 续签 SSL ----------
do_ssl() {
  local def_dom="$DOMAIN"
  if [[ -z "$def_dom" ]]; then
    def_dom="$(grep -rhoE 'server_name[[:space:]]+[^;]+;' /etc/nginx/sites-available/aiklog /etc/nginx/conf.d/aiklog.conf 2>/dev/null | head -1 | sed -E 's/server_name[[:space:]]+//; s/;//' | tr -d ' ')"
  fi
  [[ -n "$def_dom" ]] || def_dom="blog.example.com"
  prompt DOMAIN "请输入要申请证书的域名" "$def_dom"

  if ! command -v nginx >/dev/null; then
    echo "错误：未检测到 nginx。请先安装 nginx（可重跑本脚本选 1 自动安装）再申请 SSL。"
    return 1
  fi
  if ! command -v certbot >/dev/null; then
    echo "==> 安装 certbot"
    $SUDO apt-get update -qq && $SUDO apt-get install -y -qq certbot python3-certbot-nginx
  fi
  echo "==> 申请证书并改写 nginx（certbot --nginx -d $DOMAIN）"
  if $SUDO certbot --nginx -d "$DOMAIN"; then
    echo "==> 续期自检"
    $SUDO certbot renew --dry-run || true
    echo "完成：https://$DOMAIN （HTTP 自动跳转 HTTPS，续期由系统定时器托管）"
  else
    echo "错误：certbot 失败。检查：① 域名 A 记录已指向本机公网 IP ② 安全组放行 443/80 ③ 80 端口可访问"
    return 1
  fi
}

# ---------- 备份 ----------
do_backup() {
  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "错误：未找到 $INSTALL_DIR，无法备份（可能尚未安装）。"
    return 1
  fi
  local dest="$INSTALL_DIR/backups"
  local ts="$(date +%Y%m%d%H%M%S)"
  $SUDO mkdir -p "$dest"
  echo "==> 备份实例到 $dest/aiklog-backup-$ts.tar.gz"
  local sz
  sz="$(du -sh "$INSTALL_DIR/data" 2>/dev/null | awk '{print $1}')"
  [[ -n "$sz" ]] && echo "    数据目录大小约 $sz（含上传文件，可能需要一些时间）"
  if $SUDO tar --numeric-owner -czf "$dest/aiklog-backup-$ts.tar.gz" -C "$INSTALL_DIR" bin data 2>/dev/null; then
    $SUDO chmod 600 "$dest/aiklog-backup-$ts.tar.gz"
    echo "    完成: $(ls -lh "$dest/aiklog-backup-$ts.tar.gz" | awk '{print $5}')"
    echo "    恢复方法：解压后把 bin/aiklog 与 data/ 拷回 $INSTALL_DIR，再 systemctl restart aiklog"
  else
    echo "错误：备份失败（磁盘空间不足？）。可仅备份关键文件："
    echo "    sudo cp -a $INSTALL_DIR/bin/aiklog $INSTALL_DIR/data/aikmap.db /tmp/"
    return 1
  fi
}

# ---------- 查看状态 ----------
do_status() {
  echo "==> 实例状态"
  $SUDO systemctl status aiklog --no-pager 2>/dev/null | head -6 || echo "  服务未运行"
  local p
  p="$(grep -oE '127.0.0.1:[0-9]+' /etc/systemd/system/aiklog.service 2>/dev/null | head -1 | cut -d: -f2)"
  p="${p:-$PORT}"
  local h
  h="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$p/api/v1/health" 2>/dev/null || echo 000)"
  echo "  健康检查 (http://127.0.0.1:$p/api/v1/health): $h"
  echo "  安装目录: $INSTALL_DIR"
  echo "  版本: $($SUDO "$INSTALL_DIR/bin/aiklog" -version 2>/dev/null || echo '未知')"
  echo "  数据大小: $(du -sh "$INSTALL_DIR/data" 2>/dev/null | awk '{print $1}')"
  echo "  备份文件: $(ls -1 "$INSTALL_DIR/backups" 2>/dev/null | wc -l) 个"
}

# ---------- 卸载 ----------
do_uninstall() {
  echo "==> 卸载 AiKlog"
  $SUDO systemctl stop aiklog 2>/dev/null || true
  $SUDO systemctl disable aiklog 2>/dev/null || true
  $SUDO rm -f /etc/systemd/system/aiklog.service
  $SUDO systemctl daemon-reload 2>/dev/null || true
  $SUDO rm -f /etc/nginx/sites-available/aiklog /etc/nginx/sites-enabled/aiklog /etc/nginx/conf.d/aiklog.conf
  $SUDO nginx -t >/dev/null 2>&1 && $SUDO systemctl reload nginx 2>/dev/null || true

  local keep=1
  prompt_yn keep "是否保留数据目录 $INSTALL_DIR/data（含文章与上传）？" "1"
  if [[ "$keep" == "1" ]]; then
    echo "    保留数据目录：$INSTALL_DIR/data"
    $SUDO mv "$INSTALL_DIR/bin" "$INSTALL_DIR/bin.removed-$(date +%s)" 2>/dev/null || true
    $SUDO rm -rf "$INSTALL_DIR/dist" "$INSTALL_DIR/log" "$INSTALL_DIR/web" "$INSTALL_DIR/server" "$INSTALL_DIR/backups" 2>/dev/null || true
  else
    echo "    将删除整个 $INSTALL_DIR"
    $SUDO rm -rf "$INSTALL_DIR"
  fi
  echo "完成：服务已停止并禁用，nginx 配置已移除。"
}

# ---------- 菜单 ----------
menu_loop() {
  while true; do
    echo
    echo "=============================================="
    echo "   AiKlog 爱库录 · 部署管理工具"
    echo "=============================================="
    echo "   1) 安装 / 升级系统"
    echo "   2) 部署 / 续签 SSL 证书"
    echo "   3) 备份（数据库 + 上传文件 + 二进制）"
    echo "   4) 查看状态"
    echo "   5) 卸载"
    echo "   0) 退出"
    echo "----------------------------------------------"
    local c
    read -r -p "请选择 [1]: " c || c=""
    c="${c:-1}"
    case "$c" in
      1) interactive_install || true ;;
      2) do_ssl || true ;;
      3) do_backup || true ;;
      4) do_status || true ;;
      5) do_uninstall || true ;;
      0|q|Q) echo "再见。"; exit 0 ;;
      *) echo "无效选择：$c" ;;
    esac
  done
}

# ---------- 入口分发 ----------
if [[ "$ACTION" == "ssl" ]]; then       do_ssl; exit 0; fi
if [[ "$ACTION" == "backup" ]]; then    do_backup; exit 0; fi
if [[ "$ACTION" == "status" ]]; then    do_status; exit 0; fi
if [[ "$ACTION" == "uninstall" ]]; then do_uninstall; exit 0; fi
if [[ "$ACTION" == "menu" ]]; then      menu_loop; exit 0; fi

# 命令行一次性安装：只要给了安装相关参数就直接装（兼容旧用法）
if [[ -n "$DOMAIN" || -n "$SOURCE" || "$NO_NGINX" == "1" || ( -n "$GIT_URL" && "$GIT_URL" != "https://github.com/AiKdex/kuaibo.git" ) ]]; then
  do_install; exit 0
fi

# 默认：对话式菜单
menu_loop
