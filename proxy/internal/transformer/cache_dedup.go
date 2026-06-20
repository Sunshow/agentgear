package transformer

import (
	"encoding/json"

	"go.uber.org/zap"
)

// dedupCacheFromUsage modifies a usage map: subtracts cache_read_input_tokens and
// cache_creation_input_tokens from input_tokens (clamped at 0), keeping cache fields intact.
//
// This is intended for upstreams that report an input_tokens value that already includes the
// cached tokens. Without dedup, a standards-compliant downstream relay would bill both the
// full input_tokens and the cache_read_input_tokens, double-counting the cached portion.
//
// Returns true if input_tokens was modified.
func dedupCacheFromUsage(usage map[string]interface{}, logger *zap.Logger) bool {
	cacheRead, hasRead := usage["cache_read_input_tokens"].(float64)
	cacheCreation, hasCreation := usage["cache_creation_input_tokens"].(float64)

	if !hasRead && !hasCreation {
		return false
	}
	if cacheRead == 0 && cacheCreation == 0 {
		return false
	}

	inputTokens, _ := usage["input_tokens"].(float64)
	cacheTotal := cacheRead + cacheCreation

	// Only subtract when input already includes the cache portion (the broken case).
	// If input < cache (e.g. Anthropic native semantics), leave it untouched.
	if inputTokens < cacheTotal {
		if logger != nil {
			logger.Info("cache_dedup: input_tokens < cache total, skipping",
				zap.Float64("input_tokens", inputTokens),
				zap.Float64("cache_read", cacheRead),
				zap.Float64("cache_creation", cacheCreation))
		}
		return false
	}

	newInput := inputTokens - cacheTotal
	usage["input_tokens"] = newInput

	if logger != nil {
		logger.Info("cache_dedup: subtracted cache tokens from input_tokens",
			zap.Float64("original_input", inputTokens),
			zap.Float64("cache_read", cacheRead),
			zap.Float64("cache_creation", cacheCreation),
			zap.Float64("new_input", newInput))
	}
	return true
}

// DedupCacheFromMessageStart processes a message_start SSE event data,
// subtracting cache tokens from message.usage.input_tokens.
func DedupCacheFromMessageStart(data string, logger *zap.Logger) (string, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return data, false
	}

	message, ok := payload["message"].(map[string]interface{})
	if !ok {
		return data, false
	}

	usage, ok := message["usage"].(map[string]interface{})
	if !ok {
		return data, false
	}

	if !dedupCacheFromUsage(usage, logger) {
		return data, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return data, false
	}
	return string(result), true
}

// DedupCacheFromMessageDelta processes a message_delta SSE event data,
// subtracting cache tokens from usage.input_tokens.
func DedupCacheFromMessageDelta(data string, logger *zap.Logger) (string, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return data, false
	}

	usage, ok := payload["usage"].(map[string]interface{})
	if !ok {
		return data, false
	}

	if !dedupCacheFromUsage(usage, logger) {
		return data, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return data, false
	}
	return string(result), true
}

// DedupCacheFromResponse processes a non-streaming response body,
// subtracting cache tokens from the top-level usage.input_tokens.
func DedupCacheFromResponse(body []byte, logger *zap.Logger) ([]byte, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}

	usage, ok := payload["usage"].(map[string]interface{})
	if !ok {
		return body, false
	}

	if !dedupCacheFromUsage(usage, logger) {
		return body, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return result, true
}
