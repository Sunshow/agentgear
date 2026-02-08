package transformer

import (
	"testing"
)

func TestMatchTags(t *testing.T) {
	r := &Registry{}

	tests := []struct {
		name           string
		transformerTags []string
		excludeTags    []string
		requestTags    []string
		want           bool
	}{
		{
			name:           "empty transformer tags matches all",
			transformerTags: []string{},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           true,
		},
		{
			name:           "all required tags present",
			transformerTags: []string{"$a_droid", "$u_warp"},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid", "$u_warp", "$t_create"},
			want:           true,
		},
		{
			name:           "missing required tag",
			transformerTags: []string{"$a_droid", "$u_warp"},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid"},
			want:           false,
		},
		{
			name:           "excluded tag present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           false,
		},
		{
			name:           "excluded tag not present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_kiro"},
			want:           true,
		},
		{
			name:           "multiple excluded tags, one present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp", "$u_kiro"},
			requestTags:    []string{"$a_droid", "$u_kiro"},
			want:           false,
		},
		{
			name:           "multiple excluded tags, none present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp", "$u_kiro"},
			requestTags:    []string{"$a_droid", "$t_create"},
			want:           true,
		},
		{
			name:           "only exclude tags, match present",
			transformerTags: []string{},
			excludeTags:    []string{"$a_cursor"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           true,
		},
		{
			name:           "only exclude tags, excluded present",
			transformerTags: []string{},
			excludeTags:    []string{"$a_cursor"},
			requestTags:    []string{"$a_cursor", "$u_warp"},
			want:           false,
		},
		{
			name:           "required and excluded both satisfied",
			transformerTags: []string{"$a_droid", "$t_create"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$t_create", "$u_kiro"},
			want:           true,
		},
		{
			name:           "required satisfied but excluded present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.matchTags(tt.transformerTags, tt.excludeTags, tt.requestTags)
			if got != tt.want {
				t.Errorf("matchTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRegistry_OverrideBuiltinDefinitions(t *testing.T) {
	// Create user config that overrides a builtin definition
	userCfg := Config{
		Definitions: []TransformerDef{
			{
				Name:                  "$droid_upstream_kiro_force_compress",
				Direction:             "response",
				Type:                  "error_transform",
				ErrorCode:             "context_length_exceeded",
				ContextTokenLimit:     200000,
				ContextThresholdRatio: 0.6, // Override: 60% instead of builtin 70%
				TokenEstimateRatio:    3.5,
			},
		},
	}

	registry := NewRegistry(userCfg)

	// Find the overridden definition - should appear exactly once
	count := 0
	var found *TransformerDef
	for i := range registry.definitions {
		if registry.definitions[i].Name == "$droid_upstream_kiro_force_compress" {
			found = &registry.definitions[i]
			count++
		}
	}

	if count != 1 {
		t.Fatalf("Expected exactly 1 definition with name $droid_upstream_kiro_force_compress, got %d", count)
	}
	if found.ContextThresholdRatio != 0.6 {
		t.Errorf("Expected ContextThresholdRatio=0.6, got %f", found.ContextThresholdRatio)
	}
	if found.Type != "error_transform" {
		t.Errorf("Expected Type=error_transform, got %s", found.Type)
	}
	if found.ContextTokenLimit != 200000 {
		t.Errorf("Expected ContextTokenLimit=200000, got %d", found.ContextTokenLimit)
	}
}

func TestNewRegistry_OverrideBuiltinMappings(t *testing.T) {
	// Create user config that overrides a builtin mapping
	userCfg := Config{
		Mappings: []MappingRule{
			{
				Name:        "$droid_upstream_kiro_force_compress_mapping",
				Enabled:     false, // Override: disable the builtin mapping
				Tags:        []string{"$a_droid", "$u_kiro"},
				Transformer: "$droid_upstream_kiro_force_compress",
			},
		},
	}

	registry := NewRegistry(userCfg)

	// Find the overridden mapping - should appear exactly once
	count := 0
	var found *MappingRule
	for i := range registry.mappings {
		if registry.mappings[i].Name == "$droid_upstream_kiro_force_compress_mapping" {
			found = &registry.mappings[i]
			count++
		}
	}

	if count != 1 {
		t.Fatalf("Expected exactly 1 mapping with name $droid_upstream_kiro_force_compress_mapping, got %d", count)
	}
	if found.Enabled {
		t.Errorf("Expected mapping to be disabled, but it is enabled")
	}
}

func TestNewRegistry_UserDefinitionsNotOverriding(t *testing.T) {
	// Create user config with new definitions (not overriding)
	userCfg := Config{
		Definitions: []TransformerDef{
			{
				Name:       "my_custom_transformer",
				Direction:  "response",
				Type:       "tool",
				SourceTool: "CustomTool",
				TargetTool: "AnotherTool",
			},
		},
	}

	registry := NewRegistry(userCfg)

	// Count definitions
	customCount := 0
	builtinCount := 0
	for i := range registry.definitions {
		if registry.definitions[i].Name == "my_custom_transformer" {
			customCount++
		}
		if registry.definitions[i].Builtin {
			builtinCount++
		}
	}

	if customCount != 1 {
		t.Errorf("Expected exactly 1 custom definition, got %d", customCount)
	}
	if builtinCount == 0 {
		t.Errorf("Expected builtin definitions to still exist, got 0")
	}
}

func TestNewRegistry_OverrideCompressTransformer(t *testing.T) {
	// Create user config that overrides $auto_compress
	userCfg := Config{
		Definitions: []TransformerDef{
			{
				Name:                  "$auto_compress",
				Type:                  "compress",
				Direction:             "request",
				ContextTokenLimit:     200000,
				ContextThresholdRatio: 0.5, // Override: 50% instead of builtin 70%
				TokenEstimateRatio:    3.5,
				CompressTarget:        "same",
				AutoRetry:             true,
				MaxRetries:            1,
			},
		},
	}

	registry := NewRegistry(userCfg)

	// Find the overridden definition - should appear exactly once
	count := 0
	var found *TransformerDef
	for i := range registry.definitions {
		if registry.definitions[i].Name == "$auto_compress" {
			found = &registry.definitions[i]
			count++
		}
	}

	if count != 1 {
		t.Fatalf("Expected exactly 1 definition with name $auto_compress, got %d", count)
	}
	if found.ContextThresholdRatio != 0.5 {
		t.Errorf("Expected ContextThresholdRatio=0.5, got %f", found.ContextThresholdRatio)
	}
	if found.Type != "compress" {
		t.Errorf("Expected Type=compress, got %s", found.Type)
	}
	if !found.AutoRetry {
		t.Errorf("Expected AutoRetry=true, got false")
	}
}
