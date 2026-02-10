package transformer

import "encoding/json"

const DefaultImageTokenEstimate = 1600 // Anthropic 中等尺寸图片约 1600 tokens

// EstimateRequestSizeExcludingImages 解析请求体，将图片 base64 数据
// 替换为固定 token 估算值对应的等效字节数，返回调整后的"等效大小"和图片数量。
// 如果解析失败，回退返回原始字节数。
func EstimateRequestSizeExcludingImages(reqBody []byte, imageTokenEstimate int, tokenEstimateRatio float64) (effectiveSize int, imageCount int) {
	if imageTokenEstimate <= 0 {
		imageTokenEstimate = DefaultImageTokenEstimate
	}
	if tokenEstimateRatio <= 0 {
		tokenEstimateRatio = 3.5
	}

	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return len(reqBody), 0
	}

	messagesRaw, ok := req["messages"]
	if !ok {
		return len(reqBody), 0
	}

	messages, ok := messagesRaw.([]interface{})
	if !ok {
		return len(reqBody), 0
	}

	// 计算所有图片 base64 data 字段的总字节数
	totalImageDataBytes := 0
	count := 0
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		contentRaw, ok := msgMap["content"]
		if !ok {
			continue
		}
		blocks, ok := contentRaw.([]interface{})
		if !ok {
			continue // content 是 string，无图片
		}
		for _, block := range blocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			if blockMap["type"] == "image" {
				// Anthropic 格式: { "type": "image", "source": { "type": "base64", "data": "..." } }
				if source, ok := blockMap["source"].(map[string]interface{}); ok {
					if data, ok := source["data"].(string); ok {
						totalImageDataBytes += len(data)
						count++
					}
				}
			}
		}
	}

	if count == 0 {
		return len(reqBody), 0
	}

	// 等效大小 = 原始大小 - 图片base64字节数 + 图片数量 * 固定token估算 * 字节比率
	imageEquivalentBytes := int(float64(imageTokenEstimate) * tokenEstimateRatio) * count
	effectiveSize = len(reqBody) - totalImageDataBytes + imageEquivalentBytes
	if effectiveSize < 0 {
		effectiveSize = imageEquivalentBytes
	}

	return effectiveSize, count
}
