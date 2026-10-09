// marketgen —— AiKlog 官方市场目录签发工具（不随仓库分发私钥）。
//
// 用法：
//
//	go run ./cmd/marketgen -in upstream.json -out signed.json -priv <base64私钥文件>
//
// 输入：上游/原始市场索引 JSON（如 curl https://aikmap.cn/market/index.json 抓取件）。
// 输出：附加 signature{iss=aiklog-market, sig} 的官方签名目录 → 产出物提交进仓库
// （server/internal/handler/market_official_index.json，go:embed 进二进制）。
//
// 签名规范化与实例侧验签共用 handler.SignMarketIndexJSON → marketIndex.canonicalBytes，
// 字节级同源（铁律条92）。
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"github.com/AiKMAP/AiKmap/server/internal/handler"
)

func main() {
	in := flag.String("in", "", "输入索引 JSON（必填）")
	out := flag.String("out", "", "输出签名索引（必填）")
	priv := flag.String("priv", "", "私钥文件（base64，64 字节；不随仓库分发）")
	flag.Parse()
	if *in == "" || *out == "" || *priv == "" {
		fmt.Fprintln(os.Stderr, "用法：marketgen -in <索引.json> -out <签名.json> -priv <私钥>")
		os.Exit(2)
	}
	b, err := os.ReadFile(*priv)
	if err != nil {
		fatal("私钥读取失败：%v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(trimSpace(b)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		fatal("私钥格式非法（应为 base64 的 64 字节 ed25519 私钥）")
	}
	src, err := os.ReadFile(*in)
	if err != nil {
		fatal("输入读取失败：%v", err)
	}
	signed, err := handler.SignMarketIndexJSON(src, ed25519.PrivateKey(raw))
	if err != nil {
		fatal("签发失败：%v", err)
	}
	if err := os.WriteFile(*out, signed, 0o644); err != nil {
		fatal("写出失败：%v", err)
	}
	fmt.Printf("OK: %s → %s（iss=%s，%d 字节）\n", *in, *out, handler.MarketSignIssuer, len(signed))
}

func trimSpace(b []byte) []byte {
	s := 0
	e := len(b)
	for s < e && (b[s] == ' ' || b[s] == '\n' || b[s] == '\r' || b[s] == '\t') {
		s++
	}
	for e > s && (b[e-1] == ' ' || b[e-1] == '\n' || b[e-1] == '\r' || b[e-1] == '\t') {
		e--
	}
	return b[s:e]
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "marketgen: "+f+"\n", a...)
	os.Exit(1)
}
