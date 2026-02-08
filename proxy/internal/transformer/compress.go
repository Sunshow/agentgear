package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CompressHandler handles compression logic
type CompressHandler struct {
	def        *TransformerDef
	httpClient *http.Client
	logger     *zap.Logger
	gatewayMap map[string]string // gateway name -> upstream URL
}

// NewCompressHandler creates a new compress handler
func NewCompressHandler(def *TransformerDef, logger *zap.Logger, gatewayMap map[string]string) *CompressHandler {
	return &CompressHandler{
		def:        def,
		httpClient: &http.Client{Timeout: 300 * time.Second},
		logger:     logger,
		gatewayMap: gatewayMap,
	}
}

// Message represents a conversation message
type Message struct {
	Role    string                   `json:"role"`
	Content interface{}              `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in a message
type ContentBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	ToolCallID   string                 `json:"tool_call_id,omitempty"`
	ToolName     string                 `json:"tool_name,omitempty"`
	Input        map[string]interface{} `json:"input,omitempty"`
	Output       interface{}            `json:"output,omitempty"`
}

// ShouldCompress checks if compression is needed based on token estimation
func (h *CompressHandler) ShouldCompress(reqBody []byte, model string) bool {
	if h.def.ContextTokenLimit == 0 {
		return false
	}

	// Get model-specific token limit
	tokenLimit := h.getContextTokenLimit(model)
	if tokenLimit == 0 {
		return false
	}

	// Estimate tokens
	estimatedTokens := h.estimateTokens(reqBody)
	threshold := float64(tokenLimit) * h.def.ContextThresholdRatio

	h.logger.Info("compress check",
		zap.Int("estimated_tokens", estimatedTokens),
		zap.Int("token_limit", tokenLimit),
		zap.Float64("threshold_ratio", h.def.ContextThresholdRatio),
		zap.Float64("threshold", threshold),
		zap.Bool("should_compress", float64(estimatedTokens) > threshold))

	return float64(estimatedTokens) > threshold
}

// getContextTokenLimit returns the token limit for the given model
func (h *CompressHandler) getContextTokenLimit(model string) int {
	if model != "" && len(h.def.ModelContextLimits) > 0 {
		for _, limit := range h.def.ModelContextLimits {
			if matchModelPattern(model, limit.ModelPattern) {
				return limit.TokenLimit
			}
		}
	}
	return h.def.ContextTokenLimit
}

// matchModelPattern matches model name against pattern (supports * wildcard)
func matchModelPattern(model, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return model == pattern
	}
	// Simple wildcard matching
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(model, parts[0]) && strings.HasSuffix(model, parts[1])
	}
	return false
}

// estimateTokens estimates token count from byte size
func (h *CompressHandler) estimateTokens(data []byte) int {
	if h.def.TokenEstimateRatio == 0 {
		return 0
	}
	return int(float64(len(data)) / h.def.TokenEstimateRatio)
}

// SelectAnchor splits messages into prefix (to compress) and suffix (to preserve)
func (h *CompressHandler) SelectAnchor(messages []Message) (prefix, suffix []Message, anchorIndex int) {
	preserveBudget := h.def.PreserveBudget
	if preserveBudget == 0 {
		preserveBudget = 40000 // default
	}

	total := 0
	suffixMessages := []Message{}

	// Iterate from newest to oldest
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		estimate := h.estimateTokens([]byte(h.messageToString(msg)))

		if len(suffixMessages) == 0 {
			// Always keep the newest message
			suffixMessages = append([]Message{msg}, suffixMessages...)
			total = estimate
			if estimate > preserveBudget {
				break
			}
			continue
		}

		// Stop if adding this message exceeds budget
		if total+estimate > preserveBudget {
			break
		}

		suffixMessages = append([]Message{msg}, suffixMessages...)
		total += estimate
	}

	prefixCount := len(messages) - len(suffixMessages)
	if prefixCount > 0 {
		prefix = messages[:prefixCount]
	}
	suffix = suffixMessages
	anchorIndex = prefixCount

	h.logger.Info("message split",
		zap.Int("total_messages", len(messages)),
		zap.Int("prefix_count", len(prefix)),
		zap.Int("suffix_count", len(suffix)),
		zap.Int("anchor_index", anchorIndex))

	return prefix, suffix, anchorIndex
}

// messageToString converts message to string for token estimation
func (h *CompressHandler) messageToString(msg Message) string {
	if str, ok := msg.Content.(string); ok {
		return str
	}
	data, _ := json.Marshal(msg.Content)
	return string(data)
}

// SanitizeToolMessages converts tool-call and tool-result to plain text
func (h *CompressHandler) SanitizeToolMessages(messages []Message) []Message {
	result := make([]Message, 0, len(messages))

	for _, msg := range messages {
		// Handle role: tool -> role: user
		if msg.Role == "tool" {
			texts := []string{}
			if blocks, ok := msg.Content.([]interface{}); ok {
				for _, block := range blocks {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockMap["type"] == "tool-result" {
							id := ""
							if v, ok := blockMap["tool_call_id"].(string); ok {
								id = v
							}
							name := "unknown"
							if v, ok := blockMap["tool_name"].(string); ok {
								name = v
							}
							output := h.extractOutput(blockMap["output"])
							texts = append(texts, fmt.Sprintf("[Tool Result%s %s: %s]",
								func() string {
									if id != "" {
										return " (" + id + ")"
									}
									return ""
								}(), name, output))
						}
					}
				}
			}
			result = append(result, Message{
				Role:    "user",
				Content: strings.Join(texts, "\n"),
			})
			continue
		}

		// Handle assistant messages with tool-call/tool-result
		if msg.Role == "assistant" {
			if blocks, ok := msg.Content.([]interface{}); ok {
				newBlocks := []interface{}{}
				for _, block := range blocks {
					if blockMap, ok := block.(map[string]interface{}); ok {
						blockType := blockMap["type"]

						// tool-call -> text
						if blockType == "tool-call" {
							id := ""
							if v, ok := blockMap["tool_call_id"].(string); ok {
								id = v
							}
							name := "unknown"
							if v, ok := blockMap["tool_name"].(string); ok {
								name = v
							}
							input := h.safeStringify(blockMap["input"])
							newBlocks = append(newBlocks, map[string]interface{}{
								"type": "text",
								"text": fmt.Sprintf("[Tool Call%s: %s(%s)]",
									func() string {
										if id != "" {
											return " (" + id + ")"
										}
										return ""
									}(), name, input),
							})
							continue
						}

						// tool-result -> text
						if blockType == "tool-result" {
							id := ""
							if v, ok := blockMap["tool_call_id"].(string); ok {
								id = v
							}
							name := "unknown"
							if v, ok := blockMap["tool_name"].(string); ok {
								name = v
							}
							output := h.extractOutput(blockMap["output"])
							newBlocks = append(newBlocks, map[string]interface{}{
								"type": "text",
								"text": fmt.Sprintf("[Tool Result%s %s: %s]",
									func() string {
										if id != "" {
											return " (" + id + ")"
										}
										return ""
									}(), name, output),
							})
							continue
						}

						newBlocks = append(newBlocks, block)
					} else {
						newBlocks = append(newBlocks, block)
					}
				}
				result = append(result, Message{
					Role:    msg.Role,
					Content: newBlocks,
				})
				continue
			}
		}

		result = append(result, msg)
	}

	return result
}

// safeStringify safely converts value to JSON string
func (h *CompressHandler) safeStringify(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "[unserializable]"
	}
	return string(data)
}

// extractOutput extracts output from tool result
func (h *CompressHandler) extractOutput(output interface{}) string {
	if output == nil {
		return ""
	}
	if str, ok := output.(string); ok {
		return str
	}
	if outputMap, ok := output.(map[string]interface{}); ok {
		if outputMap["type"] == "text" {
			if v, ok := outputMap["value"].(string); ok {
				return v
			}
		}
		if outputMap["type"] == "json" {
			return h.safeStringify(outputMap["value"])
		}
	}
	return h.safeStringify(output)
}

// BuildCompressRequest constructs the compression request
func (h *CompressHandler) BuildCompressRequest(prefix []Message, originalModel string) ([]byte, error) {
	// Sanitize tool messages
	sanitized := h.SanitizeToolMessages(prefix)

	// Build system prompt
	systemPrompt := h.def.CompressSystemPrompt
	if systemPrompt == "" {
		systemPrompt = getDefaultCompressSystemPrompt()
	}

	// Build user prompt
	userPrompt := h.def.CompressUserPrompt
	if userPrompt == "" {
		userPrompt = "Please read the complete conversation above and generate a summary according to the guidelines. The new session will not have access to our conversation history, so the summary must contain all key information needed to continue the work."
	}

	// Construct messages
	messages := sanitized
	messages = append(messages, Message{
		Role:    "user",
		Content: userPrompt,
	})

	// Determine model
	model := h.def.CompressModel
	if model == "" {
		model = originalModel // Use original model if not specified
	}

	// Build request
	req := map[string]interface{}{
		"model":      model,
		"system":     systemPrompt,
		"messages":   messages,
		"max_tokens": 4096,
	}

	return json.Marshal(req)
}

// CallCompressAPI calls the compression API
func (h *CompressHandler) CallCompressAPI(reqBody []byte, targetURL string, headers map[string]string) (string, error) {
	h.logger.Info("calling compress API",
		zap.String("target_url", targetURL),
		zap.Int("request_size", len(reqBody)))

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract summary from content
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	firstBlock, ok := content[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid content block")
	}

	summary, ok := firstBlock["text"].(string)
	if !ok {
		return "", fmt.Errorf("no text in content block")
	}

	h.logger.Info("compress API success",
		zap.Int("summary_length", len(summary)))

	return summary, nil
}

// ReplaceMessages replaces original messages with summary + suffix
func (h *CompressHandler) ReplaceMessages(originalReq []byte, summary string, suffix []Message) ([]byte, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(originalReq, &req); err != nil {
		return nil, fmt.Errorf("failed to parse original request: %w", err)
	}

	// Build new messages: summary + suffix
	newMessages := []Message{
		{
			Role:    "user",
			Content: summary,
		},
	}
	newMessages = append(newMessages, suffix...)

	req["messages"] = newMessages

	h.logger.Info("messages replaced",
		zap.Int("new_message_count", len(newMessages)))

	return json.Marshal(req)
}

// Process executes the complete compression flow
func (h *CompressHandler) Process(reqBody []byte, targetURL string, headers map[string]string) (compressedReq []byte, compressed bool, err error) {
	h.logger.Info("starting compression process")

	// Parse original request
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return reqBody, false, fmt.Errorf("failed to parse request: %w", err)
	}

	// Extract messages
	messagesRaw, ok := req["messages"]
	if !ok {
		return reqBody, false, fmt.Errorf("no messages in request")
	}

	messagesData, err := json.Marshal(messagesRaw)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to marshal messages: %w", err)
	}

	var messages []Message
	if err := json.Unmarshal(messagesData, &messages); err != nil {
		return reqBody, false, fmt.Errorf("failed to parse messages: %w", err)
	}

	if len(messages) == 0 {
		return reqBody, false, fmt.Errorf("empty messages")
	}

	// Extract model
	model := ""
	if m, ok := req["model"].(string); ok {
		model = m
	}

	// Check if compression is needed
	if !h.ShouldCompress(reqBody, model) {
		h.logger.Info("compression not needed")
		return reqBody, false, nil
	}

	// Split messages
	prefix, suffix, _ := h.SelectAnchor(messages)
	if len(prefix) == 0 {
		h.logger.Info("no messages to compress")
		return reqBody, false, nil
	}

	// Build compress request
	compressReqBody, err := h.BuildCompressRequest(prefix, model)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to build compress request: %w", err)
	}

	// Call compress API
	summary, err := h.CallCompressAPI(compressReqBody, targetURL, headers)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to call compress API: %w", err)
	}

	// Replace messages
	newReqBody, err := h.ReplaceMessages(reqBody, summary, suffix)
	if err != nil {
		return reqBody, false, fmt.Errorf("failed to replace messages: %w", err)
	}

	h.logger.Info("compression completed",
		zap.Int("original_size", len(reqBody)),
		zap.Int("compressed_size", len(newReqBody)),
		zap.Float64("compression_ratio", float64(len(newReqBody))/float64(len(reqBody))))

	return newReqBody, true, nil
}

// getDefaultCompressSystemPrompt returns the default compression system prompt
func getDefaultCompressSystemPrompt() string {
	return `You are an AI assistant specialized in summarizing conversation history.
Read the complete conversation and generate a structured summary according to the following guidelines:

1. Detailed Chronological Record
   - Capture every important turn in order, including user messages, assistant responses, and tool calls
   - Include tool commands and their important outputs (error messages, test results, exit codes); avoid pasting lengthy logs
   - Use arrows to indicate flow
   - Paraphrase when necessary but preserve intent, technical details, and results

2. Primary Request and Intent
   - Why was this session created?
   - What is the user trying to achieve?
   - What defines success?

3. Constraints and Boundaries
   - User-specified requirements (must do / must not do)
   - Technical limitations discovered
   - Codebase conventions to follow

4. Decisions Made
   - Important decisions and their rationale
   - Rejected alternatives and why

5. Approach - How did the assistant handle the problem?

6. Key Technical Work - List all key technical work completed so far

7. Questions and Clarifications
   - Questions the assistant asked and clarifications the user provided
   - Assumptions made when not explicitly clarified (brief)

8. Files and Code Sections
   - List files created, modified, or deleted
   - External references if any (PR links, Commit SHAs)

9. Error Resolution
   - Errors encountered and how they were resolved
   - Failed approaches and their reasons — avoid retrying unless new information changes conditions

10. Pending Tasks
    - Incomplete tasks with current status
    - For partial work: what IS done vs what is NOT done

11. Current Work
    - Details of the assistant's current task
    - State snapshot if relevant (branch/commit, dirty status, last test/build result)

12. Next Steps - What should the assistant do next?

13. Critical Information
    - Key information that must be passed to subsequent conversations
    - Content that doesn't fit other categories but absolutely cannot be lost
    - Special notes emphasized by the user`
}
