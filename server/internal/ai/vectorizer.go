// vectorizer.go 向量化后台 worker：把已分块索引的文本（index_chunks）批量 embedding 写入 vectors 表。
// 与 Summarizer 同模式：单 worker 串行分批（天然限流）、事件订阅增量、启动补量。
// 向量与 embedding 模型绑定：模型切换后旧向量作废，由 RebuildModel 清空重建。
package ai

import (
	"context"
	"database/sql"
	"encoding/binary"
	"log"
	"math"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
)

const vecBatchSize = 16 // 每批向量化块数（embedding 一次请求）

// Vectorizer 后台向量化 worker。
type Vectorizer struct {
	db   *sql.DB
	gate *Gateway
	b    *bus.Bus
	dim  int
}

// NewVectorizer 创建向量化 worker。模型名不再冻结——process 每次现取 gate 当前 embedding 模型，
// 避免热切换后"按旧 model 过滤、新模型向量永不生成"的静默失效（P1-14）。
func NewVectorizer(db *sql.DB, gate *Gateway, b *bus.Bus, dim int) *Vectorizer {
	return &Vectorizer{db: db, gate: gate, b: b, dim: dim}
}

// Run 启动 worker 循环（阻塞）。有活时短间隔，空闲时拉长。
func (v *Vectorizer) Run(ctx context.Context) {
	if v.b != nil {
		// 文件入库后触发一次处理（索引同步、向量化异步跟跑）。
		// 必须异步：embedding 是慢调用（秒级），同步执行会让上传/新建文档接口阻塞到 embedding 完成
		// （曾有 createDoc 因此耗时 ~56s 的回归）。Run 循环持续兜底，这里只做即时唤醒。
		unsub := v.b.Subscribe("file.created", func(ctx context.Context, _ bus.Event) error {
			go v.process(ctx)
			return nil
		})
		defer unsub()
	}
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := v.process(ctx)
		if err != nil {
			log.Printf("[vectorizer] %v", err)
		}
		if n > 0 {
			time.Sleep(2 * time.Second) // 有活短歇，让出 CPU
		} else {
			time.Sleep(8 * time.Second)
		}
	}
}

// process 取一批待向量块 → embedding → 写库。返回处理块数。
func (v *Vectorizer) process(ctx context.Context) (int, error) {
	model := v.gate.EmbeddingModel()
	if model == "" {
		return 0, nil // 未配置模型（provider 未就绪）
	}
	rows, err := v.db.QueryContext(ctx, `
		SELECT c.id, c.file_id, c.content FROM index_chunks c
		LEFT JOIN vectors vec ON vec.chunk_id = c.id AND vec.model = ?
		WHERE vec.id IS NULL AND c.status = 'indexed'
		ORDER BY c.id LIMIT ?`, model, vecBatchSize)
	if err != nil {
		return 0, err
	}
	type chunk struct {
		id, fileID, content string
	}
	var batch []chunk
	for rows.Next() {
		var c chunk
		if err := rows.Scan(&c.id, &c.fileID, &c.content); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, c)
	}
	rows.Close()
	if len(batch) == 0 {
		return 0, nil
	}
	texts := make([]string, len(batch))
	for i, c := range batch {
		texts[i] = c.content
	}
	vecs, err := v.gate.Embedding(ctx, texts)
	if err != nil || len(vecs) != len(batch) {
		// 失败跳过（下次轮询重试），但必须留痕，避免 provider 异常时无限静默空转
		log.Printf("[vectorizer] embedding failed: batch=%d err=%v vecs=%d model=%s", len(batch), err, len(vecs), model)
		return 0, nil
	}
	now := time.Now().Unix()
	for i, c := range batch {
		if _, err := v.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO vectors (id, chunk_id, file_id, space_id, model, vec, created_at)
			 VALUES (NULL, ?, ?, (SELECT space_id FROM index_chunks WHERE id=?), ?, ?, ?)`,
			c.id, c.fileID, c.id, model, encodeVec(vecs[i]), now); err != nil {
			return 0, err
		}
	}
	// 触发索引完成事件（供依赖方感知）
	if v.b != nil {
		v.b.Publish(ctx, bus.Event{Topic: "vector.indexed", Data: map[string]any{"count": len(batch)}})
	}
	return len(batch), nil
}

// CleanupStaleModels 启动时清理与当前模型不一致的旧向量（模型切换后旧向量作废）。
// 替代原先"硬编码模型名全量重建"——避免当前模型恰为该名时每次重启清库重建。
func (v *Vectorizer) CleanupStaleModels(ctx context.Context) (int64, error) {
	model := v.gate.EmbeddingModel()
	if model == "" {
		return 0, nil
	}
	res, err := v.db.ExecContext(ctx, `DELETE FROM vectors WHERE model <> ?`, model)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// encodeVec float32 向量 → little-endian 字节（与向量检索端一致）。
func encodeVec(vec []float32) []byte {
	buf := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[4*i:], math.Float32bits(f))
	}
	return buf
}

// decodeVec 字节 → float32 向量。
func decodeVec(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out
}
