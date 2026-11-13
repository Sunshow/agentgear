package video

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	tasksPath      = "/api/v3/contents/generations/tasks"
	maxRequestBody = 8 << 20
)

// Config configures one video protocol adapter instance (one gateway).
type Config struct {
	// Protocol selects the inbound wire implemented by this adapter.
	// Supported: "doubao" (default). "ali" is reserved for the Alibaba
	// Bailian wire and currently rejects at startup.
	Protocol      string
	Prefix        string
	Upstream      string
	APIKey        string
	PublicBaseURL string
	// Store is optional. Gateways of different protocols share one store;
	// when nil the adapter opens StorePath itself.
	Store         *Store
	StorePath     string
	TaskTTL       time.Duration
	CreateTimeout time.Duration
	QueryTimeout  time.Duration
	CacheTTL      time.Duration
	Models        map[string]string
}

// Handler serves the subset of the doubao/Ark task protocol that a New API
// channel bound to the doubao plugin calls:
//
//	POST /api/v3/contents/generations/tasks
//	GET  /api/v3/contents/generations/tasks/{id}
//	GET  /api/v3/contents/generations/tasks/{id}/content  (optional proxy)
type Handler struct {
	cfg       Config
	client    *orluckyClient
	store     *Store
	ownsStore bool
	log       *zap.Logger
	cache     sync.Map
}

// Adapter is one gateway's protocol implementation. Adding a protocol means
// implementing this interface and adding a case to NewAdapter; the shared
// video settings, store and content proxy stay protocol-neutral.
type Adapter interface {
	Register(engine *gin.Engine)
	Close() error
}

// NewAdapter builds the adapter selected by cfg.Protocol. Empty defaults to
// doubao so an existing single-protocol test or config keeps working.
func NewAdapter(cfg Config, logger *zap.Logger) (Adapter, error) {
	switch cfg.Protocol {
	case "", "doubao":
		return newDoubaoHandler(cfg, logger)
	default:
		return nil, fmt.Errorf("video protocol %q is not implemented", cfg.Protocol)
	}
}

func newDoubaoHandler(cfg Config, logger *zap.Logger) (*Handler, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "doubao"
	}
	if strings.TrimSpace(cfg.Prefix) == "" {
		return nil, fmt.Errorf("video gateway path is required")
	}
	cfg.Prefix = normalizePrefix(cfg.Prefix)
	if strings.TrimSpace(cfg.Upstream) == "" {
		cfg.Upstream = "https://router.orlucky.com/model/api"
	}
	if cfg.StorePath == "" {
		cfg.StorePath = "./data/video_tasks.db"
	}
	if cfg.TaskTTL <= 0 {
		cfg.TaskTTL = 72 * time.Hour
	}
	if cfg.CreateTimeout <= 0 {
		cfg.CreateTimeout = 300 * time.Second
	}
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 30 * time.Second
	}
	if cfg.CacheTTL < 0 {
		cfg.CacheTTL = 0
	}
	if len(cfg.Models) == 0 {
		cfg.Models = defaultModelMap
	}
	store := cfg.Store
	ownsStore := false
	if store == nil {
		opened, err := OpenStore(cfg.StorePath, cfg.TaskTTL)
		if err != nil {
			return nil, err
		}
		store = opened
		ownsStore = true
	}
	return &Handler{
		cfg:       cfg,
		client:    newOrluckyClient(cfg.Upstream, cfg.CreateTimeout, cfg.QueryTimeout),
		store:     store,
		ownsStore: ownsStore,
		log:       logger,
	}, nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	return prefix
}

func (h *Handler) Close() error {
	if !h.ownsStore {
		return nil
	}
	return h.store.Close()
}

// Register mounts the adapter under prefix. A wildcard route is used so it
// never collides with an existing transparent gateway such as /v1/*path.
func (h *Handler) Register(engine *gin.Engine) {
	pattern := h.cfg.Prefix + "/*path"
	if h.cfg.Prefix == "" {
		pattern = "/*path"
	}
	engine.Any(pattern, h.Handle)
}

