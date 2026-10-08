#!/bin/bash
# B7 e2e 清理与残留核对：停沙箱 → 删沙箱 → 生产零影响校验
set -u
echo "== 1) 停沙箱（按 PID，不用 pkill -f）=="
PID=$(pgrep -x aiksandboxb7 | head -1)
if [ -n "${PID:-}" ]; then kill "$PID"; sleep 2; echo "  killed $PID"; else echo "  沙箱进程不存在"; fi
pgrep -x aiksandboxb7 >/dev/null && echo "  !! 仍存活" || echo "  沙箱已停止"

echo "== 2) 删沙箱目录 =="
rm -rf /root/b7e2e
ls -d /root/b7e2e 2>/dev/null && echo "  !! 仍存在" || echo "  已删除"

echo "== 3) 生产 storage 哈希复验（应与基线一致）=="
cat /root/b7prod_files_before.txt
find /opt/aiklog/data/files -type f -exec sha256sum {} + 2>/dev/null | sort | sha256sum | tee /root/b7prod_files_after.txt
if diff -q /root/b7prod_files_before.txt /root/b7prod_files_after.txt >/dev/null; then
  echo "  RESULT=MATCH（生产存储零改动）"
else
  echo "  RESULT=DIFF !! 需要善后"
fi
echo "== 4) 生产 storage 是否混入 e2e 痕迹 =="
grep -rl "b7-e2e" /opt/aiklog/data/files 2>/dev/null | head || echo "  无 b7-e2e 痕迹（干净）"

echo "== 5) 生产 DB 计数复验 =="
python3 - <<'PY'
import sqlite3
c = sqlite3.connect('/opt/aiklog/data/aikmap.db')
for t in ('files', 'users', 'spaces', 'im_bindings', 'im_binding_codes', 'file_media', 'skill_packages', 'skill_grants'):
    print('   %-20s %d' % (t, c.execute('select count(*) from %s' % t).fetchone()[0]))
c.close()
PY

echo "== 6) 生产端点与端口 =="
curl -s -o /dev/null -w '  health=%{http_code}\n' --max-time 3 http://127.0.0.1:8780/api/v1/health
(ss -ltnp 2>/dev/null || netstat -ltnp 2>/dev/null) | grep -E '878[01]'
echo "== 7) 生产进程 =="
ps -eo pid,etime,cmd | grep -E '[a]iklog -addr' || echo "  (无)"
echo CLEANUP_DONE
