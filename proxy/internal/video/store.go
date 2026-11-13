package video

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var taskBucket = []byte("video_tasks")

// Task is the adapter's durable view of one public video task. The public id
// is what the outer new-api stores and sends back; UpstreamID addresses the
// orlucky task.
type Task struct {
	PublicID     string          `json:"public_id"`
	UpstreamID   string          `json:"upstream_id"`
	ClientModel  string          `json:"client_model"`
	TargetModel  string          `json:"target_model"`
	Prompt       string          `json:"prompt"`
	Duration     int             `json:"duration"`
	Resolution   string          `json:"resolution"`
	Aspect       string          `json:"aspect"`
	Status       string          `json:"status"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
	CompletedAt  int64           `json:"completed_at"`
	AssetURL     string          `json:"asset_url"`
	Usage        json.RawMessage `json:"usage,omitempty"`
	LastSyncedAt int64           `json:"last_synced_at"`
}

func (t *Task) terminal() bool {
	switch t.Status {
	case "succeeded", "failed", "expired", "cancelled":
		return true
	}
	return false
}

type Store struct {
	db        *bolt.DB
	ttl       time.Duration
	mu        sync.Mutex
	lastSweep time.Time
}

func OpenStore(path string, ttl time.Duration) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("video store path is empty")
	}
	if directory := filepath.Dir(path); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create video store directory: %w", err)
		}
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open video store: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(taskBucket)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init video store: %w", err)
	}
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}
	return &Store{db: db, ttl: ttl}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Put(task *Task) error {
	if task == nil || task.PublicID == "" {
		return fmt.Errorf("task public id is required")
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(taskBucket).Put([]byte(task.PublicID), payload)
	}); err != nil {
		return err
	}
	s.sweep()
	return nil
}

func (s *Store) Get(publicID string) (*Task, bool, error) {
	if publicID == "" {
		return nil, false, nil
	}
	var payload []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(taskBucket).Get([]byte(publicID))
		if value == nil {
			return nil
		}
		payload = append(payload, value...)
		return nil
	})
	if err != nil || payload == nil {
		return nil, false, err
	}
	var task Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func (s *Store) Update(publicID string, mutate func(*Task)) (*Task, error) {
	var updated Task
	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(taskBucket)
		value := bucket.Get([]byte(publicID))
		if value == nil {
			return nil
		}
		if err := json.Unmarshal(value, &updated); err != nil {
			return err
		}
		mutate(&updated)
		updated.UpdatedAt = time.Now().Unix()
		payload, err := json.Marshal(&updated)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(publicID), payload)
	})
	if err != nil {
		return nil, err
	}
	if updated.PublicID == "" {
		return nil, fmt.Errorf("video task %s not found", publicID)
	}
	return &updated, nil
}

// sweep removes terminal tasks older than the configured TTL. It runs at most
// once an hour so writes stay cheap.
func (s *Store) sweep() {
	s.mu.Lock()
	if time.Since(s.lastSweep) < time.Hour {
		s.mu.Unlock()
		return
	}
	s.lastSweep = time.Now()
	s.mu.Unlock()

	cutoff := time.Now().Add(-s.ttl).Unix()
	_ = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(taskBucket)
		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			var task Task
			if json.Unmarshal(value, &task) != nil {
				continue
			}
			if task.terminal() && task.UpdatedAt > 0 && task.UpdatedAt < cutoff {
				if err := cursor.Delete(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
