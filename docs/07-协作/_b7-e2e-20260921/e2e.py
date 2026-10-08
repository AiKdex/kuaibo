#!/usr/bin/env python3
# B7（IM 绑定）+ B8（媒体元信息）+ B9（技能包）隔离端到端验证。
# 只打沙箱 127.0.0.1:8781，绝不触碰生产 8780。
import json
import sqlite3
import struct
import time
import urllib.error
import urllib.request
import uuid
import zlib

BASE = "http://127.0.0.1:8781/api/v1"
DB = "/root/b7e2e/data/aikmap.db"
PW = "Aiklog@ImTest2026"

RESULTS = []


def check(name, cond, detail=""):
    RESULTS.append((name, bool(cond)))
    print(("  [PASS] " if cond else "  [FAIL] ") + name + ("" if cond else "   -> got: " + str(detail)))


def call(method, path, token=None, body=None, raw=None, ctype=None):
    data = None
    headers = {}
    if raw is not None:
        data = raw
        headers["Content-Type"] = ctype or "application/octet-stream"
    elif body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            b = r.read().decode("utf-8", "replace")
            try:
                return r.status, json.loads(b)
            except Exception:
                return r.status, b
    except urllib.error.HTTPError as e:
        b = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(b)
        except Exception:
            return e.code, b


def errcode(js):
    if isinstance(js, dict) and isinstance(js.get("error"), dict):
        return js["error"].get("code")
    return None


def login(u, p):
    st, js = call("POST", "/auth/login", body={"username": u, "password": p})
    return js.get("token") if isinstance(js, dict) else None


def db(sql, args=()):
    con = sqlite3.connect(DB)
    try:
        return con.execute(sql, args).fetchall()
    finally:
        con.close()


def png(w, h):
    """生成一张真 PNG（IHDR 里带真实宽高，供 header 级探测）。"""
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    ihdr = struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)
    raw = b"".join(b"\x00" + b"\x00\x00\x00" * w for _ in range(h))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def up_id(js):
    """从 /files/upload 的 {"results":[{"file":{...}}]} 里取文件 id。"""
    if isinstance(js, dict):
        for r in (js.get("results") or []):
            f = r.get("file") or {}
            if f.get("id"):
                return f.get("id")
    return None


def upload(token, filename, content, ctype, parent=""):
    boundary = "----b7e2e" + uuid.uuid4().hex
    parts = []
    parts.append(("--" + boundary + "\r\n").encode())
    parts.append(('Content-Disposition: form-data; name="file"; filename="%s"\r\n' % filename).encode())
    parts.append(("Content-Type: %s\r\n\r\n" % ctype).encode())
    parts.append(content)
    parts.append(("\r\n--" + boundary + "--\r\n").encode())
    return call("POST", "/files/upload?parent=" + parent, token, raw=b"".join(parts),
                ctype="multipart/form-data; boundary=" + boundary)


print("=== 0 登录与准备 ===")
admin = login("admin", PW)
bob = login("e2e_b7_bob", PW)
carol = login("e2e_b7_carol", PW)
check("admin/bob/carol 三会话签发", all([admin, bob, carol]), [bool(admin), bool(bob), bool(carol)])
BOB_ID = db("select id from users where username='e2e_b7_bob'")[0][0]
CAROL_ID = db("select id from users where username='e2e_b7_carol'")[0][0]
st, js = call("PUT", "/im/telegram/config", admin, body={"bot_token": "sandbox-fake-token-7"})
check("沙箱开启 telegram（写沙箱库）", st == 200 and js.get("telegram_enabled") is True, (st, js))

print("=== 1 未鉴权拦截（新端点全部 401）===")
for m, p in (("GET", "/im/bindings"), ("POST", "/im/bindings/code"), ("DELETE", "/im/bindings/xxx"),
             ("GET", "/skills"), ("GET", "/skills/grants"), ("POST", "/skills/x/claim"),
             ("POST", "/admin/skills"), ("POST", "/admin/skills/x/grant"),
             ("GET", "/files/nonexistent/media")):
    st, js = call(m, p, None, body={} if m == "POST" else None)
    check("匿名 %s %s -> 401" % (m, p), st == 401, (st, js))

