#!/usr/bin/env bash
# AiKlog 主题交付自检脚本
# 用法: cd web/src/themes/<id> && bash theme-lint.sh [类名前缀]
#       类名前缀可省略（自动推断），如 ef- / brutal- / el-
# 覆盖: 悬空类 / 死 CSS / tokens 台账 / hash 路由锚点 / 高危文案词 / 契约外字段 / 文章页视图隔离 / 平台能力边界
# 依据: 《博客主题开发-常见缺陷清单与自检手册》v1.2 + 规范 v1.3 §7.4（平台能力与主题边界）
#
# 说明: 所有扫描均会跳过注释（HTML 注释 / JS 行注释 / 块注释），
#       避免「解释『不要这样写』的注释本身被当成违规」这类误报。
#
# v1.2 修正: 第 1 项的 CSS 侧改为同时扫描 style.css 与 post.css。
#   规范 v1.2 §0.4 要求「文章页专属样式写进 post.css」，但初版脚本只读 style.css，
#   于是所有按新约定拆分的主题，其文章页类名（模板有、style.css 无）会被整批误报成
#   悬空类（实测 brutal 1.0.1 一次报出 29 个假阳性）。改为 style.css ∪ post.css 后，
#   真·悬空类仍会被抓住，误报消失。第 6 项去掉写死的 .bt-post，改用推断出的前缀。
#
# v1.3 增补（2026-09-17）:
#   - 第 3 项修正误报：`#/blog/tag/...` 是合法 SPA 内页路由，锚点检查排除 `#/` 开头的 hash
#     （只报锚点型 hash，不报路由型）。
#   - 第 5 项扩展：臆造计数字段正则加 .view_count / .viewCount / .pv（阅读量走宿主 PV 接口）。
#   - 新增第 7 项：自建密码框/解锁逻辑检查（规范 v1.3 §7.4——401 闸门与解锁由宿主承担）。
#   - 新增第 8 项：!important 用量检查（站长 custom_css 优先于主题是预期行为，禁全局对抗）。
set -u
PREFIX="${1:-}"
fail=0

say() { printf '\n\033[1m%s\033[0m\n' "$1"; }

