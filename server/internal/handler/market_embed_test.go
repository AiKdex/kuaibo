// market_embed_test.go 内置官方目录与验签闭环回归。
//
// 背景（2026-10-09「市场索引未签名」故障）：上游索引去掉 signature 后应用中心整页
// 502。修复 = fork 自签目录嵌入二进制 + 远程失败降级。本测试锁两件事：
// ① 内置目录能通过实例侧 verifySignature（签发工具与验签结构漂移立即红）；
// ② 内置目录至少含一个可安装条目（防止签发工具把空目录带进构建）。
package handler

import "testing"

func TestEmbeddedMarketIndexVerifies(t *testing.T) {
	idx := embeddedMarketIndex()
	if idx == nil {
		t.Fatal("内置官方目录解析失败")
	}
	if err := idx.verifySignature(); err != nil {
		t.Fatalf("内置官方目录验签失败（签发/验签结构漂移？）: %v", err)
	}
	n := len(idx.Plugins) + len(idx.Themes) + len(idx.Licenses)
	if n == 0 {
		t.Fatal("内置官方目录为空（plugins/themes/licenses 均无条目）")
	}
	t.Logf("内置官方目录验签通过：iss=%s, plugins=%d, themes=%d", MarketSignIssuer, len(idx.Plugins), len(idx.Themes))
}
