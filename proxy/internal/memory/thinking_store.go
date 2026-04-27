package memory

import (
	"sync"
	"time"
)

type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type ThinkingEntry struct {
	ThinkingBlocks []ThinkingBlock
	CachedAt       time.Time
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

func (s *ThinkingStore) Put(contentHash string, blocks []ThinkingBlock) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest if at capacity
	if len(s.entries) >= s.config.MaxEntries {
		s.evictOldest()
	}

	s.entries[contentHash] = &ThinkingEntry{
		ThinkingBlocks: blocks,
		CachedAt:       time.Now(),
	}
}

func (s *ThinkingStore) Get(contentHash string) []ThinkingBlock {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[contentHash]
	if !ok {
		return nil
	}

	ttl := time.Duration(s.config.EntryTTLMinutes) * time.Minute
	if time.Since(entry.CachedAt) > ttl {
		return nil
	}

	return entry.ThinkingBlocks
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
