package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxUpstreamBody = 8 << 20

// orluckyError is a non-success answer from the orlucky task API.
type orluckyError struct {
	StatusCode int
	Code       string
	Message    string
	TraceID    string
}

func (e *orluckyError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("orlucky returned HTTP %d", e.StatusCode)
}

type orluckyClient struct {
	baseURL string
	api     *http.Client
	asset   *http.Client
}

func newOrluckyClient(baseURL string, createTimeout, queryTimeout time.Duration) *orluckyClient {
	return &orluckyClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		api: &http.Client{
			Timeout: createTimeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: queryTimeout,
			},
		},
		asset: &http.Client{
			// Media transfers are streamed; no whole-request timeout.
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          32,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}
}

func (c *orluckyClient) create(ctx context.Context, apiKey string, body *orluckyCreateRequest, idempotencyKey string) (*orluckyTask, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode orlucky request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/tasks", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-API-Key", apiKey)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return c.doTaskRequest(request)
}

func (c *orluckyClient) get(ctx context.Context, apiKey, taskID string) (*orluckyTask, error) {
	endpoint := c.baseURL + "/v1/tasks/" + url.PathEscape(taskID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-API-Key", apiKey)
	return c.doTaskRequest(request)
}

func (c *orluckyClient) doTaskRequest(request *http.Request) (*orluckyTask, error) {
	response, err := c.api.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxUpstreamBody))
	if err != nil {
		return nil, fmt.Errorf("read orlucky response: %w", err)
	}
	return decodeTaskResponse(response.StatusCode, raw)
}

func decodeTaskResponse(statusCode int, raw []byte) (*orluckyTask, error) {
	var envelope orluckyEnvelope
	decodeErr := json.Unmarshal(raw, &envelope)
	if decodeErr != nil {
		return nil, &orluckyError{
			StatusCode: statusCode,
			Message:    fmt.Sprintf("orlucky returned an invalid response (HTTP %d)", statusCode),
		}
	}
	if statusCode < 200 || statusCode >= 300 || !envelope.Success {
		apiErr := &orluckyError{StatusCode: statusCode, TraceID: envelope.TraceID}
		var payload struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		}
		if len(envelope.Data) > 0 && json.Unmarshal(envelope.Data, &payload) == nil {
			apiErr.Message = strings.TrimSpace(payload.Message)
			if code, ok := payload.Code.(string); ok {
				apiErr.Code = strings.TrimSpace(code)
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = fmt.Sprintf("orlucky returned HTTP %d", statusCode)
		}
		return nil, apiErr
	}
	if len(envelope.Data) == 0 {
		return nil, &orluckyError{StatusCode: statusCode, Message: "orlucky returned an empty task"}
	}
	var task orluckyTask
	if err := json.Unmarshal(envelope.Data, &task); err != nil {
		return nil, &orluckyError{StatusCode: statusCode, Message: "orlucky returned an invalid task"}
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, &orluckyError{StatusCode: statusCode, Message: "orlucky returned a task without an id"}
	}
	return &task, nil
}

// openAsset requests the generated media. The caller owns the response body.
func (c *orluckyClient) openAsset(ctx context.Context, rawURL, method string, headers http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := headers.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	return c.asset.Do(request)
}

func isUpstreamError(err error) (*orluckyError, bool) {
	var target *orluckyError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
