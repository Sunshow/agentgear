package memory

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type ThinkingEntry struct {
	PrefixHash      string
	VisibleHash     string
	Content         []map[string]interface{}
	CachedAt        time.Time
	lastPersistedAt time.Time
}

type ThinkingStoreConfig struct {
	MaxEntries      int    `mapstructure:"max_entries"`
	EntryTTLMinutes int    `mapstructure:"entry_ttl_minutes"`
	PersistPath     string `mapstructure:"persist_path"`
}

func DefaultThinkingStoreConfig() ThinkingStoreConfig {
	return ThinkingStoreConfig{
		MaxEntries:      5000,
		EntryTTLMinutes: 24 * 60,
		PersistPath:     "./data/thinking_store.json",
	}
}

type ThinkingStore struct {
	mu           sync.RWMutex
	entries      map[string]*ThinkingEntry
	config       ThinkingStoreConfig
	stopCleanup  chan struct{}
	stopPersist  chan struct{}
	persistDirty chan struct{}
	persistDone  chan struct{}
	closeOnce    sync.Once
}

type persistedThinkingStore struct {
	Version int              `json:"version"`
	Entries []*ThinkingEntry `json:"entries"`
}

const (
	thinkingStorePersistVersion       = 1
	thinkingStorePersistDebounce      = time.Second
	thinkingStoreTouchPersistInterval = 5 * time.Minute
)

func NewThinkingStore(cfg ThinkingStoreConfig) *ThinkingStore {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 5000
	}
	if cfg.EntryTTLMinutes == 0 {
		cfg.EntryTTLMinutes = 24 * 60
	}
	s := &ThinkingStore{
		entries:      make(map[string]*ThinkingEntry),
		config:       cfg,
		stopCleanup:  make(chan struct{}),
		stopPersist:  make(chan struct{}),
		persistDirty: make(chan struct{}, 1),
		persistDone:  make(chan struct{}),
	}
	s.loadPersisted()
	go s.cleanupLoop()
	go s.persistLoop()
	return s
}

func (s *ThinkingStore) Put(prefixHash, visibleHash string, content []map[string]interface{}) {
	if visibleHash == "" || len(content) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeThinkingStoreKey(prefixHash, visibleHash)
	if _, exists := s.entries[key]; !exists && len(s.entries) >= s.config.MaxEntries {
		s.evictOldest()
	}

	s.entries[key] = &ThinkingEntry{
		PrefixHash:  prefixHash,
		VisibleHash: visibleHash,
		Content:     cloneThinkingContent(content),
		CachedAt:    time.Now(),
	}
	s.entries[key].lastPersistedAt = s.entries[key].CachedAt
	s.markDirty()
}

func (s *ThinkingStore) Get(prefixHash, visibleHash string) ([]map[string]interface{}, string) {
	if visibleHash == "" {
		return nil, ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute

	if entry, ok := s.entries[makeThinkingStoreKey(prefixHash, visibleHash)]; ok {
		if now.Sub(entry.CachedAt) <= ttl {
			entry.CachedAt = now
			if now.Sub(entry.lastPersistedAt) >= thinkingStoreTouchPersistInterval {
				entry.lastPersistedAt = now
				s.markDirty()
			}
			return cloneThinkingContent(entry.Content), "exact"
		}
		delete(s.entries, makeThinkingStoreKey(prefixHash, visibleHash))
		s.markDirty()
	}

	var fallback *ThinkingEntry
	for key, entry := range s.entries {
		if now.Sub(entry.CachedAt) > ttl {
			delete(s.entries, key)
			continue
		}
		if entry.VisibleHash != visibleHash {
			continue
		}
		if fallback != nil {
			return nil, ""
		}
		fallback = entry
	}

	if fallback != nil {
		fallback.CachedAt = now
		if now.Sub(fallback.lastPersistedAt) >= thinkingStoreTouchPersistInterval {
			fallback.lastPersistedAt = now
			s.markDirty()
		}
		return cloneThinkingContent(fallback.Content), "visible_hash"
	}

	return nil, ""
}

func (s *ThinkingStore) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, v := range s.entries {
		if oldestKey == "" || v.CachedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.CachedAt
		}
	}

	if oldestKey != "" {
		delete(s.entries, oldestKey)
	}
}

