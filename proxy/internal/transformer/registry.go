package transformer

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

type Registry struct {
	definitions []TransformerDef
	mappings    []MappingRule
	mu          sync.RWMutex
}

// MessageInjectResult contains a transformer definition and the matched tools
type MessageInjectResult struct {
	Def          *TransformerDef
	MatchedTools []string // Original tool names from mapping that matched request tags
}

func NewRegistry(cfg Config) *Registry {
	// Merge builtin templates, definitions and user configs
	// User-defined items with the same name as builtin items will override them
	allDefs := make([]TransformerDef, 0)
	allDefs = append(allDefs, BuiltinTemplates...)
	allDefs = append(allDefs, BuiltinDefinitions...)

	// User definitions override builtin definitions with the same name
	for _, userDef := range cfg.Definitions {
		overridden := false
		for i, def := range allDefs {
			if def.Name == userDef.Name {
				allDefs[i] = userDef
				overridden = true
				break
			}
		}
		if !overridden {
			allDefs = append(allDefs, userDef)
		}
	}

	// User mappings override builtin mappings with the same name
	allMappings := make([]MappingRule, 0)
	allMappings = append(allMappings, BuiltinMappings...)

	for _, userMapping := range cfg.Mappings {
		overridden := false
		for i, mapping := range allMappings {
			if mapping.Name == userMapping.Name {
				allMappings[i] = userMapping
				overridden = true
				break
			}
		}
		if !overridden {
			allMappings = append(allMappings, userMapping)
		}
	}

	r := &Registry{
		definitions: allDefs,
		mappings:    allMappings,
	}
	// Resolve template references
	r.resolveAllTemplates()
	return r
}

// resolveAllTemplates resolves template references for all definitions
func (r *Registry) resolveAllTemplates() {
	for i := range r.definitions {
		if r.definitions[i].TemplateRef != "" {
			resolved := r.resolveTemplate(&r.definitions[i])
			if resolved != nil {
				r.definitions[i] = *resolved
			}
		}
	}
}

// resolveTemplate resolves a template reference and returns the resolved definition
func (r *Registry) resolveTemplate(def *TransformerDef) *TransformerDef {
	if def.TemplateRef == "" {
		return def
	}

	// Find template
	var tpl *TransformerDef
	for i := range r.definitions {
		if r.definitions[i].Name == def.TemplateRef && r.definitions[i].IsTemplate {
			tpl = &r.definitions[i]
			break
		}
	}
	if tpl == nil {
		return def
	}

	// Create resolved copy
	resolved := *def
	resolved.Direction = replacePlaceholder(tpl.Direction, def.TemplateArgs)
	resolved.SourceTool = replacePlaceholder(tpl.SourceTool, def.TemplateArgs)
	resolved.TargetTool = replacePlaceholder(tpl.TargetTool, def.TemplateArgs)
	resolved.Accumulate = tpl.Accumulate
	if len(tpl.ParamMapping) > 0 && len(resolved.ParamMapping) == 0 {
		resolved.ParamMapping = tpl.ParamMapping
	}
	// Clear template ref after resolution
	resolved.TemplateRef = ""
	resolved.TemplateArgs = nil

	return &resolved
}

// replacePlaceholder replaces {{key}} placeholders with values from args
func replacePlaceholder(s string, args map[string]string) string {
	for k, v := range args {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func (r *Registry) GetResponseTransformer(toolName string, tags []string) *TransformerConfig {
	return r.GetResponseTransformerWithInput(toolName, tags, nil)
}

func (r *Registry) GetResponseTransformerWithInput(toolName string, tags []string, input map[string]interface{}) *TransformerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if def, mapping := r.findTransformerWithInput(toolName, "response", tags, input); def != nil && mapping != nil {
		return r.defToConfig(def, mapping)
	}
	return nil
}

func (r *Registry) GetRequestTransformer(toolName string, tags []string) *TransformerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if def, mapping := r.findTransformerWithInput(toolName, "request", tags, nil); def != nil && mapping != nil {
		return r.defToConfig(def, mapping)
	}
	return nil
}

// findTransformerWithInput finds a matching transformer definition and mapping with input condition check
func (r *Registry) findTransformerWithInput(toolName, direction string, tags []string, input map[string]interface{}) (*TransformerDef, *MappingRule) {
	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Direction == direction && d.SourceTool == toolName {
				// Check param conditions
				if r.matchParamConditions(d.ParamConditions, input) {
					return d, m
				}
			}
		}
	}
	return nil, nil
}

