// blog_market.go 站长博客侧市场 + 公开 /market 门户（移植自上游 AiKmap blog_market.go，2026-09-18 上游回执 B2 正式授权；上游沿革说明保留）。
//
//	GET  /api/v1/blog/market              站长博客侧市场列表（与 /api/v1/admin/apps/market 同构同管线）
//	POST /api/v1/blog/market/install      站长博客侧一键安装（同上）
//	GET  /market                          公开市场门户（无需登录浏览；安装引导登录后台）
//	GET  /market/                         同上（目录形式）
//	GET  /market/index.json               公开市场索引（本实例合并视图：installed/applicable 标记）
//	GET  /market/official/{file}          本地官方包静态服务（可选自托管 zip）
//
// 品牌铁律：门户面向用户，一律「爱库录 / AiKlog」，不得出现上游品牌词（AiKmap / Knowledge Map）。
package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// marketIndexFile GET /market/index.json：公开市场索引（供第三方/读者直接拉取，不依赖登录）。
// AiKlog 市场源=上游统一索引（plugin_market.index_url，默认 https://aikmap.cn/market/index.json），
// 此处输出本实例合并视图（installed/applicable 标记），仅含目录本体（不带实例侧元数据）。
// 降级策略：远程不可达时优先用缓存；全无缓存输出空索引 200（保证门户页可开，不整页挂掉）。
func (a *API) marketIndexFile(w http.ResponseWriter, r *http.Request) {
	u := a.marketIndexURL()
	if time.Since(marketCache.at) < 5*time.Minute && marketCache.index != nil && marketCache.url == u {
		writeMarketIndexView(w, a.marketView(marketCache.index))
		return
	}
	idx, err := a.fetchMarketIndex(r)
	if err != nil {
		if marketCache.index != nil && marketCache.url == u {
			writeMarketIndexView(w, a.marketView(marketCache.index))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*") // 索引公开；壳端/第三方跨域拉取
		w.Write([]byte(`{"schema_version":"1","plugins":[],"themes":[]}`))
		return
	}
	marketCache.index = idx
	marketCache.at = time.Now()
	marketCache.url = u
	writeMarketIndexView(w, a.marketView(idx))
}

// writeMarketIndexView 输出门户索引（精简：schema_version + plugins/themes 空数组兜底）。
func writeMarketIndexView(w http.ResponseWriter, view map[string]any) {
	plugins, _ := view["plugins"].([]marketPlugin)
	themes, _ := view["themes"].([]marketTheme)
	third, _ := view["third_party"].([]marketListing)
	if plugins == nil {
		plugins = []marketPlugin{}
	}
	if themes == nil {
		themes = []marketTheme{}
	}
	if third == nil {
		third = []marketListing{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": view["schema_version"],
		"plugins":        plugins,
		"themes":         themes,
		"third_party":    third,
	})
}

// marketPortal GET /market：公开市场门户（无需登录浏览；安装引导登录后进后台应用中心）。
func (a *API) marketPortal(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(marketPortalHTML))
}

