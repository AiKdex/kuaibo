package bus

import (
	"context"
	"sync"
	"testing"
)

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := New()
	var mu sync.Mutex
	got := []string{}
	unsub := b.Subscribe("file.created", func(_ context.Context, e Event) error {
		mu.Lock()
		got = append(got, e.Key)
		mu.Unlock()
		return nil
	})
	defer unsub()

	b.Publish(context.Background(), Event{Topic: "file.created", Key: "f1"})
	b.Publish(context.Background(), Event{Topic: "file.created", Key: "f2"})

	if len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
		t.Fatalf("got %v", got)
	}
}

func TestUnsubscribe(t *testing.T) {
	b := New()
	calls := 0
	unsub := b.Subscribe("x", func(_ context.Context, e Event) error { calls++; return nil })
	unsub()
	b.Publish(context.Background(), Event{Topic: "x"})
	if calls != 0 {
		t.Fatalf("calls=%d, want 0", calls)
	}
}

func TestTopicIsolation(t *testing.T) {
	b := New()
	got := 0
	unsub := b.Subscribe("a", func(_ context.Context, e Event) error { got++; return nil })
	defer unsub()
	b.Publish(context.Background(), Event{Topic: "b"})
	if got != 0 {
		t.Fatalf("cross-topic delivery")
	}
}
