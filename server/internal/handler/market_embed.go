// market_embed.go 内置官方市场目录（go:embed 签名产物）。
//
// 背景（2026-10-09「市场索引未签名」故障）：上游 aikmap.cn/market/index.json 已不内嵌
// signature 字段，而实例侧对远程索引强制验签（不降级）→ 应用中心整页 502。
// 修复架构：fork 自签自验闭环——cmd/marketgen 用 aiklog-market 私钥签发官方目录，
// 产物 go:embed 进二进制；远程拉取/验签失败时降级使用内置目录（日志明示，非静默：
// 绝不把「未签名的远程内容」当目录用，降级的是「用哪份已签名目录」）。
// 本地 data/market/index.json 仍为最高优先（运营侧手工投放通道，不走验签）。
package handler

import (
	_ "embed"
	"encoding/json"
	"log"
	"sync"
)

//go:embed market_official_index.json
var marketOfficialIndexJSON []byte

var (
	embeddedMarketOnce sync.Once
	embeddedMarketIdx  *marketIndex
)

// embeddedMarketIndex 解析内置官方目录（进程内只解析一次；损坏视为编程错误，返回 nil
// 由调用方走原有 502 报错路径——嵌入产物在构建期由 SignMarketIndexJSON 自检兜底）。
func embeddedMarketIndex() *marketIndex {
	embeddedMarketOnce.Do(func() {
		var idx marketIndex
		if err := json.Unmarshal(marketOfficialIndexJSON, &idx); err != nil {
			log.Printf("[market] 内置官方目录解析失败（构建产物损坏？）：%v", err)
			return
		}
		embeddedMarketIdx = &idx
	})
	return embeddedMarketIdx
}
