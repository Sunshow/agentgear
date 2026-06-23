package transformer

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"go.uber.org/zap"
)

type sessionMatchResult struct {
	sessionID  string
	source     string // "new" or "matched"
	matchDepth int    // number of messages matched (0 for new sessions)
}

// SessionInjector handles session identification via content hashing
// and injects a consistent session ID into upstream requests.
type SessionInjector struct {
	store  *memory.ThinkingStore
	logger *zap.Logger
}

// NewSessionInjector creates a new SessionInjector.
func NewSessionInjector(store *memory.ThinkingStore, logger *zap.Logger) *SessionInjector {
	return &SessionInjector{store: store, logger: logger}
}

// Inject computes a content-based session ID from the request body and
// injects the X-Claude-Code-Session-Id header into the upstream request.
// Returns true if a header was injected.
func (si *SessionInjector) Inject(reqBody []byte, proxyReq *http.Request) bool {
	if si.store == nil {
		return false
	}

	result := si.resolveSessionID(reqBody)
	if result.sessionID == "" {
		return false
	}

	proxyReq.Header.Set("X-Claude-Code-Session-Id", result.sessionID)

	if result.source == "matched" {
		si.logger.Info("session_inject: matched existing session",
			zap.String("session_id", result.sessionID),
			zap.Int("match_depth", result.matchDepth))
	} else {
		si.logger.Info("session_inject: created new session",
			zap.String("session_id", result.sessionID),
			zap.Int("prefix_depth", result.matchDepth))
	}
	return true
}

func (si *SessionInjector) resolveSessionID(reqBody []byte) sessionMatchResult {
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return sessionMatchResult{}
	}

	// Build content array: system text + all messages
	var prefix []interface{}

	if sys := extractSystemText(req); sys != "" {
		prefix = append(prefix, sys)
	}

	messages, _ := req["messages"].([]interface{})
	if messages == nil {
		return sessionMatchResult{}
	}

	fullPrefix := make([]interface{}, len(prefix)+len(messages))
	copy(fullPrefix, prefix)
	for i, msg := range messages {
		fullPrefix[len(prefix)+i] = msg
	}

	// Try progressively shorter prefixes (longest first)
	for n := len(fullPrefix); n > 0; n-- {
		hash := HashMessagesPrefix(fullPrefix[:n])
		if sid := si.store.GetSession(hash); sid != "" {
			// Store the current (longer) prefix for future lookups
			currentHash := HashMessagesPrefix(fullPrefix)
			si.store.PutSession(currentHash, sid)
			return sessionMatchResult{
				sessionID:  sid,
				source:     "matched",
				matchDepth: n,
			}
		}
	}

	// No match found, create new session
	newSID := uuid.New().String()

	// Store all prefix lengths for future lookups
	for n := 1; n <= len(fullPrefix); n++ {
		hash := HashMessagesPrefix(fullPrefix[:n])
		si.store.PutSession(hash, newSID)
	}

	return sessionMatchResult{
		sessionID:  newSID,
		source:     "new",
		matchDepth: len(fullPrefix),
	}
}

// extractSystemText extracts the system prompt as a plain string from the request body.
func extractSystemText(req map[string]interface{}) string {
	sys, ok := req["system"]
	if !ok {
		return ""
	}
	switch v := sys.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, block := range v {
			if bm, ok := block.(map[string]interface{}); ok {
				if btype, _ := bm["type"].(string); btype == "text" {
					if text, _ := bm["text"].(string); text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
