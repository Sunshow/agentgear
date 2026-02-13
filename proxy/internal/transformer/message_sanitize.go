package transformer

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// MessageSanitizer handles message format sanitization
type MessageSanitizer struct {
	def    *TransformerDef
	logger *zap.Logger
}

// NewMessageSanitizer creates a new message sanitizer
func NewMessageSanitizer(def *TransformerDef, logger *zap.Logger) *MessageSanitizer {
	return &MessageSanitizer{
		def:    def,
		logger: logger,
	}
}

// Name returns the transformer definition name
func (s *MessageSanitizer) Name() string {
	return s.def.Name
}

// Sanitize fixes message format issues
func (s *MessageSanitizer) Sanitize(reqBody []byte) ([]byte, bool, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return reqBody, false, fmt.Errorf("failed to parse request: %w", err)
	}

	messagesRaw, ok := req["messages"]
	if !ok {
		return reqBody, false, nil
	}

	messagesData, _ := json.Marshal(messagesRaw)
	var messages []Message
	if err := json.Unmarshal(messagesData, &messages); err != nil {
		return reqBody, false, fmt.Errorf("failed to parse messages: %w", err)
	}

	if len(messages) == 0 {
		return reqBody, false, nil
	}

	modified := false

	// Prepend placeholder user message if conversation starts with assistant
	if messages[0].Role == "assistant" {
		placeholder := Message{Role: "user", Content: "."}
		messages = append([]Message{placeholder}, messages...)
		s.logger.Info("prepended placeholder user message for assistant-first conversation")
		modified = true
	}

	if !modified {
		return reqBody, false, nil
	}

	req["messages"] = messages
	newBody, err := json.Marshal(req)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to marshal request: %w", err)
	}

	return newBody, true, nil
}
