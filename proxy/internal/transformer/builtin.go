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
		Name:        "$droid_warp_Write_to_ExitSpecMode",
		Description: "将 Write(.claude/plans/) 转换为 ExitSpecMode，用于 Droid 规格审批",
		Direction:   "response",
		SourceTool:  "Write",
		TargetTool:  "ExitSpecMode",
		ParamConditions: []ParamCondition{
			{Param: "file_path", Op: "prefix", Value: ".claude/plans/"},
		},
		ParamMapping: []ParamMapping{
			{From: "content", To: "plan"},
			{From: "file_path", To: "title"},
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

	// Request 方向 - 工具名转换
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
		Name:                  "$droid_upstream_kiro_force_compress",
		Description:           "基于 token 估算预检测上下文超限，触发 Droid 压缩",
		Direction:             "response",
		Type:                  "error_transform",
		ErrorCode:             "context_length_exceeded",
		ErrorMessage:          "prompt is too long: request size exceeds limit",
		RequestSizeThreshold:  500000, // 响应后兜底检测阈值 500KB
		ContextTokenLimit:     200000, // 所有模型统一使用 200K token 限制
		ContextThresholdRatio: 0.7,    // 70% 触发（更早压缩）
		TokenEstimateRatio:    3.5,
		Builtin:               true,
	},
	{
		Name:         "$input_too_long_error_transform",
		Description:  "检测上游返回的 input too long / improperly formed 错误，转换为友好提示",
		Direction:    "response",
		Type:         "error_transform",
		ErrorCode:    "context_length_exceeded",
		ErrorMessage: "Input is too long: your conversation context likely exceeds the model's maximum token limit. Try starting a new session or reducing the conversation history",
		ErrorPatterns: []string{
			"(?i)input length",
			"(?i)improperly formed request",
		},
		Builtin: true,
	},

	// === 压缩类型转换器 ===
	{
		Name:        "$auto_compress",
		Description: "通用自动上下文压缩转换器",
		Type:        "compress",
		Direction:   "request",

		// 触发条件
		ContextTokenLimit:     200000,
		ContextThresholdRatio: 0.7,
		TokenEstimateRatio:    3.5,
		ModelContextLimits: []ModelContextLimit{
			{ModelPattern: "claude-opus-4-6*", TokenLimit: 1000000},
			{ModelPattern: "claude-opus-4.6*", TokenLimit: 1000000},
		},

		// 压缩配置
		CompressTarget: "same",
		CompressModel:  "", // 空字符串表示使用原请求的模型
		CompressSystemPrompt: `You are an AI assistant specialized in summarizing conversation history.
Read the complete conversation and generate a structured summary according to the following guidelines:

1. Detailed Chronological Record
   - Capture every important turn in order, including user messages, assistant responses, and tool calls
   - Include tool commands and their important outputs (error messages, test results, exit codes); avoid pasting lengthy logs
   - Use arrows to indicate flow
   - Paraphrase when necessary but preserve intent, technical details, and results

2. Primary Request and Intent
   - Why was this session created?
   - What is the user trying to achieve?
   - What defines success?

3. Constraints and Boundaries
   - User-specified requirements (must do / must not do)
   - Technical limitations discovered
   - Codebase conventions to follow

4. Decisions Made
   - Important decisions and their rationale
   - Rejected alternatives and why

5. Approach - How did the assistant handle the problem?

6. Key Technical Work - List all key technical work completed so far

7. Questions and Clarifications
   - Questions the assistant asked and clarifications the user provided
   - Assumptions made when not explicitly clarified (brief)

8. Files and Code Sections
   - List files created, modified, or deleted
   - External references if any (PR links, Commit SHAs)

9. Error Resolution
   - Errors encountered and how they were resolved
   - Failed approaches and their reasons — avoid retrying unless new information changes conditions

10. Pending Tasks
    - Incomplete tasks with current status
    - For partial work: what IS done vs what is NOT done

11. Current Work
    - Details of the assistant's current task
    - State snapshot if relevant (branch/commit, dirty status, last test/build result)

12. Next Steps - What should the assistant do next?

13. Critical Information
    - Key information that must be passed to subsequent conversations
    - Content that doesn't fit other categories but absolutely cannot be lost
    - Special notes emphasized by the user`,
		CompressUserPrompt: "Please read the complete conversation above and generate a summary according to the guidelines. The new session will not have access to our conversation history, so the summary must contain all key information needed to continue the work.",

		// 消息分割配置
		PreserveBudget: 40000,
		SummaryBudget:  4000,

		// 压缩后处理
		AutoRetry:  true,
		MaxRetries: 1,

		Builtin: true,
	},

	// === 400 错误自动压缩重试转换器 ===
	{
		Name:        "$generic_400_auto_compress_retry",
		Description: "通用 400 错误自动压缩重试（需手动配置映射）",
		Type:        "auto_compress_on_error",
		Direction:   "response",

		// 错误检测
		ErrorPatterns: []string{
			"(?i)improperly formed request",
			"(?i)input length",
		},

		// 压缩配置
		CompressTarget:        "same",
		CompressModel:         "",
		ContextTokenLimit:     200000,
		ContextThresholdRatio: 0.7,
		TokenEstimateRatio:    3.5,
		ModelContextLimits: []ModelContextLimit{
			{ModelPattern: "claude-opus-4-6*", TokenLimit: 1000000},
			{ModelPattern: "claude-opus-4.6*", TokenLimit: 1000000},
		},
		PreserveBudget: 40000,
		SummaryBudget:  4000,

		// 重试配置
		AutoRetry:  true,
		MaxRetries: 1,

		Builtin: true,
	},

	// === 消息格式修正转换器 ===
	{
		Name:        "$generic_message_sanitize",
		Description: "修正消息格式：移除开头assistant、合并连续同角色消息",
		Type:        "message_sanitize",
		Direction:   "request",
		Builtin:     true,
	},
}

// BuiltinMappings defines built-in mapping rules
var BuiltinMappings = []MappingRule{
	// Droid + WARP 规格审批 (Write -> ExitSpecMode)
	{
		Name:        "$droid_warp_spec_response_write",
		Description: "Droid+WARP: Write(.claude/plans/)->ExitSpecMode 响应转换",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_warp"},
		Transformer: "$droid_warp_Write_to_ExitSpecMode",
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
	// Droid + Kiro 大请求错误转换
	{
		Name:        "$droid_upstream_kiro_force_compress_mapping",
		Description: "Droid+Kiro: 大请求错误响应转换为上下文超限错误",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_kiro"},
		Transformer: "$droid_upstream_kiro_force_compress",
		Builtin:     true,
	},
	// Droid + Kiro input too long 错误模式匹配
	{
		Name:        "$droid_upstream_kiro_input_too_long_mapping",
		Description: "Droid+Kiro: 匹配 input too long / improperly formed 错误并转换为友好提示",
		Enabled:     true,
		Tags:        []string{"$a_droid", "$u_kiro"},
		Transformer: "$input_too_long_error_transform",
		Builtin:     true,
	},
}
