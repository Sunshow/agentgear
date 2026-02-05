package api

import (
	"time"

	"github.com/sunshow/agentgear/proxy/internal/memory"
)

type ConnectionDTO struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Sequence   int       `json:"sequence"`
	Tags       []string  `json:"tags"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     string    `json:"status"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	DurationMs int64     `json:"duration_ms"`

	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	RequestBody    string            `json:"request_body,omitempty"`
	RequestTools   []ToolInfoDTO     `json:"request_tools,omitempty"`

	ResponseStatus  int               `json:"response_status,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	ResponseTools   []ToolCallInfoDTO `json:"response_tools,omitempty"`

	TransformedRequest  bool     `json:"transformed_request"`
	TransformedResponse bool     `json:"transformed_response"`
	AppliedTransformers []string `json:"applied_transformers,omitempty"`

	ParsedData *ParsedDataDTO `json:"parsed_data,omitempty"`
}

type ParsedDataDTO struct {
	Protocol  string                 `json:"protocol,omitempty"`
	Anthropic *AnthropicParsedDataDTO `json:"anthropic,omitempty"`
}

type AnthropicParsedDataDTO struct {
	Model           string              `json:"model,omitempty"`
	MaxTokens       int                 `json:"max_tokens,omitempty"`
	SystemPrompts   []SystemPromptDTO   `json:"system_prompts,omitempty"`
	SystemReminders []SystemReminderDTO `json:"system_reminders,omitempty"`
	Tools           []ToolDefinitionDTO `json:"tools,omitempty"`
}

type SystemPromptDTO struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	CacheControl string `json:"cache_control,omitempty"`
}

type SystemReminderDTO struct {
	RawText    string            `json:"raw_text"`
	ParsedInfo map[string]string `json:"parsed_info,omitempty"`
}

type ToolDefinitionDTO struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type ToolInfoDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ToolCallInfoDTO struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input,omitempty"`
}

func ToConnectionDTO(conn *memory.ConnectionInfo) ConnectionDTO {
	dto := ConnectionDTO{
		ID:                  conn.ID,
		SessionID:           conn.SessionID,
		Sequence:            conn.Sequence,
		Tags:                conn.Tags,
		Method:              conn.Method,
		Path:                conn.Path,
		Status:              conn.Status,
		StartTime:           conn.StartTime,
		EndTime:             conn.EndTime,
		DurationMs:          conn.DurationMs,
		RequestHeaders:      conn.RequestHeaders,
		RequestBody:         string(conn.RequestBody),
		ResponseStatus:      conn.ResponseStatus,
		ResponseHeaders:     conn.ResponseHeaders,
		ResponseBody:        string(conn.ResponseBody),
		TransformedRequest:  conn.TransformedRequest,
		TransformedResponse: conn.TransformedResponse,
		AppliedTransformers: conn.AppliedTransformers,
	}

	if len(conn.RequestTools) > 0 {
		dto.RequestTools = make([]ToolInfoDTO, len(conn.RequestTools))
		for i, t := range conn.RequestTools {
			dto.RequestTools[i] = ToolInfoDTO{
				Name:        t.Name,
				Description: t.Description,
			}
		}
	}

	if len(conn.ResponseTools) > 0 {
		dto.ResponseTools = make([]ToolCallInfoDTO, len(conn.ResponseTools))
		for i, t := range conn.ResponseTools {
			dto.ResponseTools[i] = ToolCallInfoDTO{
				ID:    t.ID,
				Name:  t.Name,
				Input: t.Input,
			}
		}
	}

	if conn.ParsedData != nil {
		dto.ParsedData = toParsedDataDTO(conn.ParsedData)
	}

	return dto
}

func toParsedDataDTO(pd *memory.ParsedData) *ParsedDataDTO {
	if pd == nil {
		return nil
	}

	dto := &ParsedDataDTO{
		Protocol: pd.Protocol,
	}

	if pd.Anthropic != nil {
		dto.Anthropic = &AnthropicParsedDataDTO{
			Model:     pd.Anthropic.Model,
			MaxTokens: pd.Anthropic.MaxTokens,
		}

		if len(pd.Anthropic.SystemPrompts) > 0 {
			dto.Anthropic.SystemPrompts = make([]SystemPromptDTO, len(pd.Anthropic.SystemPrompts))
			for i, sp := range pd.Anthropic.SystemPrompts {
				dto.Anthropic.SystemPrompts[i] = SystemPromptDTO{
					Type:         sp.Type,
					Text:         sp.Text,
					CacheControl: sp.CacheControl,
				}
			}
		}

		if len(pd.Anthropic.SystemReminders) > 0 {
			dto.Anthropic.SystemReminders = make([]SystemReminderDTO, len(pd.Anthropic.SystemReminders))
			for i, sr := range pd.Anthropic.SystemReminders {
				dto.Anthropic.SystemReminders[i] = SystemReminderDTO{
					RawText:    sr.RawText,
					ParsedInfo: sr.ParsedInfo,
				}
			}
		}

		if len(pd.Anthropic.Tools) > 0 {
			dto.Anthropic.Tools = make([]ToolDefinitionDTO, len(pd.Anthropic.Tools))
			for i, t := range pd.Anthropic.Tools {
				dto.Anthropic.Tools[i] = ToolDefinitionDTO{
					Name:        t.Name,
					Description: t.Description,
					InputSchema: t.InputSchema,
				}
			}
		}
	}

	return dto
}

func ToConnectionDTOList(conns []*memory.ConnectionInfo) []ConnectionDTO {
	dtos := make([]ConnectionDTO, len(conns))
	for i, conn := range conns {
		dtos[i] = ToConnectionDTO(conn)
	}
	return dtos
}