print("=== 2 B7 绑定码生成 ===")
st, js = call("POST", "/im/bindings/code", bob, body={"platform": "telegram"})
code1 = js.get("code") if isinstance(js, dict) else None
check("生成绑定码 200", st == 200 and bool(code1), (st, js))
ALPHA = set("ABCDEFGHJKMNPQRSTUVWXYZ23456789")
check("码长 6 且只含去混字符集（无 I/L/O/0/1）",
      code1 and len(code1) == 6 and set(code1) <= ALPHA, code1)
check("TTL=600s", js.get("ttl_seconds") == 600, js.get("ttl_seconds"))
check("hint 含 /bind 指令", isinstance(js.get("hint"), str) and "/bind " in js["hint"], js.get("hint"))
row = db("select user_id, platform, used_at, expires_at from im_binding_codes where code=?", (code1,))
now_ms = int(time.time() * 1000)  # 用 Python 毫秒时钟；SQLite strftime 只到秒会引入 ~1s 向下取整偏差
check("码落库且归属 bob、未使用（used_at=0）",
      row and row[0][0] == BOB_ID and row[0][1] == "telegram" and row[0][2] == 0, row)
check("过期时间约 TTL 之后（毫秒口径，容忍时钟取整 ±2s）",
      row and 9 * 60 * 1000 < (row[0][3] - now_ms) <= 10 * 60 * 1000 + 2000,
      row[0][3] - now_ms if row else None)
st, js = call("POST", "/im/bindings/code", bob, body={"platform": "dingtalk"})
check("非法平台 -> 400 IM_BAD_PLATFORM", st == 400 and errcode(js) == "IM_BAD_PLATFORM", (st, js))
st, js = call("POST", "/im/bindings/code", bob, body={})
code_np = js.get("code") if isinstance(js, dict) else None
check("platform 可空（任意平台码）", st == 200 and bool(code_np), (st, js))
check("换平台不复用即不互相作废（同平台才替换）",
      db("select count(*) from im_binding_codes where code=?", (code1,))[0][0] == 1, "code1 被误删")
st, js = call("POST", "/im/bindings/code", bob, body={"platform": "telegram"})
code1b = js.get("code")
check("同平台再生成 -> 旧码作废（used_at 置位）",
      db("select count(*) from im_binding_codes where code=? and used_at=0", (code1,))[0][0] == 0,
      db("select code, used_at from im_binding_codes where user_id=? and platform='telegram'", (BOB_ID,)))

print("=== 3 B7 IM 端 /bind 消费 + 身份注入 ===")


def tg_update(mid, uid, text, chat=None):
    return json.dumps({
        "update_id": mid,
        "message": {"message_id": mid, "from": {"id": uid, "first_name": "B7User"},
                    "chat": {"id": chat or uid}, "text": text},
    }).encode()


U1 = 770001
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9001, U1, "/bind " + code_np))
check("IM 端 /bind 消费成功（webhook 匿名可打）", st == 200 and js.get("handled") is True, (st, js))
row = db("select user_id, platform, platform_user_id, chat_id from im_bindings where platform_user_id=?", (str(U1),))
check("绑定落库指向 bob（身份注入成功）", row and row[0][0] == BOB_ID and row[0][1] == "telegram", row)
check("chat_id 一并记录", row and row[0][3] == str(U1), row)
check("码标记已用（used_at 非空）",
      db("select used_at from im_binding_codes where code=?", (code_np,))[0][0] != 0, "used_at 仍为 0")
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9002, U1, "/bind " + code_np))
check("同码二次消费被拒（一次性，handled=false）", st == 200 and js.get("handled") is False, (st, js))
check("二次消费不改动绑定",
      db("select user_id from im_bindings where platform_user_id=?", (str(U1),))[0][0] == BOB_ID, "绑定被改")

st, js = call("GET", "/im/bindings", bob)
items = js.get("items") or [] if isinstance(js, dict) else []
check("bob 看到自己的 1 条绑定", len(items) == 1 and items[0].get("platform") == "telegram", items)
check("平台可用状态随配置（telegram enabled）",
      isinstance(js.get("platforms"), dict) and js["platforms"]["telegram"]["enabled"] is True, js.get("platforms"))
