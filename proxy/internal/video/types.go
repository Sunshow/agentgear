package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Ark (doubao/Volcengine) video task wire types. AgentGear receives these
// because a New API channel bound to the doubao plugin prefixes its native
// routes with /doubao when the upstream is another New API gateway.

type arkCreateRequest struct {
	Model         string       `json:"model"`
	Content       []arkContent `json:"content"`
	Duration      *int         `json:"duration,omitempty"`
	Resolution    string       `json:"resolution,omitempty"`
	Ratio         string       `json:"ratio,omitempty"`
	Seed          *int64       `json:"seed,omitempty"`
	Frames        *int         `json:"frames,omitempty"`
	GenerateAudio *bool        `json:"generate_audio,omitempty"`
	Watermark     *bool        `json:"watermark,omitempty"`
	CameraFixed   *bool        `json:"camera_fixed,omitempty"`
}

type arkContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ImageURL  *arkMediaURL    `json:"image_url,omitempty"`
	VideoURL  *arkMediaURL    `json:"video_url,omitempty"`
	AudioURL  *arkMediaURL    `json:"audio_url,omitempty"`
	Role      string          `json:"role,omitempty"`
	DraftTask json.RawMessage `json:"draft_task,omitempty"`
}

type arkMediaURL struct {
	URL string `json:"url"`
}

type arkTaskResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model,omitempty"`
	Status  string          `json:"status"`
	Content *arkTaskContent `json:"content,omitempty"`
	Usage   json.RawMessage `json:"usage,omitempty"`
	Error   *arkError       `json:"error,omitempty"`
}

type arkTaskContent struct {
	VideoURL     string `json:"video_url,omitempty"`
	LastFrameURL string `json:"last_frame_url,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
}

type arkError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Orlucky task API types.

type orluckyCreateRequest struct {
	Category   string         `json:"category"`
	Operation  string         `json:"operation"`
	Model      string         `json:"model"`
	Prompt     string         `json:"prompt"`
	Parameters map[string]any `json:"parameters"`
	Media      []orluckyMedia `json:"media"`
}

type orluckyMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Role string `json:"role,omitempty"`
}

type orluckyTask struct {
	TaskID   string
	State    string
	Category string
	Data     json.RawMessage
	Error    json.RawMessage
}

type orluckyTaskData struct {
	Text   string
	Assets []orluckyAsset
	Usage  json.RawMessage
}

// decodedData tolerates the shapes orlucky has used for data: an object with
// text/assets/usage, a bare array of assets, or null. Individual malformed
// assets are skipped instead of failing the whole task, so a status-only
// response still lets the adapter answer the query.
func (t *orluckyTask) decodedData() orluckyTaskData {
	var result orluckyTaskData
	if t == nil {
		return result
	}
	raw := bytes.TrimSpace(t.Data)
	if len(raw) == 0 || string(raw) == "null" {
		return result
	}
	var object struct {
		Text   string            `json:"text"`
		Assets []json.RawMessage `json:"assets"`
		Usage  json.RawMessage   `json:"usage"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		result.Text = object.Text
		result.Usage = object.Usage
		result.Assets = t.decodeAssets(object.Assets)
		return result
	}
	var assets []json.RawMessage
	if err := json.Unmarshal(raw, &assets); err == nil {
		result.Assets = t.decodeAssets(assets)
	}
	return result
}

// decodeAssets accepts both shapes orlucky has returned: objects with
// type/url fields, and bare URL strings (the video task API currently uses
// the latter). A string's kind is inferred from its extension, falling back
// to the task category.
func (t *orluckyTask) decodeAssets(rawAssets []json.RawMessage) []orluckyAsset {
	assets := make([]orluckyAsset, 0, len(rawAssets))
	for _, raw := range rawAssets {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			text = strings.TrimSpace(text)
			if text != "" {
				assets = append(assets, orluckyAsset{Type: t.assetType(text), URL: text})
			}
			continue
		}
		var asset orluckyAsset
		if json.Unmarshal(raw, &asset) == nil {
			assets = append(assets, asset)
		}
	}
	return assets
}

func (t *orluckyTask) assetType(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, ".mp4"), strings.Contains(lower, ".webm"),
		strings.Contains(lower, ".mov"), strings.Contains(lower, ".m3u8"):
		return "VIDEO"
	case strings.Contains(lower, ".png"), strings.Contains(lower, ".jpg"),
		strings.Contains(lower, ".jpeg"), strings.Contains(lower, ".webp"), strings.Contains(lower, ".gif"):
		return "IMAGE"
	}
	if strings.EqualFold(strings.TrimSpace(t.Category), "VIDEO") {
		return "VIDEO"
	}
	if strings.EqualFold(strings.TrimSpace(t.Category), "IMAGE") {
		return "IMAGE"
	}
	return ""
}

type orluckyAsset struct {
	Type        string `json:"type"`
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	DownloadURL string `json:"downloadUrl"`
	MimeType    string `json:"mimeType"`
}

type orluckyEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	TraceID string          `json:"traceId"`
}

// taskErrorMessage extracts a human readable failure reason from the task
// object. Orlucky may answer with a string or an object in error.
func taskErrorMessage(task *orluckyTask) string {
	if task == nil || len(task.Error) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(task.Error, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var object struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	}
	if err := json.Unmarshal(task.Error, &object); err == nil {
		if message := strings.TrimSpace(object.Message); message != "" {
			return message
		}
		if code, ok := object.Code.(string); ok && strings.TrimSpace(code) != "" {
			return strings.TrimSpace(code)
		}
	}
	return strings.TrimSpace(string(task.Error))
}

// videoAssetURL returns the first video asset URL, preferring url over
// downloadUrl. Last-frame artifacts are images and are never selected.
func videoAssetURL(task *orluckyTask) string {
	if task == nil {
		return ""
	}
	for _, asset := range task.decodedData().Assets {
		kind := strings.ToLower(strings.TrimSpace(asset.Type + " " + asset.Kind))
		if !strings.Contains(kind, "video") {
			continue
		}
		if url := strings.TrimSpace(asset.URL); url != "" {
			return url
		}
		if url := strings.TrimSpace(asset.DownloadURL); url != "" {
			return url
		}
	}
	return ""
}

func (r *arkCreateRequest) validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Content) == 0 {
		return fmt.Errorf("content is required")
	}
	return nil
}