# 扫描代码（自动剔除注释），输出 "file:line: content"；$1 = 正则
# 注意：用 ENVIRON 传正则而非 awk -v —— 后者会吞掉反斜杠，使 \.cover 退化成 .cover
scan() {
  RE="$1" awk '
    FNR == 1 { inblk = 0; inhtml = 0 }
    /\/\*/   { inblk = 1 }
    inblk    { if (/\*\//) inblk = 0; next }
    /<!--/   { inhtml = 1 }
    inhtml   { if (/-->/) inhtml = 0; next }
    /^[[:space:]]*(\/\/|\*)/ { next }
    $0 ~ ENVIRON["RE"] { printf "%s:%d: %s\n", FILENAME, FNR, $0 }
  ' *.vue 2>/dev/null
}

# ── 1. 类名交叉比对（悬空类 + 死 CSS）────────────────────────────
say "[1/8] 类名交叉比对"
if [ -z "$PREFIX" ]; then
  PREFIX=$(grep -ho 'class="[^"]*"' *.vue 2>/dev/null \
           | sed 's/class="//;s/"//' | tr ' ' '\n' \
           | grep -o '^[a-z][a-z]*-' | sort | uniq -c | sort -rn | head -1 | awk '{print $2}')
fi
echo "  使用类名前缀: ${PREFIX:-<未能推断，请作为第 1 个参数传入>}"
if [ -n "$PREFIX" ]; then
  # CSS 集合 = style.css ∪ post.css（v1.2 起文章页样式归 post.css，二者同属本主题的样式面）
  { grep -o "\.${PREFIX}[a-z0-9-]*" style.css
    [ -f post.css ] && grep -o "\.${PREFIX}[a-z0-9-]*" post.css
  } | sed 's/^\.//' | sort -u > /tmp/_css.txt
  grep -ho 'class="[^"]*"' *.vue | sed 's/class="//;s/"//' | tr ' ' '\n' \
    | grep "^${PREFIX}" | sort -u > /tmp/_used.txt
  # 注：仅静态 class 属性；:class 动态绑定需人工复核
  empty_used=$(comm -23 /tmp/_used.txt /tmp/_css.txt)
  empty_css=$(comm -13 /tmp/_used.txt /tmp/_css.txt)
  echo "  ── 悬空类（模板有 · CSS 无）:"
  if [ -n "$empty_used" ]; then echo "$empty_used" | sed 's/^/     [X] /'; fail=1; else echo "     [OK] 无"; fi
  echo "  ── 死 CSS（CSS 有 · 模板无）:"
  if [ -n "$empty_css" ]; then echo "$empty_css" | sed 's/^/     [!] /'; else echo "     [OK] 无"; fi
fi

# ── 2. tokens 台账一致性 ──────────────────────────────────────
say "[2/8] manifest.tokens 台账"
if [ -f manifest.js ] && [ -f style.css ]; then
  # 用 [a-z0-9-]+ 而非 *：manifest 注释里写「--th-*」这类通配写法时，
  # 旧正则会把裸前缀 --th- 当成一次声明，报出「声明了但未定义」的幻影虚报。
  grep -o "\-\-th-[a-z0-9-][a-z0-9-]*" manifest.js | sort -u > /tmp/_decl.txt
  grep -o "\-\-th-[a-z0-9-]*:" style.css | sed 's/:$//' | sort -u > /tmp/_def.txt
  miss=$(comm -13 /tmp/_decl.txt /tmp/_def.txt)
  fake=$(comm -23 /tmp/_decl.txt /tmp/_def.txt)
  echo "  ── 定义了但未声明（漏报）:"
  if [ -n "$miss" ]; then echo "$miss" | sed 's/^/     [X] /'; fail=1; else echo "     [OK] 无"; fi
  echo "  ── 声明了但未定义（虚报）:"
  if [ -n "$fake" ]; then echo "$fake" | sed 's/^/     [X] /'; fail=1; else echo "     [OK] 无"; fi
else
  echo "     [!] 未找到 manifest.js 或 style.css，跳过"
fi

# ── 3. hash 路由锚点（硬失败项）────────────────────────────────
# 覆盖两种真实写法：「href="#y-2026"」（原生锚点）与 :href="'#y-' + y"（Vue 绑定拼接）
# v1.3 修正：`#/blog/...` 系合法内页路由（tag/author/cat/search），排除 `#/` 开头，只报锚点型
say "[3/8] hash 路由锚点（宿主为 #/blog，禁止把锚点写进 hash）"
hits=$( { scan 'href=[^[:space:]]*#[^/]'; scan 'location\.hash[[:space:]]*='; } )
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [X] /'; fail=1; else echo "     [OK] 无"; fi

# ── 4. 高危文案词（需人工确认数据来源）──────────────────────────
say "[4/8] 高危文案词（命中不等于错，需人工确认有无字段支撑）"
hits=$(scan '热门|排行|热搜|精选|推荐|浏览量|阅读时长|热度')
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [!] /'; else echo "     [OK] 无"; fi

# ── 5. 契约外字段 ────────────────────────────────────────────
say "[5/8] 契约外字段（posts 条目只有 token/path/preview/created_at/file.*）"
hits=$(scan '\.views|\.likes|\.comments|\.cover|\.summary|\.readTime|\.comnum|\.view_count|\.viewCount|\.pv')
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [!] /'; else echo "     [OK] 无"; fi
echo "     注: file.author 属系统侧待透出字段（规范 §10-10），允许使用，但必须可选链降级。"
echo "         阅读量请走 GET /api/v1/public/blog/pv（规范 v1.3 §7.4），posts 无 view_count。"

# ── 6. 文章页样式视图隔离（通用作用域约定）──────────────────────
# 构建插件（vite-theme-scope）会把 themes/<id>/style.css 加 .th-<id> 作用域（列表+文章共享），
# 把 themes/<id>/post.css 加 .th-<id>.th-view-post 作用域（仅文章视图）。
# 约定：文章专属样式必须写在 post.css，不得留在 style.css，否则会泄漏到列表视图。
say "[6/8] 文章页样式视图隔离（post.css 约定）"
if grep -rl "entries" *.js 2>/dev/null | xargs grep -l "post:" 2>/dev/null | grep -q .; then
  if [ -f post.css ]; then
    echo "     [OK] 声明 entries.post 且存在 post.css（构建插件按视图隔离）"
  else
    echo "     [X] 声明 entries.post 但缺少 post.css（文章页样式必须由 post.css 提供，构建插件才加 .th-view-post 作用域）"
    fail=1
  fi
else
  echo "     [OK] 未声明 entries.post（无文章页样式隔离要求）"
fi
# style.css 不应直接含文章根选择器（会泄漏到列表）；按当前主题前缀动态判断
if [ -n "$PREFIX" ] && grep -nE "\.${PREFIX}post[ .{,]" style.css >/dev/null 2>&1; then
  echo "     [!] style.css 含文章根 .${PREFIX}post 选择器，建议移入 post.css（构建插件会按视图隔离）"
else
  echo "     [OK] style.css 无文章根选择器"
fi

# ── 7. 平台能力边界：自建密码框/解锁逻辑（规范 v1.3 §7.4）────────
# 401 PASSWORD_REQUIRED 闸门与解锁交互由宿主承担；主题拿不到正文时按"内容受保护"空态展示
say "[7/8] 自建密码框/解锁逻辑（宿主专属，主题禁实现）"
hits=$(scan 'type="password"|PASSWORD_REQUIRED|unlock_token|解锁凭证|passwordPlaceholder')
if [ -n "$hits" ]; then
  echo "$hits" | sed 's/^/     [!] /'
  echo "     主题不得自行实现密码输入/解锁流程；若为「内容受保护」空态文案命中「解锁」属误报，人工确认即可"
else
  echo "     [OK] 无"
fi

# ── 8. 平台能力边界：!important 用量（站长 custom_css 优先是预期行为）──
# 站长级 custom_css 注入在主题样式之后，主题优先级低于站长属预期；大量 !important = 全局对抗
say "[8/8] !important 用量（style.css ∪ post.css）"
imp_n=$( { grep -o '!important' style.css post.css 2>/dev/null; } | wc -l)
if [ "$imp_n" -gt 10 ]; then
  echo "     [!] 共 ${imp_n} 处 !important —— 超过 10 处视为与宿主/站长样式对抗，须改用提高选择器特异性"
  echo "     （个别覆盖第三方渲染产物的合理使用可人工豁免）"
elif [ "$imp_n" -gt 0 ]; then
  echo "     [OK] ${imp_n} 处（少量，可接受）"
else
  echo "     [OK] 无"
fi

say "== 结论 =="
if [ "$fail" -eq 0 ]; then
  echo "[OK] 自动检查项全部通过（[!] 项仍需人工确认）"
else
  echo "[X] 存在必须修复项，见上方标记"
fi
exit "$fail"