// matchParamConditions checks if input matches all param conditions
// When input is nil and conditions exist, returns true (optimistic match) for deferred evaluation
func (r *Registry) matchParamConditions(conditions []ParamCondition, input map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}
	if input == nil {
		return true // Optimistic match: defer actual validation until input is available
	}
	for _, cond := range conditions {
		value := extractValue(input, cond.Param)
		strValue, ok := value.(string)
		if !ok {
			return false
		}
		switch cond.Op {
		case "prefix":
			if !strings.HasPrefix(strValue, cond.Value) {
				return false
			}
		case "suffix":
			if !strings.HasSuffix(strValue, cond.Value) {
				return false
			}
		case "contains":
			if !strings.Contains(strValue, cond.Value) {
				return false
			}
		case "equals":
			if strValue != cond.Value {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// findTransformer finds a matching transformer definition and mapping (backward compatible)
func (r *Registry) findTransformer(toolName, direction string, tags []string) (*TransformerDef, *MappingRule) {
	return r.findTransformerWithInput(toolName, direction, tags, nil)
}

// MayNeedAccumulate checks if there's any transformer that may need accumulation for the tool
func (r *Registry) MayNeedAccumulate(toolName string, tags []string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Direction == "response" && d.SourceTool == toolName && d.Accumulate {
				return true
			}
		}
	}
	return false
}

// HasPendingTransform checks if there's a transformer with ParamConditions for the tool
func (r *Registry) HasPendingTransform(toolName string, tags []string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Direction == "response" && d.SourceTool == toolName && len(d.ParamConditions) > 0 {
				return true
			}
		}
	}
	return false
}

// defToConfig converts new structure to TransformerConfig
func (r *Registry) defToConfig(def *TransformerDef, mapping *MappingRule) *TransformerConfig {
	return &TransformerConfig{
		Name:         mapping.Name,
		Enabled:      mapping.Enabled,
		Tags:         mapping.Tags,
		Gateways:     mapping.Gateways,
		SourceTool:   def.SourceTool,
		TargetTool:   def.TargetTool,
		Accumulate:   def.Accumulate,
		ParamMapping: def.ParamMapping,
		InputSchema:  def.InputSchema,
	}
}

func (r *Registry) matchTags(transformerTags, excludeTags, requestTags []string) bool {
	// Build request tag set once
	tagSet := make(map[string]bool)
	for _, t := range requestTags {
		tagSet[t] = true
	}

	// Check required tags (all must be present)
	if len(transformerTags) > 0 {
		for _, t := range transformerTags {
			if !tagSet[t] {
				return false
			}
		}
	}

	// Check excluded tags (none must be present)
	if len(excludeTags) > 0 {
		for _, t := range excludeTags {
			if tagSet[t] {
				return false
			}
		}
	}

	return true
}

func (r *Registry) matchTools(mappingTools []string, toolOp string, requestTags []string) (bool, []string) {
	if len(mappingTools) == 0 {
		return true, nil
	}

	// Extract tool names from request tags (tags like $t_create -> create)
	requestToolSet := make(map[string]bool)
	for _, tag := range requestTags {
		if strings.HasPrefix(tag, "$t_") {
			toolName := strings.TrimPrefix(tag, "$t_")
			requestToolSet[strings.ToLower(toolName)] = true
		}
	}

	var matchedTools []string

	if toolOp == "any" {
		for _, tool := range mappingTools {
			if requestToolSet[strings.ToLower(tool)] {
				matchedTools = append(matchedTools, tool)
			}
		}
		if len(matchedTools) > 0 {
			return true, matchedTools
		}
		return false, nil
	}

	// Default: "all" - all tools must be present
	for _, tool := range mappingTools {
		if !requestToolSet[strings.ToLower(tool)] {
			return false, nil
		}
		matchedTools = append(matchedTools, tool)
	}
	return true, matchedTools
}

func (r *Registry) TransformInput(cfg *TransformerConfig, input map[string]interface{}) map[string]interface{} {
	if cfg == nil || len(cfg.ParamMapping) == 0 {
		return input
	}

	result := make(map[string]interface{})

	for k, v := range input {
		result[k] = v
	}

	for _, pm := range cfg.ParamMapping {
		value := extractValue(input, pm.From)
		if value == nil {
			continue
		}

		if pm.Transform == "string_to_array" {
			if s, ok := value.(string); ok {
				value = []string{s}
			}
		}

		setValue(result, pm.To, value)

		fromKey := strings.Split(pm.From, "[")[0]
		if fromKey != pm.To && fromKey != "" {
			delete(result, fromKey)
		}
	}

	return result
}

func (r *Registry) TransformInputJSON(cfg *TransformerConfig, inputJSON string) (string, error) {
	if cfg == nil || len(cfg.ParamMapping) == 0 {
		return inputJSON, nil
	}

	var input map[string]interface{}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return inputJSON, err
	}

	transformed := r.TransformInput(cfg, input)
	result, err := json.Marshal(transformed)
	if err != nil {
		return inputJSON, err
	}
	return string(result), nil
}

func extractValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if strings.Contains(part, "[") {
			key := part[:strings.Index(part, "[")]
			indexStr := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}

			arr, ok := obj[key].([]interface{})
			if !ok {
				return nil
			}

			var idx int
			if err := json.Unmarshal([]byte(indexStr), &idx); err != nil {
				return nil
			}

			if idx < 0 || idx >= len(arr) {
				return nil
			}
			current = arr[idx]
		} else {
			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			current = obj[part]
		}
	}

	return current
}

func setValue(data map[string]interface{}, key string, value interface{}) {
	data[key] = value
}

func (r *Registry) UpdateConfig(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.definitions = cfg.Definitions
	r.mappings = cfg.Mappings
}

func (r *Registry) GetConfig() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return Config{
		Definitions: r.definitions,
		Mappings:    r.mappings,
	}
}

// New structure methods

func (r *Registry) GetDefinitions() []TransformerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]TransformerDef, len(r.definitions))
	copy(result, r.definitions)
	return result
}

func (r *Registry) GetMappings() []MappingRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MappingRule, len(r.mappings))
	copy(result, r.mappings)
	return result
}

func (r *Registry) AddDefinition(def TransformerDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, d := range r.definitions {
		if d.Name == def.Name {
			return ErrDefinitionExists
		}
	}
	r.definitions = append(r.definitions, def)
	return nil
}

func (r *Registry) UpdateDefinition(name string, def TransformerDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, d := range r.definitions {
		if d.Name == name {
			if d.Builtin {
				return ErrBuiltinReadonly
			}
			r.definitions[i] = def
			return nil
		}
	}
	return ErrDefinitionNotFound
}

func (r *Registry) DeleteDefinition(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, d := range r.definitions {
		if d.Name == name {
			if d.Builtin {
				return ErrBuiltinReadonly
			}
			r.definitions = append(r.definitions[:i], r.definitions[i+1:]...)
			return nil
		}
	}
	return ErrDefinitionNotFound
}

func (r *Registry) AddMapping(mapping MappingRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, m := range r.mappings {
		if m.Name == mapping.Name {
			return ErrMappingExists
		}
	}
	r.mappings = append(r.mappings, mapping)
	return nil
}

func (r *Registry) UpdateMapping(name string, mapping MappingRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, m := range r.mappings {
		if m.Name == name {
			if m.Builtin {
				return ErrBuiltinReadonly
			}
			r.mappings[i] = mapping
			return nil
		}
	}
	return ErrMappingNotFound
}

func (r *Registry) DeleteMapping(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, m := range r.mappings {
		if m.Name == name {
			if m.Builtin {
				return ErrBuiltinReadonly
			}
			r.mappings = append(r.mappings[:i], r.mappings[i+1:]...)
			return nil
		}
	}
	return ErrMappingNotFound
}

// GetMessageInjectTransformers returns all message_inject type transformers that match the given tags
func (r *Registry) GetMessageInjectTransformers(tags []string) []*MessageInjectResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*MessageInjectResult

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		matched, matchedTools := r.matchTools(m.Tools, m.ToolOp, tags)
		if !matched {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Type == "message_inject" && d.Direction == "request" {
				result = append(result, &MessageInjectResult{
					Def:          d,
					MatchedTools: matchedTools,
				})
				break
			}
		}
	}

	return result
}

// GetErrorTransformer returns the first error_transform type transformer that matches the given tags
func (r *Registry) GetErrorTransformer(tags []string) *TransformerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Type == "error_transform" {
				return d
			}
		}
	}
	return nil
}

// GetHeaderInjectTransformers returns all header_inject type transformers that match the given direction and tags
func (r *Registry) GetHeaderInjectTransformers(direction string, tags []string) []*TransformerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*TransformerDef

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Type == "header_inject" && d.Direction == direction {
				result = append(result, d)
				break
			}
		}
	}

	return result
}

// GetErrorPatternTransformer returns the first error_transform transformer with ErrorPatterns that matches
// the given tags and whose patterns match the response body content
func (r *Registry) GetErrorPatternTransformer(tags []string, respBody string) *TransformerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Type == "error_transform" && len(d.ErrorPatterns) > 0 {
				// Check if any pattern matches the response body
				for _, pattern := range d.ErrorPatterns {
					re, err := regexp.Compile(pattern)
					if err != nil {
						continue
					}
					if re.MatchString(respBody) {
						return d
					}
				}
			}
		}
	}
	return nil
}

// GetCompressTransformer returns the first compress type transformer that matches the given tags
func (r *Registry) GetCompressTransformer(tags []string) *TransformerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.mappings {
		m := &r.mappings[i]
		if !m.Enabled {
			continue
		}
		if !r.matchTags(m.Tags, m.ExcludeTags, tags) {
			continue
		}
		// Find the referenced definition
		for j := range r.definitions {
			d := &r.definitions[j]
			if d.Name == m.Transformer && d.Type == "compress" && d.Direction == "request" {
				return d
			}
		}
	}
	return nil
}
