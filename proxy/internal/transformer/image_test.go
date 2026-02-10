package transformer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEstimateRequestSizeExcludingImages(t *testing.T) {
	tests := []struct {
		name                string
		reqBody             string
		imageTokenEstimate  int
		tokenEstimateRatio  float64
		wantImageCount      int
		wantEffectiveLess   bool // 是否期望 effectiveSize < len(reqBody)
		wantEffectiveEquals bool // 是否期望 effectiveSize == len(reqBody)
	}{
		{
			name: "no images - string content",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			imageTokenEstimate:  1600,
			tokenEstimateRatio:  3.5,
			wantImageCount:      0,
			wantEffectiveEquals: true,
		},
		{
			name: "no images - array content without images",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{"role": "user", "content": [{"type": "text", "text": "Hello"}]}
				]
			}`,
			imageTokenEstimate:  1600,
			tokenEstimateRatio:  3.5,
			wantImageCount:      0,
			wantEffectiveEquals: true,
		},
		{
			name: "single image",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{
						"role": "user",
						"content": [
							{"type": "text", "text": "What's in this image?"},
							{
								"type": "image",
								"source": {
									"type": "base64",
									"media_type": "image/png",
									"data": "` + strings.Repeat("A", 100000) + `"
								}
							}
						]
					}
				]
			}`,
			imageTokenEstimate: 1600,
			tokenEstimateRatio: 3.5,
			wantImageCount:     1,
			wantEffectiveLess:  true,
		},
		{
			name: "multiple images",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{
						"role": "user",
						"content": [
							{"type": "text", "text": "Compare these images"},
							{
								"type": "image",
								"source": {
									"type": "base64",
									"media_type": "image/png",
									"data": "` + strings.Repeat("A", 50000) + `"
								}
							},
							{
								"type": "image",
								"source": {
									"type": "base64",
									"media_type": "image/jpeg",
									"data": "` + strings.Repeat("B", 60000) + `"
								}
							}
						]
					}
				]
			}`,
			imageTokenEstimate: 1600,
			tokenEstimateRatio: 3.5,
			wantImageCount:     2,
			wantEffectiveLess:  true,
		},
		{
			name:                "invalid json",
			reqBody:             `{invalid json`,
			imageTokenEstimate:  1600,
			tokenEstimateRatio:  3.5,
			wantImageCount:      0,
			wantEffectiveEquals: true,
		},
		{
			name: "no messages field",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022"
			}`,
			imageTokenEstimate:  1600,
			tokenEstimateRatio:  3.5,
			wantImageCount:      0,
			wantEffectiveEquals: true,
		},
		{
			name: "default parameters",
			reqBody: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "image",
								"source": {
									"type": "base64",
									"data": "` + strings.Repeat("X", 80000) + `"
								}
							}
						]
					}
				]
			}`,
			imageTokenEstimate: 0, // 测试默认值
			tokenEstimateRatio: 0, // 测试默认值
			wantImageCount:     1,
			wantEffectiveLess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := []byte(tt.reqBody)
			effectiveSize, imageCount := EstimateRequestSizeExcludingImages(
				reqBody, tt.imageTokenEstimate, tt.tokenEstimateRatio)

			if imageCount != tt.wantImageCount {
				t.Errorf("imageCount = %d, want %d", imageCount, tt.wantImageCount)
			}

			if tt.wantEffectiveEquals && effectiveSize != len(reqBody) {
				t.Errorf("effectiveSize = %d, want %d (original size)", effectiveSize, len(reqBody))
			}

			if tt.wantEffectiveLess && effectiveSize >= len(reqBody) {
				t.Errorf("effectiveSize = %d should be less than original size %d", effectiveSize, len(reqBody))
			}

			// 验证有图片时，effectiveSize 应该显著小于原始大小
			if imageCount > 0 && tt.wantEffectiveLess {
				reduction := len(reqBody) - effectiveSize
				if reduction < 10000 { // 至少减少 10KB
					t.Errorf("effectiveSize reduction too small: %d bytes, original=%d effective=%d",
						reduction, len(reqBody), effectiveSize)
				}
			}
		})
	}
}

func TestEstimateRequestSizeExcludingImages_RealWorldScenario(t *testing.T) {
	// 模拟真实场景：一张 500KB 的图片
	largeBase64 := strings.Repeat("A", 500000)
	reqBody := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Analyze this screenshot",
					},
					map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": "image/png",
							"data":       largeBase64,
						},
					},
				},
			},
		},
	}

	reqBodyBytes, _ := json.Marshal(reqBody)
	originalSize := len(reqBodyBytes)

	effectiveSize, imageCount := EstimateRequestSizeExcludingImages(
		reqBodyBytes, 1600, 3.5)

	if imageCount != 1 {
		t.Errorf("imageCount = %d, want 1", imageCount)
	}

	// 原始大小应该 > 500KB
	if originalSize < 500000 {
		t.Errorf("originalSize = %d, expected > 500000", originalSize)
	}

	// 等效大小应该远小于原始大小（图片被替换为 1600 * 3.5 = 5600 字节）
	expectedEffective := originalSize - 500000 + 5600
	tolerance := 1000 // 允许 1KB 误差
	if effectiveSize < expectedEffective-tolerance || effectiveSize > expectedEffective+tolerance {
		t.Errorf("effectiveSize = %d, expected around %d (±%d)", effectiveSize, expectedEffective, tolerance)
	}

	t.Logf("Real-world test: original=%d effective=%d reduction=%d%%",
		originalSize, effectiveSize, (originalSize-effectiveSize)*100/originalSize)
}
