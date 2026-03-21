package transformer

import (
	"encoding/json"
	"testing"

	"go.uber.org/zap"
)

func TestStripCacheFromMessageStart(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		input          string
		wantModified   bool
		wantInput      float64
		wantCacheRead  float64
		wantCacheCreate float64
	}{
		{
			name:           "strip cache tokens",
			input:          `{"message":{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}}`,
			wantModified:   true,
			wantInput:      600,
			wantCacheRead:  0,
			wantCacheCreate: 0,
		},
		{
			name:           "no cache tokens",
			input:          `{"message":{"usage":{"input_tokens":100,"output_tokens":50}}}`,
			wantModified:   false,
		},
		{
			name:           "only cache_creation",
			input:          `{"message":{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":200}}}`,
			wantModified:   true,
			wantInput:      300,
			wantCacheRead:  0,
			wantCacheCreate: 0,
		},
		{
			name:           "zero cache values",
			input:          `{"message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
			wantModified:   true,
			wantInput:      100,
			wantCacheRead:  0,
			wantCacheCreate: 0,
		},
		{
			name:         "no usage field",
			input:        `{"message":{"content":[]}}`,
			wantModified: false,
		},
		{
			name:         "invalid json",
			input:        `not json`,
			wantModified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, modified := StripCacheFromMessageStart(tt.input, logger)
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

func TestStripCacheFromMessageDelta(t *testing.T) {
	logger := zap.NewNop()

	input := `{"type":"message_delta","usage":{"input_tokens":100,"output_tokens":500,"cache_creation_input_tokens":50,"cache_read_input_tokens":150}}`
	result, modified := StripCacheFromMessageDelta(input, logger)
	if !modified {
		t.Fatal("expected modified")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	usage := payload["usage"].(map[string]interface{})
	if got := usage["input_tokens"].(float64); got != 300 {
		t.Errorf("input_tokens = %v, want 300", got)
	}
	if got := usage["cache_read_input_tokens"].(float64); got != 0 {
		t.Errorf("cache_read_input_tokens = %v, want 0", got)
	}
	if got := usage["cache_creation_input_tokens"].(float64); got != 0 {
		t.Errorf("cache_creation_input_tokens = %v, want 0", got)
	}
}

func TestStripCacheFromResponse(t *testing.T) {
	logger := zap.NewNop()

	input := `{"id":"msg_123","usage":{"input_tokens":1000,"output_tokens":200,"cache_creation_input_tokens":500,"cache_read_input_tokens":800}}`
	result, modified := StripCacheFromResponse([]byte(input), logger)
	if !modified {
		t.Fatal("expected modified")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	usage := payload["usage"].(map[string]interface{})
	if got := usage["input_tokens"].(float64); got != 2300 {
		t.Errorf("input_tokens = %v, want 2300", got)
	}
	if got := usage["cache_read_input_tokens"].(float64); got != 0 {
		t.Errorf("cache_read_input_tokens = %v, want 0", got)
	}
	if got := usage["cache_creation_input_tokens"].(float64); got != 0 {
		t.Errorf("cache_creation_input_tokens = %v, want 0", got)
	}
}
