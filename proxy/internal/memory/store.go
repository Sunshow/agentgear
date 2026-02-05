package memory

import (
	"sync"
	"time"
)

type ConnectionInfo struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Sequence  int       `json:"sequence"`
	Tags      []string  `json:"tags"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    string    `json:"status"` // pending, completed, error
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	DurationMs int64    `json:"duration_ms"`

	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	RequestBody    []byte            `json:"request_body,omitempty"`
	RequestTools   []ToolInfo        `json:"request_tools,omitempty"`

	ResponseStatus int               `json:"response_status,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	ResponseBody   []byte            `json:"response_body,omitempty"`
	ResponseTools  []ToolCallInfo    `json:"response_tools,omitempty"`

	TransformedRequest  bool     `json:"transformed_request"`
	TransformedResponse bool     `json:"transformed_response"`
	AppliedTransformers []string `json:"applied_transformers,omitempty"`

	ParsedData *ParsedData `json:"parsed_data,omitempty"`
}

type ParsedData struct {
	Protocol  string               `json:"protocol,omitempty"`
	Anthropic *AnthropicParsedData `json:"anthropic,omitempty"`
}

type AnthropicParsedData struct {
	Model           string            `json:"model,omitempty"`
	MaxTokens       int               `json:"max_tokens,omitempty"`
	SystemPrompts   []SystemPrompt    `json:"system_prompts,omitempty"`
	SystemReminders []SystemReminder  `json:"system_reminders,omitempty"`
	Tools           []ToolDefinition  `json:"tools,omitempty"`
}

type SystemPrompt struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	CacheControl string `json:"cache_control,omitempty"`
}

type SystemReminder struct {
	RawText    string            `json:"raw_text"`
	ParsedInfo map[string]string `json:"parsed_info,omitempty"`
}

type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ToolCallInfo struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type StoreConfig struct {
	MaxConnections    int `mapstructure:"max_connections"`
	MaxRequestBodyKB  int `mapstructure:"max_request_body_kb"`
	MaxResponseBodyKB int `mapstructure:"max_response_body_kb"`
	RetentionMinutes  int `mapstructure:"retention_minutes"`
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxConnections:    1000,
		MaxRequestBodyKB:  512,
		MaxResponseBodyKB: 512,
		RetentionMinutes:  60,
	}
}

type ConnectionStore struct {
	connections *RingBuffer[*ConnectionInfo]
	index       map[string]*ConnectionInfo // id -> connection
	config      StoreConfig
	mu          sync.RWMutex
	stopCleanup chan struct{}
}

func NewConnectionStore(cfg StoreConfig) *ConnectionStore {
	store := &ConnectionStore{
		connections: NewRingBuffer[*ConnectionInfo](cfg.MaxConnections),
		index:       make(map[string]*ConnectionInfo),
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}

	go store.cleanupLoop()

	return store
}

func (s *ConnectionStore) Add(conn *ConnectionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.MaxRequestBodyKB > 0 && len(conn.RequestBody) > s.config.MaxRequestBodyKB*1024 {
		conn.RequestBody = conn.RequestBody[:s.config.MaxRequestBodyKB*1024]
	}
	if s.config.MaxResponseBodyKB > 0 && len(conn.ResponseBody) > s.config.MaxResponseBodyKB*1024 {
		conn.ResponseBody = conn.ResponseBody[:s.config.MaxResponseBodyKB*1024]
	}

	evicted, hasEvicted := s.connections.Push(conn)
	if hasEvicted && evicted != nil {
		delete(s.index, evicted.ID)
	}

	s.index[conn.ID] = conn
}

func (s *ConnectionStore) Update(id string, updater func(*ConnectionInfo)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.index[id]
	if !exists {
		return false
	}

	updater(conn)

	if s.config.MaxResponseBodyKB > 0 && len(conn.ResponseBody) > s.config.MaxResponseBodyKB*1024 {
		conn.ResponseBody = conn.ResponseBody[:s.config.MaxResponseBodyKB*1024]
	}

	return true
}

func (s *ConnectionStore) Get(id string) *ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index[id]
}

func (s *ConnectionStore) GetRecent(limit int) []*ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connections.GetRecent(limit)
}

func (s *ConnectionStore) GetAll() []*ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connections.GetAll()
}

func (s *ConnectionStore) Filter(filter func(*ConnectionInfo) bool) []*ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.connections.GetAll()
	result := make([]*ConnectionInfo, 0)
	for _, conn := range all {
		if filter(conn) {
			result = append(result, conn)
		}
	}
	return result
}

func (s *ConnectionStore) FilterByTags(tags []string, limit int) []*ConnectionInfo {
	if len(tags) == 0 {
		return s.GetRecent(limit)
	}

	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}

	result := s.Filter(func(conn *ConnectionInfo) bool {
		for _, t := range conn.Tags {
			if tagSet[t] {
				return true
			}
		}
		return false
	})

	if limit > 0 && len(result) > limit {
		return result[len(result)-limit:]
	}
	return result
}

func (s *ConnectionStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connections.Clear()
	s.index = make(map[string]*ConnectionInfo)
}

func (s *ConnectionStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connections.Size()
}

func (s *ConnectionStore) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tagCounts := make(map[string]int)
	statusCounts := make(map[string]int)

	all := s.connections.GetAll()
	for _, conn := range all {
		statusCounts[conn.Status]++
		for _, tag := range conn.Tags {
			tagCounts[tag]++
		}
	}

	return map[string]interface{}{
		"total":         s.connections.Size(),
		"capacity":      s.connections.Capacity(),
		"by_status":     statusCounts,
		"by_tag":        tagCounts,
	}
}

func (s *ConnectionStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupOld()
		case <-s.stopCleanup:
			return
		}
	}
}

func (s *ConnectionStore) cleanupOld() {
	if s.config.RetentionMinutes <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(s.config.RetentionMinutes) * time.Minute)

	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.connections.GetAll()
	for _, conn := range all {
		if conn.StartTime.Before(cutoff) {
			delete(s.index, conn.ID)
		}
	}
}

func (s *ConnectionStore) Close() {
	close(s.stopCleanup)
}
