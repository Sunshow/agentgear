package transformer

import (
	"encoding/json"
	"testing"

	"go.uber.org/zap"
)

func TestSelectAnchor(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	def := &TransformerDef{
		PreserveBudget:     100, // Small budget for testing
		TokenEstimateRatio: 1.0, // 1 byte = 1 token for simplicity
	}
	handler := NewCompressHandler(def, logger, nil)

	messages := []Message{
		{Role: "user", Content: "Message 1 with 20 chars"},      // ~24 tokens
		{Role: "assistant", Content: "Message 2 with 20 chars"}, // ~24 tokens
		{Role: "user", Content: "Message 3 with 20 chars"},      // ~24 tokens
		{Role: "assistant", Content: "Message 4 with 20 chars"}, // ~24 tokens
		{Role: "user", Content: "Message 5 with 20 chars"},      // ~24 tokens (newest)
	}

	prefix, suffix, anchorIndex := handler.SelectAnchor(messages)

	// Should preserve last ~100 tokens
	// Message 5 (24) + Message 4 (24) + Message 3 (24) + Message 2 (24) = 96 tokens < 100
	// So suffix should have 4 messages, prefix should have 1
	if len(prefix) != 1 {
		t.Errorf("Expected 1 prefix message, got %d", len(prefix))
	}
	if len(suffix) != 4 {
		t.Errorf("Expected 4 suffix messages, got %d", len(suffix))
	}
	if anchorIndex != 1 {
		t.Errorf("Expected anchor index 1, got %d", anchorIndex)
	}
}

func TestSelectAnchorWithLargePrefix(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	def := &TransformerDef{
		PreserveBudget:     50, // Small budget
		TokenEstimateRatio: 1.0,
	}
	handler := NewCompressHandler(def, logger, nil)

	messages := []Message{
		{Role: "user", Content: "Message 1 with 20 chars"},      // ~20 tokens
		{Role: "assistant", Content: "Message 2 with 20 chars"}, // ~20 tokens
		{Role: "user", Content: "Message 3 with 20 chars"},      // ~20 tokens
		{Role: "assistant", Content: "Message 4 with 20 chars"}, // ~20 tokens
		{Role: "user", Content: "Message 5 with 20 chars"},      // ~20 tokens (newest)
	}

	prefix, suffix, anchorIndex := handler.SelectAnchor(messages)

	// Should preserve last ~50 tokens (messages 4-5 = 40 tokens)
	if len(prefix) != 3 {
		t.Errorf("Expected 3 prefix messages, got %d", len(prefix))
	}
	if len(suffix) != 2 {
		t.Errorf("Expected 2 suffix messages, got %d", len(suffix))
	}
	if anchorIndex != 3 {
		t.Errorf("Expected anchor index 3, got %d", anchorIndex)
	}
}

func TestSanitizeToolMessages(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewCompressHandler(&TransformerDef{}, logger, nil)

	messages := []Message{
		{
			Role: "assistant",
			Content: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "I'll run a command",
				},
				map[string]interface{}{
					"type":         "tool-call",
					"tool_call_id": "call_123",
					"tool_name":    "bash",
					"input":        map[string]interface{}{"command": "ls -la"},
				},
			},
		},
		{
			Role: "tool",
			Content: []interface{}{
				map[string]interface{}{
					"type":         "tool-result",
					"tool_call_id": "call_123",
					"tool_name":    "bash",
					"output":       "file1.txt\nfile2.txt",
				},
			},
		},
	}

	sanitized := handler.SanitizeToolMessages(messages)

	if len(sanitized) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(sanitized))
	}

	// Check first message (assistant with tool-call)
	if sanitized[0].Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", sanitized[0].Role)
	}
	blocks, ok := sanitized[0].Content.([]interface{})
	if !ok {
		t.Fatal("Expected content to be []interface{}")
	}
	if len(blocks) != 2 {
		t.Errorf("Expected 2 content blocks, got %d", len(blocks))
	}
	// Second block should be converted to text
	block2, ok := blocks[1].(map[string]interface{})
	if !ok || block2["type"] != "text" {
		t.Error("Expected second block to be text type")
	}

	// Check second message (tool -> user)
	if sanitized[1].Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", sanitized[1].Role)
	}
	content, ok := sanitized[1].Content.(string)
	if !ok {
		t.Fatal("Expected content to be string")
	}
	if content == "" {
		t.Error("Expected non-empty content")
	}
}

