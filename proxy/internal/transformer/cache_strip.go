package transformer

import (
	"encoding/json"

	"go.uber.org/zap"
)

// stripCacheFromUsage modifies a usage map: adds cache_read_input_tokens to input_tokens,
// then zeros out cache_creation_input_tokens and cache_read_input_tokens.
// Returns true if any modification was made.
func stripCacheFromUsage(usage map[string]interface{}, logger *zap.Logger) bool {
	cacheRead, hasRead := usage["cache_read_input_tokens"].(float64)
	_, hasCreation := usage["cache_creation_input_tokens"].(float64)

	if !hasRead && !hasCreation {
		return false
	}

	if hasRead && cacheRead > 0 {
		inputTokens, _ := usage["input_tokens"].(float64)
		usage["input_tokens"] = inputTokens + cacheRead
		if logger != nil {
			logger.Info("cache_strip: merged cache_read_input_tokens into input_tokens",
				zap.Float64("cache_read", cacheRead),
				zap.Float64("original_input", inputTokens),
				zap.Float64("new_input", inputTokens+cacheRead))
		}
	}

	usage["cache_creation_input_tokens"] = float64(0)
	usage["cache_read_input_tokens"] = float64(0)
	return true
}

// StripCacheFromMessageStart processes a message_start SSE event data,
// modifying message.usage to strip cache tokens.
func StripCacheFromMessageStart(data string, logger *zap.Logger) (string, bool) {
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

	if !stripCacheFromUsage(usage, logger) {
		return data, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return data, false
	}
	return string(result), true
}

// StripCacheFromMessageDelta processes a message_delta SSE event data,
// modifying usage to strip cache tokens.
func StripCacheFromMessageDelta(data string, logger *zap.Logger) (string, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return data, false
	}

	usage, ok := payload["usage"].(map[string]interface{})
	if !ok {
		return data, false
	}

	if !stripCacheFromUsage(usage, logger) {
		return data, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return data, false
	}
	return string(result), true
}

// StripCacheFromResponse processes a non-streaming response body,
// modifying the top-level usage to strip cache tokens.
func StripCacheFromResponse(body []byte, logger *zap.Logger) ([]byte, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}

	usage, ok := payload["usage"].(map[string]interface{})
	if !ok {
		return body, false
	}

	if !stripCacheFromUsage(usage, logger) {
		return body, false
	}

	result, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return result, true
}