func (s *ThinkingStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpired()
		case <-s.stopCleanup:
			return
		}
	}
}

func (s *ThinkingStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute
	now := time.Now()
	changed := false

	for k, v := range s.entries {
		if now.Sub(v.CachedAt) > ttl {
			delete(s.entries, k)
			changed = true
		}
	}
	if changed {
		s.markDirty()
	}
}

func (s *ThinkingStore) Close() {
	s.closeOnce.Do(func() {
		close(s.stopCleanup)
		close(s.stopPersist)
		<-s.persistDone
	})
}

func (s *ThinkingStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func makeThinkingStoreKey(prefixHash, visibleHash string) string {
	return prefixHash + "\x00" + visibleHash
}

func cloneThinkingContent(content []map[string]interface{}) []map[string]interface{} {
	if len(content) == 0 {
		return nil
	}

	data, err := json.Marshal(content)
	if err != nil {
		return nil
	}

	var cloned []map[string]interface{}
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func (s *ThinkingStore) markDirty() {
	if s.config.PersistPath == "" {
		return
	}
	select {
	case s.persistDirty <- struct{}{}:
	default:
	}
}

func (s *ThinkingStore) persistLoop() {
	defer close(s.persistDone)
	if s.config.PersistPath == "" {
		<-s.stopPersist
		return
	}

	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(thinkingStorePersistDebounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(thinkingStorePersistDebounce)
		timerC = timer.C
	}

	for {
		select {
		case <-s.persistDirty:
			resetTimer()
		case <-timerC:
			if err := s.persistToDisk(); err != nil {
				log.Printf("thinking_store: failed to persist snapshot: %v", err)
			}
			stopTimer()
		case <-s.stopPersist:
			stopTimer()
			if err := s.persistToDisk(); err != nil {
				log.Printf("thinking_store: failed to persist snapshot during shutdown: %v", err)
			}
			return
		}
	}
}

func (s *ThinkingStore) persistToDisk() error {
	if s.config.PersistPath == "" {
		return nil
	}

	payload := persistedThinkingStore{
		Version: thinkingStorePersistVersion,
		Entries: s.snapshotEntries(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.config.PersistPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmpPath := s.config.PersistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.config.PersistPath); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.entries {
		entry.lastPersistedAt = entry.CachedAt
	}
	return nil
}

func (s *ThinkingStore) snapshotEntries() []*ThinkingEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute
	entries := make([]*ThinkingEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if now.Sub(entry.CachedAt) > ttl {
			continue
		}
		entries = append(entries, &ThinkingEntry{
			PrefixHash:  entry.PrefixHash,
			VisibleHash: entry.VisibleHash,
			Content:     cloneThinkingContent(entry.Content),
			CachedAt:    entry.CachedAt,
		})
	}
	return entries
}

func (s *ThinkingStore) loadPersisted() {
	if s.config.PersistPath == "" {
		return
	}

	data, err := os.ReadFile(s.config.PersistPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("thinking_store: failed to read persisted snapshot: %v", err)
		}
		return
	}

	var payload persistedThinkingStore
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Printf("thinking_store: failed to parse persisted snapshot: %v", err)
		return
	}

	now := time.Now()
	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute
	for _, entry := range payload.Entries {
		if entry == nil || entry.VisibleHash == "" || len(entry.Content) == 0 {
			continue
		}
		if now.Sub(entry.CachedAt) > ttl {
			continue
		}
		s.entries[makeThinkingStoreKey(entry.PrefixHash, entry.VisibleHash)] = &ThinkingEntry{
			PrefixHash:  entry.PrefixHash,
			VisibleHash: entry.VisibleHash,
			Content:     cloneThinkingContent(entry.Content),
			CachedAt:    entry.CachedAt,
		}
		s.entries[makeThinkingStoreKey(entry.PrefixHash, entry.VisibleHash)].lastPersistedAt = entry.CachedAt
	}
}
