#!/usr/bin/env bash
# AiKlog 主题交付自检脚本
# 用法: cd web/src/themes/<id> && bash theme-lint.sh [类名前缀]
#       类名前缀可省略（自动推断），如 ef- / brutal- / el-
# 覆盖: 悬空类 / 死 CSS / tokens 台账 / hash 路由锚点 / 高危文案词 / 契约外字段
# 依据: 《博客主题开发-常见缺陷清单与自检手册》v1.0
#
# 说明: 所有扫描均会跳过注释（HTML 注释 / JS 行注释 / 块注释），
#       避免「解释『不要这样写』的注释本身被当成违规」这类误报。
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
say "[1/5] 类名交叉比对"
if [ -z "$PREFIX" ]; then
  PREFIX=$(grep -ho 'class="[^"]*"' *.vue 2>/dev/null \
           | sed 's/class="//;s/"//' | tr ' ' '\n' \
           | grep -o '^[a-z][a-z]*-' | sort | uniq -c | sort -rn | head -1 | awk '{print $2}')
fi
echo "  使用类名前缀: ${PREFIX:-<未能推断，请作为第 1 个参数传入>}"
if [ -n "$PREFIX" ]; then
  grep -o "\.${PREFIX}[a-z0-9-]*" style.css | sed 's/^\.//' | sort -u > /tmp/_css.txt
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
say "[2/5] manifest.tokens 台账"
if [ -f manifest.js ] && [ -f style.css ]; then
  grep -o "\-\-th-[a-z0-9-]*" manifest.js | sort -u > /tmp/_decl.txt
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
say "[3/5] hash 路由锚点（宿主为 #/blog，禁止把锚点写进 hash）"
hits=$( { scan 'href=[^[:space:]]*#'; scan 'location\.hash[[:space:]]*='; } )
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [X] /'; fail=1; else echo "     [OK] 无"; fi

# ── 4. 高危文案词（需人工确认数据来源）──────────────────────────
say "[4/5] 高危文案词（命中不等于错，需人工确认有无字段支撑）"
hits=$(scan '热门|排行|热搜|精选|推荐|浏览量|阅读时长|热度')
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [!] /'; else echo "     [OK] 无"; fi

# ── 5. 契约外字段 ────────────────────────────────────────────
say "[5/5] 契约外字段（posts 条目只有 token/path/preview/created_at/file.*）"
hits=$(scan '\.views|\.likes|\.comments|\.cover|\.summary|\.readTime|\.comnum')
if [ -n "$hits" ]; then echo "$hits" | sed 's/^/     [!] /'; else echo "     [OK] 无"; fi
echo "     注: file.author 属系统侧待透出字段（规范 §10-10），允许使用，但必须可选链降级。"

say "== 结论 =="
if [ "$fail" -eq 0 ]; then
  echo "[OK] 自动检查项全部通过（[!] 项仍需人工确认）"
else
  echo "[X] 存在必须修复项，见上方标记"
fi
exit "$fail"
