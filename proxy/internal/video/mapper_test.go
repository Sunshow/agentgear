package video

import (
	"encoding/json"
	"strings"
	"testing"
)

func intPointer(value int) *int { return &value }

func TestMapCreateTextToVideo(t *testing.T) {
	profile, ok := resolveProfile("seedance-mini")
	if !ok {
		t.Fatal("seedance-mini profile missing")
	}
	request := &arkCreateRequest{
		Model:    "doubao-seedance-2-0-mini-260615",
		Content:  []arkContent{{Type: "text", Text: "  a cat  "}},
		Duration: intPointer(5),
	}

	body, meta, err := mapCreate(request, "seedance-mini", profile)
	if err != nil {
		t.Fatalf("mapCreate: %v", err)
	}
	if body.Category != "VIDEO" || body.Operation != "TEXT_TO_VIDEO" {
		t.Fatalf("unexpected category/operation: %s/%s", body.Category, body.Operation)
	}
	if body.Model != "seedance-mini" {
		t.Fatalf("unexpected model: %s", body.Model)
	}
	if body.Prompt != "a cat" {
		t.Fatalf("unexpected prompt: %q", body.Prompt)
	}
	if body.Parameters["duration"] != 5 || body.Parameters["resolution"] != "720p" || body.Parameters["aspectRatio"] != "16:9" {
		t.Fatalf("unexpected parameters: %#v", body.Parameters)
	}
	if len(body.Media) != 0 {
		t.Fatalf("unexpected media: %#v", body.Media)
	}
	if meta.Resolution != "720p" || meta.Duration != 5 || meta.ClientModel != request.Model {
		t.Fatalf("unexpected meta: %#v", meta)
	}
}

func TestMapCreateImageToVideo(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	request := &arkCreateRequest{
		Model: "doubao-seedance-2-0-mini-260615",
		Content: []arkContent{
			{Type: "image_url", ImageURL: &arkMediaURL{URL: "https://example.com/a.png"}, Role: "first_frame"},
			{Type: "text", Text: "move"},
		},
		Duration:   intPointer(4),
		Resolution: "1280x720",
		Ratio:      "9:16",
	}

	body, _, err := mapCreate(request, "seedance-mini", profile)
	if err != nil {
		t.Fatalf("mapCreate: %v", err)
	}
	if body.Operation != "IMAGE_TO_VIDEO" {
		t.Fatalf("unexpected operation: %s", body.Operation)
	}
	if len(body.Media) != 1 || body.Media[0].Type != "IMAGE" || body.Media[0].Role != "first_frame" {
		t.Fatalf("unexpected media: %#v", body.Media)
	}
	if body.Parameters["resolution"] != "720p" || body.Parameters["aspectRatio"] != "9:16" {
		t.Fatalf("unexpected parameters: %#v", body.Parameters)
	}
}

func TestMapCreateVideoToVideo(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	request := &arkCreateRequest{
		Model:   "doubao-seedance-2-0-mini-260615",
		Content: []arkContent{{Type: "video_url", VideoURL: &arkMediaURL{URL: "https://example.com/a.mp4"}}},
	}

	body, _, err := mapCreate(request, "seedance-mini", profile)
	if err != nil {
		t.Fatalf("mapCreate: %v", err)
	}
	if body.Operation != "VIDEO_TO_VIDEO" {
		t.Fatalf("unexpected operation: %s", body.Operation)
	}
	if len(body.Media) != 1 || body.Media[0].Type != "VIDEO" {
		t.Fatalf("unexpected media: %#v", body.Media)
	}
}

func TestMapCreateDurationValidation(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	for _, value := range []int{-1, 0, 3, 16} {
		request := &arkCreateRequest{
			Model:    "doubao-seedance-2-0-mini-260615",
			Content:  []arkContent{{Type: "text", Text: "x"}},
			Duration: intPointer(value),
		}
		if _, _, err := mapCreate(request, "seedance-mini", profile); err == nil {
			t.Fatalf("duration %d should fail", value)
		}
	}
}

