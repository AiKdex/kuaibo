// provider_store.go 模型提供方的 DB 持久化（ai_providers 表）。
// 与 providers.json 解耦：内置提供方首次启动种子入表（builtin=1），用户新增/编辑的
// 提供方 builtin=0；网关注册表从本持久化层全量加载，增删改即时热更新、无需重启。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// SaveProviderDef 将 provider 定义持久化到 ai_providers（UPSERT）。
// name 来自 defs 的 map key；keys 以明文存于 keys 列（与 secrets/providers.json 一致：
// 该文件同样明文存密钥且 gitignore；后续如需加密可在此对接 config 加密通道）。
func SaveProviderDef(db *sql.DB, name string, def *ProviderDef) error {
	models, _ := json.Marshal(def.Models)
	cands, _ := json.Marshal(def.ModelCands)
	caps, _ := json.Marshal(def.Caps)
	voices, _ := json.Marshal(def.VoiceCands)
	keys, _ := json.Marshal(def.Keys)
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO ai_providers (name, endpoint, model, models, model_cands, keys, caps, voice_cands, note, builtin, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET
			endpoint=excluded.endpoint, model=excluded.model, models=excluded.models,
			model_cands=excluded.model_cands, keys=excluded.keys, caps=excluded.caps,
			voice_cands=excluded.voice_cands, note=excluded.note, builtin=excluded.builtin, updated_at=excluded.updated_at`,
		name, def.Endpoint, def.Model, string(models), string(cands), string(keys), string(caps),
		string(voices), def.Note, boolToInt(def.Builtin), time.Now().UnixMilli())
	return err
}

// LoadProviderDefs 从 DB 读取全部 provider（含内置），name 作为返回的 map key。
func LoadProviderDefs(db *sql.DB) (map[string]*ProviderDef, error) {
	rows, err := db.QueryContext(context.Background(), `
		SELECT name, endpoint, model, models, model_cands, keys, caps, voice_cands, note, builtin
		FROM ai_providers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*ProviderDef{}
	for rows.Next() {
		var name, endpoint, model, models, cands, keys, caps, voices, note string
		var builtin int
		if err := rows.Scan(&name, &endpoint, &model, &models, &cands, &keys, &caps, &voices, &note, &builtin); err != nil {
			return nil, err
		}
		d := &ProviderDef{
			Endpoint: endpoint,
			Model:    model,
			Note:     note,
			Builtin:  builtin == 1,
		}
		_ = json.Unmarshal([]byte(models), &d.Models)
		_ = json.Unmarshal([]byte(cands), &d.ModelCands)
		_ = json.Unmarshal([]byte(keys), &d.Keys)
		_ = json.Unmarshal([]byte(caps), &d.Caps)
		_ = json.Unmarshal([]byte(voices), &d.VoiceCands)
		out[name] = d
	}
	return out, rows.Err()
}

// DeleteProviderDef 删除一个 provider（含 builtin 标记；调用方负责禁止删内置）。
func DeleteProviderDef(db *sql.DB, name string) error {
	_, err := db.ExecContext(context.Background(), `DELETE FROM ai_providers WHERE name=?`, name)
	return err
}

// ProviderDefExists 判断某 provider 是否已持久化（种子逻辑用：仅缺失时插入内置）。
func ProviderDefExists(db *sql.DB, name string) (bool, error) {
	var cnt int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM ai_providers WHERE name=?`, name).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
