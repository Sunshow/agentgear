package video

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type taskMeta struct {
	ClientModel string
	TargetModel string
	Prompt      string
	Duration    int
	Resolution  string
	Aspect      string
}

// mapCreate converts an Ark/doubao task creation body into an orlucky task
// creation body. It validates duration, resolution and aspect ratio against
// the target model's documented options instead of silently clamping, so a
// caller never pays for a different video than it asked for.
func mapCreate(req *arkCreateRequest, target string, profile modelProfile) (*orluckyCreateRequest, taskMeta, error) {
	if err := req.validate(); err != nil {
		return nil, taskMeta{}, err
	}

	var (
		prompt   []string
		media    []orluckyMedia
		hasImage bool
		hasVideo bool
	)
	for index, item := range req.Content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "text":
			if text := strings.TrimSpace(item.Text); text != "" {
				prompt = append(prompt, text)
			}
		case "image_url":
			url, err := contentURL(item.ImageURL)
			if err != nil {
				return nil, taskMeta{}, fmt.Errorf("content[%d]: %w", index, err)
			}
			hasImage = true
			media = append(media, orluckyMedia{Type: "IMAGE", URL: url, Role: strings.TrimSpace(item.Role)})
		case "video_url":
			url, err := contentURL(item.VideoURL)
			if err != nil {
				return nil, taskMeta{}, fmt.Errorf("content[%d]: %w", index, err)
			}
			hasVideo = true
			media = append(media, orluckyMedia{Type: "VIDEO", URL: url, Role: strings.TrimSpace(item.Role)})
		case "audio_url":
			url, err := contentURL(item.AudioURL)
			if err != nil {
				return nil, taskMeta{}, fmt.Errorf("content[%d]: %w", index, err)
			}
			media = append(media, orluckyMedia{Type: "AUDIO", URL: url, Role: strings.TrimSpace(item.Role)})
		case "draft_task":
			return nil, taskMeta{}, fmt.Errorf("content type draft_task is not supported")
		default:
			return nil, taskMeta{}, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	if err := validateMediaCounts(media, profile); err != nil {
		return nil, taskMeta{}, err
	}

	duration, err := resolveDuration(req, profile)
	if err != nil {
		return nil, taskMeta{}, err
	}
	resolution, err := resolveResolution(req.Resolution, profile)
	if err != nil {
		return nil, taskMeta{}, err
	}
	aspect, err := resolveAspect(req.Ratio, profile)
	if err != nil {
		return nil, taskMeta{}, err
	}

	parameters := map[string]any{
		"resolution":  resolution,
		"aspectRatio": aspect,
	}
	if duration > 0 {
		parameters["duration"] = duration
	}

	operation := "TEXT_TO_VIDEO"
	if hasVideo {
		operation = "VIDEO_TO_VIDEO"
	} else if hasImage {
		operation = "IMAGE_TO_VIDEO"
	}

	body := &orluckyCreateRequest{
		Category:   "VIDEO",
		Operation:  operation,
		Model:      target,
		Prompt:     strings.Join(prompt, "\n"),
		Parameters: parameters,
		Media:      media,
	}
	if body.Media == nil {
		body.Media = []orluckyMedia{}
	}
	meta := taskMeta{
		ClientModel: req.Model,
		TargetModel: target,
		Prompt:      body.Prompt,
		Duration:    duration,
		Resolution:  resolution,
		Aspect:      aspect,
	}
	return body, meta, nil
}

func contentURL(value *arkMediaURL) (string, error) {
	if value == nil {
		return "", fmt.Errorf("media url is missing")
	}
	url := strings.TrimSpace(value.URL)
	if url == "" {
		return "", fmt.Errorf("media url is empty")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("media url must be http(s): %s", url)
	}
	return url, nil
}

func validateMediaCounts(media []orluckyMedia, profile modelProfile) error {
	counts := map[string]int{}
	for _, item := range media {
		counts[item.Type]++
	}
	if counts["IMAGE"] > profile.MaxImages {
		return fmt.Errorf("too many image references: %d > %d", counts["IMAGE"], profile.MaxImages)
	}
	if counts["VIDEO"] > profile.MaxVideos {
		return fmt.Errorf("too many video references: %d > %d", counts["VIDEO"], profile.MaxVideos)
	}
	if counts["AUDIO"] > profile.MaxAudios {
		return fmt.Errorf("too many audio references: %d > %d", counts["AUDIO"], profile.MaxAudios)
	}
	return nil
}

func resolveDuration(req *arkCreateRequest, profile modelProfile) (int, error) {
	duration := 0
	switch {
	case req.Duration != nil:
		duration = *req.Duration
		if duration == -1 {
			return 0, fmt.Errorf("duration -1 is not supported by the upstream model")
		}
		if duration <= 0 {
			return 0, fmt.Errorf("duration must be a positive number of seconds")
		}
	case req.Frames != nil && *req.Frames > 0:
		duration = int(math.Round(float64(*req.Frames) / 24))
	}
	if duration == 0 {
		return 0, nil
	}
	if duration < profile.MinDuration || duration > profile.MaxDuration {
		return 0, fmt.Errorf("duration must be between %d and %d seconds", profile.MinDuration, profile.MaxDuration)
	}
	return duration, nil
}

func resolveResolution(raw string, profile modelProfile) (string, error) {
	resolution, err := normalizeResolution(raw)
	if err != nil {
		return "", err
	}
	if resolution == "" {
		resolution = profile.DefaultResolution
	}
	for _, allowed := range profile.Resolutions {
		if allowed == resolution {
			return resolution, nil
		}
	}
	return "", fmt.Errorf("resolution must be one of %s", strings.Join(profile.Resolutions, ", "))
}

func normalizeResolution(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}
	switch value {
	case "480p", "720p", "1080p", "4k":
		return value, nil
	}
	value = strings.ReplaceAll(value, "*", "x")
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return "", fmt.Errorf("resolution %q is not a tier or WxH size", raw)
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return "", fmt.Errorf("resolution %q has an invalid width", raw)
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return "", fmt.Errorf("resolution %q has an invalid height", raw)
	}
	switch {
	case height >= 2000:
		return "4k", nil
	case height >= 900:
		return "1080p", nil
	case height >= 600:
		return "720p", nil
	default:
		return "480p", nil
	}
}

func resolveAspect(raw string, profile modelProfile) (string, error) {
	aspect := strings.TrimSpace(raw)
	if aspect == "" {
		return profile.DefaultAspect, nil
	}
	for _, allowed := range profile.AspectRatios {
		if allowed == aspect {
			return aspect, nil
		}
	}
	return "", fmt.Errorf("ratio must be one of %s", strings.Join(profile.AspectRatios, ", "))
}

// mapOrluckyState converts the upstream task state into the Ark task status
// vocabulary the doubao plugin understands.
func mapOrluckyState(state string) (status string, terminal bool, success bool) {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PENDING":
		return "queued", false, false
	case "RUNNING":
		return "running", false, false
	case "SUCCEEDED", "SUCCESS", "COMPLETED":
		return "succeeded", true, true
	case "FAILED", "CLOSED":
		return "failed", true, false
	case "EXPIRED":
		return "expired", true, false
	case "CANCELLED", "CANCELED":
		return "cancelled", true, false
	default:
		return "queued", false, false
	}
}
