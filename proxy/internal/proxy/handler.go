package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sunshow/agentgear/proxy/internal/api"
	"github.com/sunshow/agentgear/proxy/internal/config"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/parser"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
	"go.uber.org/zap"
)

type Handler struct {
	gatewayName         string
	gatewayPath         string
	upstreamURL         string
	upstreamType        string
	timeout             time.Duration
	logger              *zap.Logger
	logDir              string
	logEnabled          bool
	httpClient          *http.Client
	sessionSequences    sync.Map
	toolMapper          *transformer.ToolMapper
	memoryStore         *memory.ConnectionStore
	taggingEngine       *tagging.Engine
	transformerRegistry *transformer.Registry
	wsHub               *api.WSHub
}

type Config struct {
	GatewayName         string
	GatewayPath         string
	UpstreamURL         string
	UpstreamType        string
	Timeout             time.Duration
	Logger              *zap.Logger
	LogDir              string
	LogEnabled          bool
	MemoryStore         *memory.ConnectionStore
	TaggingEngine       *tagging.Engine
	TransformerRegistry *transformer.Registry
	WSHub               *api.WSHub
}

func NewHandler(cfg Config) *Handler {
	h := &Handler{
		gatewayName:         cfg.GatewayName,
		gatewayPath:         cfg.GatewayPath,
		upstreamURL:         cfg.UpstreamURL,
		upstreamType:        cfg.UpstreamType,
		timeout:             cfg.Timeout,
		logger:              cfg.Logger,
		logDir:              cfg.LogDir,
		logEnabled:          cfg.LogEnabled,
		httpClient:          &http.Client{Timeout: cfg.Timeout},
		memoryStore:         cfg.MemoryStore,
		taggingEngine:       cfg.TaggingEngine,
		transformerRegistry: cfg.TransformerRegistry,
		wsHub:               cfg.WSHub,
	}

	if cfg.TransformerRegistry != nil {
		h.toolMapper = transformer.NewToolMapperWithConfig(cfg.TransformerRegistry.GetConfig())
	} else {
		h.toolMapper = transformer.NewToolMapper()
	}

	return h
}

