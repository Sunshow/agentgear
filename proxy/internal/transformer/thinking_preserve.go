package transformer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sunshow/agentgear/proxy/internal/memory"
	"go.uber.org/zap"
)

// ThinkingPreserver handles caching thinking blocks from responses and repairing
// assistant history messages when downstream agents drop or partially preserve
// thinking content.
type ThinkingPreserver struct {
	store  *memory.ThinkingStore
	logger *zap.Logger
}

func NewThinkingPreserver(store *memory.ThinkingStore, logger *zap.Logger) *ThinkingPreserver {
	return &ThinkingPreserver{store: store, logger: logger}
}

// CacheFromResponse stores the full assistant content keyed by the request prefix
// plus the hash of visible (non-thinking) content blocks.
func (tp *ThinkingPreserver) CacheFromResponse(requestBody []byte, contentBlocks []map[string]interface{}) {
	thinkingCount, visibleHash, prefixHash, shouldCache := tp.describeCacheCandidate(requestBody, contentBlocks)
	if !shouldCache {
		return
	}

	for i, block := range contentBlocks {
		blockType, _ := block["type"].(string)
		if blockType != "thinking" {
			continue
		}
		thinking, _ := block["thinking"].(string)
		signature, _ := block["signature"].(string)
		if signature == "" {
			continue
		}
		if thinking == "" {
			tp.logger.Info("thinking_preserve: caching empty thinking block (agent may drop this)",
				zap.Int("index", i),
				zap.Int("signature_len", len(signature)))
		} else {
			tp.logger.Debug("thinking_preserve: caching thinking block",
				zap.Int("index", i),
				zap.Int("thinking_len", len(thinking)),
				zap.Int("signature_len", len(signature)))
		}
	}

	tp.store.Put(prefixHash, visibleHash, contentBlocks)
	tp.logger.Info("thinking_preserve: cached assistant snapshot from response",
		zap.String("prefix_hash", shortHash(prefixHash)),
		zap.String("visible_hash", shortHash(visibleHash)),
		zap.Int("thinking_blocks", thinkingCount),
		zap.Int("store_size", tp.store.Size()))
}

// InjectIntoRequest repairs assistant messages using cached full assistant content.
// This covers both totally missing thinking blocks and partially preserved content.
func (tp *ThinkingPreserver) InjectIntoRequest(body []byte) ([]byte, bool) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false
	}

	messages, ok := req["messages"].([]interface{})
	if !ok {
		return body, false
	}

	injected := false
	repairedPrefix := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			repairedPrefix = append(repairedPrefix, msg)
			continue
		}

		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]interface{})
		if role != "assistant" || !ok || len(content) == 0 {
			repairedPrefix = append(repairedPrefix, msgMap)
			continue
		}

		visibleHash, hasVisibleContent := hashVisibleContent(content)
		if !hasVisibleContent {
			repairedPrefix = append(repairedPrefix, msgMap)
			continue
		}

		prefixHash := hashNormalized(repairedPrefix)
		cachedContent, source := tp.store.Get(prefixHash, visibleHash)
		if cachedContent == nil || !containsThinkingBlock(cachedContent) {
			tp.logger.Debug("thinking_preserve: cache miss for assistant message repair",
				zap.String("prefix_hash", shortHash(prefixHash)),
				zap.String("visible_hash", shortHash(visibleHash)))
			repairedPrefix = append(repairedPrefix, msgMap)
			continue
		}

		if normalizedJSON(content) == normalizedJSON(cachedContent) {
			repairedPrefix = append(repairedPrefix, msgMap)
			continue
		}

		msgMap["content"] = mapsToInterfaces(cachedContent)
		injected = true
		repairedPrefix = append(repairedPrefix, msgMap)

		tp.logger.Info("thinking_preserve: repaired assistant message in request",
			zap.String("source", source),
			zap.String("prefix_hash", shortHash(prefixHash)),
			zap.String("visible_hash", shortHash(visibleHash)),
			zap.Int("content_blocks", len(cachedContent)),
			zap.Int("store_size", tp.store.Size()))
	}

	if !injected {
		return body, false
	}

	result, err := json.Marshal(req)
	if err != nil {
		tp.logger.Error("thinking_preserve: failed to marshal repaired request", zap.Error(err))
		return body, false
	}
	return result, true
}

func (tp *ThinkingPreserver) describeCacheCandidate(requestBody []byte, contentBlocks []map[string]interface{}) (int, string, string, bool) {
	thinkingCount := 0
	for _, block := range contentBlocks {
		blockType, _ := block["type"].(string)
		if blockType == "thinking" {
			if sig, _ := block["signature"].(string); sig != "" {
				thinkingCount++
			}
		}
	}

	visibleHash, hasVisibleContent := hashVisibleContent(contentBlocks)
	if thinkingCount == 0 || !hasVisibleContent {
		return thinkingCount, "", "", false
	}

	prefixHash := hashRequestMessages(requestBody)
	return thinkingCount, visibleHash, prefixHash, true
}

func containsThinkingBlock(content []map[string]interface{}) bool {
	for _, block := range content {
		if blockType, _ := block["type"].(string); blockType == "thinking" {
			return true
		}
	}
	return false
}

func hashRequestMessages(body []byte) string {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	messages, ok := req["messages"]
	if !ok {
		return ""
	}
	return hashNormalized(messages)
}

func hashVisibleContent(content interface{}) (string, bool) {
	visible, hasVisible := stripThinkingContent(content)
	if !hasVisible {
		return "", false
	}
	return hashNormalized(visible), true
}

func stripThinkingContent(content interface{}) ([]interface{}, bool) {
	switch typed := content.(type) {
	case []interface{}:
		visible := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == "thinking" {
					continue
				}
			}
			visible = append(visible, normalizeForMatching(item))
		}
		return visible, len(visible) > 0
	case []map[string]interface{}:
		visible := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if blockType, _ := item["type"].(string); blockType == "thinking" {
				continue
			}
			visible = append(visible, normalizeForMatching(item))
		}
		return visible, len(visible) > 0
	default:
		return nil, false
	}
}

func hashNormalized(value interface{}) string {
	data, err := json.Marshal(normalizeForMatching(value))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func normalizedJSON(value interface{}) string {
	data, err := json.Marshal(normalizeForMatching(value))
	if err != nil {
		return ""
	}
	return string(data)
}

func normalizeForMatching(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			if key == "cache_control" {
				continue
			}
			normalized[key] = normalizeForMatching(val)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeForMatching(item))
		}
		return normalized
	case []map[string]interface{}:
		normalized := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeForMatching(item))
		}
		return normalized
	default:
		return value
	}
}

func mapsToInterfaces(content []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(content))
	for _, block := range content {
		result = append(result, block)
	}
	return result
}

func shortHash(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16]
}

func VisibleTextPreview(content []map[string]interface{}) string {
	visible, ok := stripThinkingContent(content)
	if !ok {
		return ""
	}

	var parts []string
	for _, item := range visible {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if blockType, _ := block["type"].(string); blockType == "text" {
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "")
}
