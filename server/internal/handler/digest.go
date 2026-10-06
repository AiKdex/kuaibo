package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ---- P1-2 每日知识日报：手动触发 + 每日定时 ----

// digestRun POST /api/v1/digest/run：立即生成并推送日报（登录；便于验证与"日报"指令）。
func (a *API) digestRun(w http.ResponseWriter, r *http.Request) {
	if err := a.digest.Send(r.Context(), a.curUserID(r)); err != nil {
		writeErr(w, http.StatusBadGateway, "DIGEST_SEND_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// digestPreview GET /api/v1/digest/preview：预览今日日报文本（不推送）。
func (a *API) digestPreview(w http.ResponseWriter, r *http.Request) {
	rep, err := a.digest.Build(r.Context(), a.curUserID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DIGEST_BUILD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": rep, "text": a.digest.Render(rep)})
}

// DigestLoop 每日定时推送（hour 点，默认 8；digest.enabled=false 停用）。
func (a *API) DigestLoop(ctxDone <-chan struct{}) { a.digestLoop(ctxDone) }

// digestLoop 每日定时推送（hour 点，默认 8；digest.enabled=false 停用）。
func (a *API) digestLoop(ctxDone <-chan struct{}) {
	if !a.cfg.GetBool("digest.enabled") {
		return
	}
	hour := a.cfg.GetInt("digest.hour")
	if hour < 0 || hour > 23 {
		hour = 8
	}
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctxDone:
			timer.Stop()
			return
		case <-timer.C:
		}
		// 触发推送（owner；失败静默，下轮再试）
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = a.digest.Send(ctx, service.SystemOwnerID)
		cancel()
	}
}