type RequestMeta struct {
	ID         string              `json:"id"`
	SessionID  string              `json:"session_id"`
	Sequence   int                 `json:"sequence"`
	Timestamp  time.Time           `json:"timestamp"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Headers    map[string][]string `json:"headers"`
	DurationMs int64               `json:"duration_ms"`
	Error      string              `json:"error,omitempty"`
	Response   *ResponseMeta       `json:"response,omitempty"`
}

type ResponseMeta struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
}

type requestContext struct {
	meta                *RequestMeta
	reqBody             []byte
	transformedReqBody  []byte
	respBody            []byte
	tags                []string
	connInfo            *memory.ConnectionInfo
	forceLog            bool     // 强制记录日志（不受 logging.enabled 影响）
	appliedTransformers []string // 收集应用的 transformers（包括响应方向）
	isFirstTurn         bool     // true when request has only one user message
}

func (h *Handler) getOrCreateSession(c *gin.Context) (string, int) {
	sessionID := c.GetHeader("X-Session-Id")
	if sessionID == "" {
		// 使用时间戳前缀 + 短UUID，格式: 20260127-134025_a1b2c3d4
		timestamp := time.Now().Format("20060102-150405")
		shortUUID := uuid.New().String()[:8]
		sessionID = fmt.Sprintf("%s_%s", timestamp, shortUUID)
	}

	var seq uint64
	if val, ok := h.sessionSequences.Load(sessionID); ok {
		seq = atomic.AddUint64(val.(*uint64), 1)
	} else {
		var newSeq uint64 = 1
		actual, loaded := h.sessionSequences.LoadOrStore(sessionID, &newSeq)
		if loaded {
			seq = atomic.AddUint64(actual.(*uint64), 1)
		} else {
			seq = 1
		}
	}

	return sessionID, int(seq)
}

func (h *Handler) ProxyRequest(c *gin.Context) {
	requestID := uuid.New().String()
	startTime := time.Now()

	sessionID, sequence := h.getOrCreateSession(c)
	c.Header("X-Session-Id", sessionID)

	reqBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// Match tags using tagging engine
	var tags []string
	if h.taggingEngine != nil {
		ctx := &tagging.RequestContext{
			Method:  c.Request.Method,
			Path:    c.Request.URL.Path,
			Query:   c.Request.URL.Query(),
			Headers: c.Request.Header,
			Body:    reqBody,
		}
		tags = h.taggingEngine.Match(ctx)
		if len(tags) > 0 {
			h.logger.Info("request matched tags", zap.Strings("tags", tags))
		}
	}

	// Inject gateway tag
	if h.gatewayName != "" {
		gatewayTag := config.GetGatewayTag(h.gatewayName)
		h.logger.Info("gateway tag injection",
			zap.String("gatewayName", h.gatewayName),
			zap.String("gatewayTag", gatewayTag))
		if gatewayTag != "" {
			tags = append(tags, gatewayTag)
		}
	} else {
		h.logger.Info("gateway name is empty")
	}

	// Inject upstream type tag
	if h.upstreamType != "" {
		upstreamTag := config.GetUpstreamTag(h.upstreamType)
		h.logger.Info("upstream tag injection",
			zap.String("upstreamType", h.upstreamType),
			zap.String("upstreamTag", upstreamTag))
		if upstreamTag != "" {
			tags = append(tags, upstreamTag)
		}
	}

	h.logger.Info("final tags before store", zap.Strings("tags", tags))

	// Only store to memory when GUI is connected (has WebSocket clients)
	shouldStore := h.wsHub != nil && h.wsHub.HasClients()

	// Create connection info for memory store
	var connInfo *memory.ConnectionInfo
	if shouldStore && h.memoryStore != nil {
		connInfo = &memory.ConnectionInfo{
			ID:             requestID,
			SessionID:      sessionID,
			Sequence:       sequence,
			Tags:           tags,
			Method:         c.Request.Method,
			Path:           c.Request.URL.Path,
			Status:         "pending",
			StartTime:      startTime,
			RequestHeaders: flattenHeaders(c.Request.Header),
			RequestBody:    reqBody,
		}

		// Parse protocol data based on tags
		connInfo.ParsedData = h.parseProtocolData(reqBody, tags)

		h.memoryStore.Add(connInfo)
	}

	// Transform request body (tools definitions) for upstream
	transformedReqBody, appliedTransformers := h.transformRequestBody(reqBody, tags)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(transformedReqBody))

	// Update connection info with applied transformers
	if connInfo != nil && len(appliedTransformers) > 0 {
		connInfo.AppliedRequestTransformers = append(connInfo.AppliedRequestTransformers, appliedTransformers...)
		connInfo.TransformedRequest = true
		connInfo.TransformedRequestBody = transformedReqBody
	}

	reqCtx := &requestContext{
		meta: &RequestMeta{
			ID:        requestID,
			SessionID: sessionID,
			Sequence:  sequence,
			Timestamp: startTime,
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			Headers:   sanitizeHeaders(c.Request.Header),
		},
		reqBody:            reqBody,
		transformedReqBody: transformedReqBody,
		tags:               tags,
		connInfo:           connInfo,
		isFirstTurn:        countUserMessages(reqBody) == 1,
	}

	// 请求前预检测：基于 token 估算检查是否超过上下文限制
	if handler := h.shouldPreemptContextError(reqCtx); handler != nil {
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = true
		h.updateConnectionStatus(connInfo, "preempt_context_error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		h.writeContextLengthError(c, handler, len(reqBody))
		return
	}

	// 压缩处理：检测是否需要压缩上下文
	if compressHandler := h.shouldCompress(reqCtx); compressHandler != nil {
		h.logger.Info("compression triggered", zap.String("transformer", compressHandler.Name))
		
		// 构建压缩目标 URL
		compressURL, err := h.buildCompressTargetURL(compressHandler)
		if err != nil {
			h.logger.Error("failed to build compress target URL", zap.Error(err))
		} else {
			// 创建压缩处理器
			compressor := transformer.NewCompressHandler(compressHandler, h.logger, h.getGatewayMap())
			
			// 准备请求头（复制认证信息）
			compressHeaders := make(map[string]string)
			for _, key := range []string{"Authorization", "X-Api-Key", "Anthropic-Api-Key"} {
				if val := c.GetHeader(key); val != "" {
					compressHeaders[key] = val
				}
			}
			
			// 执行压缩
			compressedReq, compressed, err := compressor.Process(transformedReqBody, compressURL, compressHeaders)
			if err != nil {
				h.logger.Error("compression failed", zap.Error(err))
				// 压缩失败，继续使用原请求（降级处理）
			} else if compressed {
				h.logger.Info("compression succeeded", 
					zap.Int("original_size", len(transformedReqBody)),
					zap.Int("compressed_size", len(compressedReq)))
				transformedReqBody = compressedReq
				reqCtx.transformedReqBody = compressedReq
				reqCtx.forceLog = true
				
				// 更新连接信息
				if connInfo != nil {
					connInfo.TransformedRequest = true
					connInfo.TransformedRequestBody = compressedReq
					connInfo.AppliedRequestTransformers = append(connInfo.AppliedRequestTransformers, "compress:"+compressHandler.Name)
				}
			}
		}
	}

	// 消息格式修正（在发送前）
	if sanitizer := h.transformerRegistry.GetMessageSanitizer(reqCtx.tags, h.logger); sanitizer != nil {
		sanitizedReq, sanitized, err := sanitizer.Sanitize(transformedReqBody)
		if err != nil {
			h.logger.Error("message sanitization failed", zap.Error(err))
		} else if sanitized {
			h.logger.Info("message sanitization applied")
			transformedReqBody = sanitizedReq
			reqCtx.transformedReqBody = sanitizedReq
			if connInfo != nil {
				connInfo.TransformedRequest = true
				connInfo.TransformedRequestBody = sanitizedReq
				connInfo.AppliedRequestTransformers = append(connInfo.AppliedRequestTransformers, "sanitize:"+sanitizer.Name())
			}
		}
	}

	// 去掉 gateway 路径前缀
	path := c.Request.URL.Path
	if h.gatewayPath != "" {
		path = strings.TrimPrefix(path, h.gatewayPath)
	}
	upstreamURL := h.upstreamURL + path
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, upstreamURL, bytes.NewBuffer(transformedReqBody))
	if err != nil {
		h.logger.Error("failed to create proxy request", zap.Error(err))
		reqCtx.meta.Error = err.Error()
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		h.updateConnectionStatus(connInfo, "error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Apply request header injections from transformers
	h.applyRequestHeaderInjections(proxyReq, tags, reqCtx)

	isStreaming := h.isStreamingRequest(reqBody)

	if isStreaming {
		h.handleStreamingResponse(c, proxyReq, reqCtx, startTime)
	} else {
		h.handleNormalResponse(c, proxyReq, reqCtx, startTime)
	}
}

func (h *Handler) isStreamingRequest(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	if stream, ok := req["stream"].(bool); ok {
		return stream
	}
	return false
}

// transformRequestBody transforms the request body, converting Droid tool definitions to upstream format
// Returns the transformed body and a list of applied transformer/mapping names
func (h *Handler) transformRequestBody(body []byte, tags []string) ([]byte, []string) {
	var appliedTransformers []string
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, appliedTransformers
	}

	transformed := false

	// Apply message inject transformers
	if h.transformerRegistry != nil {
		injectTransformers := h.transformerRegistry.GetMessageInjectTransformers(tags)
		for _, t := range injectTransformers {
			injectText := t.Def.InjectText
			// Replace {{tool}} placeholder with matched tool name
			if len(t.MatchedTools) > 0 {
				injectText = strings.ReplaceAll(injectText, "{{tool}}", t.MatchedTools[0])
			}
			if h.injectMessage(req, injectText, t.Def.InjectFormat) {
				transformed = true
				appliedTransformers = append(appliedTransformers, t.Def.Name)
				h.logger.Info("injected message", zap.String("transformer", t.Def.Name))
			}
		}
	}

	// Transform messages: replace "the ExitSpecMode tool" with "the create_documents tool" in system-reminder messages
	if messages, ok := req["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if content, ok := msgMap["content"].([]interface{}); ok {
					for _, c := range content {
						if cMap, ok := c.(map[string]interface{}); ok {
							if text, ok := cMap["text"].(string); ok {
								if strings.HasPrefix(text, "<system-reminder>") {
									newText := strings.ReplaceAll(text, "the ExitSpecMode tool", "the create_documents tool")
									if newText != text {
										cMap["text"] = newText
										transformed = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	tools, ok := req["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		if !transformed {
			return body, appliedTransformers
		}
		result, err := json.Marshal(req)
		if err != nil {
			return body, appliedTransformers
		}
		h.logger.Info("request messages transformed for upstream")
		return result, appliedTransformers
	}

	hasCreatePlan := false
	for i, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		if h.toolMapper.GetRequestMapping(name, tags) == nil {
			continue
		}

		transformedTool := h.toolMapper.TransformToolDefinition(toolMap, tags)
		tools[i] = transformedTool
		transformed = true

		// Get mapping name for logging
		mappingName := ""
		if h.transformerRegistry != nil {
			if cfg := h.transformerRegistry.GetRequestTransformer(name, tags); cfg != nil {
				mappingName = cfg.Name
				appliedTransformers = append(appliedTransformers, mappingName)
			}
		}
		log.Printf("[MAPPING] Request: %s -> %s (mapping: %s, tags: %v)", name, transformedTool["name"], mappingName, tags)

		// Check if this is ExitSpecMode -> create_plan transformation
		if name == "ExitSpecMode" {
			hasCreatePlan = true
		}
	}

	if !transformed {
		return body, appliedTransformers
	}

	req["tools"] = tools

	// If create_plan tool exists and no tool_choice is set, add tool_choice to hint the model
	if hasCreatePlan {
		if _, exists := req["tool_choice"]; !exists {
			req["tool_choice"] = map[string]interface{}{
				"type": "auto",
			}
			h.logger.Info("added tool_choice hint for create_plan")
		}
	}

	result, err := json.Marshal(req)
	if err != nil {
		return body, appliedTransformers
	}

	h.logger.Info("request tools transformed for upstream")
	return result, appliedTransformers
}

// isCompressRequest 检测是否为 compress/summarization 请求
func (h *Handler) isCompressRequest(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}

	checkSystemText := func(text string) bool {
		lower := strings.ToLower(text)
		return strings.Contains(lower, "produce a summary") ||
			strings.Contains(lower, "summarize the following") ||
			strings.Contains(lower, "create a summary")
	}

	// 字符串格式的 system
	if system, ok := req["system"].(string); ok {
		if checkSystemText(system) {
			return true
		}
	}

	// 数组格式的 system
	if systemArr, ok := req["system"].([]interface{}); ok {
		for _, item := range systemArr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok {
					if checkSystemText(text) {
						return true
					}
				}
			}
		}
	}

	return false
}

// shouldTransformToContextError 检查是否应该将错误转换为上下文超限错误
func (h *Handler) shouldTransformToContextError(reqCtx *requestContext, respStatus int, respBodyLen int) *transformer.TransformerDef {
	// 跳过压缩请求（避免死循环）
	if h.isCompressRequest(reqCtx.reqBody) {
		return nil
	}

	// 获取错误转换器
	if h.transformerRegistry == nil {
		return nil
	}
	handler := h.transformerRegistry.GetErrorTransformer(reqCtx.tags)
	if handler == nil {
		return nil
	}

	log.Printf("[ERROR_TRANSFORM] Request size info: transformer=%s request_size=%d threshold=%d response_status=%d response_body_len=%d",
		handler.Name, len(reqCtx.reqBody), handler.RequestSizeThreshold, respStatus, respBodyLen)

	// 检查请求大小是否超过阈值（排除图片 base64 数据）
	effectiveSize, _ := transformer.EstimateRequestSizeExcludingImages(
		reqCtx.reqBody, handler.ImageTokenEstimate, handler.TokenEstimateRatio)
	if handler.RequestSizeThreshold > 0 && effectiveSize >= handler.RequestSizeThreshold {
		return handler
	}

	return nil
}

// shouldTransformToPatternError 检查错误响应体是否匹配已配置的错误模式
func (h *Handler) shouldTransformToPatternError(reqCtx *requestContext, respStatus int, respBody []byte) *transformer.TransformerDef {
	// 只处理错误响应
	if respStatus < 400 {
		return nil
	}

	// 跳过压缩请求（避免死循环）
	if h.isCompressRequest(reqCtx.reqBody) {
		return nil
	}

	// 获取基于模式匹配的错误转换器
	if h.transformerRegistry == nil {
		return nil
	}

	handler := h.transformerRegistry.GetErrorPatternTransformer(reqCtx.tags, string(respBody))
	if handler != nil {
		log.Printf("[ERROR_TRANSFORM] Pattern matched: transformer=%s patterns=%v request_size=%d response_status=%d response_body_len=%d response_body_preview=%s",
			handler.Name, handler.ErrorPatterns, len(reqCtx.reqBody), respStatus, len(respBody), truncateString(string(respBody), 200))
	}
	return handler
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// writeContextLengthError 写入上下文超限错误响应
func (h *Handler) writeContextLengthError(c *gin.Context, handler *transformer.TransformerDef, reqSize int) {
	log.Printf("[ERROR_TRANSFORM] Writing context_length_exceeded error: transformer=%s request_size=%d error_code=%s error_message=%s",
		handler.Name, reqSize, handler.ErrorCode, handler.ErrorMessage)

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusBadRequest, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    handler.ErrorCode,
			"message": handler.ErrorMessage,
		},
	})
}

// isEmptyMessageStart 检查 message_start 事件是否表示空内容响应
func (h *Handler) isEmptyMessageStart(data string) bool {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return false
	}

	message, ok := payload["message"].(map[string]interface{})
	if !ok {
		return false
	}

	// 检查 content 是否为空数组
	content, ok := message["content"].([]interface{})
	if !ok {
		return false
	}
	if len(content) > 0 {
		return false
	}

	// 检查 output_tokens 是否为 0（增加准确性）
	if usage, ok := message["usage"].(map[string]interface{}); ok {
		if outputTokens, ok := usage["output_tokens"].(float64); ok {
			return outputTokens == 0
		}
	}

	return true
}

// shouldTransformEmptyStreamToContextError 检查空流式响应是否应该转换为上下文超限错误
func (h *Handler) shouldTransformEmptyStreamToContextError(reqCtx *requestContext) *transformer.TransformerDef {
	// 跳过压缩请求（避免死循环）
	if h.isCompressRequest(reqCtx.reqBody) {
		return nil
	}

	// 获取错误转换器
	if h.transformerRegistry == nil {
		return nil
	}
	handler := h.transformerRegistry.GetErrorTransformer(reqCtx.tags)
	if handler == nil {
		return nil
	}

	// 检查请求大小是否超过阈值
	if handler.RequestSizeThreshold > 0 && len(reqCtx.reqBody) >= handler.RequestSizeThreshold {
		return handler
	}

	return nil
}

// matchModelPattern 检查模型名是否匹配模式（支持通配符 *）
func matchModelPattern(model, pattern string) bool {
	if pattern == "" {
		return false
	}

	// 精确匹配
	if model == pattern {
		return true
	}

	// 通配符匹配
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}

	return false
}

// getContextTokenLimit 根据模型名获取对应的 token 限制
func getContextTokenLimit(handler *transformer.TransformerDef, model string) int {
	if model != "" && len(handler.ModelContextLimits) > 0 {
		for _, limit := range handler.ModelContextLimits {
			if matchModelPattern(model, limit.ModelPattern) {
				return limit.TokenLimit
			}
		}
	}
	// 返回默认值
	return handler.ContextTokenLimit
}

// shouldPreemptContextError 请求前预检测：基于 token 估算检查是否超过上下文限制
func (h *Handler) shouldPreemptContextError(reqCtx *requestContext) *transformer.TransformerDef {
	// 跳过压缩请求（避免死循环）
	if h.isCompressRequest(reqCtx.reqBody) {
		return nil
	}

	// 获取错误转换器
	if h.transformerRegistry == nil {
		return nil
	}
	handler := h.transformerRegistry.GetErrorTransformer(reqCtx.tags)
	if handler == nil {
		return nil
	}

	// 从请求体中提取模型名
	var req map[string]interface{}
	var model string
	if err := json.Unmarshal(reqCtx.reqBody, &req); err == nil {
		if m, ok := req["model"].(string); ok {
			model = m
		}
	}

	// 根据模型获取对应的 token 限制
	contextTokenLimit := getContextTokenLimit(handler, model)
	if contextTokenLimit == 0 {
		return nil
	}

	// 设置默认值
	ratio := handler.TokenEstimateRatio
	if ratio == 0 {
		ratio = 3.5
	}
	thresholdRatio := handler.ContextThresholdRatio
	if thresholdRatio == 0 {
		thresholdRatio = 0.85
	}

	// 估算 token 数（排除图片 base64 数据）
	effectiveSize, imageCount := transformer.EstimateRequestSizeExcludingImages(
		reqCtx.reqBody, handler.ImageTokenEstimate, ratio)
	if imageCount > 0 {
		log.Printf("[ERROR_TRANSFORM] Image-aware estimation: images=%d original_size=%d effective_size=%d",
			imageCount, len(reqCtx.reqBody), effectiveSize)
	}
	estimatedTokens := float64(effectiveSize) / ratio
	threshold := float64(contextTokenLimit) * thresholdRatio

	if estimatedTokens > threshold {
		log.Printf("[ERROR_TRANSFORM] Preemptive context limit check triggered: transformer=%s model=%s request_size=%d effective_size=%d estimated_tokens=%.0f threshold=%.0f context_token_limit=%d",
			handler.Name, model, len(reqCtx.reqBody), effectiveSize, estimatedTokens, threshold, contextTokenLimit)
		return handler
	}

	return nil
}

// shouldCompress 检测是否需要压缩上下文
func (h *Handler) shouldCompress(reqCtx *requestContext) *transformer.TransformerDef {
	// 跳过压缩请求（避免死循环）
	if h.isCompressRequest(reqCtx.reqBody) {
		return nil
	}

	// 获取压缩转换器
	if h.transformerRegistry == nil {
		return nil
	}
	handler := h.transformerRegistry.GetCompressTransformer(reqCtx.tags)
	if handler == nil {
		return nil
	}

	// 从请求体中提取模型名
	var req map[string]interface{}
	var model string
	if err := json.Unmarshal(reqCtx.reqBody, &req); err == nil {
		if m, ok := req["model"].(string); ok {
			model = m
		}
	}

	// 使用压缩处理器的检测逻辑
	compressor := transformer.NewCompressHandler(handler, h.logger, h.getGatewayMap())
	if compressor.ShouldCompress(reqCtx.reqBody, model) {
		return handler
	}

	return nil
}

// buildCompressTargetURL 构建压缩目标 URL
func (h *Handler) buildCompressTargetURL(handler *transformer.TransformerDef) (string, error) {
	target := handler.CompressTarget
	if target == "" || target == "same" {
		// 使用当前 gateway 的上游 URL
		return h.upstreamURL + "/v1/messages", nil
	}

	// gateway:name 格式
	if strings.HasPrefix(target, "gateway:") {
		gatewayName := strings.TrimPrefix(target, "gateway:")
		gatewayMap := h.getGatewayMap()
		if url, ok := gatewayMap[gatewayName]; ok {
			return url + "/v1/messages", nil
		}
		return "", fmt.Errorf("gateway not found: %s", gatewayName)
	}

	// url:https://... 格式
	if strings.HasPrefix(target, "url:") {
		return strings.TrimPrefix(target, "url:"), nil
	}

	return "", fmt.Errorf("invalid compress target: %s", target)
}

// getGatewayMap 获取 gateway 名称到 URL 的映射
func (h *Handler) getGatewayMap() map[string]string {
	// 这里需要从配置中获取，暂时返回空 map
	// TODO: 从全局配置中获取 gateway 映射
	return make(map[string]string)
}

// countUserMessages counts the number of user role messages in a request body.
func countUserMessages(body []byte) int {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0
	}
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return 0
	}
	count := 0
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		if role, _ := msgMap["role"].(string); role == "user" {
			count++
		}
	}
	return count
}

// injectMessage injects text into the first user message's content
func (h *Handler) injectMessage(req map[string]interface{}, text string, format string) bool {
	if text == "" {
		return false
	}

	messages, ok := req["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return false
	}

	// Format the inject text
	var injectText string
	switch format {
	case "system-reminder":
		injectText = "<system-reminder>\n" + text + "\n</system-reminder>"
	case "plain":
		injectText = text
	default:
		injectText = "<system-reminder>\n" + text + "\n</system-reminder>"
	}

	// Find the first user message and inject at the beginning of its content
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		if role != "user" {
			continue
		}

		content, ok := msgMap["content"].([]interface{})
		if !ok {
			// content might be a string
			if contentStr, ok := msgMap["content"].(string); ok {
				// Convert to array format and prepend inject text
				msgMap["content"] = []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": injectText,
					},
					map[string]interface{}{
						"type": "text",
						"text": contentStr,
					},
				}
				return true
			}
			continue
		}

		// Prepend inject text to content array
		newContent := make([]interface{}, 0, len(content)+1)
		newContent = append(newContent, map[string]interface{}{
			"type": "text",
			"text": injectText,
		})
		newContent = append(newContent, content...)
		msgMap["content"] = newContent
		return true
	}

	return false
}

// applyRequestHeaderInjections applies header injections from request transformers
func (h *Handler) applyRequestHeaderInjections(proxyReq *http.Request, tags []string, reqCtx *requestContext) {
	if h.transformerRegistry == nil {
		return
	}

	// Get header_inject type transformers
	headerInjectTransformers := h.transformerRegistry.GetHeaderInjectTransformers("request", tags)
	for _, def := range headerInjectTransformers {
		for _, header := range def.HeaderInjections {
			value := h.replaceHeaderPlaceholders(header.Value, reqCtx)
			proxyReq.Header.Set(header.Key, value)
			h.logger.Info("injected request header",
				zap.String("transformer", def.Name),
				zap.String("key", header.Key),
				zap.String("value", value))
		}
		if reqCtx.appliedTransformers == nil {
			reqCtx.appliedTransformers = []string{}
		}
		reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, def.Name)
	}

	// Also support header_injections in other transformer types (backward compatibility)
	mappings := h.transformerRegistry.GetMappings()
	definitions := h.transformerRegistry.GetDefinitions()

	for _, mapping := range mappings {
		if !mapping.Enabled {
			continue
		}
		if !h.matchMappingTags(mapping.Tags, tags) {
			continue
		}

		// Find the transformer definition
		for _, def := range definitions {
			if def.Name == mapping.Transformer && def.Direction == "request" && def.Type != "header_inject" && len(def.HeaderInjections) > 0 {
				for _, header := range def.HeaderInjections {
					value := h.replaceHeaderPlaceholders(header.Value, reqCtx)
					proxyReq.Header.Set(header.Key, value)
					h.logger.Info("injected request header",
						zap.String("transformer", def.Name),
						zap.String("key", header.Key),
						zap.String("value", value))
				}
				if reqCtx.appliedTransformers == nil {
					reqCtx.appliedTransformers = []string{}
				}
				reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, def.Name)
			}
		}
	}
}

// applyResponseHeaderInjections applies header injections from response transformers
func (h *Handler) applyResponseHeaderInjections(c *gin.Context, tags []string, reqCtx *requestContext) {
	if h.transformerRegistry == nil {
		return
	}

	// Get header_inject type transformers
	headerInjectTransformers := h.transformerRegistry.GetHeaderInjectTransformers("response", tags)
	for _, def := range headerInjectTransformers {
		for _, header := range def.HeaderInjections {
			value := h.replaceHeaderPlaceholders(header.Value, reqCtx)
			c.Header(header.Key, value)
			h.logger.Info("injected response header",
				zap.String("transformer", def.Name),
				zap.String("key", header.Key),
				zap.String("value", value))
		}
		if reqCtx.appliedTransformers == nil {
			reqCtx.appliedTransformers = []string{}
		}
		reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, def.Name)
	}

	// Also support header_injections in other transformer types (backward compatibility)
	mappings := h.transformerRegistry.GetMappings()
	definitions := h.transformerRegistry.GetDefinitions()

	for _, mapping := range mappings {
		if !mapping.Enabled {
			continue
		}
		if !h.matchMappingTags(mapping.Tags, tags) {
			continue
		}

		// Find the transformer definition
		for _, def := range definitions {
			if def.Name == mapping.Transformer && def.Direction == "response" && def.Type != "header_inject" && len(def.HeaderInjections) > 0 {
				for _, header := range def.HeaderInjections {
					value := h.replaceHeaderPlaceholders(header.Value, reqCtx)
					c.Header(header.Key, value)
					h.logger.Info("injected response header",
						zap.String("transformer", def.Name),
						zap.String("key", header.Key),
						zap.String("value", value))
				}
				if reqCtx.appliedTransformers == nil {
					reqCtx.appliedTransformers = []string{}
				}
				reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, def.Name)
			}
		}
	}
}

// matchMappingTags checks if all mapping tags are present in request tags
func (h *Handler) matchMappingTags(mappingTags, requestTags []string) bool {
	if len(mappingTags) == 0 {
		return true
	}

	tagSet := make(map[string]bool)
	for _, t := range requestTags {
		tagSet[t] = true
	}

	for _, t := range mappingTags {
		if !tagSet[t] {
			return false
		}
	}
	return true
}

// replaceHeaderPlaceholders replaces placeholders in header values
func (h *Handler) replaceHeaderPlaceholders(value string, reqCtx *requestContext) string {
	result := value
	result = strings.ReplaceAll(result, "{{session_id}}", reqCtx.meta.SessionID)
	result = strings.ReplaceAll(result, "{{request_id}}", reqCtx.meta.ID)
	result = strings.ReplaceAll(result, "{{gateway}}", h.gatewayName)
	return result
}

func (h *Handler) handleNormalResponse(c *gin.Context, proxyReq *http.Request, reqCtx *requestContext, startTime time.Time) {
	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		h.logger.Error("upstream request failed", zap.Error(err))
		reqCtx.meta.Error = err.Error()
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = true // 请求失败，强制记录
		h.updateConnectionStatus(reqCtx.connInfo, "error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("failed to read response body", zap.Error(err))
		reqCtx.meta.Error = err.Error()
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = true // 读取失败，强制记录
		h.updateConnectionStatus(reqCtx.connInfo, "error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response body"})
		return
	}

	reqCtx.meta.Response = &ResponseMeta{
		Status:  resp.StatusCode,
		Headers: sanitizeHeaders(resp.Header),
	}
	decompressedBody := decompressIfNeeded(respBody, resp.Header)
	reqCtx.respBody = decompressedBody
	reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()

	// 设置强制记录标记（非200响应或200但body为空）
	reqCtx.forceLog = h.shouldForceLog(resp.StatusCode, len(decompressedBody), false)

	// 检查是否需要转换为上下文超限错误
	if handler := h.shouldTransformToContextError(reqCtx, resp.StatusCode, len(decompressedBody)); handler != nil {
		h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
		return
	}

	// 检查错误响应体是否匹配已配置的错误模式（如 input too long）
	if handler := h.shouldTransformToPatternError(reqCtx, resp.StatusCode, decompressedBody); handler != nil {
		h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
		return
	}

	// Update memory store
	if reqCtx.connInfo != nil && h.memoryStore != nil {
		h.memoryStore.Update(reqCtx.connInfo.ID, func(c *memory.ConnectionInfo) {
			c.Status = "completed"
			c.DurationMs = reqCtx.meta.DurationMs
			c.EndTime = time.Now()
			c.ResponseStatus = resp.StatusCode
			c.ResponseHeaders = flattenHeaders(resp.Header)
			c.ResponseBody = decompressedBody
		})
		if h.wsHub != nil {
			h.wsHub.Broadcast(reqCtx.connInfo)
		}
	}

	h.saveLog(reqCtx)

	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Apply response header injections
	h.applyResponseHeaderInjections(c, reqCtx.tags, reqCtx)

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

type toolBlockState struct {
	index            int
	toolID           string
	toolName         string
	inputParts       []string
	needsTransform   bool
	needsAccumulate  bool
	pendingTransform bool // Has ParamConditions, needs deferred evaluation
}

type toolBlockAccumulator struct {
	toolName     string
	firstToolID  string
	fullInput    map[string]interface{} // Store complete input for param mapping
	titleParts   []string               // Legacy: for create_documents compatibility
	contentParts []string               // Legacy: for create_documents compatibility
	blockCount   int
}

func (h *Handler) handleStreamingResponse(c *gin.Context, proxyReq *http.Request, reqCtx *requestContext, startTime time.Time) {
	client := &http.Client{
		Timeout: 0,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		h.logger.Error("upstream streaming request failed", zap.Error(err))
		reqCtx.meta.Error = err.Error()
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = true // 请求失败，强制记录
		h.updateConnectionStatus(reqCtx.connInfo, "error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	// 检查非 2xx 响应是否需要转换为上下文超限错误
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		reqCtx.meta.Response = &ResponseMeta{
			Status:  resp.StatusCode,
			Headers: sanitizeHeaders(resp.Header),
		}
		reqCtx.respBody = respBody
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = h.shouldForceLog(resp.StatusCode, len(respBody), false) // 设置强制记录标记

		if handler := h.shouldTransformToContextError(reqCtx, resp.StatusCode, len(respBody)); handler != nil {
			h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
			h.saveLog(reqCtx)
			h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
			return
		}

		// 检查错误响应体是否匹配已配置的错误模式（如 input too long）
		if handler := h.shouldTransformToPatternError(reqCtx, resp.StatusCode, respBody); handler != nil {
			h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
			h.saveLog(reqCtx)
			h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
			return
		}

		// 不需要转换，透传原始错误
		h.updateConnectionStatus(reqCtx.connInfo, "error", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		for key, values := range resp.Header {
			for _, value := range values {
				c.Header(key, value)
			}
		}

		// Apply response header injections
		h.applyResponseHeaderInjections(c, reqCtx.tags, reqCtx)

		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}

	// 检查成功响应是否也需要转换（基于请求大小阈值）
	if handler := h.shouldTransformToContextError(reqCtx, resp.StatusCode, 0); handler != nil {
		// 读取部分响应用于日志
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		reqCtx.meta.Response = &ResponseMeta{
			Status:  resp.StatusCode,
			Headers: sanitizeHeaders(resp.Header),
		}
		reqCtx.respBody = preview
		reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
		reqCtx.forceLog = true
		h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
		h.saveLog(reqCtx)
		h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
		return
	}

	// === 只读取第一个事件进行检测 ===
	var originalBuffer bytes.Buffer
	teeReader := io.TeeReader(resp.Body, &originalBuffer)
	reader := bufio.NewReader(teeReader)

	// 读取第一个事件
	type rawEvent struct {
		eventType string
		data      string
	}
	var firstEvent *rawEvent

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSuffix(line, "\n")

		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			dataLine, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				break
			}
			dataLine = strings.TrimSuffix(dataLine, "\n")

			if strings.HasPrefix(dataLine, "data: ") {
				data := strings.TrimPrefix(dataLine, "data: ")
				firstEvent = &rawEvent{eventType: eventType, data: data}
				// 读取空行分隔符
				_, _ = reader.ReadString('\n')
				break
			}
		}
	}

	// === 检测 message_start 事件中的空内容 ===
	isEmptyContent := false
	if firstEvent != nil && firstEvent.eventType == "message_start" {
		isEmptyContent = h.isEmptyMessageStart(firstEvent.data)
	}

	// 如果是空内容且满足转换条件，返回上下文超限错误
	if isEmptyContent {
		if handler := h.shouldTransformEmptyStreamToContextError(reqCtx); handler != nil {
			// 读取剩余内容用于日志
			remaining, _ := io.ReadAll(reader)
			originalBuffer.Write(remaining)

			reqCtx.meta.Response = &ResponseMeta{
				Status:  resp.StatusCode,
				Headers: sanitizeHeaders(resp.Header),
			}
			reqCtx.respBody = originalBuffer.Bytes()
			reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()
			reqCtx.forceLog = true
			h.updateConnectionStatus(reqCtx.connInfo, "error_transformed", reqCtx.meta.DurationMs)
			h.saveLog(reqCtx)
			log.Printf("[ERROR_TRANSFORM] Empty stream response (message_start with empty content): transformer=%s request_size=%d",
				handler.Name, len(reqCtx.reqBody))
			h.writeContextLengthError(c, handler, len(reqCtx.reqBody))
			return
		}
	}

	// === 正常流式处理：设置响应头 ===
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Apply response header injections for streaming
	h.applyResponseHeaderInjections(c, reqCtx.tags, reqCtx)

	c.Status(resp.StatusCode)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.logger.Error("streaming not supported")
		return
	}

	var transformedBuffer bytes.Buffer
	toolBlocks := make(map[int]*toolBlockState)
	var accumulator *toolBlockAccumulator
	var pendingMessageDelta *sseEvent
	nextOutputIndex := 0
	hasContentBlock := false

	// Initialize content replacer definitions for text blocks
	var contentReplacerDefs []*transformer.TransformerDef
	if h.transformerRegistry != nil {
		contentReplacerDefs = h.transformerRegistry.GetContentReplacerDefs(reqCtx.tags)
	}
	textBlockReplacers := make(map[int][]*transformer.ContentReplacer)

	// Initialize ad remover definitions (only active on first turn)
	var adRemoverDefs []*transformer.TransformerDef
	if reqCtx.isFirstTurn && h.transformerRegistry != nil {
		adRemoverDefs = h.transformerRegistry.GetAdRemoverDefs(reqCtx.tags)
	}
	textBlockAdRemovers := make(map[int][]*transformer.AdRemover)

	writeEvents := func(events []sseEvent) {
		for _, evt := range events {
			outLine := fmt.Sprintf("event: %s\ndata: %s\n\n", evt.eventType, evt.data)
			transformedBuffer.WriteString(outLine)
			c.Writer.WriteString(outLine)
			flusher.Flush()
		}
	}

	// 先处理已读取的第一个事件
	if firstEvent != nil {
		if firstEvent.eventType == "content_block_start" {
			hasContentBlock = true
		}
		outputEvents := h.processSSEEvent(firstEvent.eventType, firstEvent.data, toolBlocks, &nextOutputIndex, &accumulator, &pendingMessageDelta, writeEvents, reqCtx.tags, reqCtx, contentReplacerDefs, textBlockReplacers, adRemoverDefs, textBlockAdRemovers)
		writeEvents(outputEvents)
	}

	// 继续实时处理剩余事件
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				h.logger.Error("error reading stream", zap.Error(err))
				reqCtx.meta.Error = err.Error()
			}
			if accumulator != nil && accumulator.blockCount > 0 {
				flushEvents := h.flushAccumulator(accumulator, &nextOutputIndex, reqCtx.tags, reqCtx)
				writeEvents(flushEvents)
			}
			break
		}

		line = strings.TrimSuffix(line, "\n")

		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			dataLine, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				break
			}
			dataLine = strings.TrimSuffix(dataLine, "\n")

			if strings.HasPrefix(dataLine, "data: ") {
				data := strings.TrimPrefix(dataLine, "data: ")

				if eventType == "content_block_start" {
					hasContentBlock = true
				}

				outputEvents := h.processSSEEvent(eventType, data, toolBlocks, &nextOutputIndex, &accumulator, &pendingMessageDelta, writeEvents, reqCtx.tags, reqCtx, contentReplacerDefs, textBlockReplacers, adRemoverDefs, textBlockAdRemovers)
				writeEvents(outputEvents)
			}
			_, _ = reader.ReadString('\n')
		} else if line == "" {
			// skip
		} else {
			transformedBuffer.WriteString(line + "\n")
			c.Writer.WriteString(line + "\n")
			flusher.Flush()
		}
	}

	reqCtx.meta.Response = &ResponseMeta{
		Status:  resp.StatusCode,
		Headers: sanitizeHeaders(resp.Header),
	}
	reqCtx.respBody = originalBuffer.Bytes()
	reqCtx.meta.DurationMs = time.Since(startTime).Milliseconds()

	// 设置强制记录标记（非200响应或200但body为空或流式响应无内容块）
	reqCtx.forceLog = h.shouldForceLog(resp.StatusCode, originalBuffer.Len(), !hasContentBlock)

	// Update memory store
	if reqCtx.connInfo != nil && h.memoryStore != nil {
		h.memoryStore.Update(reqCtx.connInfo.ID, func(c *memory.ConnectionInfo) {
			c.Status = "completed"
			c.DurationMs = reqCtx.meta.DurationMs
			c.EndTime = time.Now()
			c.ResponseStatus = resp.StatusCode
			c.ResponseHeaders = flattenHeaders(resp.Header)
			c.ResponseBody = originalBuffer.Bytes()
			// Update applied transformers from response processing
			if len(reqCtx.appliedTransformers) > 0 {
				c.AppliedResponseTransformers = append(c.AppliedResponseTransformers, reqCtx.appliedTransformers...)
				c.TransformedResponse = true
				c.TransformedResponseBody = transformedBuffer.Bytes()
			}
		})
		if h.wsHub != nil {
			h.wsHub.Broadcast(reqCtx.connInfo)
		}
	}

	h.saveLog(reqCtx)

	// Always save transformed response for debugging
	if transformedBuffer.Len() > 0 {
		h.saveTransformedResponse(reqCtx, transformedBuffer.Bytes())
	}
}

type sseEvent struct {
	eventType string
	data      string
}

func (h *Handler) processSSEEvent(eventType, data string, toolBlocks map[int]*toolBlockState, nextOutputIndex *int, accumulator **toolBlockAccumulator, pendingMessageDelta **sseEvent, writeEvents func([]sseEvent), tags []string, reqCtx *requestContext, contentReplacerDefs []*transformer.TransformerDef, textBlockReplacers map[int][]*transformer.ContentReplacer, adRemoverDefs []*transformer.TransformerDef, textBlockAdRemovers map[int][]*transformer.AdRemover) []sseEvent {
	switch eventType {
	case "content_block_start":
		return h.handleBlockStart(data, toolBlocks, nextOutputIndex, accumulator, writeEvents, tags, reqCtx, contentReplacerDefs, textBlockReplacers, adRemoverDefs, textBlockAdRemovers)
	case "content_block_delta":
		return h.handleBlockDelta(data, toolBlocks, accumulator, tags, textBlockReplacers, textBlockAdRemovers)
	case "content_block_stop":
		return h.handleBlockStop(data, toolBlocks, accumulator, nextOutputIndex, tags, reqCtx, textBlockReplacers, textBlockAdRemovers)
	case "message_delta":
		// Cache message_delta, send it before message_stop
		*pendingMessageDelta = &sseEvent{eventType: eventType, data: data}
		return nil
	case "message_stop":
		// Flush accumulator before message_stop
		if *accumulator != nil && (*accumulator).blockCount > 0 {
			flushEvents := h.flushAccumulator(*accumulator, nextOutputIndex, tags, reqCtx)
			writeEvents(flushEvents)
			*accumulator = nil
		}
		// Send cached message_delta before message_stop
		var result []sseEvent
		if *pendingMessageDelta != nil {
			result = append(result, **pendingMessageDelta)
			*pendingMessageDelta = nil
		}
		result = append(result, sseEvent{eventType: eventType, data: data})
		return result
	default:
		return []sseEvent{{eventType: eventType, data: data}}
	}
}

func (h *Handler) handleBlockStart(data string, toolBlocks map[int]*toolBlockState, nextOutputIndex *int, accumulator **toolBlockAccumulator, writeEvents func([]sseEvent), tags []string, reqCtx *requestContext, contentReplacerDefs []*transformer.TransformerDef, textBlockReplacers map[int][]*transformer.ContentReplacer, adRemoverDefs []*transformer.TransformerDef, textBlockAdRemovers map[int][]*transformer.AdRemover) []sseEvent {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return []sseEvent{{eventType: "content_block_start", data: data}}
	}

	indexFloat, ok := payload["index"].(float64)
	if !ok {
		return []sseEvent{{eventType: "content_block_start", data: data}}
	}
	index := int(indexFloat)

	contentBlock, ok := payload["content_block"].(map[string]interface{})
	if !ok {
		return []sseEvent{{eventType: "content_block_start", data: data}}
	}

	blockType, _ := contentBlock["type"].(string)
	if blockType != "tool_use" {
		// Non-tool block: flush accumulator first if exists
		if *accumulator != nil && (*accumulator).blockCount > 0 {
			flushEvents := h.flushAccumulator(*accumulator, nextOutputIndex, tags, reqCtx)
			writeEvents(flushEvents)
			*accumulator = nil
		}
		// Register content replacers for text blocks (fresh instances per block)
		if blockType == "text" && len(contentReplacerDefs) > 0 {
			var replacers []*transformer.ContentReplacer
			for _, def := range contentReplacerDefs {
				replacers = append(replacers, transformer.NewContentReplacer(def, h.logger))
			}
			textBlockReplacers[index] = replacers
		}
		if blockType == "text" && len(adRemoverDefs) > 0 {
			var removers []*transformer.AdRemover
			for _, def := range adRemoverDefs {
				removers = append(removers, transformer.NewAdRemover(def, h.logger))
			}
			textBlockAdRemovers[index] = removers
		}
		// Increment output index for passthrough block
		*nextOutputIndex++
		return []sseEvent{{eventType: "content_block_start", data: data}}
	}

	toolName, _ := contentBlock["name"].(string)
	toolID, _ := contentBlock["id"].(string)

	needsTransform := h.toolMapper.NeedsTransform(toolName, tags)
	needsAccumulate := h.toolMapper.NeedsAccumulate(toolName, tags)
	pendingTransform := h.toolMapper.HasPendingTransform(toolName, tags)

	toolBlocks[index] = &toolBlockState{
		index:            index,
		toolID:           toolID,
		toolName:         toolName,
		inputParts:       []string{},
		needsTransform:   needsTransform,
		needsAccumulate:  needsAccumulate,
		pendingTransform: pendingTransform,
	}

	if needsTransform && needsAccumulate {
		// Accumulate type: Reset accumulator for each new block
		// Only the last complete block will be kept (upstream sends fragments first, then full content)
		*accumulator = &toolBlockAccumulator{
			toolName:     toolName,
			firstToolID:  toolID,
			titleParts:   []string{},
			contentParts: []string{},
			blockCount:   0,
		}
		// Suppress output, will accumulate
		return nil
	}

	if pendingTransform {
		// Has ParamConditions that need deferred evaluation with complete input
		// Suppress output, collect input, evaluate in handleBlockStop
		return nil
	}

	if needsTransform && !needsAccumulate {
		// Simple transform type: output transformed tool name immediately
		targetToolName := h.toolMapper.TransformToolName(toolName, tags)
		contentBlock["name"] = targetToolName
		payload["content_block"] = contentBlock
		transformedData, _ := json.Marshal(payload)
		// Increment output index for transformed block
		*nextOutputIndex++

		// Get mapping name for logging
		mappingName := ""
		if h.transformerRegistry != nil {
			if cfg := h.transformerRegistry.GetResponseTransformer(toolName, tags); cfg != nil {
				mappingName = cfg.Name
				if reqCtx != nil {
					reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, mappingName)
				}
			}
		}
		log.Printf("[MAPPING] Response: %s -> %s (mapping: %s, tags: %v)", toolName, targetToolName, mappingName, tags)

		return []sseEvent{{eventType: "content_block_start", data: string(transformedData)}}
	}

	// Non-transformable tool: flush accumulator first
	if *accumulator != nil && (*accumulator).blockCount > 0 {
		flushEvents := h.flushAccumulator(*accumulator, nextOutputIndex, tags, reqCtx)
		writeEvents(flushEvents)
		*accumulator = nil
	}

	// Increment output index for passthrough block
	*nextOutputIndex++
	return []sseEvent{{eventType: "content_block_start", data: data}}
}

func (h *Handler) handleBlockDelta(data string, toolBlocks map[int]*toolBlockState, accumulator **toolBlockAccumulator, tags []string, textBlockReplacers map[int][]*transformer.ContentReplacer, textBlockAdRemovers map[int][]*transformer.AdRemover) []sseEvent {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return []sseEvent{{eventType: "content_block_delta", data: data}}
	}

	indexFloat, ok := payload["index"].(float64)
	if !ok {
		return []sseEvent{{eventType: "content_block_delta", data: data}}
	}
	index := int(indexFloat)

	// Apply content replacers and ad removers for text_delta
	replacers := textBlockReplacers[index]
	adRemovers := textBlockAdRemovers[index]
	if len(replacers) > 0 || len(adRemovers) > 0 {
		delta, ok := payload["delta"].(map[string]interface{})
		if ok {
			if text, ok := delta["text"].(string); ok {
				for _, cr := range replacers {
					text = cr.Process(text)
				}
				for _, ar := range adRemovers {
					text = ar.Process(text)
				}
				if text == "" {
					return nil // suppress empty delta
				}
				delta["text"] = text
				payload["delta"] = delta
				transformedData, _ := json.Marshal(payload)
				return []sseEvent{{eventType: "content_block_delta", data: string(transformedData)}}
			}
		}
	}

	block, exists := toolBlocks[index]
	if !exists || (!block.needsTransform && !block.pendingTransform) {
		return []sseEvent{{eventType: "content_block_delta", data: data}}
	}

	delta, ok := payload["delta"].(map[string]interface{})
	if !ok {
		return []sseEvent{{eventType: "content_block_delta", data: data}}
	}

	if partialJSON, ok := delta["partial_json"].(string); ok {
		block.inputParts = append(block.inputParts, partialJSON)

		if block.needsAccumulate {
			// Accumulate type: just collect inputParts, extraction happens in handleBlockStop
			// Suppress delta for accumulate type
			return nil
		}

		if block.pendingTransform {
			// Pending transform: collect input, suppress delta, evaluate in handleBlockStop
			return nil
		}

		// Simple transform type: transform the JSON params inline
		transformedJSON, err := h.toolMapper.TransformInputJSON(block.toolName, partialJSON, tags)
		if err == nil && transformedJSON != partialJSON {
			delta["partial_json"] = transformedJSON
			payload["delta"] = delta
			transformedData, _ := json.Marshal(payload)
			return []sseEvent{{eventType: "content_block_delta", data: string(transformedData)}}
		}
	}

	// Simple transform type: pass through delta as-is
	return []sseEvent{{eventType: "content_block_delta", data: data}}
}

func (h *Handler) handleBlockStop(data string, toolBlocks map[int]*toolBlockState, accumulator **toolBlockAccumulator, nextOutputIndex *int, tags []string, reqCtx *requestContext, textBlockReplacers map[int][]*transformer.ContentReplacer, textBlockAdRemovers map[int][]*transformer.AdRemover) []sseEvent {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return []sseEvent{{eventType: "content_block_stop", data: data}}
	}

	indexFloat, ok := payload["index"].(float64)
	if !ok {
		return []sseEvent{{eventType: "content_block_stop", data: data}}
	}
	index := int(indexFloat)

	// Clean up text block replacers and ad removers
	delete(textBlockReplacers, index)
	delete(textBlockAdRemovers, index)

	block, exists := toolBlocks[index]
	if !exists {
		return []sseEvent{{eventType: "content_block_stop", data: data}}
	}

	// For pending transform tools, evaluate with complete input
	if block.pendingTransform && len(block.inputParts) > 0 {
		fullJSON := strings.Join(block.inputParts, "")
		var input map[string]interface{}
		if err := json.Unmarshal([]byte(fullJSON), &input); err == nil {
			// Check if any transformer with ParamConditions matches
			if h.toolMapper.NeedsTransformWithInput(block.toolName, tags, input) {
				// Transform matched! Generate complete transformed block
				targetToolName := h.toolMapper.TransformToolNameWithInput(block.toolName, tags, input)
				transformedInput := h.toolMapper.TransformInputWithInput(block.toolName, input, tags, input)
				transformedInputJSON, _ := json.Marshal(transformedInput)

				outputIndex := *nextOutputIndex
				*nextOutputIndex++

				// Emit complete transformed tool block
				startData := fmt.Sprintf(`{"content_block":{"id":"%s","input":{},"name":"%s","type":"tool_use"},"index":%d,"type":"content_block_start"}`,
					block.toolID, targetToolName, outputIndex)
				deltaData := fmt.Sprintf(`{"delta":{"partial_json":%s,"type":"input_json_delta"},"index":%d,"type":"content_block_delta"}`,
					jsonEscape(string(transformedInputJSON)), outputIndex)
				stopData := fmt.Sprintf(`{"index":%d,"type":"content_block_stop"}`, outputIndex)

				// Log the transformation
				mappingName := ""
				if h.transformerRegistry != nil {
					if cfg := h.transformerRegistry.GetResponseTransformerWithInput(block.toolName, tags, input); cfg != nil {
						mappingName = cfg.Name
						if reqCtx != nil {
							reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, mappingName)
						}
					}
				}
				log.Printf("[MAPPING] Response (pending): %s -> %s (mapping: %s, tags: %v)", block.toolName, targetToolName, mappingName, tags)

				delete(toolBlocks, index)
				return []sseEvent{
					{eventType: "content_block_start", data: startData},
					{eventType: "content_block_delta", data: deltaData},
					{eventType: "content_block_stop", data: stopData},
				}
			}
		}
		// ParamConditions not matched, try fallback to unconditional transformer (e.g., Write -> Create)
		if h.toolMapper.NeedsTransform(block.toolName, tags) {
			targetToolName := h.toolMapper.TransformToolName(block.toolName, tags)
			transformedInput := h.toolMapper.TransformInput(block.toolName, input, tags)
			transformedInputJSON, _ := json.Marshal(transformedInput)

			outputIndex := *nextOutputIndex
			*nextOutputIndex++

			startData := fmt.Sprintf(`{"content_block":{"id":"%s","input":{},"name":"%s","type":"tool_use"},"index":%d,"type":"content_block_start"}`,
				block.toolID, targetToolName, outputIndex)
			deltaData := fmt.Sprintf(`{"delta":{"partial_json":%s,"type":"input_json_delta"},"index":%d,"type":"content_block_delta"}`,
				jsonEscape(string(transformedInputJSON)), outputIndex)
			stopData := fmt.Sprintf(`{"index":%d,"type":"content_block_stop"}`, outputIndex)

			// Log the fallback transformation
			mappingName := ""
			if h.transformerRegistry != nil {
				if cfg := h.transformerRegistry.GetResponseTransformer(block.toolName, tags); cfg != nil {
					mappingName = cfg.Name
					if reqCtx != nil {
						reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, mappingName)
					}
				}
			}
			log.Printf("[MAPPING] Response (fallback): %s -> %s (mapping: %s, tags: %v)", block.toolName, targetToolName, mappingName, tags)

			delete(toolBlocks, index)
			return []sseEvent{
				{eventType: "content_block_start", data: startData},
				{eventType: "content_block_delta", data: deltaData},
				{eventType: "content_block_stop", data: stopData},
			}
		}

		// No transform matched at all, output original block
		outputIndex := *nextOutputIndex
		*nextOutputIndex++

		startData := fmt.Sprintf(`{"content_block":{"id":"%s","input":{},"name":"%s","type":"tool_use"},"index":%d,"type":"content_block_start"}`,
			block.toolID, block.toolName, outputIndex)
		deltaData := fmt.Sprintf(`{"delta":{"partial_json":%s,"type":"input_json_delta"},"index":%d,"type":"content_block_delta"}`,
			jsonEscape(fullJSON), outputIndex)
		stopData := fmt.Sprintf(`{"index":%d,"type":"content_block_stop"}`, outputIndex)

		delete(toolBlocks, index)
		return []sseEvent{
			{eventType: "content_block_start", data: startData},
			{eventType: "content_block_delta", data: deltaData},
			{eventType: "content_block_stop", data: stopData},
		}
	}

	// For accumulate type tools, merge all inputParts and parse complete JSON at block end
	if block.needsAccumulate && *accumulator != nil && len(block.inputParts) > 0 {
		fullJSON := strings.Join(block.inputParts, "")

		var input map[string]interface{}
		if err := json.Unmarshal([]byte(fullJSON), &input); err == nil {
			// Extract new_documents[0].title and new_documents[0].content
			if newDocs, ok := input["new_documents"].([]interface{}); ok && len(newDocs) > 0 {
				if doc, ok := newDocs[0].(map[string]interface{}); ok {
					if title, ok := doc["title"].(string); ok {
						(*accumulator).titleParts = append((*accumulator).titleParts, title)
					}
					if content, ok := doc["content"].(string); ok {
						(*accumulator).contentParts = append((*accumulator).contentParts, content)
					}
				}
			}
		}
		(*accumulator).blockCount++
	}

	needsAccumulate := block.needsAccumulate
	delete(toolBlocks, index)

	if needsAccumulate {
		// Suppress, output will be generated when accumulator is flushed
		return nil
	}

	return []sseEvent{{eventType: "content_block_stop", data: data}}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (h *Handler) extractAndAccumulate(partialJSON string, acc *toolBlockAccumulator) {
	// Try to parse as JSON object to extract title/content
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(partialJSON), &obj); err != nil {
		// Not valid JSON, might be a fragment - try to extract values directly
		// Handle patterns like {"title":"xxx"} or {"content":"xxx"} or {"new_documents":[{"content":"xxx"}]}
		if strings.Contains(partialJSON, `"title"`) {
			if title := extractJSONStringValue(partialJSON, "title"); title != "" {
				acc.titleParts = append(acc.titleParts, title)
			}
		}
		if strings.Contains(partialJSON, `"content"`) {
			if content := extractJSONStringValue(partialJSON, "content"); content != "" {
				acc.contentParts = append(acc.contentParts, content)
			}
		}
		return
	}

	// Valid JSON object
	if title, ok := obj["title"].(string); ok {
		acc.titleParts = append(acc.titleParts, title)
	}
	if content, ok := obj["content"].(string); ok {
		acc.contentParts = append(acc.contentParts, content)
	}
	// Handle new_documents array structure
	if newDocs, ok := obj["new_documents"].([]interface{}); ok {
		for _, doc := range newDocs {
			if docMap, ok := doc.(map[string]interface{}); ok {
				if title, ok := docMap["title"].(string); ok {
					acc.titleParts = append(acc.titleParts, title)
				}
				if content, ok := docMap["content"].(string); ok {
					acc.contentParts = append(acc.contentParts, content)
				}
			}
		}
	}
	acc.blockCount++
}

func extractJSONStringValue(s, key string) string {
	// Simple extraction for patterns like "key":"value"
	pattern := `"` + key + `":"`
	idx := strings.Index(s, pattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(pattern)
	// Find the closing quote, handling escapes
	var result strings.Builder
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			result.WriteByte(c)
			escaped = false
		} else if c == '\\' {
			escaped = true
		} else if c == '"' {
			break
		} else {
			result.WriteByte(c)
		}
	}
	return result.String()
}

