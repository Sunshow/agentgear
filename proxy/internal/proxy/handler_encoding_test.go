package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

func newEncodingTestProxy(t *testing.T, cfg Config) (*httptest.Server, *Handler) {
	t.Helper()
	cfg.Timeout = 5 * time.Second
	cfg.LogDir = t.TempDir()
	cfg.LogEnabled = &atomic.Bool{}
	cfg.LogEnabled.Store(true)
	if cfg.TransformerRegistry == nil {
		cfg.TransformerRegistry = transformer.NewRegistry(transformer.Config{})
	}
	handler := NewHandler(cfg)
	router := gin.New()
	router.Any("/v1/messages", handler.ProxyRequest)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, handler
}

func sendEncodingTestRequest(t *testing.T, server *httptest.Server, wireBody []byte, encoding string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", bytes.NewReader(wireBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", encoding)
	req.Header.Set("Accept-Encoding", "br, gzip, zstd")
	req.Header.Set("X-Session-Id", "encoding-test")
	client := server.Client()
	// Automatic gzip decoding would hide stale Content-Encoding headers.
	client.Transport = &http.Transport{DisableCompression: true}
	client.Timeout = 5 * time.Second
	t.Cleanup(client.CloseIdleConnections)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readEncodingTestResponse(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" {
		t.Errorf("decoded response still has Content-Encoding %q", encoding)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != int64(len(body)) {
		t.Errorf("Content-Length = %d, actual body size = %d", resp.ContentLength, len(body))
	}
	return body
}

func TestProxyCompressedRequestIsTaggedTransformedAndLogged(t *testing.T) {
	for _, encoding := range []string{"", "gzip", "zstd", "gzip, zstd"} {
		t.Run(encoding, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if !json.Valid(body) || !bytes.Contains(body, []byte("injected after decoding")) {
					t.Errorf("upstream did not receive transformed JSON: %q", body)
				}
				if got := r.Header.Get("Content-Encoding"); got != "" {
					t.Errorf("upstream received stale Content-Encoding: %q", got)
				}
				if r.ContentLength != int64(len(body)) {
					t.Errorf("upstream Content-Length = %d, want %d", r.ContentLength, len(body))
				}
				if got := r.Header.Get("Accept-Encoding"); got != "gzip, zstd" {
					t.Errorf("upstream Accept-Encoding = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"ok":true}`)
			}))
			defer upstream.Close()
			server, handler := newEncodingTestProxy(t, Config{
				UpstreamURL: upstream.URL,
				TaggingEngine: tagging.NewEngine(tagging.Config{Rules: []tagging.Rule{{
					Name: "body_match", Tags: []string{"decoded_request"},
					Matchers: []tagging.Matcher{{Type: tagging.MatcherTypeBodyJSON, Key: "model", Match: tagging.ValueMatcher{Op: tagging.MatchOpEquals, Value: "test-model"}}},
				}}}),
				TransformerRegistry: transformer.NewRegistry(transformer.Config{
					Definitions: []transformer.TransformerDef{{Name: "inject", Type: "message_inject", Direction: "request", InjectText: "injected after decoding", InjectFormat: "plain"}},
					Mappings:    []transformer.MappingRule{{Name: "inject", Enabled: true, Tags: []string{"decoded_request"}, Transformer: "inject"}},
				}),
			})
			body := []byte(`{"model":"test-model","messages":[{"role":"user","content":"你好"}],"stream":false}`)
			resp := sendEncodingTestRequest(t, server, encodeTestBody(t, body, encoding), encoding)
			result := readEncodingTestResponse(t, resp)
			if resp.StatusCode != http.StatusOK || string(result) != `{"ok":true}` {
				t.Fatalf("response = %d %s", resp.StatusCode, result)
			}
			logged, err := os.ReadFile(filepath.Join(handler.logDir, "sessions", "encoding-test", "001_request.body"))
			if err != nil || !bytes.Equal(logged, body) {
				t.Fatalf("request log = %q, err = %v; want decoded JSON", logged, err)
			}
			metaData, err := os.ReadFile(filepath.Join(handler.logDir, "sessions", "encoding-test", "001_request.json"))
			if err != nil {
				t.Fatal(err)
			}
			var meta RequestMeta
			if err := json.Unmarshal(metaData, &meta); err != nil {
				t.Fatal(err)
			}
			if http.Header(meta.Headers).Get("Content-Encoding") != "" {
				t.Fatal("decoded request log has a stale encoding header, which would break replay")
			}
		})
	}
}

const encodingTestSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"content\":[],\"usage\":{\"input_tokens\":5,\"cache_read_input_tokens\":10}}}\n\n" +
	"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"hello\"}}\n\n" +
	"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
	"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

func encodingTestCacheStripRegistry() *transformer.Registry {
	return transformer.NewRegistry(transformer.Config{
		Definitions: []transformer.TransformerDef{{Name: "strip", Type: "cache_strip", Direction: "response"}},
		Mappings:    []transformer.MappingRule{{Name: "strip", Enabled: true, Transformer: "strip"}},
	})
}

func TestProxyCompressedResponses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, encoding := range []string{"", "gzip", "zstd", "zstd, gzip"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, encoding), func(t *testing.T) {
				body := []byte(`{"id":"msg_test","usage":{"input_tokens":5,"cache_read_input_tokens":10}}`)
				contentType := "application/json"
				if stream {
					body = []byte(encodingTestSSE)
					contentType = "text/event-stream"
				}
				wireBody := encodeTestBody(t, body, encoding)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Encoding", encoding)
					w.Header().Set("Content-Type", contentType)
					w.Header().Set("Content-Length", strconv.Itoa(len(wireBody)))
					w.Write(wireBody)
				}))
				defer upstream.Close()
				server, handler := newEncodingTestProxy(t, Config{UpstreamURL: upstream.URL, TransformerRegistry: encodingTestCacheStripRegistry()})
				request := []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
				resp := sendEncodingTestRequest(t, server, encodeTestBody(t, request, encoding), encoding)
				result := readEncodingTestResponse(t, resp)
				if resp.StatusCode != http.StatusOK || !bytes.Contains(result, []byte(`"input_tokens":15`)) {
					t.Fatalf("response was not decoded and transformed: %d %s", resp.StatusCode, result)
				}
				if stream && (!bytes.Contains(result, []byte("event: message_stop")) || resp.ContentLength != -1) {
					t.Fatalf("invalid streaming response: Content-Length = %d, body = %s", resp.ContentLength, result)
				}
				if !stream && !json.Valid(result) {
					t.Fatalf("invalid JSON response: %s", result)
				}
				// The normal handler saves before sending its body. Streaming logs
				// are saved before the handler closes the response (EOF above).
				logged, err := os.ReadFile(filepath.Join(handler.logDir, "sessions", "encoding-test", "001_response.body"))
				if err != nil || !bytes.Equal(logged, body) {
					t.Fatalf("response log = %q, err = %v; want decoded body", logged, err)
				}
			})
		}
	}
}

func TestProxyCompressedErrorResponses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, encoding := range []string{"gzip", "zstd"} {
			for _, transform := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/transform=%t/%s", stream, transform, encoding), func(t *testing.T) {
					body := []byte(`{"error":{"message":"input too long"}}`)
					wireBody := encodeTestBody(t, body, encoding)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("Content-Encoding", encoding)
						w.Header().Set("Content-Length", strconv.Itoa(len(wireBody)))
						w.WriteHeader(http.StatusBadRequest)
						w.Write(wireBody)
					}))
					defer upstream.Close()
					cfg := Config{UpstreamURL: upstream.URL}
					if transform {
						cfg.TransformerRegistry = transformer.NewRegistry(transformer.Config{
							Definitions: []transformer.TransformerDef{{Name: "context_error", Type: "error_transform", Direction: "response", ErrorPatterns: []string{"input too long"}, ErrorCode: "context_length_exceeded"}},
							Mappings:    []transformer.MappingRule{{Name: "context_error", Enabled: true, Transformer: "context_error"}},
						})
					}
					server, _ := newEncodingTestProxy(t, cfg)
					resp := sendEncodingTestRequest(t, server, []byte(fmt.Sprintf(`{"stream":%t}`, stream)), "")
					result := readEncodingTestResponse(t, resp)
					if resp.StatusCode != http.StatusBadRequest || !json.Valid(result) {
						t.Fatalf("response = %d %s", resp.StatusCode, result)
					}
					if transform && !bytes.Contains(result, []byte("context_length_exceeded")) {
						t.Fatalf("compressed error was not matched: %s", result)
					}
					if !transform && !bytes.Equal(result, body) {
						t.Fatalf("error response = %s, want %s", result, body)
					}
				})
			}
		}
	}
}

