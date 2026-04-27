package transformer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/sunshow/agentgear/proxy/internal/memory"
	"go.uber.org/zap"
)

// ThinkingPreserver handles caching thinking blocks from responses and injecting them into requests.
type ThinkingPreserver struct {
	store  *memory.ThinkingStore
	logger *zap.Logger
}

func NewThinkingPreserver(store *memory.ThinkingStore, logger *zap.Logger) *ThinkingPreserver {
	return &ThinkingPreserver{store: store, logger: logger}
}

// CacheFromResponse extracts thinking blocks from a complete assistant message's content blocks
// and stores them keyed by the hash of non-thinking content.
func (tp *ThinkingPreserver) CacheFromResponse(contentBlocks []map[string]interface{}) {
	var thinkingBlocks []memory.ThinkingBlock
	var nonThinkingBlocks []map[string]interface{}

	for _, block := range contentBlocks {
		blockType, _ := block["type"].(string)
		if blockType == "thinking" {
			sig, _ := block["signature"].(string)
			thinking, _ := block["thinking"].(string)
			if sig != "" {
				thinkingBlocks = append(thinkingBlocks, memory.ThinkingBlock{
					Type:      "thinking",
					Thinking:  thinking,
					Signature: sig,
				})
			}
		} else {
			nonThinkingBlocks = append(nonThinkingBlocks, block)
		}
	}

	if len(thinkingBlocks) == 0 || len(nonThinkingBlocks) == 0 {
		return
	}

	// Log thinking content details
	for i, tb := range thinkingBlocks {
		if tb.Thinking == "" {
			tp.logger.Info("thinking_preserve: caching empty thinking block (agent may drop this)",
				zap.Int("index", i),
				zap.Int("signature_len", len(tb.Signature)))
		} else {
			tp.logger.Debug("thinking_preserve: caching thinking block",
				zap.Int("index", i),
				zap.Int("thinking_len", len(tb.Thinking)),
				zap.Int("signature_len", len(tb.Signature)))
		}
	}

	hash := computeContentHash(nonThinkingBlocks)
	tp.store.Put(hash, thinkingBlocks)
	tp.logger.Info("thinking_preserve: cached thinking blocks from response",
		zap.String("hash", hash[:16]),
		zap.Int("thinking_blocks", len(thinkingBlocks)),
		zap.Int("non_thinking_blocks", len(nonThinkingBlocks)),
		zap.Int("store_size", tp.store.Size()))
}

// InjectIntoRequest scans assistant messages in the request body and injects
// cached thinking blocks where missing. Returns modified body and whether any injection occurred.
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
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role != "assistant" {
			continue
		}
		content, ok := msgMap["content"].([]interface{})
		if !ok {
			continue
		}

		// Check if thinking blocks already present
		hasThinking := false
		var nonThinkingBlocks []map[string]interface{}
		for _, c := range content {
			cMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cMap["type"] == "thinking" {
				hasThinking = true
				break
			}
			nonThinkingBlocks = append(nonThinkingBlocks, cMap)
		}

		if hasThinking || len(nonThinkingBlocks) == 0 {
			continue
		}

		hash := computeContentHash(nonThinkingBlocks)
		cached := tp.store.Get(hash)
		if cached == nil {
			tp.logger.Debug("thinking_preserve: cache miss for assistant message without thinking",
				zap.String("hash", hash[:16]),
				zap.Int("non_thinking_blocks", len(nonThinkingBlocks)))
			continue
		}

		// Inject thinking blocks at the beginning of content
		newContent := make([]interface{}, 0, len(cached)+len(content))
		for _, tb := range cached {
			newContent = append(newContent, map[string]interface{}{
				"type":      tb.Type,
				"thinking":  tb.Thinking,
				"signature": tb.Signature,
			})
		}
		newContent = append(newContent, content...)
		msgMap["content"] = newContent
		injected = true

		tp.logger.Info("thinking_preserve: injected thinking blocks into request",
			zap.String("hash", hash[:16]),
			zap.Int("thinking_blocks", len(cached)),
			zap.Int("store_size", tp.store.Size()))
	}

	if !injected {
		return body, false
	}

	result, err := json.Marshal(req)
	if err != nil {
		tp.logger.Error("thinking_preserve: failed to marshal", zap.Error(err))
		return body, false
	}
	return result, true
}

// computeContentHash computes SHA256 hash of non-thinking content blocks.
// Normalizes blocks to use only stable fields for deterministic hashing.
func computeContentHash(blocks []map[string]interface{}) string {
	var normalized []map[string]interface{}
	for _, b := range blocks {
		blockType, _ := b["type"].(string)
		nb := make(map[string]interface{})

		switch blockType {
		case "tool_use":
			nb["type"] = blockType
			if id, ok := b["id"]; ok {
				nb["id"] = id
			}
			if name, ok := b["name"]; ok {
				nb["name"] = name
			}
			if input, ok := b["input"]; ok {
				nb["input"] = input
			}
		case "text":
			nb["type"] = blockType
			if text, ok := b["text"]; ok {
				nb["text"] = text
			}
		default:
			nb["type"] = blockType
			if id, ok := b["id"]; ok {
				nb["id"] = id
			}
		}

		normalized = append(normalized, nb)
	}

	data, _ := json.Marshal(normalized)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