func (h *Handler) flushAccumulator(acc *toolBlockAccumulator, nextOutputIndex *int, tags []string, reqCtx *requestContext) []sseEvent {
	if acc == nil || (len(acc.titleParts) == 0 && len(acc.contentParts) == 0) {
		return nil
	}

	// Merge all parts
	mergedTitle := strings.Join(acc.titleParts, "")
	mergedContent := strings.Join(acc.contentParts, "")

	// Transform to target tool
	targetToolName := h.toolMapper.TransformToolName(acc.toolName, tags)

	// Build transformed input
	transformedInput := map[string]interface{}{
		"plan": mergedContent,
	}
	if mergedTitle != "" {
		transformedInput["title"] = mergedTitle
	}

	transformedInputJSON, _ := json.Marshal(transformedInput)

	outputIndex := *nextOutputIndex
	*nextOutputIndex++

	// Emit single merged tool block
	startData := fmt.Sprintf(`{"content_block":{"id":"%s","input":{},"name":"%s","type":"tool_use"},"index":%d,"type":"content_block_start"}`,
		acc.firstToolID, targetToolName, outputIndex)

	deltaData := fmt.Sprintf(`{"delta":{"partial_json":%s,"type":"input_json_delta"},"index":%d,"type":"content_block_delta"}`,
		jsonEscape(string(transformedInputJSON)), outputIndex)

	stopData := fmt.Sprintf(`{"index":%d,"type":"content_block_stop"}`, outputIndex)

	h.logger.Info("tool blocks merged and transformed",
		zap.String("from", acc.toolName),
		zap.String("to", targetToolName),
		zap.Int("merged_blocks", acc.blockCount),
		zap.Int("title_parts", len(acc.titleParts)),
		zap.Int("content_parts", len(acc.contentParts)),
		zap.Int("output_index", outputIndex),
	)

	// Get mapping name for logging
	mappingName := ""
	if h.transformerRegistry != nil {
		if cfg := h.transformerRegistry.GetResponseTransformer(acc.toolName, tags); cfg != nil {
			mappingName = cfg.Name
			if reqCtx != nil {
				reqCtx.appliedTransformers = append(reqCtx.appliedTransformers, mappingName)
			}
		}
	}
	log.Printf("[MAPPING] Response (accumulated): %s -> %s (mapping: %s, merged %d blocks, tags: %v)", acc.toolName, targetToolName, mappingName, acc.blockCount, tags)

	return []sseEvent{
		{eventType: "content_block_start", data: startData},
		{eventType: "content_block_delta", data: deltaData},
		{eventType: "content_block_stop", data: stopData},
	}
}

