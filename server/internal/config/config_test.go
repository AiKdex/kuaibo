package config

import (
	"context"
	"os"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func TestDefaultsLoaded(t *testing.T) {
	c := New(nil, nil)
	if v, ok := c.Get("site.name"); !ok || v != "爱库录" {
		t.Fatalf("site.name=%v", v)
	}
	if c.GetString("site.language") != "zh-CN" {
		t.Fatal("language default")
	}
	if c.GetInt("index.chunk_size") != 800 {
		t.Fatal("chunk_size default")
	}
	if !c.GetBool("blog.rss_enabled") {
		t.Fatal("rss default")
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("AIKMAP_SITE_NAME", "MyServer")
	t.Setenv("AIKMAP_INDEX_CHUNK_SIZE", "1200")
	t.Setenv("AIKMAP_SECURITY_AUDIT_ENABLED", "true")
	c := New(nil, nil)
	if c.GetString("site.name") != "MyServer" {
		t.Fatalf("env override name=%s", c.GetString("site.name"))
	}
	if c.GetInt("index.chunk_size") != 1200 {
		t.Fatalf("env override chunk=%d", c.GetInt("index.chunk_size"))
	}
	if !c.GetBool("security.audit_enabled") {
		t.Fatal("env override bool")
	}
	// 未设置的不受影响
	if c.GetString("site.domain") != "localhost:8080" {
		t.Fatal("unset key changed")
	}
}

func TestSetPersistsAndBroadcasts(t *testing.T) {
	db, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	b := bus.New()
	c := New(db, b)

	got := ""
	b.Subscribe("config.changed", func(_ context.Context, e bus.Event) error {
		got = e.Key
		return nil
	})

	old, err := c.Set(context.Background(), "site.name", "Team Space", "string", "站点名", "admin")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if old != "爱库录" {
		t.Fatalf("old=%v", old)
	}
	if c.GetString("site.name") != "Team Space" {
		t.Fatal("memory not updated")
	}
	if got != "site.name" {
		t.Fatal("config.changed 未广播")
	}

	// 重启后从 DB 恢复
	c2 := New(db, nil)
	if err := c2.LoadFromDB(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}
	if c2.GetString("site.name") != "Team Space" {
		t.Fatalf("db not restored: %s", c2.GetString("site.name"))
	}
}

func TestSensitiveRedaction(t *testing.T) {
	db, _ := repo.Open(":memory:")
	defer db.Close()
	c := New(db, nil)
	_, _ = c.Set(context.Background(), "ai.llm.api_key", "sk-secret-123", "secret", "LLM key", "admin")
	snap := c.Snapshot()
	if e, ok := snap["ai.llm.api_key"]; ok && !e.Encrypted {
		t.Fatal("敏感项未标记 encrypted")
	}
}

func TestUnknownKeyGet(t *testing.T) {
	c := New(nil, nil)
	if v, ok := c.Get("no.such.key"); ok {
		t.Fatalf("unexpected %v", v)
	}
	_ = os.Getenv("NOOP")
}
