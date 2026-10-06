// vectorsearch.go 向量检索：query embedding → 与库内文件向量算余弦相似度 → TopN。
// 与关键词检索（Search）在工具层做 RRF 融合，构成"真语义检索"。
// 规模纪律：全表扫描算余弦在 SQLite 数万块内可接受；量上来后经 VectorSink 迁 Qdrant。
package ai

import (
	"context"
	"database/sql"
	"math"
	"sort"
)

// VecHit 向量检索命中（携带最佳块，供溯源定位/片段展示）。
type VecHit struct {
	FileID  string
	ChunkID string
	Content string
	Score   float64 // 余弦相似度 0..1
}

// VectorSearch 检索与 query 语义最相近的文件（按文件聚合取最高块分，携带该块定位信息）。
func VectorSearch(ctx context.Context, db *sql.DB, gate *Gateway, model string, spaceID, query string, limit int) ([]VecHit, error) {
	if gate == nil || model == "" {
		return nil, nil
	}
	vecs, err := gate.Embedding(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, nil // embedding 失败降级：返回空（调用方走关键词）
	}
	q := vecs[0]
	rows, err := db.QueryContext(ctx, `
		SELECT v.file_id, v.chunk_id, c.content, v.vec FROM vectors v
		JOIN files f ON f.id = v.file_id
		LEFT JOIN index_chunks c ON c.id = v.chunk_id
		WHERE v.model = ? AND f.space_id = ? AND f.deleted_at IS NULL`, model, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type agg struct {
		score   float64
		chunkID string
		content string
	}
	seen := map[string]*agg{}
	for rows.Next() {
		var fid, cid string
		var content sql.NullString
		var blob []byte
		if err := rows.Scan(&fid, &cid, &content, &blob); err != nil {
			return nil, err
		}
		vec := decodeVec(blob)
		if len(vec) != len(q) {
			continue
		}
		s := cosine(q, vec)
		if a, ok := seen[fid]; ok {
			if s > a.score {
				a.score = s
				a.chunkID = cid
				a.content = content.String
			}
		} else {
			seen[fid] = &agg{score: s, chunkID: cid, content: content.String}
		}
	}
	sorted := make([]VecHit, 0, len(seen))
	for fid, a := range seen {
		sorted = append(sorted, VecHit{FileID: fid, ChunkID: a.chunkID, Content: a.content, Score: a.score})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted, nil
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// RelatedByVector 以源文件的「块向量质心」为查询，对同空间其他文件做余弦相似度检索。
// 用于「相关推荐」——比同分类+时间排序更贴语义；每文件取其最高分块（与 VectorSearch 一致）。
// excludeID 为源文件自身（不出现在结果里）。向量缺失/维度不一致时跳过。
func RelatedByVector(ctx context.Context, db *sql.DB, model, spaceID, excludeID string, limit int) ([]VecHit, error) {
	if db == nil || model == "" || spaceID == "" || limit <= 0 {
		return nil, nil
	}
	// 1) 源文件向量质心（对其所有块向量按分量求均值）
	srcRows, err := db.QueryContext(ctx,
		`SELECT vec FROM vectors WHERE file_id=? AND space_id=? AND model=?`, excludeID, spaceID, model)
	if err != nil {
		return nil, err
	}
	defer srcRows.Close()
	var sum []float64
	var n int
	for srcRows.Next() {
		var blob []byte
		if err := srcRows.Scan(&blob); err != nil {
			return nil, err
		}
		v := decodeVec(blob)
		if len(v) == 0 {
			continue
		}
		if sum == nil {
			sum = make([]float64, len(v))
		}
		if len(v) != len(sum) {
			continue // 维度不一致（模型切换后残留）→ 跳过该块
		}
		for i, f := range v {
			sum[i] += float64(f)
		}
		n++
	}
	if n == 0 || sum == nil {
		return nil, nil // 源文件未 embedding → 调用方降级
	}
	q := make([]float32, len(sum))
	for i := range sum {
		q[i] = float32(sum[i] / float64(n))
	}

	// 2) 同空间其他文件向量与质心比对
	rows, err := db.QueryContext(ctx, `
		SELECT v.file_id, v.chunk_id, v.vec FROM vectors v
		JOIN files f ON f.id = v.file_id
		WHERE v.model = ? AND v.space_id = ? AND f.deleted_at IS NULL AND f.id <> ?`,
		model, spaceID, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	best := map[string]*VecHit{}
	for rows.Next() {
		var fid, cid string
		var blob []byte
		if err := rows.Scan(&fid, &cid, &blob); err != nil {
			return nil, err
		}
		v := decodeVec(blob)
		if len(v) != len(q) {
			continue
		}
		s := cosine(q, v)
		if cur, ok := best[fid]; !ok || s > cur.Score {
			best[fid] = &VecHit{FileID: fid, ChunkID: cid, Score: s}
		}
	}
	out := make([]VecHit, 0, len(best))
	for _, v := range best {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// RRF 融合关键词与向量两个排序结果（Reciprocal Rank Fusion）。
func RRF(kw []string, vec []VecHit, k float64, limit int) []string {
	rank := map[string]float64{}
	for i, id := range kw {
		rank[id] += 1 / (k + float64(i+1))
	}
	for i, h := range vec {
		rank[h.FileID] += 1 / (k + float64(i+1))
	}
	ids := make([]string, 0, len(rank))
	for id := range rank {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return rank[ids[i]] > rank[ids[j]] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

// toolRerankFuse Agent 工具侧精排（K18）：对 RRF 融合后的候选文件取首索引块内容，
// 交 rerank provider 按 query 相关性二次打分重排；未配置/失败保持原序。
func toolRerankFuse(ctx context.Context, db *sql.DB, gate *Gateway, q string, ids []string) []string {
	if gate == nil || db == nil || q == "" || len(ids) < 2 {
		return ids
	}
	topN := gate.cfg.GetInt("ai.rerank.top_n")
	if topN < 5 || topN > 100 {
		topN = 15
	}
	if len(ids) > topN {
		ids = ids[:topN]
	}
	docs := make([]string, 0, len(ids))
	for _, id := range ids {
		var content string
		_ = db.QueryRowContext(ctx,
			`SELECT content FROM index_chunks WHERE file_id=? ORDER BY chunk_idx LIMIT 1`,
			id).Scan(&content)
		if r := []rune(content); len(r) > 300 {
			content = string(r[:300])
		}
		docs = append(docs, content)
	}
	scores, err := gate.Rerank(ctx, q, docs)
	if err != nil || len(scores) != len(ids) {
		return ids
	}
	type pair struct {
		id string
		s  float32
	}
	ps := make([]pair, len(ids))
	for i, id := range ids {
		ps[i] = pair{id, scores[i]}
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].s > ps[j].s })
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.id
	}
	return out
}