func (h *Handler) saveLog(reqCtx *requestContext) {
	// 如果不是强制记录且日志未启用，则跳过
	if !reqCtx.forceLog && !h.logEnabled {
		return
	}

	sessionDir := filepath.Join(h.logDir, "sessions", reqCtx.meta.SessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		h.logger.Error("failed to create session directory", zap.Error(err))
		return
	}

	prefix := fmt.Sprintf("%03d", reqCtx.meta.Sequence)

	metaData, err := json.MarshalIndent(reqCtx.meta, "", "  ")
	if err != nil {
		h.logger.Error("failed to marshal meta", zap.Error(err))
		return
	}
	metaPath := filepath.Join(sessionDir, prefix+"_request.json")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		h.logger.Error("failed to write meta file", zap.Error(err))
		return
	}

	if len(reqCtx.reqBody) > 0 {
		reqBodyPath := filepath.Join(sessionDir, prefix+"_request.body")
		if err := os.WriteFile(reqBodyPath, reqCtx.reqBody, 0644); err != nil {
			h.logger.Error("failed to write request body file", zap.Error(err))
		}
	}

	// Save transformed request body if it differs from original
	if len(reqCtx.transformedReqBody) > 0 && !bytes.Equal(reqCtx.reqBody, reqCtx.transformedReqBody) {
		transformedReqPath := filepath.Join(sessionDir, prefix+"_request_transformed.body")
		if err := os.WriteFile(transformedReqPath, reqCtx.transformedReqBody, 0644); err != nil {
			h.logger.Error("failed to write transformed request body file", zap.Error(err))
		}
	}

	if len(reqCtx.respBody) > 0 {
		respBodyPath := filepath.Join(sessionDir, prefix+"_response.body")
		if err := os.WriteFile(respBodyPath, reqCtx.respBody, 0644); err != nil {
			h.logger.Error("failed to write response body file", zap.Error(err))
		}
	}

	h.logger.Info("request logged",
		zap.String("id", reqCtx.meta.ID),
		zap.String("session_id", reqCtx.meta.SessionID),
		zap.Int("sequence", reqCtx.meta.Sequence),
		zap.String("method", reqCtx.meta.Method),
		zap.String("path", reqCtx.meta.Path),
		zap.Int64("duration_ms", reqCtx.meta.DurationMs),
	)
}

