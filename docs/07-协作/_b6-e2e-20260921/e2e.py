#!/usr/bin/env python3
# B6（订阅与通知）隔离端到端验证 —— 只打沙箱 127.0.0.1:8781，不碰生产 8780。
# 覆盖：未鉴权拦截 / 订阅 CRUD 与幂等 / 内容更新投递订阅通知（跳过编辑者 + 窗口去重）/
#       @提及（文章·评论·显式·纯写法·邮件误判）/ 提及已读 / 通知中心多用户隔离 /
#       payload 落 TEXT（BLOB 缺陷修复）/ 退订后不再通知 / Purge 级联清理。
import json
import sqlite3
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8781/api/v1"
DB = "/root/b6e2e/data/aikmap.db"
PW = "Aiklog@ImTest2026"

RESULTS = []


def check(name, cond, detail=""):
    RESULTS.append((name, bool(cond)))
    print(("  [PASS] " if cond else "  [FAIL] ") + name + ("" if cond else "   -> got: " + str(detail)))


def call(method, path, token=None, body=None):
    data = None
    headers = {}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            raw = r.read().decode("utf-8", "replace")
            try:
                return r.status, json.loads(raw)
            except Exception:
                return r.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw


def errcode(js):
    if isinstance(js, dict) and isinstance(js.get("error"), dict):
        return js["error"].get("code")
    return None


def login(u, p):
    st, js = call("POST", "/auth/login", body={"username": u, "password": p})
    return js.get("token") if isinstance(js, dict) else None


def notif(token):
    st, js = call("GET", "/notifications", token)
    return js if isinstance(js, dict) else {"items": [], "unread_count": -1}


def cnt(js, t):
    return sum(1 for it in js.get("items", []) if it.get("type") == t)


def mentions(token):
    st, js = call("GET", "/mentions", token)
    return js if isinstance(js, dict) else {"items": [], "unread_count": -1}


def db(sql, args=()):
    con = sqlite3.connect(DB)
    try:
        return con.execute(sql, args).fetchall()
    finally:
        con.close()


print("=== 0 登录 ===")
admin = login("admin", PW)
bob = login("e2e_bob", PW)
carol = login("e2e_carol", PW)
check("admin/bob/carol 三个会话签发成功", all([admin, bob, carol]), [bool(admin), bool(bob), bool(carol)])
BOB_ID = db("select id from users where username='e2e_bob'")[0][0]
CAROL_ID = db("select id from users where username='e2e_carol'")[0][0]
ADMIN_ID = db("select id from users where username='admin'")[0][0]

print("=== 1 未鉴权拦截 ===")
for p in ("/subscriptions", "/subscriptions/status", "/mentions", "/mentions/unread-count", "/notifications"):
    st, _ = call("GET", p)
    check("匿名 GET %s → 401" % p, st == 401, st)

print("=== 2 造测试素材 ===")
st, d = call("POST", "/files/mkdir", admin, {"parent_id": "", "name": "B6E2E"})
DIR = d.get("id") if isinstance(d, dict) else None
check("建目录 B6E2E", st == 200 and DIR, (st, d))
st, d = call("POST", "/files/doc", admin, {"parent_id": DIR, "name": "B6 订阅测试", "content": "初始内容\n"})
DOC = d.get("id") if isinstance(d, dict) else None
check("建文章 B6 订阅测试", st == 200 and DOC, (st, d))
st, _ = call("PUT", "/files/" + DOC + "/status", admin, {"status": "published"})
check("发布测试文章（草稿不可分享）", st == 200, st)
st, s = call("POST", "/shares", admin, {"file_id": DOC})
SHARE = s.get("token") if isinstance(s, dict) else None
check("建公开分享（评论入口用）", st == 201 and SHARE, (st, s))

print("=== 3 订阅 CRUD / 鉴权 / 幂等 ===")
st, js = call("POST", "/subscriptions", bob, {"target_type": "bogus", "target_id": DIR})
check("非法 target_type → 400 SUB_BAD_TARGET", st == 400 and errcode(js) == "SUB_BAD_TARGET", (st, js))
st, js = call("POST", "/subscriptions", bob, {"target_type": "dir", "target_id": ""})
check("空 target_id → 400 SUB_BAD_TARGET", st == 400 and errcode(js) == "SUB_BAD_TARGET", (st, js))
st, js = call("POST", "/subscriptions", bob, {"target_type": "file", "target_id": "ghost-not-exist"})
check("目标不存在仍可订阅（设计：订阅先于内容存在）", st == 200 and js.get("created") is True, (st, js))
st, js = call("DELETE", "/subscriptions/file/ghost-not-exist", bob)
check("退订幽灵目标 removed=true", st == 200 and js.get("removed") is True, (st, js))

