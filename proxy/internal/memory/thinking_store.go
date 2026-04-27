package memory

import (
	"encoding/json"
	"sync"
	"time"
)

type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type ThinkingEntry struct {
	PrefixHash  string
	VisibleHash string
	Content     []map[string]interface{}
	CachedAt    time.Time
}

type ThinkingStoreConfig struct {
	MaxEntries      int `mapstructure:"max_entries"`
	EntryTTLMinutes int `mapstructure:"entry_ttl_minutes"`
}

func DefaultThinkingStoreConfig() ThinkingStoreConfig {
	return ThinkingStoreConfig{
		MaxEntries:      200,
		EntryTTLMinutes: 30,
	}
}

type ThinkingStore struct {
	mu          sync.RWMutex
	entries     map[string]*ThinkingEntry
	config      ThinkingStoreConfig
	stopCleanup chan struct{}
}

func NewThinkingStore(cfg ThinkingStoreConfig) *ThinkingStore {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 200
	}
	if cfg.EntryTTLMinutes == 0 {
		cfg.EntryTTLMinutes = 30
	}
	s := &ThinkingStore{
		entries:     make(map[string]*ThinkingEntry),
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}
	go s.cleanupLoop()
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
}

func (s *ThinkingStore) Get(prefixHash, visibleHash string) ([]map[string]interface{}, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if visibleHash == "" {
		return nil, ""
	}

	now := time.Now()
	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute

	if entry, ok := s.entries[makeThinkingStoreKey(prefixHash, visibleHash)]; ok && now.Sub(entry.CachedAt) <= ttl {
		return cloneThinkingContent(entry.Content), "exact"
	}

	var fallback *ThinkingEntry
	for _, entry := range s.entries {
		if entry.VisibleHash != visibleHash || now.Sub(entry.CachedAt) > ttl {
			continue
		}
		if fallback != nil {
			return nil, ""
		}
		fallback = entry
	}

	if fallback != nil {
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

	for k, v := range s.entries {
		if now.Sub(v.CachedAt) > ttl {
			delete(s.entries, k)
		}
	}
}

func (s *ThinkingStore) Close() {
	close(s.stopCleanup)
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
