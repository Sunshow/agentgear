package transformer

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/sunshow/agentgear/proxy/internal/memory"
	"go.uber.org/zap"
)

func TestThinkingPreserver_CachesNonStreamingResponseAndRepairsTextOnlyHistory(t *testing.T) {
	logger := zap.NewNop()
	cfg := memory.DefaultThinkingStoreConfig()
	cfg.PersistPath = filepath.Join(t.TempDir(), "thinking-store.json")
	store := memory.NewThinkingStore(cfg)
	defer store.Close()

	preserver := NewThinkingPreserver(store, logger)
	requestBody := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hi"}]}
		]
	}`)
	responseContent := []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-json"},
		{"type": "text", "text": "OK"},
	}

	preserver.CacheFromResponse(requestBody, responseContent)

	nextRequest := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hi"}]},
			{"role": "assistant", "content": [{"type": "text", "text": "OK"}]},
			{"role": "user", "content": [{"type": "text", "text": "repeat"}]}
		]
	}`)

	injectedBody, injected := preserver.InjectIntoRequest(nextRequest)
	if !injected {
		t.Fatal("expected text-only assistant history to be repaired")
	}

	content := extractAssistantContent(t, injectedBody, 1)
	assertThinkingSignature(t, content, "sig-json")
}

func TestThinkingPreserver_ReplacesPartialThinkingContent(t *testing.T) {
	logger := zap.NewNop()
	cfg := memory.DefaultThinkingStoreConfig()
	cfg.PersistPath = filepath.Join(t.TempDir(), "thinking-store.json")
	store := memory.NewThinkingStore(cfg)
	defer store.Close()

	preserver := NewThinkingPreserver(store, logger)
	requestBody := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "run it"}]}
		]
	}`)
	fullContent := []map[string]interface{}{
		{"type": "thinking", "thinking": "", "signature": "sig-a"},
		{"type": "text", "text": "working"},
		{"type": "thinking", "thinking": "", "signature": "sig-b"},
		{
			"type": "tool_use",
			"id":   "call_1",
			"name": "Execute",
			"input": map[string]interface{}{
				"command": "date",
			},
		},
	}
	preserver.CacheFromResponse(requestBody, fullContent)

	nextRequest := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "run it"}]},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "", "signature": "sig-a"},
					{"type": "text", "text": "working"},
					{"type": "tool_use", "id": "call_1", "name": "Execute", "input": {"command": "date"}}
				]
			}
		]
	}`)

	injectedBody, injected := preserver.InjectIntoRequest(nextRequest)
	if !injected {
		t.Fatal("expected partial thinking content to be repaired")
	}

	content := extractAssistantContent(t, injectedBody, 1)
	if got := len(content); got != 4 {
		t.Fatalf("expected repaired assistant content to have 4 blocks, got %d", got)
	}
	assertThinkingSignature(t, content, "sig-a")
	if got := content[2].(map[string]interface{})["signature"]; got != "sig-b" {
		t.Fatalf("expected second thinking block signature sig-b, got %v", got)
	}
}

func TestThinkingPreserver_PrefersExactPrefixMatchForVisibleHashCollisions(t *testing.T) {
	logger := zap.NewNop()
	cfg := memory.DefaultThinkingStoreConfig()
	cfg.PersistPath = filepath.Join(t.TempDir(), "thinking-store.json")
	store := memory.NewThinkingStore(cfg)
	defer store.Close()

	preserver := NewThinkingPreserver(store, logger)
	preserver.CacheFromResponse(
		[]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"alpha"}]}]}`),
		[]map[string]interface{}{
			{"type": "thinking", "thinking": "", "signature": "sig-alpha"},
			{"type": "text", "text": "OK"},
		},
	)
	preserver.CacheFromResponse(
		[]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"beta"}]}]}`),
		[]map[string]interface{}{
			{"type": "thinking", "thinking": "", "signature": "sig-beta"},
			{"type": "text", "text": "OK"},
		},
	)

	betaRequest := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "beta"}]},
			{"role": "assistant", "content": [{"type": "text", "text": "OK"}]}
		]
	}`)

	injectedBody, injected := preserver.InjectIntoRequest(betaRequest)
	if !injected {
		t.Fatal("expected exact prefix match to repair assistant content")
	}
	content := extractAssistantContent(t, injectedBody, 1)
	assertThinkingSignature(t, content, "sig-beta")

	ambiguousRequest := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "gamma"}]},
			{"role": "assistant", "content": [{"type": "text", "text": "OK"}]}
		]
	}`)
	_, injected = preserver.InjectIntoRequest(ambiguousRequest)
	if injected {
		t.Fatal("expected ambiguous visible-hash match without exact prefix to be ignored")
	}
}

func extractAssistantContent(t *testing.T, body []byte, index int) []interface{} {
	t.Helper()

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	messages := req["messages"].([]interface{})
	return messages[index].(map[string]interface{})["content"].([]interface{})
}

func assertThinkingSignature(t *testing.T, content []interface{}, signature string) {
	t.Helper()

	if got := content[0].(map[string]interface{})["type"]; got != "thinking" {
		t.Fatalf("expected first content block to be thinking, got %v", got)
	}
	if got := content[0].(map[string]interface{})["signature"]; got != signature {
		t.Fatalf("expected injected signature %s, got %v", signature, got)
	}
}
