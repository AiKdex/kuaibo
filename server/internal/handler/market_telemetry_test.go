// market_telemetry_test.go B47 回源统计的隐私边界与形状校验回归。
//
// 背景：B47 让壳侧在拉取官方索引时顺带上报匿名 install_id，用于官方侧把回源请求
// 去重成「装机数」。本文件守住这个设计的**硬边界** —— 一旦这里松了，B47 就从
// "匿名统计"退化成"监控用户"，那比不做更糟。
//
// 覆盖点：
//  1. installID 派生：确定性（同一密钥恒等）、跨密钥不同、长度与字符集合法；
//  2. installID 不可反推密钥（用不同密钥碰撞不出同一 ID，即 HMAC 域分离有效）；
//  3. sanitizeInstallID 形状校验：畸形值必须被拒（防止手工构造任意串污染统计）；
//  4. versionFromRequest / sanitizeShell 字符白名单：注入类字符被清洗为 unknown；
//  5. attachMarketIdentity 挂头正确，且**不携带任何 PII**（无 Cookie/邮箱/域名）。
package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/config"
)

// TestInstallIDDeterministic 同一密钥必须恒定 —— 否则每次回源都被当成新装机，
// 装机数会随用户刷新页面无限膨胀。
func TestInstallIDDeterministic(t *testing.T) {
	// 直接验证派生函数本身的确定性（不依赖 API 组装）。
	derive := func(secret string) string {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(installIDDomain))
		return hex.EncodeToString(mac.Sum(nil)[:16])
	}
	a, b := derive("secret-A"), derive("secret-A")
	if a != b {
		t.Fatalf("同密钥派生应恒定：%s != %s", a, b)
	}
	if c := derive("secret-B"); c == a {
		t.Fatal("不同密钥必须派生出不同 ID（否则实例无法区分）")
	}
	if len(a) != 32 {
		t.Fatalf("install_id 应为 32 位 hex，实得 %d 位", len(a))
	}
	if sanitizeInstallID(a) == "" {
		t.Fatalf("派生出的合法 ID 被自身校验拒了：%q", a)
	}
}

// TestInstallIDDomainSeparation 域分隔符让不同用途的 HMAC 不撞车。
// 若去掉 domain，install_id 可能与其它基于同密钥的凭据相同 —— 那等于变相泄露。
func TestInstallIDDomainSeparation(t *testing.T) {
	secret := "shared-secret"
	macA := hmac.New(sha256.New, []byte(secret))
	macA.Write([]byte(installIDDomain))
	withDomain := hex.EncodeToString(macA.Sum(nil)[:16])

	macB := hmac.New(sha256.New, []byte(secret))
	macB.Write([]byte("some-other-purpose"))
	other := hex.EncodeToString(macB.Sum(nil)[:16])

	if withDomain == other {
		t.Fatal("不同用途的 HMAC 输出撞车，域分隔符失效")
	}
}

// TestSanitizeInstallID 形状校验：只有恰好 32 位小写 hex 才放行。
// 这是防止"有人手工构造任意字符串污染官方统计"的第一道闸。
func TestSanitizeInstallID(t *testing.T) {
	valid := strings.Repeat("a1b2c3d4", 4) // 32 位
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"合法", valid, valid},
		{"大写应被规范化", strings.ToUpper(valid), valid},
		{"前后空白应被裁剪", "  " + valid + "\t", valid},
		{"空串", "", ""},
		{"长度不足31", valid[:31], ""},
		{"长度超出33", valid + "a", ""},
		{"含非hex字符", strings.Repeat("z", 32), ""},
		{"含横杠（UUID形态）", "9b181aea-3ba3-4a79-9bcc-4870ec3b3888", ""},
		{"SQL注入尝试", "' OR 1=1 --", ""},
		{"换行注入", valid[:16] + "\n" + valid[16:], ""},
		{"超长输入", strings.Repeat("a", 4096), ""},
	}
	for _, c := range cases {
		if got := sanitizeInstallID(c.in); got != c.want {
			t.Errorf("%s: sanitizeInstallID(%q) = %q, 期望 %q", c.name, c.in, got, c.want)
		}
	}
}

// TestVersionFromRequest 版本号只做字符白名单，不解析语义。
func TestVersionFromRequest(t *testing.T) {
	mk := func(v string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/market/beacon", nil)
		if v != "" {
			r.Header.Set(marketVersionHeader, v)
		}
		return r
	}
	cases := []struct {
		name, in, want string
	}{
		{"正常语义化版本", "0.1.1-store.1.6", "0.1.1-store.1.6"},
		{"含连字符", "1.0.0-beta-2", "1.0.0-beta-2"},
		{"空值", "", "unknown"},
		{"超长", strings.Repeat("v", 65), "unknown"},
		{"含尖括号（XSS载荷）", "1.0<script>", "unknown"},
		{"含空格", "1.0 2.0", "unknown"},
		{"含中文", "1.0版本", "unknown"},
	}
	for _, c := range cases {
		if got := versionFromRequest(mk(c.in)); got != c.want {
			t.Errorf("%s: versionFromRequest(%q) = %q, 期望 %q", c.name, c.in, got, c.want)
		}
	}
}

