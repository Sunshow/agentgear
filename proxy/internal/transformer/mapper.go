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
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}
	return tm.registry.GetResponseTransformer(toolName, tags) != nil
}

func (tm *ToolMapper) NeedsAccumulate(toolName string, tags []string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return false
	}

	cfg := tm.registry.GetResponseTransformer(toolName, tags)
	if cfg != nil {
		return cfg.Accumulate
	}
	return false
}

func (tm *ToolMapper) TransformToolName(toolName string, tags []string) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return toolName
	}

	cfg := tm.registry.GetResponseTransformer(toolName, tags)
	if cfg != nil {
		return cfg.TargetTool
	}
	return toolName
}

func (tm *ToolMapper) TransformInput(toolName string, input map[string]interface{}, tags []string) map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.registry == nil {
		return input
	}

	cfg := tm.registry.GetResponseTransformer(toolName, tags)
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