st, js = call("GET", "/im/bindings", carol)
check("carol 看不到 bob 的绑定（用户隔离）", not (js.get("items") or []), js)

print("=== 4 B7 平台错配 / 改绑 / 解绑 / 非法码 ===")
st, js = call("POST", "/im/bindings/code", carol, body={"platform": "wecom"})
code_wecom = js.get("code")
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9050, U1, "/bind " + code_wecom))
check("限定平台码在错配平台消费被拒（handled=false）", st == 200 and js.get("handled") is False, (st, js))
check("错配消费未产生任何绑定改动",
      db("select user_id from im_bindings where platform_user_id=?", (str(U1),))[0][0] == BOB_ID, "绑定被改")

st, js = call("POST", "/im/bindings/code", carol, body={"platform": "telegram"})
code_carol = js.get("code")
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9003, U1, "/bind " + code_carol))
rows = db("select user_id from im_bindings where platform_user_id=?", (str(U1),))
check("同 IM 身份改绑 carol -> 只有 1 行且归属 carol",
      len(rows) == 1 and rows[0][0] == CAROL_ID, rows)

st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9004, U1, "/unbind"))
check("IM 端 /unbind 解除绑定", st == 200 and js.get("handled") is True, (st, js))
check("解绑后行被删除", db("select count(*) from im_bindings where platform_user_id=?", (str(U1),))[0][0] == 0,
      "仍有行")

st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9005, U1, "/unbind"))
check("未绑定时 /unbind 有提示不报错", st == 200 and js.get("handled") is True, (st, js))

st, js = call("POST", "/im/bindings/code", bob, body={"platform": "telegram"})
code2 = js.get("code")
call("POST", "/im/webhook/telegram", None, raw=tg_update(9006, U1, "/bind " + code2))
check("重新绑定成功（可逆）",
      db("select user_id from im_bindings where platform_user_id=?", (str(U1),))[0][0] == BOB_ID, "未绑上")
before = db("select user_id from im_bindings where platform_user_id=?", (str(U1),))[0][0]
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9007, U1, "/bind ZZZZZZ"))
check("不存在的码被拒（handled=false）", st == 200 and js.get("handled") is False, (st, js))
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9008, U1, "/bind"))
check("缺参 /bind 走用法提示（handled=false）", st == 200 and js.get("handled") is False, (st, js))
check("被拒的三次均未改动绑定",
      db("select user_id from im_bindings where platform_user_id=?", (str(U1),))[0][0] == before, "绑定被改")

st, js = call("GET", "/im/bindings", bob)
bid = (js.get("items") or [{}])[0].get("id")
st, js = call("DELETE", "/im/bindings/" + str(bid), carol)
check("越权删除他人绑定 -> 404", st == 404 and errcode(js) == "IM_BIND_NOT_FOUND", (st, js))
st, js = call("DELETE", "/im/bindings/" + str(bid), bob)
check("本人删除绑定 200", st == 200 and js.get("ok") is True, (st, js))
check("删除后无残留", db("select count(*) from im_bindings where user_id=?", (BOB_ID,))[0][0] == 0, "仍有行")

print("=== 5 B7 未绑定回退 owner（零破坏）===")
st, js = call("POST", "/im/webhook/telegram", None, raw=tg_update(9010, 770999, "/status"))
check("未绑定 IM 身份仍可 /status（回退 owner，不报错）", st == 200 and js.get("handled") is True, (st, js))

print("=== 6 B8 媒体元信息（图片 header 级探测）===")
st, js = upload(admin, "b7-e2e-pic.png", png(120, 45), "image/png")
fid = up_id(js)
check("上传 PNG 成功", st == 200 and bool(fid), (st, js))
st, js = call("GET", "/files/%s/media" % fid, admin)
check("媒体探测 200 且 probe_status=ok", st == 200 and js.get("probe_status") == "ok", (st, js))
check("宽高正确（120x45）", (js.get("width"), js.get("height")) == (120, 45), (js.get("width"), js.get("height")))
check("format=png", js.get("codec") == "png", js.get("codec"))
row = db("select probe_status, width, height from file_media where file_id=?", (fid,))
check("落库 file_media 一行", row and row[0][0] == "ok" and row[0][1] == 120, row)
st, js2 = call("GET", "/files/%s/media" % fid, admin)
check("二次调用命中缓存（值一致）", st == 200 and js2.get("width") == 120 and js2.get("height") == 45, js2)

