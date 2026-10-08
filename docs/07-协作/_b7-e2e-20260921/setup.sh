#!/bin/bash
# B7+B8+B9 隔离 e2e 沙箱启动（沙箱 8781，生产 8780 不动）
set -u
SB=/root/b7e2e
rm -rf "$SB"
mkdir -p "$SB/bin" "$SB/data" "$SB/log"
cp -f /root/b7up/aiklog.b7 "$SB/bin/aiksandboxb7"
chmod +x "$SB/bin/aiksandboxb7"
echo "--- 生产 storage 基线哈希（e2e 前后必须一致）---"
find /opt/aiklog/data/files -type f -exec sha256sum {} + 2>/dev/null | sort | sha256sum | tee /root/b7prod_files_before.txt
echo "--- 生产 DB 基线计数 ---"
python3 - <<'PY'
import sqlite3
c = sqlite3.connect('/opt/aiklog/data/aikmap.db')
for t in ('files', 'users', 'spaces', 'im_bindings', 'im_binding_codes', 'file_media', 'skill_packages', 'skill_grants'):
    print('   %-20s %d' % (t, c.execute('select count(*) from %s' % t).fetchone()[0]))
c.close()
PY
python3 /root/b7_e2e_copydb.py
cd "$SB" || exit 1
setsid nohup ./bin/aiksandboxb7 -addr 127.0.0.1:8781 -db "$SB/data/aikmap.db" > "$SB/log/sandbox.log" 2>&1 </dev/null &
sleep 5
PID=$(pgrep -x aiksandboxb7 | head -1)
echo "sandbox_pid=$PID"
echo "exe -> $(readlink -f /proc/$PID/exe)"
echo "cwd -> $(readlink -f /proc/$PID/cwd)"
curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:8781/api/v1/health
echo "--- sandbox data 目录 ---"
ls -la "$SB/data/"
echo "--- 端口 ---"
(ss -ltnp 2>/dev/null || netstat -ltnp 2>/dev/null) | grep -E '878[01]'
echo "--- 生产进程仍在 ---"
ps -eo pid,etime,cmd | grep -E '[a]iklog -addr' || echo "  (无)"
