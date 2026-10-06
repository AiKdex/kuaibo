package handler

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// publicLinkCheck GET /api/v1/public/link-check?url=...
// 公开链接存活体检（生产站底座 E 的本地实现）：服务端 HEAD 探测。
// 仅允许 http/https，6s 超时；拒绝指向保留/私网地址（SSRF 防护）。
// 注：上游同名端点为权威实现；本实现为自托管的独立可用版本，二者契约一致。
func (a *API) publicLinkCheck(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "URL_REQUIRED", "缺少 url 参数")
		return
	}
	res := probeLink(raw)
	writeJSON(w, http.StatusOK, res)
}

type linkCheckResult struct {
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Reachable bool   `json:"reachable"`
	Status    int    `json:"status"`
	Error     string `json:"error,omitempty"`
}

// probeLink 对目标 URL 做 HEAD 探测（带超时与私网防护）。
func probeLink(raw string) linkCheckResult {
	out := linkCheckResult{URL: raw}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		out.Error = "仅支持 http/https 链接"
		return out
	}
	host := u.Hostname()
	if isPrivateHost(host) {
		out.Error = "拒绝探测内网/保留地址"
		return out
	}
	client := &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("重定向次数过多")
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodHead, raw, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	req.Header.Set("User-Agent", "AiKlog-LinkCheck/1.0")
	resp, err := client.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	out.Status = resp.StatusCode
	out.Reachable = true
	out.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	return out
}

// isPrivateHost 判断主机是否为保留/私网地址（SSRF 防护）。
// 直接 IP 字面量：拒绝 loopback / private / 未指定 / 链路本地。
// 域名：解析后逐一检查（尽力而为；不防 DNS rebinding，但超时+HEAD 限制了爆破面）。
func isPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// 解析失败：保守放行（可能是临时 DNS 波动），由超时与 HEAD 兜底
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