func (h *Handler) Handle(c *gin.Context) {
	path := c.Param("path")
	if path == "" {
		path = "/"
	}
	method := c.Request.Method
	switch {
	case method == http.MethodPost && path == tasksPath:
		h.create(c)
	case (method == http.MethodGet || method == http.MethodHead) && strings.HasPrefix(path, tasksPath+"/"):
		rest := strings.TrimPrefix(path, tasksPath+"/")
		if rest == "" {
			h.writeNotFound(c)
			return
		}
		if id, found := strings.CutSuffix(rest, "/content"); found {
			if id == "" || strings.Contains(id, "/") {
				h.writeNotFound(c)
				return
			}
			h.content(c, decodeParam(id))
			return
		}
		if strings.Contains(rest, "/") {
			h.writeNotFound(c)
			return
		}
		h.get(c, decodeParam(rest))
	default:
		h.writeNotFound(c)
	}
}

func decodeParam(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func (h *Handler) create(c *gin.Context) {
	apiKey, ok := h.upstreamKey(c)
	if !ok {
		h.writeError(c, http.StatusUnauthorized, "unauthorized", "Authorization bearer token is required")
		return
	}

	if contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type"))); contentType != "" &&
		!strings.HasPrefix(contentType, "application/json") {
		h.writeError(c, http.StatusUnsupportedMediaType, "unsupported_media_type",
			"JSON body required; multipart is not supported")
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.writeError(c, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 8 MiB")
			return
		}
		h.writeError(c, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}
	var request arkCreateRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_request_error", "request body must be a JSON object")
		return
	}
	if err := request.validate(); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	target, heuristic, ok := mapModel(h.cfg.Models, request.Model)
	if !ok {
		h.writeError(c, http.StatusBadRequest, "model_not_supported",
			fmt.Sprintf("model %s is not mapped to an orlucky video model", request.Model))
		return
	}
	profile, ok := resolveProfile(target)
	if !ok {
		h.writeError(c, http.StatusBadRequest, "model_not_supported",
			fmt.Sprintf("orlucky model %s has no adapter profile", target))
		return
	}

	body, meta, err := mapCreate(&request, target, profile)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if heuristic && h.log != nil {
		h.log.Info("video model mapped by name pattern",
			zap.String("model", request.Model),
			zap.String("target", target))
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.CreateTimeout)
	defer cancel()
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}

	started := time.Now()
	upstream, err := h.client.create(ctx, apiKey, body, idempotencyKey)
	if err != nil {
		h.log.Warn("video task creation failed",
			zap.String("model", request.Model),
			zap.String("target", target),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err))
		h.writeUpstreamError(c, err)
		return
	}

	now := time.Now().Unix()
	task := &Task{
		PublicID:    newTaskID(),
		UpstreamID:  upstream.TaskID,
		ClientModel: meta.ClientModel,
		TargetModel: meta.TargetModel,
		Prompt:      meta.Prompt,
		Duration:    meta.Duration,
		Resolution:  meta.Resolution,
		Aspect:      meta.Aspect,
		Status:      "queued",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.store.Put(task); err != nil {
		h.log.Error("failed to persist video task", zap.String("upstream_id", upstream.TaskID), zap.Error(err))
		h.writeError(c, http.StatusInternalServerError, "internal_error", "failed to persist video task")
		return
	}

	h.log.Info("video task created",
		zap.String("task_id", task.PublicID),
		zap.String("upstream_id", upstream.TaskID),
		zap.String("model", task.ClientModel),
		zap.String("target_model", task.TargetModel),
		zap.String("resolution", task.Resolution),
		zap.Int("duration", task.Duration),
		zap.Duration("elapsed", time.Since(started)))

	c.JSON(http.StatusOK, gin.H{"id": task.PublicID})
}

func (h *Handler) get(c *gin.Context, publicID string) {
	apiKey, ok := h.upstreamKey(c)
	if !ok {
		h.writeError(c, http.StatusUnauthorized, "unauthorized", "Authorization bearer token is required")
		return
	}
	task, found, err := h.store.Get(publicID)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "internal_error", "failed to load video task")
		return
	}
	if !found {
		h.writeError(c, http.StatusNotFound, "task_not_found", "video task not found")
		return
	}
	if entry, ok := h.cached(publicID); ok {
		c.JSON(http.StatusOK, entry)
		return
	}

	response, err := h.refresh(c.Request.Context(), task, apiKey)
	if err != nil {
		h.log.Warn("video task query failed",
			zap.String("task_id", publicID),
			zap.String("upstream_id", task.UpstreamID),
			zap.Error(err))
		h.writeUpstreamError(c, err)
		return
	}
	h.setCache(publicID, response)
	c.JSON(http.StatusOK, response)
}