func TestMapCreateFramesToDuration(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	frames := 120
	request := &arkCreateRequest{
		Model:   "doubao-seedance-2-0-mini-260615",
		Content: []arkContent{{Type: "text", Text: "x"}},
		Frames:  &frames,
	}
	body, meta, err := mapCreate(request, "seedance-mini", profile)
	if err != nil {
		t.Fatalf("mapCreate: %v", err)
	}
	if meta.Duration != 5 || body.Parameters["duration"] != 5 {
		t.Fatalf("unexpected duration: %d / %#v", meta.Duration, body.Parameters)
	}
}

func TestMapCreateRejectsInvalidMedia(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	cases := []arkCreateRequest{
		{Model: "m", Content: []arkContent{{Type: "image_url"}}},
		{Model: "m", Content: []arkContent{{Type: "image_url", ImageURL: &arkMediaURL{URL: "file:///a.png"}}}},
		{Model: "m", Content: []arkContent{{Type: "draft_task", DraftTask: []byte(`{}`)}}},
		{Model: "m", Content: []arkContent{{Type: "unknown"}}},
	}
	for index, request := range cases {
		if _, _, err := mapCreate(&request, "seedance-mini", profile); err == nil {
			t.Fatalf("case %d should fail", index)
		}
	}
}

func TestMapCreateMediaLimit(t *testing.T) {
	profile, _ := resolveProfile("seedance-mini")
	content := make([]arkContent, 0, profile.MaxImages+1)
	for index := 0; index < profile.MaxImages+1; index++ {
		content = append(content, arkContent{
			Type:     "image_url",
			ImageURL: &arkMediaURL{URL: "https://example.com/a.png"},
		})
	}
	request := &arkCreateRequest{Model: "m", Content: content}
	if _, _, err := mapCreate(request, "seedance-mini", profile); err == nil || !strings.Contains(err.Error(), "too many image") {
		t.Fatalf("expected media limit error, got %v", err)
	}
}

func TestNormalizeResolution(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"720p":      "720p",
		"1080p":     "1080p",
		"4k":        "4k",
		"1280x720":  "720p",
		"1280*720":  "720p",
		"1920x1080": "1080p",
		"3840x2160": "4k",
		"854x480":   "480p",
	}
	for input, expected := range cases {
		actual, err := normalizeResolution(input)
		if err != nil {
			t.Fatalf("normalizeResolution(%q): %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalizeResolution(%q) = %q, want %q", input, actual, expected)
		}
	}
	if _, err := normalizeResolution("not-a-size"); err == nil {
		t.Fatal("expected error for invalid resolution")
	}
}

func TestResolveResolutionTierValidation(t *testing.T) {
	mini, _ := resolveProfile("seedance-mini")
	if _, err := resolveResolution("1080p", mini); err == nil {
		t.Fatal("seedance-mini should reject 1080p")
	}
	full, _ := resolveProfile("seedance-2.0")
	if resolution, err := resolveResolution("1920x1080", full); err != nil || resolution != "1080p" {
		t.Fatalf("seedance-2.0 1080p: %q %v", resolution, err)
	}
}

func TestResolveAspectValidation(t *testing.T) {
	mini, _ := resolveProfile("seedance-mini")
	if _, err := resolveAspect("adaptive", mini); err != nil {
		t.Fatalf("seedance-mini should accept adaptive: %v", err)
	}
	full, _ := resolveProfile("seedance-2.0")
	if _, err := resolveAspect("adaptive", full); err == nil {
		t.Fatal("seedance-2.0 should reject adaptive")
	}
	if _, err := resolveAspect("5:4", mini); err == nil {
		t.Fatal("unexpected ratio should fail")
	}
}

func TestMapOrluckyState(t *testing.T) {
	cases := []struct {
		state    string
		status   string
		terminal bool
		success  bool
	}{
		{"PENDING", "queued", false, false},
		{"RUNNING", "running", false, false},
		{"SUCCEEDED", "succeeded", true, true},
		{"COMPLETED", "succeeded", true, true},
		{"FAILED", "failed", true, false},
		{"EXPIRED", "expired", true, false},
		{"CANCELLED", "cancelled", true, false},
		{"", "queued", false, false},
	}
	for _, testCase := range cases {
		status, terminal, success := mapOrluckyState(testCase.state)
		if status != testCase.status || terminal != testCase.terminal || success != testCase.success {
			t.Fatalf("mapOrluckyState(%q) = %s/%v/%v", testCase.state, status, terminal, success)
		}
	}
}