st, js = call("POST", "/subscriptions", bob, {"target_type": "dir", "target_id": DIR})
check("bob 订阅目录 created=true", st == 200 and js.get("created") is True, (st, js))
st, js = call("POST", "/subscriptions", bob, {"target_type": "dir", "target_id": DIR})
check("重复订阅 created=false（幂等）", st == 200 and js.get("created") is False, (st, js))
st, js = call("GET", "/subscriptions/status?target_type=dir&target_id=" + DIR, bob)
check("status 已订阅且 count=1", js.get("subscribed") is True and js.get("count") == 1, (st, js))
st, js = call("GET", "/subscriptions/status?target_type=dir&target_id=" + DIR, carol)
check("他人视角未订阅（订阅按用户隔离）", js.get("subscribed") is False, (st, js))
st, js = call("GET", "/subscriptions", bob)
it = (js.get("items") or [{}])[0]
check("列表含目标名/类型（免二次查询）", it.get("target_id") == DIR and it.get("name") == "B6E2E" and it.get("kind") == "dir", js)
st, js = call("DELETE", "/subscriptions/dir/" + DIR, bob)
check("退订 removed=true", js.get("removed") is True, (st, js))
st, js = call("DELETE", "/subscriptions/dir/" + DIR, bob)
check("重复退订 removed=false（幂等）", js.get("removed") is False, (st, js))
st, js = call("GET", "/subscriptions", bob)
check("退订后列表为空数组而不是 null", js.get("items") == [] and js.get("count") == 0, js)

print("=== 4 内容更新 → 订阅通知（跳过编辑者 / 去重） ===")
call("POST", "/subscriptions", carol, {"target_type": "file", "target_id": DOC})
call("POST", "/subscriptions", bob, {"target_type": "file", "target_id": DOC})
call("POST", "/subscriptions", admin, {"target_type": "file", "target_id": DOC})
st, _ = call("PUT", "/files/" + DOC + "/content", admin, {"content": "第一版内容\n"})
check("admin 更新正文 200", st == 200, st)
nb, nc, na = notif(bob), notif(carol), notif(admin)
check("bob（订阅者）收到 1 条 subscription 通知", cnt(nb, "subscription") == 1, cnt(nb, "subscription"))
check("carol（订阅者）收到 1 条 subscription 通知", cnt(nc, "subscription") == 1, cnt(nc, "subscription"))
check("admin（编辑者）不发给自己：0 条 subscription", cnt(na, "subscription") == 0, cnt(na, "subscription"))
sub = [i for i in nb["items"] if i.get("type") == "subscription"][0]
check("通知含标题/正文/跳转/extra.file_id",
      sub.get("title") == "《B6 订阅测试》已更新" and sub.get("link", "").startswith("/read/")
      and (sub.get("extra") or {}).get("file_id") == DOC, sub)
st, _ = call("PUT", "/files/" + DOC + "/content", admin, {"content": "第二版内容\n"})
check("同窗口内二次更新不重复推送（去重窗口 600s）", cnt(notif(carol), "subscription") == 1,
      cnt(notif(carol), "subscription"))

print("=== 5 @提及：文章（纯写法 / 显式写法 / 邮件误判 / 幂等） ===")
body1 = "第一版内容\n\n请 @e2e_bob 帮忙看看。\n"
call("PUT", "/files/" + DOC + "/content", admin, {"content": body1})
mb, mc = mentions(bob), mentions(carol)
check("纯写法 @e2e_bob 落库 1 条", len(mb["items"]) == 1, mb)
m = (mb["items"] or [{}])[0]
check("提及元信息补全（来源名/来源类型/发起人/跳转）",
      m.get("source_type") == "post" and m.get("source_id") == DOC and m.get("source_name") == "B6 订阅测试"
      and m.get("from_name") == "管理员" and m.get("link") == "/read/" + DOC, m)
check("提及同步投递通知（type=mention）", cnt(notif(bob), "mention") == 1, cnt(notif(bob), "mention"))
st, js = call("GET", "/mentions/unread-count", bob)
check("未读提及计数 = 1", js.get("count") == 1, js)
check("未被提及者 carol 无提及", len(mc["items"]) == 0, mc)

call("PUT", "/files/" + DOC + "/content", admin, {"content": body1})
check("重复保存同内容不重复落库（幂等）", len(mentions(bob)["items"]) == 1, mentions(bob)["items"])
check("重复保存不重复通知", cnt(notif(bob), "mention") == 1, cnt(notif(bob), "mention"))

body2 = body1 + "\n显式写法：@[卡萝](%s) 也看看。\n" % CAROL_ID
call("PUT", "/files/" + DOC + "/content", admin, {"content": body2})
mc = mentions(carol)
check("显式写法 @[卡萝](id) 落库 1 条", len(mc["items"]) == 1, mc)
check("显式写法 name/from 解析正确",
      (mc["items"] or [{}])[0].get("from_name") == "管理员" and (mc["items"] or [{}])[0].get("source_type") == "post", mc)