func TestBuildCompressRequest(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	def := &TransformerDef{
		CompressModel:        "claude-3-5-sonnet-20241022",
		CompressSystemPrompt: "Test system prompt",
		CompressUserPrompt:   "Test user prompt",
	}
	handler := NewCompressHandler(def, logger, nil)

	prefix := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	reqBody, err := handler.BuildCompressRequest(prefix, "claude-opus-4-6-20260101")
	if err != nil {
		t.Fatalf("BuildCompressRequest failed: %v", err)
	}

	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	// Check model
	if req["model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("Expected model 'claude-3-5-sonnet-20241022', got '%v'", req["model"])
	}

	// Check system
	if req["system"] != "Test system prompt" {
		t.Errorf("Expected system 'Test system prompt', got '%v'", req["system"])
	}

	// Check messages
	messagesRaw, ok := req["messages"].([]interface{})
	if !ok {
		t.Fatal("Expected messages to be []interface{}")
	}
	if len(messagesRaw) != 3 { // 2 prefix + 1 user prompt
		t.Errorf("Expected 3 messages, got %d", len(messagesRaw))
	}
	
	// Check last message is user prompt
	lastMsg, ok := messagesRaw[2].(map[string]interface{})
	if !ok {
		t.Fatal("Expected last message to be map")
	}
	if lastMsg["role"] != "user" || lastMsg["content"] != "Test user prompt" {
		t.Error("Expected last message to be user prompt")
	}
}

func TestShouldCompress(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	def := &TransformerDef{
		ContextTokenLimit:     1000,
		ContextThresholdRatio: 0.7,
		TokenEstimateRatio:    1.0, // 1 byte = 1 token
	}
	handler := NewCompressHandler(def, logger, nil)

	// Test case 1: Below threshold
	smallReq := make([]byte, 600) // 600 tokens < 700 threshold
	if handler.ShouldCompress(smallReq, "test-model") {
		t.Error("Should not compress small request")
	}

	// Test case 2: Above threshold
	largeReq := make([]byte, 800) // 800 tokens > 700 threshold
	if !handler.ShouldCompress(largeReq, "test-model") {
		t.Error("Should compress large request")
	}
}

func TestShouldCompressWithModelLimits(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	def := &TransformerDef{
		ContextTokenLimit:     1000,
		ContextThresholdRatio: 0.7,
		TokenEstimateRatio:    1.0,
		ModelContextLimits: []ModelContextLimit{
			{ModelPattern: "claude-opus-4-6*", TokenLimit: 10000},
		},
	}
	handler := NewCompressHandler(def, logger, nil)

	// Test with opus model (higher limit)
	largeReq := make([]byte, 5000) // 5000 tokens < 7000 threshold (10000 * 0.7)
	if handler.ShouldCompress(largeReq, "claude-opus-4-6-20260101") {
		t.Error("Should not compress for opus model with higher limit")
	}

	// Test with other model (default limit)
	if !handler.ShouldCompress(largeReq, "claude-3-5-sonnet-20241022") {
		t.Error("Should compress for other model with default limit")
	}
}

func TestReplaceMessages(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewCompressHandler(&TransformerDef{}, logger, nil)

	originalReq := map[string]interface{}{
		"model": "test-model",
		"messages": []Message{
			{Role: "user", Content: "Old message 1"},
			{Role: "assistant", Content: "Old message 2"},
			{Role: "user", Content: "Old message 3"},
		},
	}
	originalReqBody, _ := json.Marshal(originalReq)

	summary := "This is a summary of the conversation"
	suffix := []Message{
		{Role: "user", Content: "Recent message"},
	}

	newReqBody, err := handler.ReplaceMessages(originalReqBody, summary, suffix)
	if err != nil {
		t.Fatalf("ReplaceMessages failed: %v", err)
	}

	var newReq map[string]interface{}
	if err := json.Unmarshal(newReqBody, &newReq); err != nil {
		t.Fatalf("Failed to parse new request: %v", err)
	}

	// Check messages
	messagesRaw, ok := newReq["messages"]
	if !ok {
		t.Fatal("No messages in new request")
	}

	messagesData, _ := json.Marshal(messagesRaw)
	var messages []Message
	json.Unmarshal(messagesData, &messages)

	if len(messages) != 2 { // 1 summary + 1 suffix
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != summary {
		t.Error("Expected first message to be summary")
	}
	if messages[1].Role != "user" || messages[1].Content != "Recent message" {
		t.Error("Expected second message to be suffix")
	}
}

func TestMatchModelPattern(t *testing.T) {
	tests := []struct {
		model   string
		pattern string
		want    bool
	}{
		{"claude-opus-4-6-20260101", "claude-opus-4-6*", true},
		{"claude-opus-4.6-20260101", "claude-opus-4.6*", true},
		{"claude-3-5-sonnet-20241022", "claude-opus-4-6*", false},
		{"any-model", "*", true},
		{"exact-match", "exact-match", true},
		{"not-match", "exact-match", false},
	}

	for _, tt := range tests {
		got := matchModelPattern(tt.model, tt.pattern)
		if got != tt.want {
			t.Errorf("matchModelPattern(%q, %q) = %v, want %v", tt.model, tt.pattern, got, tt.want)
		}
	}
}
