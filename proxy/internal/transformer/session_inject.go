package transformer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"go.uber.org/zap"
)

// ccUserID mirrors the Claude Code metadata.user_id JSON string shape.
// Field order is significant: device_id, account_uuid, session_id.
type ccUserID struct {
	DeviceID    string `json:"device_id"`
	AccountUUID string `json:"account_uuid"`
	SessionID   string `json:"session_id"`
}

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

// Inject computes a content-based session ID from the request body, injects
// the X-Claude-Code-Session-Id header into the upstream request, and simulates
// Claude Code's metadata.user_id in the request body using the same session ID.
// Returns the (possibly modified) body and true if a session was resolved.
// When metadata is left unchanged, the returned body is byte-identical to reqBody.
func (si *SessionInjector) Inject(reqBody []byte, proxyReq *http.Request) ([]byte, bool) {
	if si.store == nil {
		return reqBody, false
	}

	result := si.resolveSessionID(reqBody)
	if result.sessionID == "" {
		return reqBody, false
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

	newBody, changed := injectMetadata(reqBody, result.sessionID)
	if changed {
		si.applyBody(proxyReq, newBody)
		si.logger.Info("session_inject: metadata.user_id injected",
			zap.String("session_id", result.sessionID))
	}
	return newBody, true
}

// applyBody replaces the proxy request body so upstream framing matches newBody.
func (si *SessionInjector) applyBody(proxyReq *http.Request, newBody []byte) {
	proxyReq.Body = io.NopCloser(bytes.NewReader(newBody))
	proxyReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}
	proxyReq.ContentLength = int64(len(newBody))
	proxyReq.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
}

// deriveDeviceID derives a stable 64-hex device id from the session ID.
func deriveDeviceID(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

// buildUserID builds the Claude Code metadata.user_id JSON string.
func buildUserID(sessionID string) string {
	b, _ := json.Marshal(ccUserID{
		DeviceID:    deriveDeviceID(sessionID),
		AccountUUID: "",
		SessionID:   sessionID,
	})
	return string(b)
}

// injectMetadata simulates Claude Code's metadata.user_id in the request body.
//   - no metadata key: add {"user_id": <built>}
//   - metadata present without user_id: inject user_id, keep other fields
//   - metadata already has user_id: leave unchanged
//   - metadata present but not a JSON object: leave unchanged
//
// Returns the re-marshaled body when changed, otherwise the original bytes.
func injectMetadata(reqBody []byte, sessionID string) ([]byte, bool) {
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return reqBody, false
	}

	rawMeta, hasMeta := req["metadata"]
	if !hasMeta {
		req["metadata"] = map[string]interface{}{"user_id": buildUserID(sessionID)}
		return remarshal(reqBody, req)
	}

	meta, ok := rawMeta.(map[string]interface{})
	if !ok {
		return reqBody, false
	}
	if _, hasUserID := meta["user_id"]; hasUserID {
		return reqBody, false
	}
	meta["user_id"] = buildUserID(sessionID)
	return remarshal(reqBody, req)
}

func remarshal(orig []byte, req map[string]interface{}) ([]byte, bool) {
	b, err := json.Marshal(req)
	if err != nil {
		return orig, false
	}
	return b, true
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
