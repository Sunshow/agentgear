package proxy

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
	"go.uber.org/zap"
)

func TestCacheThinkingBlocksFromSSE_ReinjectsForStreamedTextResponses(t *testing.T) {
	logger := zap.NewNop()
	cfg := memory.DefaultThinkingStoreConfig()
	cfg.PersistPath = filepath.Join(t.TempDir(), "thinking-store.db")
	store := memory.NewThinkingStore(cfg)
	defer store.Close()

	handler := &Handler{
		businessLogger:    logger,
		thinkingStore:     store,
		thinkingPreserver: transformer.NewThinkingPreserver(store, logger),
	}

	requestBody := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "请总结一下结果"}
				]
			}
		]
	}`)

	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-123"}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"全部完成"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"。"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_stop
data: {"type":"message_stop"}
`

	handler.cacheThinkingBlocksFromSSE(requestBody, []byte(sse))

	reqBody := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "请总结一下结果"}
				]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "全部完成。"}
				]
			}
		]
	}`)

	injectedBody, injected := handler.thinkingPreserver.InjectIntoRequest(reqBody)
	if !injected {
		t.Fatal("expected thinking blocks to be injected for streamed text response")
	}

	var req map[string]interface{}
	if err := json.Unmarshal(injectedBody, &req); err != nil {
		t.Fatalf("failed to parse injected request: %v", err)
	}

	messages := req["messages"].([]interface{})
	content := messages[1].(map[string]interface{})["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks after injection, got %d", len(content))
	}

	thinking := content[0].(map[string]interface{})
	if got := thinking["type"]; got != "thinking" {
		t.Fatalf("expected first content block to be thinking, got %v", got)
	}
	if got := thinking["signature"]; got != "sig-123" {
		t.Fatalf("expected injected signature sig-123, got %v", got)
	}

	text := content[1].(map[string]interface{})
	if got := text["text"]; got != "全部完成。" {
		t.Fatalf("expected original text block to be preserved, got %v", got)
	}
}