func TestProxyRejectsInvalidCompressedRequests(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	server, _ := newEncodingTestProxy(t, Config{UpstreamURL: upstream.URL})
	for _, encoding := range []string{"gzip", "zstd", "br"} {
		t.Run(encoding, func(t *testing.T) {
			resp := sendEncodingTestRequest(t, server, []byte("invalid compressed data"), encoding)
			readEncodingTestResponse(t, resp)
			wantStatus := http.StatusBadRequest
			if encoding == "br" {
				wantStatus = http.StatusUnsupportedMediaType
			}
			if resp.StatusCode != wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, wantStatus)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests were forwarded upstream %d times", calls.Load())
	}
}

func TestProxyRejectsInvalidCompressedResponses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
			for _, encoding := range []string{"gzip", "zstd", "br"} {
				t.Run(fmt.Sprintf("stream=%t/status=%d/%s", stream, status, encoding), func(t *testing.T) {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Encoding", encoding)
						w.WriteHeader(status)
						io.WriteString(w, "invalid compressed data")
					}))
					defer upstream.Close()
					server, _ := newEncodingTestProxy(t, Config{UpstreamURL: upstream.URL})
					resp := sendEncodingTestRequest(t, server, []byte(fmt.Sprintf(`{"stream":%t}`, stream)), "")
					body := readEncodingTestResponse(t, resp)
					if resp.StatusCode != http.StatusBadGateway || !json.Valid(body) {
						t.Fatalf("invalid upstream body should return a JSON 502 error, got %d %q", resp.StatusCode, body)
					}
					if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
						t.Fatalf("error response Content-Type = %q", got)
					}
				})
			}
		}
	}
}

func TestProxyCompressedStreamFlushesBeforeUpstreamEOF(t *testing.T) {
	for _, encoding := range []string{"gzip", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			releaseStream := func() { once.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", encoding)
				w.Header().Set("Content-Type", "text/event-stream")
				var writer interface {
					io.WriteCloser
					Flush() error
				}
				if encoding == "gzip" {
					writer = gzip.NewWriter(w)
				} else {
					var err error
					writer, err = zstd.NewWriter(w, zstd.WithEncoderConcurrency(1))
					if err != nil {
						t.Error(err)
						return
					}
				}
				defer writer.Close()
				firstEvent := strings.SplitN(encodingTestSSE, "\n\n", 2)[0] + "\n\n"
				if _, err := io.WriteString(writer, firstEvent); err != nil {
					t.Error(err)
					return
				}
				if err := writer.Flush(); err != nil {
					t.Error(err)
					return
				}
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			defer upstream.Close()
			defer releaseStream()
			server, _ := newEncodingTestProxy(t, Config{UpstreamURL: upstream.URL, TransformerRegistry: encodingTestCacheStripRegistry()})
			resp := sendEncodingTestRequest(t, server, []byte(`{"stream":true}`), "")
			reader := bufio.NewReader(resp.Body)
			event, err := reader.ReadString('\n')
			if err != nil || event != "event: message_start\n" {
				t.Fatalf("first event = %q, err = %v", event, err)
			}
			data, err := reader.ReadString('\n')
			if err != nil || !strings.Contains(data, `"input_tokens":15`) {
				t.Fatalf("first event data = %q, err = %v", data, err)
			}
			if resp.Header.Get("Content-Encoding") != "" {
				t.Fatal("decoded stream still has Content-Encoding")
			}
			releaseStream()
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProxyEncodedResponseWithoutBody(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodHead, http.StatusOK},
		{http.MethodGet, http.StatusNoContent},
		{http.MethodGet, http.StatusNotModified},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.method, tc.status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				if tc.method == http.MethodHead {
					w.Header().Set("Content-Length", "123")
				}
				w.WriteHeader(tc.status)
			}))
			defer upstream.Close()
			server, _ := newEncodingTestProxy(t, Config{UpstreamURL: upstream.URL})
			req, err := http.NewRequest(tc.method, server.URL+"/v1/messages", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != tc.status || len(body) != 0 {
				t.Fatalf("response = %d %q, err = %v", resp.StatusCode, body, err)
			}
			if tc.method == http.MethodHead && resp.ContentLength != 123 {
				t.Fatalf("HEAD representation length = %d, want 123", resp.ContentLength)
			}
		})
	}
}
