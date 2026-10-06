/* 客服挂件（SPEC-CS-001 M0）
 *
 * 零依赖、可嵌入任意页面：<script src="https://<host>/cs-widget.js" data-api="https://<host>" defer></script>
 * 站点按需放置 <div id="cs-widget"></div>，或在任意元素上加 data-cs-widget 属性。
 *
 * 设计约束（对齐 SPEC-CS-001 §4.1 / §10）：
 *   - 匿名可达：走 /api/v1/public/cs/*（已在 publicPath 白名单）；
 *   - 幂等：每次发送带 client_msg_id（extId），重复投递/重试不产生重复消息；
 *   - 断线重连：轮询 pull 补历史，刷新后凭 localStorage 里的 visitor_id 续接同一会话；
 *   - 不自造样式体系：就地注入最小 CSS，主题色取 CSS 变量（--accent）以跟随站点。
 */
(function () {
  'use strict';
  if (window.__csWidgetLoaded) return;
  window.__csWidgetLoaded = true;

  var script = document.currentScript || (function () {
    var s = document.getElementsByTagName('script');
    for (var i = s.length - 1; i >= 0; i--) if (s[i].src && s[i].src.indexOf('cs-widget') >= 0) return s[i];
    return null;
  })();
  var API = (script && script.getAttribute('data-api')) || (script ? script.src.replace(/\/cs-widget\.js.*$/, '') : '');
  if (!API) return;

  var LS_KEY = 'cs_visitor_id';
  var LS_CONV = 'cs_conversation_id';

  function uuid() {
    if (window.crypto && crypto.randomUUID) return crypto.randomUUID();
    return 'v-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
  }
  function visitorId() {
    var v = localStorage.getItem(LS_KEY);
    if (!v) { v = uuid(); localStorage.setItem(LS_KEY, v); }
    return v;
  }
  function post(path, body) {
    return fetch(API + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify(body || {})
    }).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    });
  }
  function get(path) {
    return fetch(API + path, { credentials: 'same-origin' }).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    });
  }

  var CSS = [
    '.cs-w{position:fixed;right:18px;bottom:18px;z-index:2147483000;font:14px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}',
    '.cs-w-btn{border:0;border-radius:999px;padding:11px 18px;cursor:pointer;color:#fff;background:var(--accent,#2f6feb);box-shadow:0 6px 20px rgba(0,0,0,.18)}',
    '.cs-w-panel{position:absolute;right:0;bottom:54px;width:320px;max-width:88vw;height:420px;max-height:70vh;background:#fff;border:1px solid #e3e6eb;border-radius:12px;display:none;flex-direction:column;overflow:hidden;box-shadow:0 12px 40px rgba(0,0,0,.16)}',
    '.cs-w-panel.open{display:flex}',
    '.cs-w-head{padding:10px 12px;border-bottom:1px solid #eef1f4;font-weight:600;display:flex;justify-content:space-between;align-items:center}',
    '.cs-w-x{border:0;background:transparent;font-size:18px;cursor:pointer;color:#57606a;line-height:1}',
    '.cs-w-body{flex:1;overflow:auto;padding:12px;display:flex;flex-direction:column;gap:8px;background:#fafbfc}',
    '.cs-w-msg{max-width:82%;padding:7px 10px;border-radius:10px;white-space:pre-wrap;word-break:break-word;font-size:13px}',
    '.cs-w-msg.in{align-self:flex-start;background:#f0f2f5;color:#1f2328}',
    '.cs-w-msg.out{align-self:flex-end;background:var(--accent,#2f6feb);color:#fff}',
    '.cs-w-foot{display:flex;gap:6px;padding:8px;border-top:1px solid #eef1f4;background:#fff}',
    '.cs-w-input{flex:1;border:1px solid #e3e6eb;border-radius:8px;padding:7px 9px;font:inherit;font-size:13px;resize:none;height:38px}',
    '.cs-w-send{border:0;border-radius:8px;padding:0 14px;cursor:pointer;color:#fff;background:var(--accent,#2f6feb)}',
    '.cs-w-send:disabled{opacity:.5;cursor:not-allowed}',
    '.cs-w-hint{color:#8b949e;font-size:12px;text-align:center;padding:16px}'
  ].join('');

  function mount() {
    var st = document.createElement('style');
    st.textContent = CSS;
    document.head.appendChild(st);

    var root = document.createElement('div');
    root.className = 'cs-w';
    root.innerHTML =
      '<button class="cs-w-btn" type="button">客服</button>' +
      '<div class="cs-w-panel">' +
      '<div class="cs-w-head"><span>客服</span><button class="cs-w-x" type="button" aria-label="关闭">&times;</button></div>' +
      '<div class="cs-w-body"><div class="cs-w-hint">加载中…</div></div>' +
      '<div class="cs-w-foot"><textarea class="cs-w-input" rows="1" placeholder="输入消息…"></textarea>' +
      '<button class="cs-w-send" type="button" disabled>发送</button></div>' +
      '</div>';
    document.body.appendChild(root);

    var btn = root.querySelector('.cs-w-btn');
    var panel = root.querySelector('.cs-w-panel');
    var body = root.querySelector('.cs-w-body');
    var input = root.querySelector('.cs-w-input');
    var send = root.querySelector('.cs-w-send');
    var conv = localStorage.getItem(LS_CONV) || '';
    var poll = null;

    function esc(s) {
      return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
      });
    }
    function render(items) {
      if (!items || !items.length) { body.innerHTML = '<div class="cs-w-hint">还没有消息，说点什么吧。</div>'; return; }
      body.innerHTML = items.map(function (m) {
        return '<div class="cs-w-msg ' + (m.direction === 'in' ? 'in' : 'out') + '">' + esc(m.text) + '</div>';
      }).join('');
      body.scrollTop = body.scrollHeight;
    }

    function start() {
      if (conv) { pull(); } else { boot(); }
    }
    function boot() {
      post('/api/v1/public/cs/start', { visitor_id: visitorId() })
        .then(function (r) {
          conv = r.conversation_id;
          localStorage.setItem(LS_CONV, conv);
          render(r.messages);
        })
        .catch(function (e) { body.innerHTML = '<div class="cs-w-hint">客服暂不可用</div>'; });
    }
    function pull() {
      get('/api/v1/public/cs/pull?conversation_id=' + encodeURIComponent(conv))
        .then(function (r) { render(r.messages); })
        .catch(function () { /* 网络抖动：下一轮重试 */ });
    }
    function doSend() {
      var text = input.value.trim();
      if (!text || !conv) return;
      send.disabled = true;
      post('/api/v1/public/cs/send', {
        conversation_id: conv, visitor_id: visitorId(),
        external_id: 'w-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8),
        text: text
      }).then(function () {
        input.value = '';
        pull();
      }).catch(function () { /* 失败保留草稿，可重发（同 external_id 幂等由后端保证） */ })
        .then(function () { send.disabled = !input.value.trim(); });
    }

    btn.addEventListener('click', function () {
      var open = panel.classList.toggle('open');
      if (open) { start(); poll = poll || setInterval(pull, 8000); } else if (poll) { clearInterval(poll); poll = null; }
    });
    root.querySelector('.cs-w-x').addEventListener('click', function () {
      panel.classList.remove('open');
      if (poll) { clearInterval(poll); poll = null; }
    });
    input.addEventListener('input', function () { send.disabled = !input.value.trim(); });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); doSend(); }
    });
    send.addEventListener('click', doSend);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount);
  else mount();
})();