before_b, before_c = len(mentions(bob)["items"]), len(mentions(carol)["items"])
call("PUT", "/files/" + DOC + "/content", admin, {"content": body2 + "\n联系方式 a@b.com 谢谢。\n"})
check("邮件地址 a@b.com 不被误判为提及",
      len(mentions(bob)["items"]) == before_b and len(mentions(carol)["items"]) == before_c,
      (len(mentions(bob)["items"]), len(mentions(carol)["items"])))

print("=== 6 @提及：评论 + 删除级联 ===")
st, js = call("POST", "/comments", carol, {"token": SHARE, "body": "@管理员 请看一下这条评论"})
CID = js.get("id") if isinstance(js, dict) else None
check("carol 发评论 201", st == 201 and CID, (st, js))
ma = mentions(admin)
check("评论里的 @管理员 落库（source_type=comment）", len(ma["items"]) == 1 and ma["items"][0].get("source_type") == "comment", ma)
check("评论提及跳转带锚点 #comment-<id>", (ma["items"] or [{}])[0].get("link", "").endswith("#comment-" + str(CID)), ma.get("items"))
st, js = call("POST", "/mentions/read", admin)
check("全部已读 200", st == 200 and js.get("ok") is True, (st, js))
st, js = call("GET", "/mentions/unread-count", admin)
check("已读后未读归零", js.get("count") == 0, js)
st, js = call("DELETE", "/blog/comments/" + str(CID), admin)
check("删除评论 200", st == 200, (st, js))
check("评论删除后提及级联清理", len(mentions(admin)["items"]) == 0, mentions(admin)["items"])

print("=== 7 通知中心多用户隔离（本轮安全修复） ===")
ex_b, ex_c = len(notif(bob)["items"]), len(notif(carol)["items"])
rows = dict(db("select user_id, count(*) from notifications group by user_id"))
check("API 计数与库内按 user_id 计数一致（无越权可见）",
      ex_b == rows.get(BOB_ID, 0) and ex_c == rows.get(CAROL_ID, 0),
      {"bob_api": ex_b, "bob_db": rows.get(BOB_ID, 0), "carol_api": ex_c, "carol_db": rows.get(CAROL_ID, 0)})
api_b = set(i["id"] for i in notif(bob)["items"])
api_c = set(i["id"] for i in notif(carol)["items"])
api_a = set(i["id"] for i in notif(admin)["items"])
db_b = set(r[0] for r in db("select id from notifications where user_id=?", (BOB_ID,)))
db_c = set(r[0] for r in db("select id from notifications where user_id=?", (CAROL_ID,)))
check("bob 可见集合 = 库内其本人行（逐条 id 相等）", api_b == db_b and len(api_b) > 0, (len(api_b), len(db_b)))
check("carol 可见集合 = 库内其本人行（逐条 id 相等）", api_c == db_c and len(api_c) > 0, (len(api_c), len(db_c)))
check("两用户通知无交集（不互相可见）", not (api_b & api_c), api_b & api_c)
check("管理员不因 isAdmin 看到他人通知", not (api_a & api_b) and not (api_a & api_c), (api_a & api_b, api_a & api_c))
types = db("select typeof(payload), count(*) from notifications group by typeof(payload)")
check("新写入 payload 落 TEXT 而非 BLOB（BLOB 缺陷已修）", all(t == "text" for t, _ in types), types)

print("=== 8 退订后不再通知 ===")
call("DELETE", "/subscriptions/file/" + DOC, carol)
before = cnt(notif(carol), "subscription")
call("PUT", "/files/" + DOC + "/content", admin, {"content": body2 + "\n第三版\n"})
check("carol 退订后不再收到订阅通知", cnt(notif(carol), "subscription") == before, (before, cnt(notif(carol), "subscription")))

print("=== 9 彻底删除 → 订阅/提及级联清理 ===")
n_sub_before = db("select count(*) from doc_subscriptions where target_id=?", (DOC,))[0][0]
n_men_before = db("select count(*) from mentions where source_id=?", (DOC,))[0][0]
check("删除前确有订阅行与提及行", n_sub_before > 0 and n_men_before > 0, (n_sub_before, n_men_before))
st, js = call("DELETE", "/files/" + DOC + "/purge", admin)
check("彻底删除 200", st == 200, (st, js))
n_sub_after = db("select count(*) from doc_subscriptions where target_id=?", (DOC,))[0][0]
n_men_after = db("select count(*) from mentions where source_id=?", (DOC,))[0][0]
check("订阅行随目标一并清理", n_sub_after == 0, n_sub_after)
check("提及行随内容一并清理", n_men_after == 0, n_men_after)

print("=== 10 收尾清理：目录 ===")
st, js = call("DELETE", "/files/" + DIR + "/purge", admin)
check("清理测试目录", st == 200, (st, js))

print()
ok = sum(1 for _, c in RESULTS if c)
bad = [(n, c) for n, c in RESULTS if not c]
print("RESULT: %d/%d passed" % (ok, len(RESULTS)))
if bad:
    print("FAILED:")
    for n, _ in bad:
        print("  - " + n)
    raise SystemExit(1)
print("ALL GREEN")
