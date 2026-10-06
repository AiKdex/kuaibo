// Package storage 实现存储抽象层（受管多后端，架构 §10 / 实施文档 §4.4）。
//
// 统一接口 Put/Get/Delete/List/Stat/Move；后端实现：本地磁盘、NAS(SMB/NFS)、S3/OSS、WebDAV。
// 核心边界：内容留在存储后端原位，系统只存索引与元数据；本层不负责语义，只负责字节。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNotFound 后端不存在该对象。
var ErrNotFound = errors.New("storage: not found")

// ObjectMeta 对象元信息（系统在 files 表冗余部分字段，此处用于对账）。
type ObjectMeta struct {
	Key      string    // 后端内相对路径
	Size     int64
	ModTime  time.Time
	IsDir    bool
	Checksum string // 后端可提供的强校验（如 S3 ETag），可空
}

// Backend 存储后端统一接口。
type Backend interface {
	Name() string
	// Put 写入对象；overwrite 语义由调用方保证（文件引擎层做版本管理）。
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	// Mkdir 创建目录（幂等；用于文件系统类后端表达目录语义）。
	Mkdir(ctx context.Context, key string) error
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]ObjectMeta, error)
	Stat(ctx context.Context, key string) (ObjectMeta, error)
	Move(ctx context.Context, from, to string) error
}

// Local 是本地磁盘后端（默认；也用于 NAS 挂载点/SMB 等本地路径）。
type Local struct {
	name string
	root string
}

// NewLocal 创建本地后端。root 为宿主机绝对路径（如 ./data/files 或 /mnt/nas）。
func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("storage local mkdir: %w", err)
	}
	return &Local{name: "local", root: abs}, nil
}

// Name 返回后端名。
func (l *Local) Name() string { return l.name }

// Mkdir 创建目录（幂等）。
func (l *Local) Mkdir(ctx context.Context, key string) error {
	full, err := l.full(key)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0o755)
}

func (l *Local) full(key string) (string, error) {
	// 防路径穿越：拒绝绝对路径、.. 段与空路径
	clean := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(key), "/"))
	if clean == "." || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(key) || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: unsafe key %q", key)
	}
	final := filepath.Join(l.root, clean)
	// 纵深防御：解析后必须仍位于 root 内（防未来接口直传 key 引入的边界漏洞）
	if rel, err := filepath.Rel(l.root, final); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: unsafe key %q", key)
	}
	return final, nil
}

// Put 写入对象（父目录自动创建）。
func (l *Local) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	full, err := l.full(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return f.Sync()
}

// Get 读取对象。
func (l *Local) Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	full, err := l.full(key)
	if err != nil {
		return nil, ObjectMeta{}, err
	}
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ObjectMeta{}, ErrNotFound
		}
		return nil, ObjectMeta{}, err
	}
	if st.IsDir() {
		return nil, ObjectMeta{}, errors.New("storage: key is dir")
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, ObjectMeta{}, err
	}
	return f, ObjectMeta{Key: key, Size: st.Size(), ModTime: st.ModTime()}, nil
}

// Delete 删除对象（仅文件；空目录清理交给对账任务）。
func (l *Local) Delete(ctx context.Context, key string) error {
	full, err := l.full(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// List 列出前缀下的对象（递归，供对账/备份）。
func (l *Local) List(ctx context.Context, prefix string) ([]ObjectMeta, error) {
	base, err := l.full(prefix)
	if err != nil {
		return nil, err
	}
	var out []ObjectMeta
	err = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if p == base {
			return nil
		}
		rel, _ := filepath.Rel(l.root, p)
		out = append(out, ObjectMeta{
			Key: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime(), IsDir: info.IsDir(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Stat 获取对象元信息。
func (l *Local) Stat(ctx context.Context, key string) (ObjectMeta, error) {
	full, err := l.full(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, err
	}
	return ObjectMeta{Key: key, Size: st.Size(), ModTime: st.ModTime(), IsDir: st.IsDir()}, nil
}

// Move 移动/重命名对象（同后端内）。
func (l *Local) Move(ctx context.Context, from, to string) error {
	fullFrom, err := l.full(from)
	if err != nil {
		return err
	}
	fullTo, err := l.full(to)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullTo), 0o755); err != nil {
		return err
	}
	if err := os.Rename(fullFrom, fullTo); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// PathLocator 是「本地类后端」可选实现的能力接口：把后端内 key 解析为宿主机绝对路径。
//
// 为什么需要：有些操作必须**就地读取**宿主机文件，而不是拿到 io.Reader 顺序读 ——
// 典型如 ffprobe 解析 mp4（moov 原子可能在文件尾，纯流式读取不可靠）。
// S3 / OSS / WebDAV 等远程后端不实现该接口，调用方应据此**降级**
// （记 probe_status='unsupported'），而不是把整个对象先拉回本地。
type PathLocator interface {
	// LocalPath 返回 key 对应的宿主机绝对路径；key 非法、对象不存在或为目录时 ok=false。
	LocalPath(key string) (string, bool)
}

// LocalPath 实现 PathLocator（仅本地磁盘后端；也适用于挂载到本地路径的 NAS 场景）。
func (l *Local) LocalPath(key string) (string, bool) {
	full, err := l.full(key)
	if err != nil {
		return "", false
	}
	st, err := os.Stat(full)
	if err != nil || st.IsDir() {
		return "", false
	}
	return full, true
}
