package memory

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type ThinkingEntry struct {
	PrefixHash      string                   `json:"prefix_hash"`
	VisibleHash     string                   `json:"visible_hash"`
	Content         []map[string]interface{} `json:"content"`
	CachedAt        time.Time                `json:"cached_at"`
	lastPersistedAt time.Time
}

type ThinkingStoreConfig struct {
	MaxEntries      int    `mapstructure:"max_entries"`
	EntryTTLMinutes int    `mapstructure:"entry_ttl_minutes"`
	PersistPath     string `mapstructure:"persist_path"`
}

func DefaultThinkingStoreConfig() ThinkingStoreConfig {
	return ThinkingStoreConfig{
		MaxEntries:      50000,
		EntryTTLMinutes: 24 * 60,
		PersistPath:     "./data/thinking_store.db",
	}
}

type ThinkingStore struct {
	mu             sync.RWMutex
	entries        map[string]*ThinkingEntry
	pendingUpserts map[string]*ThinkingEntry
	pendingDeletes map[string]struct{}
	config         ThinkingStoreConfig
	db             *bolt.DB
	stopCleanup    chan struct{}
	stopPersist    chan struct{}
	persistDirty   chan struct{}
	persistDone    chan struct{}
	closeOnce      sync.Once
}

type persistedThinkingEntry struct {
	Key   string
	Entry *ThinkingEntry
}

const (
	thinkingStoreBucketName           = "thinking_entries"
	thinkingStorePersistDebounce      = time.Second
	thinkingStoreTouchPersistInterval = 5 * time.Minute
	thinkingStoreDBTimeout            = time.Second
)

func NewThinkingStore(cfg ThinkingStoreConfig) *ThinkingStore {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 50000
	}
	if cfg.EntryTTLMinutes == 0 {
		cfg.EntryTTLMinutes = 24 * 60
	}

	s := &ThinkingStore{
		entries:        make(map[string]*ThinkingEntry),
		pendingUpserts: make(map[string]*ThinkingEntry),
		pendingDeletes: make(map[string]struct{}),
		config:         cfg,
		stopCleanup:    make(chan struct{}),
		stopPersist:    make(chan struct{}),
		persistDirty:   make(chan struct{}, 1),
		persistDone:    make(chan struct{}),
	}

	if err := s.openDB(); err != nil {
		log.Printf("thinking_store: failed to open bbolt store, falling back to memory-only mode: %v", err)
	}
	s.loadPersisted()
	go s.cleanupLoop()
	if s.db != nil {
		go s.persistLoop()
	} else {
		close(s.persistDone)
	}
	return s
}

func (s *ThinkingStore) Put(prefixHash, visibleHash string, content []map[string]interface{}) {
	if visibleHash == "" || len(content) == 0 {
		return
	}

	now := time.Now()
	key := makeThinkingStoreKey(prefixHash, visibleHash)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[key]; !exists && len(s.entries) >= s.config.MaxEntries {
		if evictedKey := s.evictOldestLocked(); evictedKey != "" {
			s.queueDeleteLocked(evictedKey)
		}
	}

	entry := &ThinkingEntry{
		PrefixHash:  prefixHash,
		VisibleHash: visibleHash,
		Content:     cloneThinkingContent(content),
		CachedAt:    now,
	}
	s.entries[key] = entry
	s.queueUpsertLocked(key, entry)
}

func (s *ThinkingStore) Get(prefixHash, visibleHash string) ([]map[string]interface{}, string) {
	if visibleHash == "" {
		return nil, ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ttl := s.entryTTL()
	key := makeThinkingStoreKey(prefixHash, visibleHash)

	if entry, ok := s.entries[key]; ok {
		if now.Sub(entry.CachedAt) <= ttl {
			entry.CachedAt = now
			if s.shouldPersistTouch(entry, now) {
				s.queueUpsertLocked(key, entry)
			}
			return cloneThinkingContent(entry.Content), "exact"
		}
		delete(s.entries, key)
		s.queueDeleteLocked(key)
	}

	var fallbackKey string
	var fallback *ThinkingEntry
	for candidateKey, entry := range s.entries {
		if now.Sub(entry.CachedAt) > ttl {
			delete(s.entries, candidateKey)
			s.queueDeleteLocked(candidateKey)
			continue
		}
		if entry.VisibleHash != visibleHash {
			continue
		}
		if fallback != nil {
			return nil, ""
		}
		fallbackKey = candidateKey
		fallback = entry
	}

	if fallback != nil {
		fallback.CachedAt = now
		if s.shouldPersistTouch(fallback, now) {
			s.queueUpsertLocked(fallbackKey, fallback)
		}
		return cloneThinkingContent(fallback.Content), "visible_hash"
	}

	return nil, ""
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

	now := time.Now()
	ttl := s.entryTTL()
	for key, entry := range s.entries {
		if now.Sub(entry.CachedAt) > ttl {
			delete(s.entries, key)
			s.queueDeleteLocked(key)
		}
	}
}

func (s *ThinkingStore) Close() {
	s.closeOnce.Do(func() {
		close(s.stopCleanup)
		if s.db != nil {
			close(s.stopPersist)
			<-s.persistDone
			if err := s.db.Close(); err != nil {
				log.Printf("thinking_store: failed to close bbolt store: %v", err)
			}
		}
	})
}

func (s *ThinkingStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *ThinkingStore) persistLoop() {
	defer close(s.persistDone)

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
			if err := s.flushPending(); err != nil {
				log.Printf("thinking_store: failed to flush bbolt updates: %v", err)
			}
			stopTimer()
		case <-s.stopPersist:
			stopTimer()
			if err := s.flushPending(); err != nil {
				log.Printf("thinking_store: failed to flush bbolt updates during shutdown: %v", err)
			}
			return
		}
	}
}

