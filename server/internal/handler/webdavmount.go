// webdavmount.go 外部 WebDAV 挂载 HTTP 入口。
//
//	GET    /api/v1/webdav/mounts                挂载列表（密码脱敏）
//	POST   /api/v1/webdav/mounts                新增挂载（含可选连通测试）
//	PUT    /api/v1/webdav/mounts/{id}           更新挂载（password 空=保留）
//	DELETE /api/v1/webdav/mounts/{id}           删除挂载
//	POST   /api/v1/webdav/mounts/{id}/test      连通测试
//	GET    /api/v1/webdav/mounts/{id}/list?path= 浏览远端目录
//	GET    /api/v1/webdav/mounts/{id}/content?path= 文本直读预览
//	POST   /api/v1/webdav/mounts/{id}/import    导入远端文件/目录到知识库（异步 jobs）
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// webdavMountsList GET /api/v1/webdav/mounts。
func (a *API) webdavMountsList(w http.ResponseWriter, r *http.Request) {
	items, err := a.webdav.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MOUNTS_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// webdavMountsCreate POST /api/v1/webdav/mounts。
func (a *API) webdavMountsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		Test     bool   `json:"test"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "MOUNT_FIELDS_REQUIRED", "名称和地址必填")
		return
	}
	if req.Test {
		msg := a.webdavTestConn(req.URL, req.Username, req.Password)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, "MOUNT_TEST_FAILED", msg)
			return
		}
	}
	m, err := a.webdav.Create(r.Context(), req.Name, req.URL, req.Username, req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MOUNT_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// webdavMountsUpdate PUT /api/v1/webdav/mounts/{id}。
func (a *API) webdavMountsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		Test     bool   `json:"test"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "MOUNT_FIELDS_REQUIRED", "名称和地址必填")
		return
	}
	if req.Test {
		msg := a.webdavTestConn(req.URL, req.Username, req.Password)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, "MOUNT_TEST_FAILED", msg)
			return
		}
	}
	m, err := a.webdav.Update(r.Context(), id, req.Name, req.URL, req.Username, req.Password)
	if err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", "挂载不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "MOUNT_UPDATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// webdavMountsDelete DELETE /api/v1/webdav/mounts/{id}。
func (a *API) webdavMountsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.webdav.Delete(r.Context(), id); err != nil {
		if err == service.ErrNotFound {
			writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", "挂载不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "MOUNT_DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// webdavMountTest POST /api/v1/webdav/mounts/{id}/test。
func (a *API) webdavMountTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pass, err := a.webdav.Password(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", "挂载不存在")
		return
	}
	m, _ := a.webdav.Get(r.Context(), id)
	msg := a.webdavTestConn(m.URL, m.Username, pass)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, "MOUNT_TEST_FAILED", msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "连接正常"})
}

func (a *API) webdavTestConn(rawURL, username, password string) string {
	c, err := service.NewWmClient(rawURL, username, password)
	if err != nil {
		return err.Error()
	}
	if err := c.Test(); err != nil {
		return err.Error()
	}
	return ""
}

// webdavMountList GET /api/v1/webdav/mounts/{id}/list?path=。
func (a *API) webdavMountList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	c, err := a.wmClientFor(r, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", err.Error())
		return
	}
	items, err := c.ListDir(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "MOUNT_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// webdavMountContent GET /api/v1/webdav/mounts/{id}/content?path=。
func (a *API) webdavMountContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	c, err := a.wmClientFor(r, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", err.Error())
		return
	}
	data, err := c.Read(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "MOUNT_READ_FAILED", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// webdavMountImport POST /api/v1/webdav/mounts/{id}/import。
// body: {path, parent_id} —— 远端文件导入知识库（目录导入为递归第一层文件）。
func (a *API) webdavMountImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Path     string `json:"path"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, "IMPORT_PATH_REQUIRED", "远端路径必填")
		return
	}
	c, err := a.wmClientFor(r, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "MOUNT_NOT_FOUND", err.Error())
		return
	}
	uid := a.curUserID(r)
	spaceID := a.curHomeSpaceID(r)
	imported, skipped, err := a.importWmPath(r, c, req.Path, spaceID, req.ParentID, uid)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "IMPORT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped})
}

// wmClientFor 取挂载并构造客户端。
func (a *API) wmClientFor(r *http.Request, id string) (*service.WmClient, error) {
	m, err := a.webdav.Get(r.Context(), id)
	if err != nil {
		return nil, err
	}
	pass, err := a.webdav.Password(r.Context(), id)
	if err != nil {
		return nil, err
	}
	return service.NewWmClient(m.URL, m.Username, pass)
}

// importWmPath 导入远端路径：文件→直接入库；目录→递归（仅第一层，防风暴）。
func (a *API) importWmPath(r *http.Request, c *service.WmClient, relPath, spaceID, parentID, uid string) (int, int, error) {
	// 判断是文件还是目录：用父目录 ListDir 探测（depth=1）
	parent := ""
	if i := strings.LastIndex(relPath, "/"); i >= 0 {
		parent = relPath[:i]
	}
	items, err := c.ListDir(parent)
	if err != nil {
		return 0, 0, err
	}
	var target *service.WmItem
	for i := range items {
		if items[i].Path == relPath || items[i].Name == relPath {
			target = &items[i]
			break
		}
	}
	if target == nil {
		return 0, 0, errors.New("远端路径不存在")
	}
	imported, skipped := 0, 0
	if target.Type == "file" {
		ok, err := a.importWmFile(r, c, *target, spaceID, parentID, uid)
		if err != nil {
			return 0, 0, err
		}
		if ok {
			imported++
		} else {
			skipped++
		}
		return imported, skipped, nil
	}
	// 目录：导入第一层文件
	sub, err := c.ListDir(relPath)
	if err != nil {
		return 0, 0, err
	}
	var firstErr error
	for _, it := range sub {
		if it.Type != "file" {
			continue
		}
		ok, err := a.importWmFile(r, c, it, spaceID, parentID, uid)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			imported++
		} else {
			skipped++
		}
	}
	if firstErr != nil {
		return imported, skipped, firstErr
	}
	return imported, skipped, nil
}

// importWmFile 导入单个远端文件：文本→CreateDoc；二进制→Upload。
func (a *API) importWmFile(r *http.Request, c *service.WmClient, it service.WmItem, spaceID, parentID, uid string) (bool, error) {
	if !it.Importable {
		return false, nil
	}
	if it.Text {
		data, err := c.Read(it.Path)
		if err != nil {
			return false, err
		}
		_, err = a.files.CreateDoc(r.Context(), uid, spaceID, parentID, it.Name, string(data), service.DefaultSiteID)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	rc, err := c.ReadStream(it.Path)
	if err != nil {
		return false, err
	}
	defer rc.Close()
	_, err = a.files.Upload(r.Context(), uid, spaceID, parentID, it.Name, "application/octet-stream", rc, it.Size, service.DefaultSiteID)
	if err != nil {
		return false, err
	}
	return true, nil
}