func (h *Handler) saveTransformedResponse(reqCtx *requestContext, transformedBody []byte) {
	// 如果不是强制记录且日志未启用，则跳过
	if !reqCtx.forceLog && !h.logEnabled {
		return
	}

	sessionDir := filepath.Join(h.logDir, "sessions", reqCtx.meta.SessionID)
	prefix := fmt.Sprintf("%03d", reqCtx.meta.Sequence)

	transformedPath := filepath.Join(sessionDir, prefix+"_response_transformed.body")
	if err := os.WriteFile(transformedPath, transformedBody, 0644); err != nil {
		h.logger.Error("failed to write transformed response file", zap.Error(err))
	}
}

func sanitizeHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string)
	sensitiveHeaders := map[string]bool{
		"Authorization":     true,
		"X-Api-Key":         true,
		"Anthropic-Api-Key": true,
	}

	for key, values := range headers {
		if sensitiveHeaders[key] {
			result[key] = []string{"[REDACTED]"}
		} else {
			result[key] = values
		}
	}
	return result
}

func flattenHeaders(headers http.Header) map[string]string {
	result := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"Authorization":     true,
		"X-Api-Key":         true,
		"Anthropic-Api-Key": true,
	}

	for key, values := range headers {
		if sensitiveHeaders[key] {
			result[key] = "[REDACTED]"
		} else if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func decompressIfNeeded(body []byte, headers http.Header) []byte {
	if headers.Get("Content-Encoding") != "gzip" {
		return body
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return body
	}
	defer reader.Close()
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return body
	}
	return decompressed
}