// refresh fetches the upstream task, maps it, persists the new state and
// returns the Ark-shaped task response.
func (h *Handler) refresh(parent context.Context, task *Task, apiKey string) (*arkTaskResponse, error) {
	ctx, cancel := context.WithTimeout(parent, h.cfg.QueryTimeout)
	defer cancel()
	upstream, err := h.client.get(ctx, apiKey, task.UpstreamID)
	if err != nil {
		return nil, err
	}

	status, terminal, success := mapOrluckyState(upstream.State)
	assetURL := videoAssetURL(upstream)
	if success && assetURL == "" {
		// A video task that claims success without an artifact is not
		// downloadable; keep it pending so the outer gateway queries again
		// instead of marking a broken success.
		h.log.Warn("video task succeeded without an asset",
			zap.String("task_id", task.PublicID),
			zap.String("upstream_id", task.UpstreamID))
		status, terminal, success = "running", false, false
	}
	message := taskErrorMessage(upstream)
	var usage json.RawMessage
	if upstream.Data != nil && len(upstream.Data.Usage) > 0 {
		usage = upstream.Data.Usage
	}
	now := time.Now().Unix()

	updated, err := h.store.Update(task.PublicID, func(stored *Task) {
		stored.Status = status
		stored.LastSyncedAt = now
		if assetURL != "" {
			stored.AssetURL = assetURL
		}
		if len(usage) > 0 {
			stored.Usage = usage
		}
		if terminal && stored.CompletedAt == 0 {
			stored.CompletedAt = now
		}
	})
	if err != nil {
		return nil, err
	}

	response := &arkTaskResponse{
		ID:     updated.PublicID,
		Model:  updated.ClientModel,
		Status: status,
	}
	if success {
		videoURL := updated.AssetURL
		if h.cfg.PublicBaseURL != "" {
			videoURL = h.publicContentURL(updated.PublicID)
		}
		response.Content = &arkTaskContent{
			VideoURL:   videoURL,
			Resolution: updated.Resolution,
		}
		if len(updated.Usage) > 0 && string(updated.Usage) != "null" {
			response.Usage = updated.Usage
		}
		return response, nil
	}
	if terminal {
		response.Error = &arkError{
			Code:    failureCode(status),
			Message: failureMessage(message, status),
		}
	}
	return response, nil
}

func failureCode(status string) string {
	switch status {
	case "cancelled":
		return "video_generation_cancelled"
	case "expired":
		return "video_generation_expired"
	default:
		return "video_generation_failed"
	}
}

func failureMessage(message, status string) string {
	if message != "" {
		return message
	}
	return "video generation " + status
}

func (h *Handler) content(c *gin.Context, publicID string) {
	task, found, err := h.store.Get(publicID)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "internal_error", "failed to load video task")
		return
	}
	if !found {
		h.writeError(c, http.StatusNotFound, "task_not_found", "video task not found")
		return
	}

	if task.AssetURL == "" {
		apiKey, ok := h.upstreamKey(c)
		if !ok {
			h.writeError(c, http.StatusUnauthorized, "unauthorized", "Authorization bearer token is required")
			return
		}
		if _, err := h.refresh(c.Request.Context(), task, apiKey); err != nil {
			h.writeUpstreamError(c, err)
			return
		}
		task, found, err = h.store.Get(publicID)
		if err != nil || !found {
			h.writeError(c, http.StatusInternalServerError, "internal_error", "failed to load video task")
			return
		}
	}
	if task.AssetURL == "" {
		h.writeError(c, http.StatusConflict, "content_not_ready",
			fmt.Sprintf("video task is %s, content is not available yet", task.Status))
		return
	}

	response, err := h.client.openAsset(c.Request.Context(), task.AssetURL, c.Request.Method, c.Request.Header)
	if err != nil {
		h.writeError(c, http.StatusBadGateway, "upstream_error", "failed to fetch video content")
		return
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified, http.StatusRequestedRangeNotSatisfiable:
		copyAssetHeaders(c.Writer.Header(), response.Header)
		c.Writer.Header().Set("Cache-Control", "private, no-store")
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		if c.Writer.Header().Get("Content-Type") == "" {
			c.Writer.Header().Set("Content-Type", "video/mp4")
		}
		c.Status(response.StatusCode)
		if c.Request.Method == http.MethodHead || response.StatusCode == http.StatusNotModified {
			return
		}
		if _, err := io.Copy(c.Writer, response.Body); err != nil {
			h.log.Warn("video content stream interrupted", zap.String("task_id", publicID), zap.Error(err))
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		h.writeError(c, http.StatusBadGateway, "upstream_auth_failed", "video asset rejected the request")
	case http.StatusNotFound, http.StatusGone:
		h.writeError(c, http.StatusNotFound, "content_gone", "video content is no longer available")
	case http.StatusTooManyRequests:
		h.writeError(c, http.StatusTooManyRequests, "upstream_rate_limited", "video asset rate limited")
	default:
		h.writeError(c, http.StatusBadGateway, "upstream_error",
			fmt.Sprintf("video asset returned HTTP %d", response.StatusCode))
	}
}

