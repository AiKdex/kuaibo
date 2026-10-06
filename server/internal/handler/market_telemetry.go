// B47：应用中心回源统计 —— 实例匿名标识的派生与请求头。
//
// 目的：让官方市场侧能把「索引回源请求」去重成「装机数」，并看到版本分布，
// 而**不需要用户交出任何信息**。用户点开应用中心 → 实例回源官方索引 → 官方侧计数。
// 这是用户主动发起的、指向官方服务器的、为了取功能的请求（不是监控），
// 信任成本最低，信号质量最高。
//
// 隐私红线（本文件的全部意义所在）：
//   - 只发**不可逆派生**的 install_id，不发原始密钥、不发域名/文件/笔记/账号。
//   - install_id 由 SecretKey（env AIKMAP_SECRET 或首次启动生成并持久化）派生，
//     因此同一台安装稳定不变、跨实例各不相同、**无法从 install_id 反推密钥**。
//   - 官方侧只能知道「有 N 台机器、版本分别是什么」，不知道是谁、装在哪、装了什么。
package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// installIDDomain 派生域分隔符。与业务其它 HMAC 用途隔离，避免同一密钥在不同用途
// 产生相同输出（否则一个 install_id 可能同时充当别的凭据）。
const installIDDomain = "aiklog/install-id/v1"

// marketInstanceHeader 回源时携带的匿名实例标识头。
const marketInstanceHeader = "X-AiKlog-Instance"

// marketVersionHeader 回源时携带的版本头（走官方侧版本分布看板）。
const marketVersionHeader = "X-AiKlog-Version"

// marketShellHeader 回源时携带的壳标识（多壳模型；默认 aiklog）。
const marketShellHeader = "X-AiKlog-Shell"

// installID 派生本实例的匿名稳定标识。
//
// 用 HMAC-SHA256 而非裸 SHA256：裸哈希在密钥为低熵时可被穷举反推，
// HMAC 提供了标准的伪随机函数安全性。
//
// 返回值取前 16 字节（32 个 hex 字符）——足够随机（128 位）且短到能安全放进 HTTP 头。
// 永不返回空串：即使 SecretKey 异常，也要给出一个稳定降级值，否则统计会把所有实例
// 聚成一坨，反而比没有统计更具误导性。
func (a *API) installID() string {
	mac := hmac.New(sha256.New, a.cfg.SecretKey())
	mac.Write([]byte(installIDDomain))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// marketReportIdentity 读上报开关。**默认开**（key 缺失/空串/非法值都算开）。
//
// 语义选择：默认开而非默认关。理由是这属于"披露型"而非"秘密型"数据 ——
// 既然我们明写"可关闭"并给了开关，默认关会让统计长期失真（用户无从知晓），
// 而这批数据的价值恰恰在于覆盖真实装机面。真正需要绝对匿名的人有那个开关。
func (a *API) marketReportIdentity() bool {
	return parseReportIdentity(a.cfg.GetString("plugin_market.report_identity"))
}

// parseReportIdentity 解析上报开关取值（抽成纯函数便于单测）。
// 未设置/空串/无法识别的值一律按"开"处理 —— 宁可多统计，也不要因配置笔误静默失效
// （静默失效是最坏的失败模式：看板显示 0，看起来像"没人用"，实际是配置写错了）。
func parseReportIdentity(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// attachMarketIdentity 给回源请求挂上匿名身份头。
//
// 三个头都是**实例自身可推导**的，不含任何用户输入：
//   - Instance：SecretKey 派生的匿名 ID
//   - Version：编译期 version 变量
//   - Shell：多壳标识（默认 aiklog）
//
// 官方侧若不接收这些头，功能不受任何影响（纯附加信息）——
// 这保证「即使官方忽略统计，用户的功能也完全正常」。
//
// 开关关闭时**完全不发**这些头：拉索引照常工作，只是官方侧看不到记录。
func (a *API) attachMarketIdentity(req *http.Request) {
	if req == nil || !a.marketReportIdentity() {
		return
	}
	req.Header.Set(marketInstanceHeader, a.installID())
	req.Header.Set(marketVersionHeader, version)
	req.Header.Set(marketShellHeader, a.shellID())
}

// installIDFromRequest 官方侧用：从回源请求提取实例 ID。
//
// 安全处理：只接受**形状合法**的值（32 位小写 hex），其余一律返回 ""。
// 官方侧拿到的串只用于去重计数，绝不落库存原文之外的任何关联数据；
// 过滤掉畸形值是为了防止有人手工构造任意字符串污染统计。
func installIDFromRequest(r *http.Request) string {
	return sanitizeInstallID(r.Header.Get(marketInstanceHeader))
}

// sanitizeInstallID 校验 install_id 形状：恰好 32 位小写十六进制。
func sanitizeInstallID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) != 32 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return s
}

// versionFromRequest 官方侧用：提取版本号（只做长度与字符白名单过滤，不解析语义）。
func versionFromRequest(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get(marketVersionHeader))
	if v == "" || len(v) > 64 {
		return "unknown"
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '_' || c == '+'
		if !ok {
			return "unknown"
		}
	}
	return v
}

// shortID 取 install_id 前 8 位用于日志，避免日志里出现完整标识。
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
