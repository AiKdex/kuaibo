// system_backup.go 备份动态管理（后台开关 + 周期设置）。
// GET  /api/v1/system/backup —— 读当前备份配置（enabled/schedule/keep/last_at）
// PUT  /api/v1/system/backup —— 写备份配置（cfg.Set 持久化 settings 表 + 广播，≤1min 生效）
package handler

import (
	"encoding/json"
	"net/http"
)

// backupConfig 备份配置视图。
type backupConfig struct {
	Enabled    bool   `json:"enabled"`
	Schedule   string `json:"schedule"`    // HH:MM，每日
	KeepLocal  int    `json:"keep_local"`  // 本地保留份数
	KeepRemote int    `json:"keep_remote"` // 远端保留份数
	LastBackup string `json:"last_backup"` // 最近一次成功备份时间（RFC3339，空=尚未备份）
	Note       string `json:"note"`
}

// systemBackupGet GET /api/v1/system/backup（管理员）。
func (a *API) systemBackupGet(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "仅管理员可查看备份配置")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "backup": a.backupConfigView()})
}

// systemBackupPut PUT /api/v1/system/backup（管理员）。
// body：{"enabled":bool?, "schedule":"HH:MM"?, "keep_local":int?, "keep_remote":int?}
// 缺省字段表示不修改该键。
func (a *API) systemBackupPut(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "ADMIN_REQUIRED", "仅管理员可修改备份配置")
		return
	}
	var req struct {
		Enabled    *bool   `json:"enabled"`
		Schedule   *string `json:"schedule"`
		KeepLocal  *int    `json:"keep_local"`
		KeepRemote *int    `json:"keep_remote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if req.Schedule != nil {
		if !validHHMM(*req.Schedule) {
			writeErr(w, http.StatusBadRequest, "BAD_SCHEDULE", "schedule 格式应为 HH:MM（如 03:00），小时 0-23、分钟 0-59")
			return
		}
	}
	if req.KeepLocal != nil && *req.KeepLocal < 1 {
		writeErr(w, http.StatusBadRequest, "BAD_KEEP", "keep_local 至少为 1")
		return
	}
	if req.KeepRemote != nil && *req.KeepRemote < 1 {
		writeErr(w, http.StatusBadRequest, "BAD_KEEP", "keep_remote 至少为 1")
		return
	}
	uid := a.curUserID(r)
	if req.Enabled != nil {
		_, _ = a.cfg.Set(r.Context(), "ops.backup_enabled", *req.Enabled, "bool", "备份总开关（后台可动态开关）", uid)
	}
	if req.Schedule != nil {
		_, _ = a.cfg.Set(r.Context(), "ops.backup_schedule", *req.Schedule, "string", "每日备份时刻 HH:MM（改设置≤1min 生效）", uid)
	}
	if req.KeepLocal != nil {
		_, _ = a.cfg.Set(r.Context(), "ops.backup_keep_local", *req.KeepLocal, "int", "本地保留份数", uid)
	}
	if req.KeepRemote != nil {
		_, _ = a.cfg.Set(r.Context(), "ops.backup_keep_remote", *req.KeepRemote, "int", "远端保留份数", uid)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "backup": a.backupConfigView()})
}

// backupConfigView 组装配置视图。
func (a *API) backupConfigView() backupConfig {
	return backupConfig{
		Enabled:    a.cfg.GetBool("ops.backup_enabled"),
		Schedule:   a.cfg.GetString("ops.backup_schedule"),
		KeepLocal:  a.cfg.GetInt("ops.backup_keep_local"),
		KeepRemote: a.cfg.GetInt("ops.backup_keep_remote"),
		LastBackup: a.cfg.GetString("ops.backup_last_at"),
		Note:       "备份=VACUUM INTO 一致性快照 → storage 远端；本地/远端保留份数独立控制。",
	}
}

// validHHMM 校验 HH:MM（小时 0-23、分钟 0-59）。
func validHHMM(s string) bool {
	if len(s) < 3 || len(s) > 5 {
		return false
	}
	colon := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			colon = i
			break
		}
	}
	if colon <= 0 || colon >= len(s)-1 {
		return false
	}
	h, m := 0, 0
	for i := 0; i < colon; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		h = h*10 + int(s[i]-'0')
	}
	for i := colon + 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		m = m*10 + int(s[i]-'0')
	}
	return h <= 23 && m <= 59
}
