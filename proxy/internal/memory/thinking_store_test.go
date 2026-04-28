package memory

import (
	"path/filepath"
	"testing"
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