// marketPortalHTML 门户静态页（AiKlog 品牌版，基于上游门户结构改造：去 Agent 工具区块、安装引导指向本壳后台）。
const marketPortalHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>爱库录 AiKlog 应用中心</title>
<style>
:root{--bg:#0f1117;--card:#181b24;--card2:#1f2330;--fg:#e7e9ef;--dim:#9aa1b5;--accent:#4f8cff;--ok:#3ddc97;--paid:#ffb454;--bd:#2a2f3f}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--fg);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;line-height:1.6;min-height:100vh}
.wrap{max-width:1080px;margin:0 auto;padding:40px 20px 80px}
header{text-align:center;padding:24px 0 8px}
header .logo{font-size:30px;font-weight:800;letter-spacing:.5px}
header .logo span{color:var(--accent)}
header .sub{color:var(--dim);margin-top:8px;font-size:14px}
.badge{display:inline-block;padding:2px 10px;border-radius:99px;font-size:12px;font-weight:600;vertical-align:middle}
.badge-free{background:rgba(61,220,151,.14);color:var(--ok)}
.badge-paid{background:rgba(255,180,84,.14);color:var(--paid)}
.badge-target{background:rgba(79,140,255,.12);color:var(--accent);margin-left:6px}
.note{text-align:center;color:var(--dim);font-size:13px;margin:14px 0 26px}
.note a{color:var(--accent);text-decoration:none}
section{margin-top:30px}
section h2{font-size:18px;margin-bottom:14px;display:flex;align-items:center;gap:8px}
section h2 .cnt{color:var(--dim);font-weight:400;font-size:14px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:14px}
.card{background:var(--card);border:1px solid var(--bd);border-radius:12px;padding:18px;display:flex;flex-direction:column;gap:8px}
.card h3{font-size:15px;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.card p.desc{color:var(--dim);font-size:13px;flex:1}
.card .meta{font-size:12px;color:var(--dim);display:flex;gap:12px;flex-wrap:wrap}
.card .actions{margin-top:4px}
.btn{display:inline-block;background:var(--accent);color:#fff;border:none;border-radius:8px;padding:8px 16px;font-size:13px;font-weight:600;text-decoration:none;cursor:pointer}
.btn:hover{opacity:.88}
.empty{color:var(--dim);font-size:14px;padding:12px 2px}
footer{text-align:center;color:var(--dim);font-size:12px;margin-top:60px}
.modal{display:none;position:fixed;inset:0;background:rgba(8,10,16,.72);z-index:50;padding:30px 16px;overflow:auto}
.modal.open{display:block}
.modal .box{max-width:720px;margin:20px auto;background:var(--card2);border:1px solid var(--bd);border-radius:14px;padding:24px}
.modal .box h3{font-size:18px;margin-bottom:4px;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.modal .box .x{float:right;cursor:pointer;color:var(--dim);font-size:20px;line-height:1;padding:2px 6px}
.modal .box .x:hover{color:var(--fg)}
.modal .box .meta{color:var(--dim);font-size:13px;margin:4px 0 12px}
.modal .box .rm{font-size:14px;line-height:1.75;word-break:break-word}
.modal .box .rm h1,.modal .box .rm h2,.modal .box .rm h3{margin:14px 0 6px;font-size:15px}
.modal .box .rm p{margin:8px 0}
.modal .box .rm code{background:rgba(255,255,255,.08);padding:1px 6px;border-radius:4px;font-size:13px}
.modal .box .rm pre{background:rgba(0,0,0,.35);padding:12px;border-radius:8px;overflow:auto;font-size:13px}
.modal .box .rm pre code{background:none;padding:0}
.modal .box .rm ul,.modal .box .rm ol{margin:8px 0 8px 22px}
.modal .box .rm a{color:var(--accent)}
.modal .box .install{margin-top:16px}
@media(max-width:640px){.grid{grid-template-columns:1fr}.wrap{padding:24px 14px 60px}}
</style>
</head>
<body>
<div class="wrap">
<header>
<div class="logo">爱库录 AiKlog <span>应用中心</span></div>
<div class="sub">插件 · 主题 · AI 知识库博客生态应用</div>
</header>
<div class="note">公开浏览无需登录；安装请<a href="/app#/settings?tab=apps" target="_blank" rel="noopener">登录后台 → 设置 → 应用</a>（一键安装，免编译生效）。</div>

<section id="sec-plugins"><h2>插件 <span class="cnt" id="c-plugins"></span></h2><div class="grid" id="g-plugins"></div></section>
<section id="sec-themes"><h2>主题 <span class="cnt" id="c-themes"></span></h2><div class="grid" id="g-themes"></div></section>

<div class="modal" id="mdl" onclick="if(event.target===this)closeDetail()"><div class="box" id="mdlBox"></div></div>
<footer>爱库录 AiKlog · 市场索引 <code>/market/index.json</code></footer>
</div>
<script>
function esc(s){return (s||'').toString().replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];});}
function goInstall(){window.open('/app#/settings?tab=apps','_blank');}
function md(s){
  s=(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  var lines=s.split('\n'),out=[],inpre=false;
  for(var i=0;i<lines.length;i++){
    var l=lines[i];
    if(/^\u0060\u0060\u0060/.test(l)){if(inpre){out.push('</pre>');inpre=false;}else{out.push('<pre><code>');inpre=true;}continue;}
    if(inpre){out.push(esc(l));continue;}
    l=l.replace(/^###\s+/, '<h3>').replace(/^##\s+/, '<h2>').replace(/^#\s+/, '<h1>');
    if(/^<h[123]>/.test(l)){out.push(l);continue;}
    if(/^-\s+/.test(l)){out.push('<li>'+l.replace(/^-\s+/,'')+'</li>');continue;}
    if(/^\d+\.\s+/.test(l)){out.push('<li>'+l.replace(/^\d+\.\s+/,'')+'</li>');continue;}
    if(l===''){out.push('</ul>');continue;}
    l=l.replace(/\u0060([^\u0060]+)\u0060/g,'<code>$1</code>').replace(/\*\*([^*]+)\*\*/g,'<b>$1</b>');
    out.push('<p>'+l+'</p>');
  }
  if(inpre)out.push('</pre>');
  return out.join('\n').replace(/<\/ul><p><\/p><ul>/g,'').replace(/<\/ul><p><\/p>/g,'').replace(/<p><\/p>/g,'');
}
function showDetail(o){
  var rm=o.readme||'（该条目暂未提供使用说明）';
  if(/^https?:\/\//.test(rm)){window.open(rm,'_blank');return;}
  var target=(o.target||[]).length?o.target.join(', '):'全壳兼容';
  var tier=o.tier==='paid'?'付费':'免费';
  document.getElementById('mdlBox').innerHTML='<span class="x" onclick="closeDetail()">×</span><h3>'+esc(o.name)+' <span class="badge '+(o.tier==='paid'?'badge-paid':'badge-free')+'">'+tier+'</span></h3>'+
    '<div class="meta">作者 '+esc(o.author||'-')+' · v'+esc(o.version||'-')+' · 适用 '+esc(target)+'</div>'+
    '<div class="rm">'+md(rm)+'</div>'+
    (o.homepage?'<div class="install"><a class="btn" href="'+esc(o.homepage)+'" target="_blank">文档主页</a> ':'')+
    '<a class="btn" href="/app#/settings?tab=apps" target="_blank">去安装</a></div>';
  document.getElementById('mdl').classList.add('open');
}
function closeDetail(){document.getElementById('mdl').classList.remove('open');}
function card(it){
  var target=(it.target||[]).length?it.target.join(', '):'全壳兼容';
  var tier=it.tier==='paid'?'<span class="badge badge-paid">付费</span>':'<span class="badge badge-free">免费</span>';
  return '<div class="card" style="cursor:pointer" onclick="showDetail('+esc(JSON.stringify({id:it.id,name:it.name,version:it.version,author:it.author,tier:it.tier,target:it.target,readme:it.readme,homepage:it.homepage}))+')"><h3>'+esc(it.name)+' '+tier+'<span class="badge badge-target">'+esc(target)+'</span></h3>'+
    '<p class="desc">'+esc(it.description||'')+'</p>'+
    '<div class="meta"><span>作者 '+(it.author||'-')+'</span><span>v'+(it.version||'-')+'</span></div>'+
    '<div class="actions"><a class="btn" href="/app#/settings?tab=apps" target="_blank" onclick="event.stopPropagation()">去安装</a></div></div>';
}
fetch('/market/index.json').then(function(r){return r.json();}).then(function(d){
  var plugins=d.plugins||[],themes=d.themes||[];
  document.getElementById('c-plugins').textContent='('+plugins.length+')';
  document.getElementById('c-themes').textContent='('+themes.length+')';
  document.getElementById('g-plugins').innerHTML=plugins.length?plugins.map(card).join(''):'<div class="empty">暂无插件</div>';
  document.getElementById('g-themes').innerHTML=themes.length?themes.map(card).join(''):'<div class="empty">暂无主题</div>';
}).catch(function(err){
  document.getElementById('g-plugins').innerHTML='<div class="empty">市场索引加载失败：'+esc(err.message)+'</div>';
});
</script>
</body>
</html>`

// officialMarketFile GET /market/official/{file}：本地官方包静态服务（zip；AiKlog 默认不分发自托管包，
// 下载地址均指上游统一索引；保留本端点用于未来自托管，配置 market.official_dir 可指定目录）。
func (a *API) officialMarketFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !strings.HasSuffix(name, ".zip") || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		writeErr(w, http.StatusBadRequest, "BAD_FILE", "非法文件名")
		return
	}
	dir := strings.TrimSpace(a.cfg.GetString("market.official_dir"))
	if dir == "" {
		dir = "market/official"
	}
	p := filepath.Join(dir, name)
	absDir, _ := filepath.Abs(dir)
	absP, _ := filepath.Abs(p)
	if !strings.HasPrefix(absP, absDir+string(os.PathSeparator)) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "文件不存在")
		return
	}
	if _, err := os.Stat(absP); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "文件不存在")
		return
	}
	http.ServeFile(w, r, absP)
}
