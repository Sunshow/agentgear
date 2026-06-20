package transformer

import (
	"encoding/json"
	"testing"

	"go.uber.org/zap"
)

func TestDedupCacheFromMessageStart(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name             string
		input            string
		wantModified     bool
		wantInput        float64
		wantCacheRead    float64
		wantCacheCreate  float64
	}{
		{
			name:            "upstream double-counts cache (the reported bug)",
			input:           `{"message":{"usage":{"input_tokens":24336,"output_tokens":134,"cache_creation_input_tokens":0,"cache_read_input_tokens":24256}}}`,
			wantModified:    true,
			wantInput:       80, // 24336 - 24256 = 80 (actual non-cached input)
			wantCacheRead:   24256,
			wantCacheCreate: 0,
		},
		{
			name:            "both cache_creation and cache_read subtracted",
			input:           `{"message":{"usage":{"input_tokens":1000,"output_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}}`,
			wantModified:    true,
			wantInput:       500,
			wantCacheRead:   300,
			wantCacheCreate: 200,
		},
		{
			name:        "native Anthropic semantics: input < cache, skip",
			input:       `{"message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":300}}}`,
			wantModified: false,
		},
		{
			name:        "no cache tokens",
			input:       `{"message":{"usage":{"input_tokens":100,"output_tokens":50}}}`,
			wantModified: false,
		},
		{
			name:        "zero cache values",
			input:       `{"message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
			wantModified: false,
		},
		{
			name:        "no usage field",
			input:       `{"message":{"content":[]}}`,
			wantModified: false,
		},
		{
			name:        "invalid json",
			input:       `not json`,
			wantModified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, modified := DedupCacheFromMessageStart(tt.input, logger)
			if modified != tt.wantModified {
				t.Errorf("modified = %v, want %v", modified, tt.wantModified)
			}
			if !modified {
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(result), &payload); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}
			msg := payload["message"].(map[string]interface{})
			usage := msg["usage"].(map[string]interface{})
			if got := usage["input_tokens"].(float64); got != tt.wantInput {
				t.Errorf("input_tokens = %v, want %v", got, tt.wantInput)
			}
			if got := usage["cache_read_input_tokens"].(float64); got != tt.wantCacheRead {
				t.Errorf("cache_read_input_tokens = %v, want %v", got, tt.wantCacheRead)
			}
			if got := usage["cache_creation_input_tokens"].(float64); got != tt.wantCacheCreate {
				t.Errorf("cache_creation_input_tokens = %v, want %v", got, tt.wantCacheCreate)
			}
		})
	}
}

func TestDedupCacheFromMessageDelta(t *testing.T) {
	logger := zap.NewNop()

	input := `{"type":"message_delta","usage":{"input_tokens":24336,"output_tokens":500,"cache_creation_input_tokens":50,"cache_read_input_tokens":150}}`
	result, modified := DedupCacheFromMessageDelta(input, logger)
	if !modified {
		t.Fatal("expected modified")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	usage := payload["usage"].(map[string]interface{})
	// 24336 - 50 - 150 = 24136
	if got := usage["input_tokens"].(float64); got != 24136 {
		t.Errorf("input_tokens = %v, want 24136", got)
	}
	// Cache fields preserved
	if got := usage["cache_read_input_tokens"].(float64); got != 150 {
		t.Errorf("cache_read_input_tokens = %v, want 150", got)
	}
	if got := usage["cache_creation_input_tokens"].(float64); got != 50 {
		t.Errorf("cache_creation_input_tokens = %v, want 50", got)
	}
}

func TestDedupCacheFromResponse(t *testing.T) {
	logger := zap.NewNop()

	// Reproduces the user's reported scenario: 80 input + 24256 cache_read but upstream
	// reports input_tokens as 24336 (already including the cached portion).
	input := `{"id":"msg_123","usage":{"input_tokens":24336,"output_tokens":134,"cache_creation_input_tokens":0,"cache_read_input_tokens":24256}}`
	result, modified := DedupCacheFromResponse([]byte(input), logger)
	if !modified {
		t.Fatal("expected modified")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	usage := payload["usage"].(map[string]interface{})
	if got := usage["input_tokens"].(float64); got != 80 {
		t.Errorf("input_tokens = %v, want 80 (after dedup)", got)
	}
	if got := usage["cache_read_input_tokens"].(float64); got != 24256 {
		t.Errorf("cache_read_input_tokens = %v, want 24256 (preserved)", got)
	}
}