func TestVideoAssetURL(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "object assets",
			data: `{"assets":[
				{"type":"IMAGE","url":"https://example.com/frame.png"},
				{"type":"VIDEO","downloadUrl":"https://example.com/video.mp4"}
			]}`,
			want: "https://example.com/video.mp4",
		},
		{
			name: "bare string assets",
			data: `{"text":"","assets":[
				"https://example.com/frame.png",
				"https://example.com/video.mp4?Expires=1"
			]}`,
			want: "https://example.com/video.mp4?Expires=1",
		},
		{
			name: "bare assets array",
			data: `[{"type":"VIDEO","url":"https://example.com/a.mp4"}]`,
			want: "https://example.com/a.mp4",
		},
		{
			name: "malformed asset skipped",
			data: `{"assets":[{"url":123},{"type":"VIDEO","url":"https://example.com/b.mp4"}]}`,
			want: "https://example.com/b.mp4",
		},
		{
			name: "null data",
			data: `null`,
			want: "",
		},
	}
	for _, testCase := range cases {
		task := &orluckyTask{Category: "VIDEO", Data: json.RawMessage(testCase.data)}
		if url := videoAssetURL(task); url != testCase.want {
			t.Fatalf("%s: asset url = %q, want %q", testCase.name, url, testCase.want)
		}
	}
	if url := videoAssetURL(nil); url != "" {
		t.Fatalf("unexpected asset url for nil task: %s", url)
	}
}

func TestParseOrluckyTask(t *testing.T) {
	raw := []byte(`{
		"success": true,
		"traceId": "outer",
		"taskId": "bce5981f-4c21-499b-906e-7aa0a4903d09",
		"state": "SUCCEEDED",
		"category": "VIDEO",
		"data": {
			"text": "",
			"assets": ["https://example.com/out.mp4?Expires=1"]
		},
		"attempts": 1,
		"error": null
	}`)
	task, err := parseOrluckyTask(raw)
	if err != nil {
		t.Fatalf("parseOrluckyTask: %v", err)
	}
	if task.TaskID != "bce5981f-4c21-499b-906e-7aa0a4903d09" || task.State != "SUCCEEDED" || task.Category != "VIDEO" {
		t.Fatalf("unexpected task: %#v", task)
	}
	if url := videoAssetURL(task); url != "https://example.com/out.mp4?Expires=1" {
		t.Fatalf("asset url = %q", url)
	}
}

func TestMapModel(t *testing.T) {
	target, heuristic, ok := mapModel(nil, "doubao-seedance-2-0-mini-260615")
	if !ok || !heuristic || target != "seedance-mini" {
		t.Fatalf("default map: %s %v %v", target, heuristic, ok)
	}
	target, heuristic, ok = mapModel(defaultModelMap, "doubao-seedance-2-0-260128")
	if !ok || heuristic || target != "seedance-2.0" {
		t.Fatalf("configured map: %s %v %v", target, heuristic, ok)
	}
	target, heuristic, ok = mapModel(nil, "doubao-seedance-2-5-999999")
	if !ok || !heuristic || target != "seedance-2.5" {
		t.Fatalf("heuristic map: %s %v %v", target, heuristic, ok)
	}
	if _, _, ok := mapModel(nil, "doubao-seedance-1-0-pro-250528"); ok {
		t.Fatal("1.0 models should stay unmapped")
	}
}

func TestTaskErrorMessage(t *testing.T) {
	if message := taskErrorMessage(&orluckyTask{Error: []byte(`"boom"`)}); message != "boom" {
		t.Fatalf("string error: %q", message)
	}
	if message := taskErrorMessage(&orluckyTask{Error: []byte(`{"message":"bad","code":"x"}`)}); message != "bad" {
		t.Fatalf("object error: %q", message)
	}
	if message := taskErrorMessage(nil); message != "" {
		t.Fatalf("nil error: %q", message)
	}
}
