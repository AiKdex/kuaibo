// ssrf_redirect_test.go H6 回归：http.Client 重定向逐跳 SSRF 校验。
//
// 背景（CODE_REVIEW_REPORT H6）：采集/导入/挂载/Webhook 四处 http.Client 此前只用
// 默认 CheckRedirect —— 默认策略会**自动跟随最多 10 跳**且不校验目标。远端只要返回
// `302 Location: http://169.254.169.254/latest/meta-data/` 即可借这些通道读内网。
// 修复：全部 Client 接 safeCheckRedirect（≤5 跳 + 每跳目标命中私网即中止）。
//
// 覆盖两层：
//   - 纯函数：直接喂 req/via，覆盖私网各段、跳数上限、缺主机名；
//   - 真实行为：httptest 起一个只做 302 的跳板，用挂了 safeCheckRedirect 的
//     Client 去请求 —— 必须返回错误，且**只发出 1 次请求**（不跟随到私网目标）。
package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func mkReq(t *testing.T, raw string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		t.Fatalf("构造请求失败 %s: %v", raw, err)
	}
	return req
}

// TestSafeCheckRedirectRejectsPrivate 逐类私网/保留地址必须被拒。
func TestSafeCheckRedirectRejectsPrivate(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/",                        // 环回
		"http://127.0.0.1:8780/health",             // 环回 + 端口
		"http://localhost/",                        // localhost → 解析到环回
		"http://169.254.169.254/latest/meta-data/", // 云元数据（链路本地）
		"http://10.0.0.1/",                         // 私网 A
		"http://172.16.0.1/",                       // 私网 B
		"http://192.168.1.1/",                      // 私网 C
		"http://100.64.0.1/",                       // CGNAT
		"http://0.0.0.0/",                          // 本网络
	}
	for _, raw := range cases {
		if err := safeCheckRedirect(mkReq(t, raw), nil); err == nil {
			t.Errorf("safeCheckRedirect(%s) = nil，应拒绝私网/保留地址", raw)
		}
	}
}

// TestSafeCheckRedirectAllowsPublic 公网目标不得被误杀（否则采集/Webhook 全线不可用）。
func TestSafeCheckRedirectAllowsPublic(t *testing.T) {
	for _, raw := range []string{
		"http://1.1.1.1/", "https://93.184.216.34/x", "http://8.8.8.8:8080/p",
	} {
		if err := safeCheckRedirect(mkReq(t, raw), nil); err != nil {
			t.Errorf("safeCheckRedirect(%s) = %v，公网地址应放行", raw, err)
		}
	}
}

// TestSafeCheckRedirectHopLimit 跳数上限：via 长度 ≥5 即中止（默认 Client 是 10 跳）。
func TestSafeCheckRedirectHopLimit(t *testing.T) {
	dummy := mkReq(t, "http://1.1.1.1/")
	via := []*http.Request{dummy, dummy, dummy, dummy} // 4 跳：仍允许
	if err := safeCheckRedirect(mkReq(t, "http://1.1.1.1/"), via); err != nil {
		t.Errorf("4 跳应放行，得到 %v", err)
	}
	via = append(via, dummy) // 5 跳：中止
	if err := safeCheckRedirect(mkReq(t, "http://1.1.1.1/"), via); err == nil {
		t.Error("5 跳应中止，得到 nil")
	}
}

// TestSafeCheckRedirectEmptyHost 缺主机名必须拒（防相对 Location 被解析成空 host 后放行）。
func TestSafeCheckRedirectEmptyHost(t *testing.T) {
	if err := safeCheckRedirect(mkReq(t, "/relative"), nil); err == nil {
		t.Error("缺少主机名应被拒，得到 nil")
	}
}

// TestClientFollowsRedirectToPrivateIsBlocked 端到端行为：真实 Client 不允许跟随到私网。
//
// 跳板：/redir → 302 Location: http://169.254.169.254/latest/meta-data/
// 断言不依赖"内网恰好不可达"，而是直接证明「Client 未跟随」：
// 返回 error、错误信息指明内网/保留地址、且跳板只被命中 1 次。
func TestClientFollowsRedirectToPrivateIsBlocked(t *testing.T) {
	var hits int32
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer hop.Close()

	cli := &http.Client{CheckRedirect: safeCheckRedirect}
	resp, err := cli.Get(hop.URL + "/redir")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("跟随重定向到 169.254.169.254 竟然成功（status=%d），CheckRedirect 未生效", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "内网") && !strings.Contains(err.Error(), "保留地址") {
		t.Errorf("错误信息应指明内网/保留地址拦截，实际: %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("跳板命中次数应为 1（只发初始请求），实际 %d", n)
	}
}

// TestClientRedirectChainToPrivateBlocked 多跳链：公网跳板 → 公网跳板 → 私网，
// 中间若干跳为私网地址（httptest 监听 127.0.0.1）也会被拦 —— 这正是逐跳校验的意义：
// 只校验初始 URL 的方案会在第二跳放行。
func TestClientRedirectChainToPrivateBlocked(t *testing.T) {
	var second int32
	hop2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&second, 1)
		http.Redirect(w, r, "http://10.0.0.5/internal", http.StatusFound)
	}))
	defer hop2.Close()

	hop1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, hop2.URL+"/next", http.StatusFound)
	}))
	defer hop1.Close()

	cli := &http.Client{CheckRedirect: safeCheckRedirect}
	resp, err := cli.Get(hop1.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("重定向到 10.0.0.5 竟然成功（status=%d）", resp.StatusCode)
	}
	// hop2 自身监听 127.0.0.1，故在第 2 跳就被拦（不会打到 hop2 的第二跳逻辑）。
	if n := atomic.LoadInt32(&second); n != 0 {
		t.Errorf("第二跳（10.0.0.5）之前就被拦截，hop2 命中次数应为 0，实际 %d", n)
	}
}

// TestSafeCheckRedirectExportedContract 契约测试：导出的 SafeCheckRedirect
// 必须与内部实现策略一致（handler 层 webhook 投递器依赖导出版，不得漂移）。
func TestSafeCheckRedirectExportedContract(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/", "http://169.254.169.254/", "http://192.168.0.1/",
	} {
		if err := SafeCheckRedirect(mkReq(t, raw), nil); err == nil {
			t.Errorf("SafeCheckRedirect(%s) 应拒绝私网目标", raw)
		}
	}
	if err := SafeCheckRedirect(mkReq(t, "http://1.1.1.1/"), nil); err != nil {
		t.Errorf("SafeCheckRedirect 对公网地址应放行: %v", err)
	}
}
