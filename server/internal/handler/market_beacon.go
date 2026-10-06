// B47：官方侧回源统计 —— 接收端点 + 看板。
//
// 这个文件**只在官方平台侧生效**（aikmap.cn），免费版实例用不到。
// 它与 market_telemetry.go（壳侧派生 install_id）是一对：那边发，这边收。
//
// 设计上的自我约束（重要）：
//   1. 只收三个头：install_id（形状校验）、version（字符白名单）、shell（限长）。
//      不读 body、不读 UA 指纹、不落 IP、不落任何其他请求信息。
//   2. 公开端点（无鉴权）—— 因为壳侧可能是未注册的免费实例，鉴权会把统计口径变成
//      "只有付费用户才被计数"，那就没有意义了。**因为不收 PII，公开采集不构成隐私问题。**
//   3. 计数口径 = **行数**（去重安装数），不是 hits。用户刷页面不应放大"装机数"。
//   4. 写入用 UPSERT + 条件更新，单条 SQL 完成，天然幂等、可并发。
package handler

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// marketBeacon 看板返回结构。
type marketBeacon struct {
	// 口径说明 —— 前端必须原样展示，避免把"回源请求数"误读成"装机数"。
	Note string `json:"note"`
	// 去重安装数（本表行数）
	Instances int `json:"instances"`
	// 最近 24h 活跃安装数（last_seen 在窗口内）
	Active24h int `json:"active_24h"`
	// 最近 7 天活跃安装数
	Active7d int `json:"active_7d"`
	// 累计回源次数（仅供观察活跃强度，不作装机口径）
	Hits int64 `json:"hits"`
	// 版本分布（version -> 安装数），降序由 SQL 保证
	Versions []marketVersionCount `json:"versions"`
	// 壳分布
	Shells []marketShellCount `json:"shells"`
	// 首次回源至今天数（装机曲线的粗粒度参考）
	FirstSeenAt int64 `json:"first_seen_at,omitempty"`
	GeneratedAt int64 `json:"generated_at"`
}

type marketVersionCount struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

type marketShellCount struct {
	Shell string `json:"shell"`
	Count int    `json:"count"`
}

// marketBeaconIngest 在**官方侧输出市场索引时**就地记录一次回源。
//
// 为什么是"就地记录"而不是让壳侧另外 POST 一个 beacon 端点：
//   - 壳侧本来就在拉索引，身份头已经挂在那个请求上（见 market_telemetry.go）；
//     多发一次请求 = 多一次往返、多一个可被拦截的点，收益为零。
//   - 就地记录还能保证信号语义准确：「官方**确实把索引发出去了**」，
//     而不是「壳侧声称它来过了」。卖指标就该用对方无法伪造的那一侧。
//   - 缓存命中时同样记录（见调用处），因为 5 分钟缓存期内用户是"活跃使用应用中心"的，
//     若缓存命中不记，装机数会随缓存周期漂移，5 分钟后集体消失。
//
// 安全性：只取请求头里的自证数据（install_id 形状校验 + 版本字符白名单），
// 不读 body、不读 UA、不落 IP。见文件头说明。
func (a *API) marketBeaconIngest(r *http.Request) {
	id := installIDFromRequest(r)
	if id == "" {
		// 形状不合法 = 不是本项目的壳（扫描器/爬虫/手工构造）。
		// 静默忽略即可：这是旁路统计，不需要为噪声返回错误或暴露格式要求。
		return
	}
	ver := versionFromRequest(r)
	shell := sanitizeShell(r.Header.Get(marketShellHeader))
	now := time.Now().UnixMilli()

	// UPSERT：一行 = 一台安装。重复回源只累加 hits 与刷新 last_seen，**不新增行** ——
	// 这是"装机数按去重算"的技术保证（否则用户刷页面就能把装机数刷上去）。
	// 版本/壳取最新上报值：用户可能升级过，历史值对"当前分布"没有意义。
	if _, err := a.db.ExecContext(r.Context(), `
		INSERT INTO market_instances (install_id, version, shell, hits, first_seen, last_seen)
		VALUES (?,?,?,1,?,?)
		ON CONFLICT(install_id) DO UPDATE SET
			hits      = hits + 1,
			last_seen = excluded.last_seen,
			version   = excluded.version,
			shell     = excluded.shell`,
		id, ver, shell, now, now); err != nil {
		// 旁路语义：写不进去只记日志。索引该发还是发 —— 统计失败绝不能影响用户拿目录。
		log.Printf("market: beacon 写入失败 id=%s: %v", shortID(id), err)
	}
}

// marketBeaconStats GET /api/v1/admin/market/beacon —— 看板数据（登录态，看板本身在后台里）。
//
// 注意：这是**读自己的聚合数**，不是读用户数据，因此不涉及隐私边界。
func (a *API) marketBeaconStats(w http.ResponseWriter, r *http.Request) {
	out := marketBeacon{
		Note:        "装机数 = 去重安装数（本表行数），按匿名 install_id 统计；回源次数仅反映活跃强度，不能当装机数读。内网/离线安装不回源，故此数为下界。",
		Versions:    []marketVersionCount{},
		Shells:      []marketShellCount{},
		GeneratedAt: time.Now().UnixMilli(),
	}
	now := time.Now().UnixMilli()
	day := int64(24 * time.Hour)
	week := 7 * day

	// 表可能还没建（首次部署前的旧库）：错误降级为空看板，不 500。
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*), COALESCE(SUM(hits),0), COALESCE(MIN(first_seen),0) FROM market_instances`,
	).Scan(&out.Instances, &out.Hits, &out.FirstSeenAt); err != nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	// 活跃窗口用 SQL 侧聚合，避免把全表拉进内存。
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM market_instances WHERE last_seen >= ?`, now-day).Scan(&out.Active24h)
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM market_instances WHERE last_seen >= ?`, now-week).Scan(&out.Active7d)

	if rows, err := a.db.QueryContext(r.Context(),
		`SELECT version, COUNT(*) c FROM market_instances GROUP BY version ORDER BY c DESC, version ASC LIMIT 50`,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var vc marketVersionCount
			if err := rows.Scan(&vc.Version, &vc.Count); err == nil {
				out.Versions = append(out.Versions, vc)
			}
		}
	}
	if rows, err := a.db.QueryContext(r.Context(),
		`SELECT shell, COUNT(*) c FROM market_instances GROUP BY shell ORDER BY c DESC, shell ASC LIMIT 20`,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var sc marketShellCount
			if err := rows.Scan(&sc.Shell, &sc.Count); err == nil {
				out.Shells = append(out.Shells, sc)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// sanitizeShell 壳标识：限长 + 字符白名单（与 version 同口径的防御性过滤）。
func sanitizeShell(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			c == '.' || c == '-' || c == '_'
		if !ok {
			return "unknown"
		}
	}
	return s
}