// shouldForceLog 检查是否需要强制记录日志（不受 logging.enabled 影响）
// 条件：响应状态码非 200，或响应状态码 200 但响应体为空，或流式响应无内容块
func (h *Handler) shouldForceLog(respStatus int, respBodyLen int, hasEmptyContent bool) bool {
	// 非 200 响应
	if respStatus != http.StatusOK {
		return true
	}
	// 200 响应但 body 为空
	if respBodyLen == 0 {
		return true
	}
	// 流式响应无内容块
	if hasEmptyContent {
		return true
	}
	return false
}

func (h *Handler) updateConnectionStatus(connInfo *memory.ConnectionInfo, status string, durationMs int64) {
	if connInfo == nil || h.memoryStore == nil {
		return
	}

	h.memoryStore.Update(connInfo.ID, func(c *memory.ConnectionInfo) {
		c.Status = status
		c.DurationMs = durationMs
		c.EndTime = time.Now()
	})

	if h.wsHub != nil {
		h.wsHub.Broadcast(connInfo)
	}
}

func (h *Handler) parseProtocolData(body []byte, tags []string) *memory.ParsedData {
	hasAnthropicTag := false

	for _, tag := range tags {
		if tag == "$p_anthropic" {
			hasAnthropicTag = true
			break
		}
	}

	if !hasAnthropicTag {
		return nil
	}

	parsed := parser.ParseAnthropicRequest(body)
	if parsed == nil {
		return nil
	}

	return &memory.ParsedData{
		Protocol: parsed.Protocol,
		Anthropic: &memory.AnthropicParsedData{
			Model:     parsed.Anthropic.Model,
			MaxTokens: parsed.Anthropic.MaxTokens,
			SystemPrompts: func() []memory.SystemPrompt {
				result := make([]memory.SystemPrompt, len(parsed.Anthropic.SystemPrompts))
				for i, sp := range parsed.Anthropic.SystemPrompts {
					result[i] = memory.SystemPrompt{
						Type:         sp.Type,
						Text:         sp.Text,
						CacheControl: sp.CacheControl,
					}
				}
				return result
			}(),
			SystemReminders: func() []memory.SystemReminder {
				result := make([]memory.SystemReminder, len(parsed.Anthropic.SystemReminders))
				for i, sr := range parsed.Anthropic.SystemReminders {
					result[i] = memory.SystemReminder{
						RawText:    sr.RawText,
						ParsedInfo: sr.ParsedInfo,
					}
				}
				return result
			}(),
			Tools: func() []memory.ToolDefinition {
				result := make([]memory.ToolDefinition, len(parsed.Anthropic.Tools))
				for i, t := range parsed.Anthropic.Tools {
					result[i] = memory.ToolDefinition{
						Name:        t.Name,
						Description: t.Description,
						InputSchema: t.InputSchema,
					}
				}
				return result
			}(),
		},
	}
}