st, js = upload(admin, "b7-e2e-note.txt", "hello b7 e2e".encode(), "text/plain")
fid_txt = up_id(js)
st, js = call("GET", "/files/%s/media" % fid_txt, admin)
check("非图片 -> unsupported（音视频字段留空）",
      st == 200 and js.get("probe_status") == "unsupported" and js.get("width") == 0, (st, js))
st, js = call("GET", "/files/does-not-exist/media", admin)
check("不存在的文件 -> 404 FILE_NOT_FOUND", st == 404 and errcode(js) == "FILE_NOT_FOUND", (st, js))

print("=== 7 B9 技能包目录与领取 ===")
st, js = call("POST", "/admin/skills", bob, body={"name": "e2e_forbidden", "kind": "tool"})
check("非管理员上架 -> 403 FORBIDDEN", st == 403 and errcode(js) == "FORBIDDEN", (st, js))

st, js = call("POST", "/admin/skills", admin,
              body={"name": "e2e_free", "version": "1.0.0", "kind": "tool", "title": "E2E 免费包",
                    "description": "免费自助领取", "status": "published", "price_cents": 0, "license": "MIT"})
FREE_ID = js.get("id") if isinstance(js, dict) else None
check("管理员上架免费包 200", st == 200 and bool(FREE_ID), (st, js))
st, js = call("POST", "/admin/skills", admin,
              body={"name": "e2e_paid", "version": "1.0.0", "kind": "skill", "title": "E2E 付费包",
                    "status": "published", "price_cents": 1990, "license": "COMMERCIAL"})
PAID_ID = js.get("id") if isinstance(js, dict) else None
check("管理员上架付费包 200", st == 200 and bool(PAID_ID), (st, js))
st, js = call("POST", "/admin/skills", admin,
              body={"name": "e2e_draft", "version": "1.0.0", "kind": "workflow", "title": "E2E 草稿包",
                    "status": "draft", "price_cents": 0})
DRAFT_ID = js.get("id") if isinstance(js, dict) else None
check("管理员上架草稿包 200", st == 200 and bool(DRAFT_ID), (st, js))
st, js = call("POST", "/admin/skills", admin, body={"name": "e2e_badkind", "kind": "nope"})
check("非法 kind -> 400 SKILL_BAD_KIND", st == 400 and errcode(js) == "SKILL_BAD_KIND", (st, js))
st, js = call("POST", "/admin/skills", admin, body={"name": "", "kind": "tool"})
check("空 name -> 400 SKILL_BAD_NAME", st == 400 and errcode(js) == "SKILL_BAD_NAME", (st, js))

st, js = call("GET", "/skills", bob)
items = js.get("items") or [] if isinstance(js, dict) else []
ids = [i.get("id") for i in items]
check("目录只含 published（草稿不外露）", FREE_ID in ids and PAID_ID in ids and DRAFT_ID not in ids, ids)
free_item = next((i for i in items if i.get("id") == FREE_ID), {})
check("未领取时 granted=false", free_item.get("granted") is False, free_item.get("granted"))
st, js = call("GET", "/skills?kind=tool", bob)
check("kind 过滤生效", all(i.get("kind") == "tool" for i in (js.get("items") or [])), js)
st, js = call("GET", "/skills?kind=zzz", bob)
check("非法 kind 过滤 -> 400 SKILL_BAD_KIND", st == 400 and errcode(js) == "SKILL_BAD_KIND", (st, js))

st, js = call("POST", "/skills/%s/claim" % DRAFT_ID, bob)
check("领取草稿包 -> 403 SKILL_NOT_PUBLISHED", st == 403 and errcode(js) == "SKILL_NOT_PUBLISHED", (st, js))
st, js = call("POST", "/skills/%s/claim" % PAID_ID, bob)
check("领取付费包 -> 402 SKILL_NOT_FREE", st == 402 and errcode(js) == "SKILL_NOT_FREE", (st, js))
st, js = call("POST", "/skills/%s/claim" % FREE_ID, bob)
check("领取免费包 200", st == 200 and js.get("ok") is True, (st, js))
st, js = call("POST", "/skills/%s/claim" % FREE_ID, bob)
check("重复领取幂等（仍 200）", st == 200, (st, js))
check("授权落库且 grantee=user/bob",
      db("select grantee_type, grantee_id, status from skill_grants where package_id=?", (FREE_ID,))[0][:2] == ("user", BOB_ID),
      db("select grantee_type, grantee_id from skill_grants where package_id=?", (FREE_ID,)))
