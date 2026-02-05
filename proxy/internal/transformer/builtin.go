package transformer

// BuiltinTemplates defines reusable transformer templates
var BuiltinTemplates = []TransformerDef{
	{
		Name:        "$tpl_tool_alias",
		Description: "工具别名模板：将一个工具名转换为另一个，不做参数转换",
		Direction:   "{{direction}}",
		SourceTool:  "{{source}}",
		TargetTool:  "{{target}}",
		IsTemplate:  true,
		Builtin:     true,
	},
}

// BuiltinDefinitions defines built-in transformer definitions
var BuiltinDefinitions = []TransformerDef{
	// === Droid + WARP 场景转换器 ===

	// Response 方向 - 带参数转换
	{
		Name:        "$droid_warp_create_documents_to_ExitSpecMode",
		Description: "将 create_documents 转换为 ExitSpecMode，用于 Droid 规格审批",
		Direction:   "response",
		SourceTool:  "create_documents",
		TargetTool:  "ExitSpecMode",
		Accumulate:  true,
		ParamMapping: []ParamMapping{
			{From: "new_documents[0].content", To: "plan"},
			{From: "new_documents[0].title", To: "title"},
		},
		Builtin: true,
	},
	{
		Name:        "$droid_warp_Edit_normalize",
		Description: "标准化 Edit 参数 (old_string->old_str, new_string->new_str)",
		Direction:   "response",
		SourceTool:  "Edit",
		TargetTool:  "Edit",
		ParamMapping: []ParamMapping{
			{From: "old_string", To: "old_str"},
			{From: "new_string", To: "new_str"},
		},
		Builtin: true,
	},
	{
		Name:        "$droid_warp_Glob_normalize",
		Description: "标准化 Glob 参数 (path->folder, pattern->patterns[])",
		Direction:   "response",
		SourceTool:  "Glob",
		TargetTool:  "Glob",
		ParamMapping: []ParamMapping{
			{From: "path", To: "folder"},
			{From: "pattern", To: "patterns", Transform: "string_to_array"},
		},
		Builtin: true,
	},

	// Response 方向 - 基于模板的纯名称转换
	{
		Name:        "$droid_warp_Bash_to_Execute",
		Description: "Bash -> Execute 工具名转换",
		TemplateRef: "$tpl_tool_alias",
		TemplateArgs: map[string]string{
			"direction": "response",
			"source":    "Bash",
			"target":    "Execute",
		},
		Builtin: true,
	},
	{
		Name:        "$droid_warp_Write_to_Create",
		Description: "Write -> Create 工具名转换",
		TemplateRef: "$tpl_tool_alias",
		TemplateArgs: map[string]string{
			"direction": "response",
			"source":    "Write",
			"target":    "Create",
		},
		Builtin: true,
	},

	// Request 方向 - 带 schema 转换
	{
		Name:        "$droid_warp_ExitSpecMode_to_create_documents",
		Description: "ExitSpecMode -> create_documents 请求转换",
		Direction:   "request",
		SourceTool:  "ExitSpecMode",
		TargetTool:  "create_documents",
		InputSchema: map[string]interface{}{
			"$schema":              "http://json-schema.org/draft-07/schema#",
			"type":                 "object",
			"additionalProperties": false,
			"required":             []interface{}{"new_documents"},
			"properties": map[string]interface{}{
				"new_documents": map[string]interface{}{
					"type":        "array",
					"description": "Array of documents to create",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"title": map[string]interface{}{
								"type":        "string",
								"description": "Document title",
							},
							"content": map[string]interface{}{
								"type":        "string",
								"description": "Document content (the plan/spec)",
							},
						},
						"required": []interface{}{"content"},
					},
				},
			},
		},
		Builtin: true,
	},
	{
		Name:        "$droid_warp_Create_to_Write",
		Description: "Create -> Write 请求转换",
		TemplateRef: "$tpl_tool_alias",
		TemplateArgs: map[string]string{
			"direction": "request",
			"source":    "Create",
			"target":    "Write",
		},
		Builtin: true,
	},
	{
		Name:        "$droid_warp_Execute_to_Bash",
		Description: "Execute -> Bash 请求转换",
		TemplateRef: "$tpl_tool_alias",
		TemplateArgs: map[string]string{
			"direction": "request",
			"source":    "Execute",
			"target":    "Bash",
		},
		Builtin: true,
	},

	// === 消息注入类型转换器 ===
	{
		Name:         "$upstream_kiro_chunked_write_hint",
		Description:  "Upstream Kiro: 提示模型分段创建长文件，避免输出截断",
		Direction:    "request",
		Type:         "message_inject",
		InjectFormat: "system-reminder",
		InjectText: `IMPORTANT: When using the {{tool}} tool to create files, if the content exceeds 150 lines:
1. First create the file with a skeleton (imports, class/function signatures, brief TODO comments)
2. Then use multiple Edit tool calls to add the implementation in chunks of ~100 lines each
This prevents output truncation with long content.`,
		Builtin: true,
	},

	// === 错误转换类型转换器 ===
	{
		Name:                 "$droid_large_request_to_context_error",
		Description:          "将大请求的错误响应转换为上下文超限错误，触发 Droid 压缩",
		Direction:            "response",
		Type:                 "error_transform",
		ErrorCode:            "context_length_exceeded",
		ErrorMessage:         "prompt is too long: request size exceeds limit",
		RequestSizeThreshold: 500000, // 默认 500KB
		Builtin:              true,
	},
}