func copyAssetHeaders(destination, source http.Header) {
	for _, name := range []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
		"ETag", "Last-Modified", "Content-Disposition",
	} {
		for _, value := range source.Values(name) {
			destination.Add(name, value)
		}
	}
}

func (h *Handler) publicContentURL(publicID string) string {
	return strings.TrimRight(h.cfg.PublicBaseURL, "/") + h.cfg.Prefix + tasksPath + "/" + url.PathEscape(publicID) + "/content"
}

func (h *Handler) upstreamKey(c *gin.Context) (string, bool) {
	if key := strings.TrimSpace(h.cfg.APIKey); key != "" {
		return key, true
	}
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization == "" {
		return "", false
	}
	scheme, value, ok := strings.Cut(authorization, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	key := strings.TrimSpace(value)
	return key, key != ""
}

func (h *Handler) writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func (h *Handler) writeNotFound(c *gin.Context) {
	h.writeError(c, http.StatusNotFound, "not_found", "unsupported video endpoint")
}

func (h *Handler) writeUpstreamError(c *gin.Context, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		h.writeError(c, http.StatusGatewayTimeout, "upstream_timeout", "orlucky request timed out")
		return
	}
	if upstream, ok := isUpstreamError(err); ok {
		switch {
		case upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden:
			h.writeError(c, http.StatusBadGateway, "upstream_auth_failed", "orlucky rejected the API key")
		case upstream.StatusCode == http.StatusTooManyRequests:
			h.writeError(c, http.StatusTooManyRequests, "upstream_rate_limited", upstream.Message)
		case upstream.StatusCode < 400:
			h.writeError(c, http.StatusBadRequest, "upstream_request_rejected", upstream.Message)
		case upstream.StatusCode >= 400 && upstream.StatusCode < 500:
			h.writeError(c, upstream.StatusCode, "upstream_request_rejected", upstream.Message)
		default:
			h.writeError(c, http.StatusBadGateway, "upstream_error", upstream.Message)
		}
		return
	}
	h.writeError(c, http.StatusBadGateway, "upstream_error", "orlucky request failed")
}

type cacheEntry struct {
	at       time.Time
	response arkTaskResponse
}

func (h *Handler) cached(publicID string) (*arkTaskResponse, bool) {
	if h.cfg.CacheTTL <= 0 {
		return nil, false
	}
	value, ok := h.cache.Load(publicID)
	if !ok {
		return nil, false
	}
	entry, ok := value.(cacheEntry)
	if !ok || time.Since(entry.at) > h.cfg.CacheTTL {
		h.cache.Delete(publicID)
		return nil, false
	}
	response := entry.response
	return &response, true
}

func (h *Handler) setCache(publicID string, response *arkTaskResponse) {
	if h.cfg.CacheTTL <= 0 || response == nil {
		return
	}
	h.cache.Store(publicID, cacheEntry{at: time.Now(), response: *response})
}

func newTaskID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "task_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return "task_" + hex.EncodeToString(buffer)
}
