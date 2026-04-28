package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestThinkingStoreGetTouchesExactEntry(t *testing.T) {
	store := NewThinkingStore(ThinkingStoreConfig{
		MaxEntries:      2,
		EntryTTLMinutes: 60,
		PersistPath:     filepath.Join(t.TempDir(), "thinking-store.db"),
	})
	defer store.Close()

	store.Put("prefix-a", "visible-a", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-a"},
		{"type": "text", "text": "A"},
	})
	store.Put("prefix-b", "visible-b", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-b"},
		{"type": "text", "text": "B"},
	})

	if content, source := store.Get("prefix-a", "visible-a"); content == nil || source != "exact" {
		t.Fatalf("expected exact match before eviction, got content=%v source=%q", content, source)
	}

	store.Put("prefix-c", "visible-c", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-c"},
		{"type": "text", "text": "C"},
	})

	if content, _ := store.Get("prefix-b", "visible-b"); content != nil {
		t.Fatal("expected untouched entry to be evicted first")
	}
	if content, source := store.Get("prefix-a", "visible-a"); content == nil || source != "exact" {
		t.Fatalf("expected recently accessed entry to survive eviction, got content=%v source=%q", content, source)
	}
}

func TestThinkingStorePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thinking-store.db")
	cfg := ThinkingStoreConfig{
		MaxEntries:      10,
		EntryTTLMinutes: 60,
		PersistPath:     path,
	}

	store := NewThinkingStore(cfg)
	store.Put("prefix-a", "visible-a", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-a"},
		{"type": "text", "text": "A"},
	})
	store.Close()

	reloaded := NewThinkingStore(cfg)
	defer reloaded.Close()

	content, source := reloaded.Get("prefix-a", "visible-a")
	if content == nil || source != "exact" {
		t.Fatalf("expected persisted exact match after restart, got content=%v source=%q", content, source)
	}
	if got := content[0]["signature"]; got != "sig-a" {
		t.Fatalf("expected persisted signature sig-a, got %v", got)
	}
}

func TestThinkingStoreReloadPurgesExpiredAndExcessEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thinking-store.db")
	cfg := ThinkingStoreConfig{
		MaxEntries:      4,
		EntryTTLMinutes: 60,
		PersistPath:     path,
	}

	store := NewThinkingStore(cfg)
	store.Put("prefix-a", "visible-a", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-a"},
		{"type": "text", "text": "A"},
	})
	store.Put("prefix-b", "visible-b", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-b"},
		{"type": "text", "text": "B"},
	})
	store.Put("prefix-c", "visible-c", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-c"},
		{"type": "text", "text": "C"},
	})
	store.Put("prefix-d", "visible-d", []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-d"},
		{"type": "text", "text": "D"},
	})

	now := time.Now()
	store.mu.Lock()
	store.entries[makeThinkingStoreKey("prefix-a", "visible-a")].CachedAt = now.Add(-2 * time.Hour)
	store.entries[makeThinkingStoreKey("prefix-b", "visible-b")].CachedAt = now.Add(-50 * time.Minute)
	store.entries[makeThinkingStoreKey("prefix-c", "visible-c")].CachedAt = now.Add(-10 * time.Minute)
	store.entries[makeThinkingStoreKey("prefix-d", "visible-d")].CachedAt = now.Add(-5 * time.Minute)
	for key, entry := range store.entries {
		store.queueUpsertLocked(key, entry)
	}
	store.mu.Unlock()
	store.Close()

	reloaded := NewThinkingStore(ThinkingStoreConfig{
		MaxEntries:      2,
		EntryTTLMinutes: 60,
		PersistPath:     path,
	})
	defer reloaded.Close()

	if got := reloaded.Size(); got != 2 {
		t.Fatalf("expected 2 active entries after reload cleanup, got %d", got)
	}

	if content, _ := reloaded.Get("prefix-a", "visible-a"); content != nil {
		t.Fatal("expected expired entry to be purged on reload")
	}
	if content, _ := reloaded.Get("prefix-b", "visible-b"); content != nil {
		t.Fatal("expected excess oldest active entry to be purged on reload")
	}
	if content, source := reloaded.Get("prefix-c", "visible-c"); content == nil || source != "exact" {
		t.Fatalf("expected recent entry C to survive reload cleanup, got content=%v source=%q", content, source)
	}
	if content, source := reloaded.Get("prefix-d", "visible-d"); content == nil || source != "exact" {
		t.Fatalf("expected recent entry D to survive reload cleanup, got content=%v source=%q", content, source)
	}
}