// BuiltinMappings defines built-in mapping rules
var BuiltinMappings = []MappingRule{
	// Droid + WARP 规格审批
	{
		Name:        "$droid_warp_spec_response",
		Description: "Droid+WARP: create_documents->ExitSpecMode 响应转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_create_documents_to_ExitSpecMode",
		Builtin:     true,
	},
	{
		Name:        "$droid_warp_spec_request",
		Description: "Droid+WARP: ExitSpecMode->create_documents 请求转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_ExitSpecMode_to_create_documents",
		Builtin:     true,
	},
	// Droid + WARP Edit 参数标准化
	{
		Name:        "$droid_warp_edit_normalize",
		Description: "Droid+WARP: Edit 参数标准化",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Edit_normalize",
		Builtin:     true,
	},
	// Droid + WARP Glob 参数标准化
	{
		Name:        "$droid_warp_glob_normalize",
		Description: "Droid+WARP: Glob 参数标准化",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Glob_normalize",
		Builtin:     true,
	},
	// Droid + WARP Bash/Execute 互转
	{
		Name:        "$droid_warp_bash_execute_response",
		Description: "Droid+WARP: Bash->Execute 响应转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Bash_to_Execute",
		Builtin:     true,
	},
	{
		Name:        "$droid_warp_execute_bash_request",
		Description: "Droid+WARP: Execute->Bash 请求转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Execute_to_Bash",
		Builtin:     true,
	},
	// Droid + WARP Write/Create 互转
	{
		Name:        "$droid_warp_write_create_response",
		Description: "Droid+WARP: Write->Create 响应转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Write_to_Create",
		Builtin:     true,
	},
	{
		Name:        "$droid_warp_create_write_request",
		Description: "Droid+WARP: Create->Write 请求转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Create_to_Write",
		Builtin:     true,
	},
	// Upstream Kiro 分段写入提示
	{
		Name:        "$upstream_kiro_chunked_write",
		Description: "Upstream Kiro: 分段写入提示，避免长文件输出截断",
		Enabled:     true,
		Tags:        []string{"$u_kiro"},
		Tools:       []string{"Create", "Write"},
		ToolOp:      "any",
		Transformer: "$upstream_kiro_chunked_write_hint",
		Builtin:     true,
	},
	// Droid 大请求错误转换
	{
		Name:        "$droid_large_request_error",
		Description: "Droid: 大请求错误响应转换为上下文超限错误",
		Enabled:     true,
		Tags:        []string{"$a_droid"},
		Transformer: "$droid_large_request_to_context_error",
		Builtin:     true,
	},
}
