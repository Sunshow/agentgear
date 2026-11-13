package video

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type fakeUpstream struct {
	mu sync.Mutex

	apiKey      string
	createBody  orluckyCreateRequest
	createCalls int
	queryCalls  int

	createStatus  int
	createMessage string
	state         string
	failure       string
	assetURL      string
}

func (f *fakeUpstream) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var body orluckyCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.apiKey = r.Header.Get("X-API-Key")
		f.createBody = body
		f.createCalls++
		status := f.createStatus
		message := f.createMessage
		f.mu.Unlock()
		if status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"data":    map[string]any{"message": message},
			})
			return
		}
		writeJSON(w, map[string]any{
			"success": true,
			"data": map[string]any{
				"taskId": "up1",
				"state":  "PENDING",
			},
		})
	})
	mux.HandleFunc("/v1/tasks/up1", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		state := f.state
		failure := f.failure
		assetURL := f.assetURL
		f.queryCalls++
		f.mu.Unlock()
		data := map[string]any{"taskId": "up1", "state": state}
		if assetURL != "" {
			data["data"] = map[string]any{
				"assets": []map[string]any{{"type": "VIDEO", "url": assetURL}},
			}
		}
		if failure != "" {
			data["error"] = map[string]any{"message": failure}
		}
		writeJSON(w, map[string]any{"success": true, "data": data})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func newTestHandler(t *testing.T, upstreamURL string, mutate func(*Config)) (*Handler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := Config{
		Prefix:        "/doubao",
		Upstream:      upstreamURL,
		StorePath:     filepath.Join(t.TempDir(), "video_tasks.db"),
		CreateTimeout: 5 * time.Second,
		QueryTimeout:  5 * time.Second,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	handler, err := newDoubaoHandler(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	engine := gin.New()
	handler.Register(engine)
	return handler, engine
}

func doRequest(engine *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func createTask(t *testing.T, engine *gin.Engine) string {
	t.Helper()
	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks",
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"a cat"}],"duration":5}`,
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !strings.HasPrefix(response.ID, "task_") {
		t.Fatalf("unexpected task id: %q", response.ID)
	}
	return response.ID
}

func TestCreateAndQuery(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	id := createTask(t, engine)

	fake.mu.Lock()
	apiKey := fake.apiKey
	body := fake.createBody
	fake.mu.Unlock()
	if apiKey != "sk-test" {
		t.Fatalf("upstream api key = %q", apiKey)
	}
	if body.Category != "VIDEO" || body.Operation != "TEXT_TO_VIDEO" || body.Model != "seedance-mini" {
		t.Fatalf("unexpected upstream body: %#v", body)
	}
	if body.Prompt != "a cat" || body.Parameters["duration"] != float64(5) {
		t.Fatalf("unexpected upstream body: %#v", body)
	}
	if body.Parameters["resolution"] != "720p" || body.Parameters["aspectRatio"] != "16:9" {
		t.Fatalf("unexpected upstream parameters: %#v", body.Parameters)
	}

	recorder := doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id, "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("query status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var pending arkTaskResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &pending); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if pending.Status != "queued" || pending.Content != nil {
		t.Fatalf("unexpected pending response: %#v", pending)
	}

	fake.mu.Lock()
	fake.state = "RUNNING"
	fake.mu.Unlock()
	recorder = doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id, "",
		map[string]string{"Authorization": "Bearer sk-test"})
	var running arkTaskResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &running)
	if running.Status != "running" {
		t.Fatalf("unexpected running response: %s", recorder.Body.String())
	}

	fake.mu.Lock()
	fake.state = "SUCCEEDED"
	fake.assetURL = "https://cdn.example.com/out.mp4"
	fake.mu.Unlock()
	recorder = doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id, "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("succeeded query status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var succeeded arkTaskResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &succeeded); err != nil {
		t.Fatalf("decode succeeded response: %v", err)
	}
	if succeeded.Status != "succeeded" {
		t.Fatalf("unexpected status: %#v", succeeded)
	}
	if succeeded.Content == nil || succeeded.Content.VideoURL != fake.assetURL || succeeded.Content.Resolution != "720p" {
		t.Fatalf("unexpected content: %#v", succeeded.Content)
	}
}

func TestCreateRequiresAuthorization(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks",
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"a cat"}]}`, nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateRejectsInvalidRequest(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	cases := []string{
		`{"content":[{"type":"text","text":"x"}]}`,
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"x"}],"duration":3}`,
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"x"}],"duration":-1}`,
		`{"model":"unknown-model","content":[{"type":"text","text":"x"}]}`,
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"image_url","image_url":{"url":"file:///a.png"}}]}`,
	}
	for index, body := range cases {
		recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks", body,
			map[string]string{"Authorization": "Bearer sk-test"})
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("case %d: status = %d, body = %s", index, recorder.Code, recorder.Body.String())
		}
	}
}

func TestQueryUnknownTask(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	recorder := doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/task_missing", "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestQueryFailedTask(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING", failure: "content rejected"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	id := createTask(t, engine)
	fake.mu.Lock()
	fake.state = "FAILED"
	fake.mu.Unlock()

	recorder := doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id, "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response arkTaskResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "failed" || response.Error == nil || response.Error.Message != "content rejected" {
		t.Fatalf("unexpected failure response: %#v", response)
	}
	if response.Content != nil {
		t.Fatalf("failed task must not carry content: %#v", response.Content)
	}
}

func TestCreateUpstreamAuthFailure(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING", createStatus: http.StatusUnauthorized, createMessage: "bad key"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks",
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"a cat"}]}`,
		map[string]string{"Authorization": "Bearer sk-wrong"})
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "upstream_auth_failed") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestConfiguredAPIKeyOverridesInbound(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, func(cfg *Config) { cfg.APIKey = "sk-fixed" })

	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks",
		`{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"a cat"}]}`, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.apiKey != "sk-fixed" {
		t.Fatalf("upstream api key = %q", fake.apiKey)
	}
}

func TestContentProxy(t *testing.T) {
	asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("video-bytes"))
	}))
	defer asset.Close()

	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	id := createTask(t, engine)
	fake.mu.Lock()
	fake.state = "SUCCEEDED"
	fake.assetURL = asset.URL + "/out.mp4"
	fake.mu.Unlock()

	recorder := doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id+"/content", "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "video-bytes" {
		t.Fatalf("unexpected content: %q", recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "video/mp4" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "private, no-store" {
		t.Fatalf("unexpected cache control: %q", cacheControl)
	}
}

func TestContentNotReady(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	id := createTask(t, engine)
	recorder := doRequest(engine, http.MethodGet, "/doubao/api/v3/contents/generations/tasks/"+id+"/content", "",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestUnsupportedEndpoint(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks/foo", "{}",
		map[string]string{"Authorization": "Bearer sk-test"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestCreateRejectsMultipart(t *testing.T) {
	fake := &fakeUpstream{state: "PENDING"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	_, engine := newTestHandler(t, server.URL, nil)

	body := "--boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nx\r\n--boundary--\r\n"
	recorder := doRequest(engine, http.MethodPost, "/doubao/api/v3/contents/generations/tasks", body,
		map[string]string{
			"Authorization": "Bearer sk-test",
			"Content-Type":  "multipart/form-data; boundary=boundary",
		})
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "multipart is not supported") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestUnsupportedProtocolRejected(t *testing.T) {
	_, err := NewAdapter(Config{Protocol: "ali"}, nil)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected unsupported protocol error, got %v", err)
	}
}
