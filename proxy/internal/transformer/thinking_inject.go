package transformer

import (
	"encoding/json"
	"strings"

	"go.uber.org/zap"
)

const thinkingModelSuffix = "-thinking"

const thinkingPrompt = `<thinking_mode>enabled</thinking_mode>
<max_thinking_length>16000</max_thinking_length>`

// ApplyThinkingInject checks if the model name ends with "-thinking",
// injects thinking prompt into the system field, and strips the suffix from model.
// Returns the modified body and whether any transformation was applied.
func ApplyThinkingInject(body []byte, logger *zap.Logger) ([]byte, bool) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false
	}

	model, ok := req["model"].(string)
	if !ok || !strings.HasSuffix(model, thinkingModelSuffix) {
		return body, false
	}

	// Strip -thinking suffix from model
	req["model"] = strings.TrimSuffix(model, thinkingModelSuffix)

	// Inject thinking prompt into system field
	injectThinkingIntoSystem(req)

	result, err := json.Marshal(req)
	if err != nil {
		logger.Error("thinking_inject: failed to marshal", zap.Error(err))
		return body, false
	}

	logger.Info("thinking_inject: applied",
		zap.String("original_model", model),
		zap.String("new_model", req["model"].(string)))

	return result, true
}

func injectThinkingIntoSystem(req map[string]interface{}) {
	system, exists := req["system"]
	if !exists {
		req["system"] = thinkingPrompt
		return
	}

	switch v := system.(type) {
	case string:
		if v == "" {
			req["system"] = thinkingPrompt
		} else {
			req["system"] = thinkingPrompt + "\n\n" + v
		}
	case []interface{}:
		thinkingBlock := map[string]interface{}{
			"type": "text",
			"text": thinkingPrompt,
		}
		req["system"] = append([]interface{}{thinkingBlock}, v...)
	default:
		req["system"] = thinkingPrompt
	}
}
