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

	originalCount := len(messages)

	// 1. Remove leading assistant messages
	for len(messages) > 0 && messages[0].Role == "assistant" {
		s.logger.Info("removing leading assistant message")
		messages = messages[1:]
	}

	// 2. Merge consecutive same-role messages
	merged := []Message{}
	for i := 0; i < len(messages); i++ {
		current := messages[i]

		// Collect consecutive same-role messages
		for i+1 < len(messages) && messages[i+1].Role == current.Role {
			next := messages[i+1]
			current = s.mergeMessages(current, next)
			i++
		}

		merged = append(merged, current)
	}

	if len(merged) == originalCount {
		return reqBody, false, nil // No changes
	}

	req["messages"] = merged
	newBody, err := json.Marshal(req)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to marshal request: %w", err)
	}

	s.logger.Info("message sanitization completed",
		zap.Int("original_count", originalCount),
		zap.Int("sanitized_count", len(merged)))

	return newBody, true, nil
}

// mergeMessages merges two messages with the same role
func (s *MessageSanitizer) mergeMessages(m1, m2 Message) Message {
	// Handle content arrays
	if arr1, ok := m1.Content.([]interface{}); ok {
		if arr2, ok := m2.Content.([]interface{}); ok {
			return Message{
				Role:    m1.Role,
				Content: append(arr1, arr2...),
			}
		}
	}

	// Handle string content
	str1 := s.contentToString(m1.Content)
	str2 := s.contentToString(m2.Content)

	return Message{
		Role:    m1.Role,
		Content: str1 + "\n" + str2,
	}
}

func (s *MessageSanitizer) contentToString(content interface{}) string {
	if str, ok := content.(string); ok {
		return str
	}
	data, _ := json.Marshal(content)
	return string(data)
}
