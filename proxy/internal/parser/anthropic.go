package parser

import (
	"encoding/json"
	"regexp"
	"strings"
)

type AnthropicParsedData struct {
	Model           string            `json:"model,omitempty"`
	MaxTokens       int               `json:"max_tokens,omitempty"`
	SystemPrompts   []SystemPrompt    `json:"system_prompts,omitempty"`
	SystemReminders []SystemReminder  `json:"system_reminders,omitempty"`
	Tools           []ToolDefinition  `json:"tools,omitempty"`
}

type SystemPrompt struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	CacheControl string `json:"cache_control,omitempty"`
}

type SystemReminder struct {
	RawText    string            `json:"raw_text"`
	ParsedInfo map[string]string `json:"parsed_info,omitempty"`
}

type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type ParsedData struct {
	Protocol  string               `json:"protocol,omitempty"`
	Anthropic *AnthropicParsedData `json:"anthropic,omitempty"`
}

var systemReminderRegex = regexp.MustCompile(`<system-reminder>([\s\S]*?)</system-reminder>`)

func ParseAnthropicRequest(body []byte) *ParsedData {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}

	data := &AnthropicParsedData{}

	if model, ok := req["model"].(string); ok {
		data.Model = model
	}

	if maxTokens, ok := req["max_tokens"].(float64); ok {
		data.MaxTokens = int(maxTokens)
	}

	data.SystemPrompts = parseSystemPrompts(req)
	data.Tools = parseTools(req)
	data.SystemReminders = parseSystemReminders(req)

	return &ParsedData{
		Protocol:  "anthropic",
		Anthropic: data,
	}
}

func parseSystemPrompts(req map[string]interface{}) []SystemPrompt {
	var prompts []SystemPrompt

	system := req["system"]
	if system == nil {
		return prompts
	}

	switch v := system.(type) {
	case string:
		prompts = append(prompts, SystemPrompt{
			Type: "text",
			Text: v,
		})
	case []interface{}:
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				prompt := SystemPrompt{}
				if t, ok := itemMap["type"].(string); ok {
					prompt.Type = t
				}
				if text, ok := itemMap["text"].(string); ok {
					prompt.Text = text
				}
				if cc, ok := itemMap["cache_control"].(map[string]interface{}); ok {
					if ccType, ok := cc["type"].(string); ok {
						prompt.CacheControl = ccType
					}
				}
				prompts = append(prompts, prompt)
			}
		}
	}

	return prompts
}

func parseTools(req map[string]interface{}) []ToolDefinition {
	var tools []ToolDefinition

	toolsRaw, ok := req["tools"].([]interface{})
	if !ok {
		return tools
	}

	for _, item := range toolsRaw {
		toolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		tool := ToolDefinition{}
		if name, ok := toolMap["name"].(string); ok {
			tool.Name = name
		}
		if desc, ok := toolMap["description"].(string); ok {
			tool.Description = desc
		}
		if schema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
			tool.InputSchema = schema
		}

		tools = append(tools, tool)
	}

	return tools
}

func parseSystemReminders(req map[string]interface{}) []SystemReminder {
	var reminders []SystemReminder

	messages, ok := req["messages"].([]interface{})
	if !ok {
		return reminders
	}

	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		content := msgMap["content"]
		if content == nil {
			continue
		}

		switch c := content.(type) {
		case string:
			reminders = append(reminders, extractRemindersFromText(c)...)
		case []interface{}:
			for _, item := range c {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if text, ok := itemMap["text"].(string); ok {
						reminders = append(reminders, extractRemindersFromText(text)...)
					}
				}
			}
		}
	}

	return reminders
}

func extractRemindersFromText(text string) []SystemReminder {
	var reminders []SystemReminder

	matches := systemReminderRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			rawText := strings.TrimSpace(match[1])
			reminder := SystemReminder{
				RawText:    rawText,
				ParsedInfo: extractReminderInfo(rawText),
			}
			reminders = append(reminders, reminder)
		}
	}

	return reminders
}

func extractReminderInfo(text string) map[string]string {
	info := make(map[string]string)

	if strings.Contains(text, "User system info") {
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "User system info") {
				info["system_info"] = line
			}
			if strings.HasPrefix(line, "Model:") {
				info["model"] = strings.TrimPrefix(line, "Model:")
				info["model"] = strings.TrimSpace(info["model"])
			}
			if strings.HasPrefix(line, "Today's date:") {
				info["date"] = strings.TrimPrefix(line, "Today's date:")
				info["date"] = strings.TrimSpace(info["date"])
			}
		}
	}

	if strings.Contains(text, "Spec mode is active") {
		info["spec_mode"] = "active"
	}

	if strings.Contains(text, "TodoWrite was not called") {
		info["todo_reminder"] = "true"
	}

	return info
}