check("重复领取不产生第二行",
      db("select count(*) from skill_grants where package_id=? and grantee_id=?", (FREE_ID, BOB_ID))[0][0] == 1, "行数>1")
st, js = call("POST", "/skills/not-exist/claim", bob)
check("不存在的包 -> 404 SKILL_NOT_FOUND", st == 404 and errcode(js) == "SKILL_NOT_FOUND", (st, js))

st, js = call("GET", "/skills", bob)
free_item = next((i for i in (js.get("items") or []) if i.get("id") == FREE_ID), {})
check("领取后 granted=true", free_item.get("granted") is True, free_item.get("granted"))
st, js = call("GET", "/skills", carol)
free_item_c = next((i for i in (js.get("items") or []) if i.get("id") == FREE_ID), {})
check("carol 的 granted 仍 false（按用户隔离）", free_item_c.get("granted") is False, free_item_c.get("granted"))

st, js = call("GET", "/skills/grants", bob)
gs = js.get("items") or [] if isinstance(js, dict) else []
check("我的授权列表含免费包", any(g.get("package_id") == FREE_ID for g in gs), gs)
st, js = call("GET", "/skills/grants", carol)
check("carol 授权列表为空（用户隔离）", not (js.get("items") or []), js)

print("=== 8 B9 管理员手工发放 ===")
st, js = call("POST", "/admin/skills/%s/grant" % PAID_ID, admin,
              body={"grantee_type": "user", "grantee_id": CAROL_ID, "source": "manual"})
check("管理员给 carol 发放付费包 200", st == 200 and js.get("ok") is True, (st, js))
st, js = call("GET", "/skills/grants", carol)
check("carol 授权列表出现付费包", any(g.get("package_id") == PAID_ID for g in (js.get("items") or [])), js)
st, js = call("POST", "/admin/skills/%s/grant" % PAID_ID, bob, body={"grantee_type": "user"})
check("非管理员发放 -> 403 FORBIDDEN", st == 403 and errcode(js) == "FORBIDDEN", (st, js))
st, js = call("POST", "/admin/skills/%s/grant" % PAID_ID, admin, body={"grantee_type": "team", "grantee_id": "x"})
check("非法 grantee_type -> 400 SKILL_BAD_GRANTEE", st == 400 and errcode(js) == "SKILL_BAD_GRANTEE", (st, js))
st, js = call("POST", "/admin/skills/not-exist/grant", admin, body={"grantee_type": "user"})
check("给不存在的包发放 -> 404 SKILL_NOT_FOUND", st == 404 and errcode(js) == "SKILL_NOT_FOUND", (st, js))

print("=== 9 向后兼容（既有能力未回归）===")
for p in ("/health", "/files", "/notifications", "/subscriptions", "/mentions", "/im/status",
          "/admin/sites", "/admin/apps/market"):
    st, js = call("GET", p, admin)
    check("既有端点 GET %s -> 200" % p, st == 200, (st, str(js)[:120]))
st, js = call("GET", "/blog/feed.xml", None)
check("对外 feed 仍 200（/api/v1/blog/feed.xml）", st == 200, st)

print("=== 10 库内残留核对 ===")
print("  im_bindings       =", db("select count(*) from im_bindings")[0][0])
print("  im_binding_codes  =", db("select count(*) from im_binding_codes")[0][0])
print("  file_media        =", db("select count(*) from file_media")[0][0])
print("  skill_packages    =", db("select count(*) from skill_packages")[0][0])
print("  skill_grants      =", db("select count(*) from skill_grants")[0][0])

ok = sum(1 for _, c in RESULTS if c)
print("\n===== 结果：%d/%d PASS =====" % (ok, len(RESULTS)))
if ok != len(RESULTS):
    print("失败项：")
    for n, c in RESULTS:
        if not c:
            print("  - " + n)
