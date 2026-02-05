package transformer

import (
	"encoding/json"
	"sync"
)

type ToolMapper struct {
	registry *Registry
	mu       sync.RWMutex
}

func NewToolMapper() *ToolMapper {
	return &ToolMapper{
		registry: NewRegistry(Config{}),
	}
}

func NewToolMapperWithConfig(cfg Config) *ToolMapper {
	return &ToolMapper{
		registry: NewRegistry(cfg),
	}
}

func (tm *ToolMapper) SetRegistry(registry *Registry) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.registry = registry
}

func (tm *ToolMapper) GetRegistry() *Registry {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.registry
}

func (tm *ToolMapper) NeedsTransform(toolName string, tags []string) bool {
	return tm.NeedsTransformWithInput(toolName, tags, nil)
}

func (tm *ToolMapper) NeedsTransformWithInput(toolName string, tags []string, input map[string]interface{}) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}
	return tm.registry.GetResponseTransformerWithInput(toolName, tags, input) != nil
}

func (tm *ToolMapper) NeedsAccumulate(toolName string, tags []string) bool {
	return tm.NeedsAccumulateWithInput(toolName, tags, nil)
}

func (tm *ToolMapper) NeedsAccumulateWithInput(toolName string, tags []string, input map[string]interface{}) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}

	cfg := tm.registry.GetResponseTransformerWithInput(toolName, tags, input)
	if cfg != nil {
		return cfg.Accumulate
	}
	return false
}

func (tm *ToolMapper) TransformToolName(toolName string, tags []string) string {
	return tm.TransformToolNameWithInput(toolName, tags, nil)
}

func (tm *ToolMapper) TransformToolNameWithInput(toolName string, tags []string, input map[string]interface{}) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return toolName
	}

	cfg := tm.registry.GetResponseTransformerWithInput(toolName, tags, input)
	if cfg != nil {
		return cfg.TargetTool
	}
	return toolName
}

func (tm *ToolMapper) TransformInput(toolName string, input map[string]interface{}, tags []string) map[string]interface{} {
	return tm.TransformInputWithInput(toolName, input, tags, input)
}

func (tm *ToolMapper) TransformInputWithInput(toolName string, input map[string]interface{}, tags []string, conditionInput map[string]interface{}) map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return input
	}

	cfg := tm.registry.GetResponseTransformerWithInput(toolName, tags, conditionInput)
	if cfg != nil {
		return tm.registry.TransformInput(cfg, input)
	}
	return input
}

func (tm *ToolMapper) TransformInputJSON(toolName string, inputJSON string, tags []string) (string, error) {
	if !tm.NeedsTransform(toolName, tags) {
		return inputJSON, nil
	}

	var input map[string]interface{}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return inputJSON, err
	}

	transformed := tm.TransformInput(toolName, input, tags)
	result, err := json.Marshal(transformed)
	if err != nil {
		return inputJSON, err
	}
	return string(result), nil
}

// MayNeedAccumulate checks if there's any transformer (with or without conditions) that may need accumulation
func (tm *ToolMapper) MayNeedAccumulate(toolName string, tags []string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}

	return tm.registry.MayNeedAccumulate(toolName, tags)
}

// HasPendingTransform checks if there's a transformer with ParamConditions that needs deferred evaluation
func (tm *ToolMapper) HasPendingTransform(toolName string, tags []string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}

	return tm.registry.HasPendingTransform(toolName, tags)
}

func (tm *ToolMapper) GetRequestMapping(toolName string, tags []string) *TransformerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return nil
	}

	return tm.registry.GetRequestTransformer(toolName, tags)
}

func (tm *ToolMapper) TransformRequestToolName(toolName string, tags []string) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return toolName
	}

	cfg := tm.registry.GetRequestTransformer(toolName, tags)
	if cfg != nil {
		return cfg.TargetTool
	}
	return toolName
}

func (tm *ToolMapper) TransformToolDefinition(tool map[string]interface{}, tags []string) map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	name, ok := tool["name"].(string)
	if !ok {
		return tool
	}

	if tm.registry == nil {
		return tool
	}

	cfg := tm.registry.GetRequestTransformer(name, tags)
	if cfg == nil {
		return tool
	}

	result := make(map[string]interface{})
	for k, v := range tool {
		result[k] = v
	}

	result["name"] = cfg.TargetTool

	// 如果定义了 InputSchema，替换工具的 input_schema
	if cfg.InputSchema != nil {
		result["input_schema"] = cfg.InputSchema
	}

	return result
}
