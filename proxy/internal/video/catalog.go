package video

import "strings"

// modelProfile is the subset of an orlucky video model's documented
// capabilities the adapter needs to validate and shape a request. Values are
// taken from the public catalog (GET /models/public/catalog?categories=VIDEO).
type modelProfile struct {
	Target            string
	MinDuration       int
	MaxDuration       int
	Resolutions       []string
	DefaultResolution string
	AspectRatios      []string
	DefaultAspect     string
	MaxImages         int
	MaxVideos         int
	MaxAudios         int
}

var seedanceProfiles = map[string]modelProfile{
	"seedance-mini": {
		Target: "seedance-mini", MinDuration: 4, MaxDuration: 15,
		Resolutions: []string{"480p", "720p"}, DefaultResolution: "720p",
		AspectRatios:  []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "adaptive"},
		DefaultAspect: "16:9",
		MaxImages:     9, MaxVideos: 3, MaxAudios: 3,
	},
	"seedance-fast": {
		Target: "seedance-fast", MinDuration: 4, MaxDuration: 15,
		Resolutions: []string{"480p", "720p"}, DefaultResolution: "720p",
		AspectRatios:  []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"},
		DefaultAspect: "16:9",
		MaxImages:     9, MaxVideos: 3, MaxAudios: 3,
	},
	"seedance-2.0": {
		Target: "seedance-2.0", MinDuration: 4, MaxDuration: 15,
		Resolutions: []string{"480p", "720p", "1080p", "4k"}, DefaultResolution: "720p",
		AspectRatios:  []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"},
		DefaultAspect: "16:9",
		MaxImages:     9, MaxVideos: 3, MaxAudios: 3,
	},
	"seedance-2.5": {
		Target: "seedance-2.5", MinDuration: 4, MaxDuration: 30,
		Resolutions: []string{"480p", "720p", "1080p"}, DefaultResolution: "720p",
		AspectRatios:  []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "adaptive"},
		DefaultAspect: "16:9",
		MaxImages:     30, MaxVideos: 10, MaxAudios: 10,
	},
}

// defaultModelMap mirrors the doubao plugin's Seedance model list. The
// adapter only claims the 2.x family; 1.0 models stay unmapped so a caller
// gets a clear error instead of a silently wrong orlucky model.
var defaultModelMap = map[string]string{
	"doubao-seedance-2-0-260128":      "seedance-2.0",
	"doubao-seedance-2-0-fast-260128": "seedance-fast",
	"doubao-seedance-2-0-mini-260615": "seedance-mini",
	"doubao-seedance-2-5-260628":      "seedance-2.5",
}

func resolveProfile(target string) (modelProfile, bool) {
	profile, ok := seedanceProfiles[target]
	return profile, ok
}

// mapModel resolves the client model to an orlucky model. Explicit
// configuration wins; otherwise the Seedance 2.x naming pattern is used so a
// newly dated Ark model id still maps to the right family.
func mapModel(configured map[string]string, model string) (target string, heuristic bool, ok bool) {
	if configured != nil {
		if target, exists := configured[model]; exists && strings.TrimSpace(target) != "" {
			return strings.TrimSpace(target), false, true
		}
	}
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(lower, "seedance-2-5"):
		return "seedance-2.5", true, true
	case strings.Contains(lower, "seedance-2-0-fast"):
		return "seedance-fast", true, true
	case strings.Contains(lower, "seedance-2-0-mini"):
		return "seedance-mini", true, true
	case strings.Contains(lower, "seedance-2-0"):
		return "seedance-2.0", true, true
	}
	return "", false, false
}