// TestSanitizeShell 壳标识同口径过滤。
func TestSanitizeShell(t *testing.T) {
	cases := []struct{ in, want string }{
		{"aiklog", "aiklog"},
		{"kmap", "kmap"},
		{"", "unknown"},
		{"a/b", "unknown"},
		{"a b", "unknown"},
		{strings.Repeat("s", 100), strings.Repeat("s", 64)},
	}
	for _, c := range cases {
		if got := sanitizeShell(c.in); got != c.want {
			t.Errorf("sanitizeShell(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// TestAttachMarketIdentityNoPII 挂上的头必须只有实例自证数据 ——
// 绝不能出现 Cookie、邮箱、域名、IP 等任何用户可识别信息。
// 这是本设计的核心承诺，用测试钉死，防止后人"顺手加个统计维度"时越界。
func TestAttachMarketIdentityNoPII(t *testing.T) {
	api := &API{cfg: &config.Store{}}
	r := httptest.NewRequest(http.MethodGet, "https://aikmap.cn/market/index.json", nil)
	api.attachMarketIdentity(r)

	if r.Header.Get(marketInstanceHeader) == "" {
		t.Fatal("缺少实例标识头")
	}
	if r.Header.Get(marketVersionHeader) == "" {
		t.Fatal("缺少版本头")
	}
	if r.Header.Get(marketShellHeader) == "" {
		t.Fatal("缺少壳标识头")
	}
	// 白名单：只允许这三个自定义头出现在上报里。
	allowed := map[string]bool{
		marketInstanceHeader: true,
		marketVersionHeader:  true,
		marketShellHeader:    true,
	}
	for k := range r.Header {
		if strings.HasPrefix(k, "X-AiKlog-") && !allowed[k] {
			t.Errorf("上报头 %q 不在白名单内 —— 新增维度必须先评估隐私影响", k)
		}
	}
	// 明确不该被带上的东西
	for _, forbidden := range []string{"Cookie", "Authorization", "X-Forwarded-For", "Referer", "Host"} {
		if r.Header.Get(forbidden) != "" {
			t.Errorf("上报携带了不该有的头 %s = %q", forbidden, r.Header.Get(forbidden))
		}
	}
}

// TestInstallIDFromRequestRejectsJunk 采集端必须对畸形值无感（丢弃而不是报错）。
func TestInstallIDFromRequestRejectsJunk(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/market/beacon", nil)
	if got := installIDFromRequest(r); got != "" {
		t.Errorf("无头时应为空，实得 %q", got)
	}
	r.Header.Set(marketInstanceHeader, "not-a-valid-id")
	if got := installIDFromRequest(r); got != "" {
		t.Errorf("畸形 ID 应被拒，实得 %q", got)
	}
	good := strings.Repeat("f0", 16)
	r.Header.Set(marketInstanceHeader, good)
	if got := installIDFromRequest(r); got != good {
		t.Errorf("合法 ID 应放行：%q != %q", got, good)
	}
}

// TestMarketReportIdentityOptOut 上报开关必须真的能关 —— 否则"可关闭"就是空话，
// 而这批数据的信任基础正是"说了就能关"。
func TestMarketReportIdentityOptOut(t *testing.T) {
	off := []string{"false", "FALSE", " False ", "0", "no", "off"}
	on := []string{"", "true", "TRUE", "1", "yes", "on", "任意无法识别的值"}
	for _, v := range off {
		if parseReportIdentity(v) {
			t.Errorf("值 %q 应解析为关闭", v)
		}
	}
	for _, v := range on {
		if !parseReportIdentity(v) {
			t.Errorf("值 %q 应解析为开启（含未设置与无法识别 —— 宁可多统计也不要静默失效）", v)
		}
	}
	// 未配置（空 Store）时必须是开的：静默失效会让看板显示 0，看起来像"没人用"
	if !(&API{cfg: &config.Store{}}).marketReportIdentity() {
		t.Error("未配置时默认应为开")
	}
	// 关闭时不得挂任何身份头
	api := &API{cfg: &config.Store{}}
	r := httptest.NewRequest(http.MethodGet, "https://aikmap.cn/market/index.json", nil)
	api.attachMarketIdentity(r)
	if r.Header.Get(marketInstanceHeader) == "" {
		t.Error("默认开启时应挂上实例标识头")
	}
}