func (s *ThinkingStore) flushPending() error {
	upserts, deletes := s.drainPending()
	if len(upserts) == 0 && len(deletes) == 0 {
		return nil
	}

	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(thinkingStoreBucketName))
		for key := range deletes {
			if err := bucket.Delete([]byte(key)); err != nil {
				return err
			}
		}
		for key, entry := range upserts {
			payload, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(key), payload); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.requeuePending(upserts, deletes)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, persisted := range upserts {
		current := s.entries[key]
		if current == nil {
			continue
		}
		if current.CachedAt.Equal(persisted.CachedAt) {
			current.lastPersistedAt = persisted.CachedAt
		}
	}
	return nil
}

func (s *ThinkingStore) drainPending() (map[string]*ThinkingEntry, map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	upserts := s.pendingUpserts
	deletes := s.pendingDeletes
	s.pendingUpserts = make(map[string]*ThinkingEntry)
	s.pendingDeletes = make(map[string]struct{})
	return upserts, deletes
}

func (s *ThinkingStore) requeuePending(upserts map[string]*ThinkingEntry, deletes map[string]struct{}) {
	s.mu.Lock()
	for key, entry := range upserts {
		delete(s.pendingDeletes, key)
		s.pendingUpserts[key] = cloneEntry(entry)
	}
	for key := range deletes {
		delete(s.pendingUpserts, key)
		s.pendingDeletes[key] = struct{}{}
	}
	s.mu.Unlock()
	s.markDirty()
}

func (s *ThinkingStore) openDB() error {
	if s.config.PersistPath == "" {
		return nil
	}
	dir := filepath.Dir(s.config.PersistPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	db, err := bolt.Open(s.config.PersistPath, 0o600, &bolt.Options{Timeout: thinkingStoreDBTimeout})
	if err != nil {
		return err
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(thinkingStoreBucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return err
	}

	s.db = db
	return nil
}

func (s *ThinkingStore) loadPersisted() {
	if s.db == nil {
		return
	}

	now := time.Now()
	ttl := s.entryTTL()
	loaded := make([]persistedThinkingEntry, 0)
	staleKeys := make([]string, 0)

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(thinkingStoreBucketName))
		return bucket.ForEach(func(key, value []byte) error {
			var entry ThinkingEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				staleKeys = append(staleKeys, string(append([]byte(nil), key...)))
				return nil
			}
			if entry.VisibleHash == "" || len(entry.Content) == 0 || now.Sub(entry.CachedAt) > ttl {
				staleKeys = append(staleKeys, string(append([]byte(nil), key...)))
				return nil
			}
			entry.lastPersistedAt = entry.CachedAt
			loaded = append(loaded, persistedThinkingEntry{
				Key:   string(append([]byte(nil), key...)),
				Entry: cloneEntry(&entry),
			})
			return nil
		})
	})
	if err != nil {
		log.Printf("thinking_store: failed to load persisted entries: %v", err)
		return
	}

	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].Entry.CachedAt.After(loaded[j].Entry.CachedAt)
	})

	if len(loaded) > s.config.MaxEntries {
		for _, item := range loaded[s.config.MaxEntries:] {
			staleKeys = append(staleKeys, item.Key)
		}
		loaded = loaded[:s.config.MaxEntries]
	}

	for _, item := range loaded {
		s.entries[item.Key] = item.Entry
	}

	if len(staleKeys) > 0 {
		if err := s.db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte(thinkingStoreBucketName))
			for _, key := range staleKeys {
				if err := bucket.Delete([]byte(key)); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			log.Printf("thinking_store: failed to purge stale persisted entries: %v", err)
		}
	}
}

func (s *ThinkingStore) shouldPersistTouch(entry *ThinkingEntry, now time.Time) bool {
	if s.db == nil {
		return false
	}
	return entry.lastPersistedAt.IsZero() || now.Sub(entry.lastPersistedAt) >= thinkingStoreTouchPersistInterval
}

func (s *ThinkingStore) queueUpsertLocked(key string, entry *ThinkingEntry) {
	if s.db == nil {
		return
	}
	delete(s.pendingDeletes, key)
	s.pendingUpserts[key] = cloneEntry(entry)
	s.markDirty()
}

func (s *ThinkingStore) queueDeleteLocked(key string) {
	if s.db == nil {
		return
	}
	delete(s.pendingUpserts, key)
	s.pendingDeletes[key] = struct{}{}
	s.markDirty()
}

func (s *ThinkingStore) evictOldestLocked() string {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range s.entries {
		if oldestKey == "" || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
		}
	}
	if oldestKey != "" {
		delete(s.entries, oldestKey)
	}
	return oldestKey
}

func (s *ThinkingStore) entryTTL() time.Duration {
	return time.Duration(s.config.EntryTTLMinutes) * time.Minute
}

func (s *ThinkingStore) markDirty() {
	if s.db == nil {
		return
	}
	select {
	case s.persistDirty <- struct{}{}:
	default:
	}
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

func cloneEntry(entry *ThinkingEntry) *ThinkingEntry {
	if entry == nil {
		return nil
	}
	return &ThinkingEntry{
		PrefixHash:      entry.PrefixHash,
		VisibleHash:     entry.VisibleHash,
		Content:         cloneThinkingContent(entry.Content),
		CachedAt:        entry.CachedAt,
		lastPersistedAt: entry.lastPersistedAt,
	}
}
