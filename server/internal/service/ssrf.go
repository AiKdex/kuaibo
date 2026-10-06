// ssrf.go 采集器 SSRF 防护：拒绝私网/保留地址目标。
// 采集源由用户/源配置提供 URL，若被配成内网地址，采集器将成为 SSRF 跳板访问内网服务。
// 参照 Kmap backend/app/services/ssrf.py 语义：解析 host →（域名则 DNS 解析）→ IP 校验。
// 域名解析后逐条校验 A 记录（基础防 DNS rebinding：任一记录为私网即拒绝）。
package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// checkPublicURL 校验 URL 指向公网（非私网/保留/环回/链路本地）。
// 返回错误时调用方应拒绝发起请求。
func checkPublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("ssrf: 非法 URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("ssrf: 仅允许 http/https 目标: %s", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("ssrf: URL 缺少 host: %s", rawURL)
	}
	// 直接 IP
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("ssrf: 目标为私网/保留地址，拒绝采集: %s", host)
		}
		return nil
	}
	// 域名：解析并逐条校验
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("ssrf: 域名解析失败: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("ssrf: 域名无解析记录: %s", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("ssrf: 域名 %s 解析到私网/保留地址 %s，拒绝采集", host, ip.String())
		}
	}
	return nil
}

// isPrivateIP 判断 IP 是否属于私网/保留/环回/链路本地/文档地址。
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// 文档地址段（TEST-NET）与 CGNAT 段
	for _, cidr := range []string{
		"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", // TEST-NET-1/2/3
		"100.64.0.0/10",  // CGNAT
		"169.254.0.0/16", // 链路本地（IsLinkLocal 未覆盖 IPv4 部分场景）
		"0.0.0.0/8",      // 本网络
		"::/128", "fc00::/7", "fe80::/10", "::1/128",
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// ensurePublicURL 便捷封装：空串直接通过（未配置源不校验），非空校验。
func ensurePublicURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	return checkPublicURL(rawURL)
}

// safeCheckRedirect 防 SSRF 重定向（适用 http.Client.CheckRedirect，H6）：
// 仅校验初始 URL 不够——被采集/被挂载/被回调的远端只需返回 302 Location: http://169.254.169.254/
// 就能借默认 Client（自动跟随最多 10 跳）读到内网元数据。这里限制最多 5 跳，
// 且每一跳的目标主机（含 DNS 解析结果）命中私网/保留地址即中止。
// 注意：DNS 解析与真正建连之间仍有 TOCTOU 窗口（rebinding），本函数收敛的是"明显重定向到内网"
// 这一类，彻底消除需在 DialContext 层绑定已校验 IP，属增量加固位（当前各 Client 未接自定义 Dial）。
func safeCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("重定向次数过多")
	}
	host := req.URL.Hostname()
	if host == "" {
		return errors.New("重定向目标缺少主机名")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return errors.New("禁止重定向到内网/保留地址")
		}
		return nil
	}
	if addrs, err := net.LookupIP(host); err == nil {
		for _, a := range addrs {
			if isPrivateIP(a) {
				return errors.New("禁止重定向到内网/保留地址")
			}
		}
	}
	return nil
}

// SafeCheckRedirect 导出包装：handler 层（webhook 投递器）与本包各 http.Client 复用同一策略，
// 避免两处各写一份逐渐漂移。
func SafeCheckRedirect(req *http.Request, via []*http.Request) error {
	return safeCheckRedirect(req, via)
}

// safeDialer 带 Control 钩子的拨号器：真正建连的 IP（DNS 解析结果）在 Control 里再校验一次。
// H6 收尾——checkPublicURL/safeCheckRedirect 校验的是"解析时"的记录，解析与建连之间存在
// DNS rebinding 窗口（TOCTOU）；Control 在内核 connect 前拿到最终拨号地址，逐次复验后
// 该窗口即关闭。
var safeDialer = &net.Dialer{
	Timeout: 10 * time.Second,
	Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("ssrf: 非法拨号地址: %w", err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("ssrf: 非法拨号主机: %s", address)
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("ssrf: 拨号目标解析为私网/保留地址: %s", address)
		}
		return nil
	},
}

// SafeDialContext 供各采集/导入/探测 http.Transport 的 DialContext 字段使用
//（替换裸 net.Dialer，与 CheckRedirect 组成完整的逐跳+建连双层校验）。
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return safeDialer.DialContext(ctx, network, addr)
}
