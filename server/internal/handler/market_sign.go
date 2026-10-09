// market_sign.go 市场索引签发（marketgen 工具的服务端同源部分）。
//
// 为什么放在 handler 包：签名的载荷 = marketIndex 结构体去掉 signature 字段后的
// json.Marshal（见 apps_market_catalog.go canonicalBytes）。签发方若在别处重新定义
// 结构体，字段顺序/omitempty 一旦漂移，字节就对不上、验签必挂——所以签发逻辑
// 必须与验签共用同一份结构体与规范化代码（铁律条92：签名载荷与验签同源代码）。
package handler

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"

	"github.com/AiKMAP/AiKmap/server/internal/entitle"
)

// MarketSignIssuer 官方目录签发方（须在 entitle.public_keys.go 内嵌表中）。
const MarketSignIssuer = "aiklog-market"

// SignMarketIndexJSON 把（未签名或旧签名的）市场索引 JSON 重新用 iss 私钥签名。
// 输入：任意市场索引 JSON（多余未知字段会在规范化时被结构体丢弃——与实例侧验签
// 时的行为一致，因此丢字段不影响验签）。输出：附加 signature{iss,sig} 的完整 JSON。
func SignMarketIndexJSON(in []byte, priv ed25519.PrivateKey) ([]byte, error) {
	var idx marketIndex
	if err := json.Unmarshal(in, &idx); err != nil {
		return nil, fmt.Errorf("市场索引 JSON 非法：%w", err)
	}
	idx.Signature = nil
	payload, err := json.Marshal(&idx) // 与 canonicalBytes 完全同源
	if err != nil {
		return nil, err
	}
	sig := entitle.SignDetached(priv, payload)
	if sig == "" {
		return nil, fmt.Errorf("签名失败：私钥长度非法")
	}
	idx.Signature = &marketSignature{Iss: MarketSignIssuer, Sig: sig}
	out, err := json.Marshal(&idx)
	if err != nil {
		return nil, err
	}
	// 自检：产物必须能通过实例侧同一验签路径（防结构漂移直接流出到构建里）
	var check marketIndex
	if err := json.Unmarshal(out, &check); err != nil {
		return nil, err
	}
	if err := check.verifySignature(); err != nil {
		return nil, fmt.Errorf("签发产物自检失败：%w", err)
	}
	return out, nil
}
