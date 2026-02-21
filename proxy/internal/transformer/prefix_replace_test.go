package transformer

import (
	"testing"
)

func TestTransformInput_PrefixReplace(t *testing.T) {
	registry := NewRegistry(Config{})

	tests := []struct {
		name     string
		cfg      *TransformerConfig
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "prefix_replace: .claude/plans/ to .factory/specs/",
			cfg: &TransformerConfig{
				ParamMapping: []ParamMapping{
					{
						From:      "file_path",
						To:        "file_path",
						Transform: "prefix_replace",
						TransformArgs: map[string]string{
							"old_prefix": ".claude/plans/",
							"new_prefix": ".factory/specs/",
						},
					},
				},
			},
			input: map[string]interface{}{
				"file_path": ".claude/plans/my-spec.md",
			},
			expected: map[string]interface{}{
				"file_path": ".factory/specs/my-spec.md",
			},
		},
		{
			name: "prefix_replace: no match, passthrough",
			cfg: &TransformerConfig{
				ParamMapping: []ParamMapping{
					{
						From:      "file_path",
						To:        "file_path",
						Transform: "prefix_replace",
						TransformArgs: map[string]string{
							"old_prefix": ".claude/plans/",
							"new_prefix": ".factory/specs/",
						},
					},
				},
			},
			input: map[string]interface{}{
				"file_path": "/absolute/path/to/file.md",
			},
			expected: map[string]interface{}{
				"file_path": "/absolute/path/to/file.md",
			},
		},
		{
			name: "prefix_replace: nested path",
			cfg: &TransformerConfig{
				ParamMapping: []ParamMapping{
					{
						From:      "file_path",
						To:        "file_path",
						Transform: "prefix_replace",
						TransformArgs: map[string]string{
							"old_prefix": ".claude/plans/",
							"new_prefix": ".factory/specs/",
						},
					},
				},
			},
			input: map[string]interface{}{
				"file_path": ".claude/plans/subdir/nested-spec.md",
			},
			expected: map[string]interface{}{
				"file_path": ".factory/specs/subdir/nested-spec.md",
			},
		},
		{
			name: "prefix_replace: empty new_prefix",
			cfg: &TransformerConfig{
				ParamMapping: []ParamMapping{
					{
						From:      "file_path",
						To:        "file_path",
						Transform: "prefix_replace",
						TransformArgs: map[string]string{
							"old_prefix": ".claude/plans/",
							"new_prefix": "",
						},
					},
				},
			},
			input: map[string]interface{}{
				"file_path": ".claude/plans/my-spec.md",
			},
			expected: map[string]interface{}{
				"file_path": "my-spec.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.TransformInput(tt.cfg, tt.input)
			
			if result["file_path"] != tt.expected["file_path"] {
				t.Errorf("TransformInput() file_path = %v, want %v", result["file_path"], tt.expected["file_path"])
			}
		})
	}
}

func TestBuiltinTransformer_ReadClaudePlans(t *testing.T) {
	registry := NewRegistry(Config{})

	// Find the builtin transformer
	var found *TransformerDef
	for i := range registry.definitions {
		if registry.definitions[i].Name == "$droid_warp_Read_claude_plans" {
			found = &registry.definitions[i]
			break
		}
	}

	if found == nil {
		t.Fatal("Expected to find $droid_warp_Read_claude_plans definition")
	}

	// Verify configuration
	if found.Direction != "response" {
		t.Errorf("Expected Direction=response, got %s", found.Direction)
	}
	if found.SourceTool != "Read" {
		t.Errorf("Expected SourceTool=Read, got %s", found.SourceTool)
	}
	if found.TargetTool != "Read" {
		t.Errorf("Expected TargetTool=Read, got %s", found.TargetTool)
	}
	if len(found.ParamConditions) != 1 {
		t.Fatalf("Expected 1 ParamCondition, got %d", len(found.ParamConditions))
	}
	if found.ParamConditions[0].Param != "file_path" {
		t.Errorf("Expected ParamCondition.Param=file_path, got %s", found.ParamConditions[0].Param)
	}
	if found.ParamConditions[0].Op != "prefix" {
		t.Errorf("Expected ParamCondition.Op=prefix, got %s", found.ParamConditions[0].Op)
	}
	if found.ParamConditions[0].Value != ".claude/plans/" {
		t.Errorf("Expected ParamCondition.Value=.claude/plans/, got %s", found.ParamConditions[0].Value)
	}
	if len(found.ParamMapping) != 1 {
		t.Fatalf("Expected 1 ParamMapping, got %d", len(found.ParamMapping))
	}
	if found.ParamMapping[0].Transform != "prefix_replace" {
		t.Errorf("Expected ParamMapping.Transform=prefix_replace, got %s", found.ParamMapping[0].Transform)
	}
	if found.ParamMapping[0].TransformArgs["old_prefix"] != ".claude/plans/" {
		t.Errorf("Expected old_prefix=.claude/plans/, got %s", found.ParamMapping[0].TransformArgs["old_prefix"])
	}
	if found.ParamMapping[0].TransformArgs["new_prefix"] != ".factory/specs/" {
		t.Errorf("Expected new_prefix=.factory/specs/, got %s", found.ParamMapping[0].TransformArgs["new_prefix"])
	}
}

func TestBuiltinMapping_ReadClaudePlans(t *testing.T) {
	registry := NewRegistry(Config{})

	// Find the builtin mapping
	var found *MappingRule
	for i := range registry.mappings {
		if registry.mappings[i].Name == "$droid_warp_read_claude_plans" {
			found = &registry.mappings[i]
			break
		}
	}

	if found == nil {
		t.Fatal("Expected to find $droid_warp_read_claude_plans mapping")
	}

	// Verify configuration
	if !found.Enabled {
		t.Errorf("Expected mapping to be enabled")
	}
	if len(found.Tags) != 2 {
		t.Fatalf("Expected 2 tags, got %d", len(found.Tags))
	}
	expectedTags := map[string]bool{"$a_droid": true, "$u_warp": true}
	for _, tag := range found.Tags {
		if !expectedTags[tag] {
			t.Errorf("Unexpected tag: %s", tag)
		}
	}
	if found.Transformer != "$droid_warp_Read_claude_plans" {
		t.Errorf("Expected Transformer=$droid_warp_Read_claude_plans, got %s", found.Transformer)
	}
}
