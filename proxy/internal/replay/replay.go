package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RequestMeta mirrors the logged request metadata structure.
type RequestMeta struct {
	ID        string              `json:"id"`
	SessionID string              `json:"session_id"`
	Sequence  int                 `json:"sequence"`
	Timestamp time.Time           `json:"timestamp"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
}

// Options holds replay configuration.
type Options struct {
	Server       string
	Dir          string
	Transformed  bool
	Headers      map[string]string // extra/override headers Key->Value
	Timeout      int
	Quiet        bool
	Sequences    map[int]bool // nil means all
	PathOverride string       // if set, overrides the request path from logs
}

// Result tracks the outcome of a single replayed request.
type Result struct {
	SessionID  string
	Sequence   int
	Method     string
	Path       string
	StatusCode int
	Duration   time.Duration
	BodySize   int64
	Streaming  bool
	FirstEvent string
	Err        error
}

// Run executes the replay against the target server.
func Run(opts Options) error {
	sessions, err := discoverSessions(opts.Dir)
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found in %s", opts.Dir)
	}

	client := &http.Client{
		Timeout: time.Duration(opts.Timeout) * time.Second,
		// Don't follow redirects automatically
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var allResults []Result
	totalSuccess, totalFailed := 0, 0

	for _, sess := range sessions {
		fmt.Printf("\n=== Session: %s ===\n", filepath.Base(sess.dir))

		requests, err := discoverRequests(sess.dir)
		if err != nil {
			fmt.Printf("  [ERROR] failed to read session: %v\n", err)
			totalFailed++
			continue
		}

		for _, req := range requests {
			if opts.Sequences != nil && !opts.Sequences[req.sequence] {
				continue
			}

			result := replayRequest(client, opts, sess.dir, req)
			allResults = append(allResults, result)

			if result.Err != nil {
				totalFailed++
				fmt.Printf("[%03d] %s %s\n", result.Sequence, result.Method, result.Path)
				fmt.Printf("  → Error: %v\n", result.Err)
			} else {
				totalSuccess++
				fmt.Printf("[%03d] %s %s\n", result.Sequence, result.Method, result.Path)
				fmt.Printf("  → Status: %d, Duration: %dms\n", result.StatusCode, result.Duration.Milliseconds())
				if result.Streaming {
					fmt.Printf("  → Response: %d bytes (streaming)", result.BodySize)
					if result.FirstEvent != "" {
						fmt.Printf(", First event: %s", result.FirstEvent)
					}
					fmt.Println()
				} else {
					fmt.Printf("  → Response: %d bytes\n", result.BodySize)
				}
			}
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total: %d requests, %d success, %d failed\n",
		totalSuccess+totalFailed, totalSuccess, totalFailed)

	return nil
}

// session represents a discovered session directory.
type session struct {
	dir  string
	name string
}

// request represents a discovered request within a session.
type request struct {
	sequence int
	metaFile string
}

// discoverSessions detects whether dir is a single session or a parent of multiple sessions.
func discoverSessions(dir string) ([]session, error) {
	// Check if dir itself is a session (contains NNN_request.json files)
	if isSingleSession(dir) {
		return []session{{dir: dir, name: filepath.Base(dir)}}, nil
	}

	// Scan subdirectories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		if isSingleSession(subDir) {
			sessions = append(sessions, session{dir: subDir, name: entry.Name()})
		}
	}

	// Sort by directory name (timestamp prefix ensures chronological order)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].name < sessions[j].name
	})

	return sessions, nil
}

// isSingleSession checks if a directory contains request.json files.
func isSingleSession(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_request.json") {
			return true
		}
	}
	return false
}

// discoverRequests finds all request entries in a session directory, sorted by sequence.
func discoverRequests(dir string) ([]request, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var requests []request
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, "_request.json") {
			continue
		}
		// Parse sequence number from prefix like "001"
		prefix := strings.TrimSuffix(name, "_request.json")
		seq, err := strconv.Atoi(prefix)
		if err != nil {
			continue
		}
		requests = append(requests, request{
			sequence: seq,
			metaFile: filepath.Join(dir, name),
		})
	}

	sort.Slice(requests, func(i, j int) bool {
		return requests[i].sequence < requests[j].sequence
	})

	return requests, nil
}

// replayRequest sends a single request and returns the result.
func replayRequest(client *http.Client, opts Options, sessionDir string, req request) Result {
	result := Result{Sequence: req.sequence}

	// Read metadata
	metaData, err := os.ReadFile(req.metaFile)
	if err != nil {
		result.Err = fmt.Errorf("read meta: %w", err)
		return result
	}

	var meta RequestMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		result.Err = fmt.Errorf("parse meta: %w", err)
		return result
	}

	result.SessionID = meta.SessionID
	result.Method = meta.Method
	result.Path = meta.Path

	// Read request body
	prefix := fmt.Sprintf("%03d", req.sequence)
	bodyFile := filepath.Join(sessionDir, prefix+"_request.body")
	if opts.Transformed {
		transformedFile := filepath.Join(sessionDir, prefix+"_request_transformed.body")
		if _, err := os.Stat(transformedFile); err == nil {
			bodyFile = transformedFile
		}
		// Fall back to original if transformed doesn't exist
	}

	var body []byte
	if _, err := os.Stat(bodyFile); err == nil {
		body, err = os.ReadFile(bodyFile)
		if err != nil {
			result.Err = fmt.Errorf("read body: %w", err)
			return result
		}
	}

	// Build URL
	reqPath := meta.Path
	if opts.PathOverride != "" {
		reqPath = opts.PathOverride
	}
	url := strings.TrimRight(opts.Server, "/") + reqPath

	// Create HTTP request
	httpReq, err := http.NewRequest(meta.Method, url, bytes.NewReader(body))
	if err != nil {
		result.Err = fmt.Errorf("create request: %w", err)
		return result
	}

	// Restore headers from log (skip REDACTED values)
	for key, values := range meta.Headers {
		for _, val := range values {
			if val == "[REDACTED]" {
				continue
			}
			httpReq.Header.Add(key, val)
		}
	}

	// Apply override headers
	for key, val := range opts.Headers {
		httpReq.Header.Set(key, val)
	}

	// Ensure Content-Length is correct for the body we're sending
	if len(body) > 0 {
		httpReq.Header.Set("Content-Length", strconv.Itoa(len(body)))
		httpReq.ContentLength = int64(len(body))
	}

	// Send request
	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		result.Err = fmt.Errorf("send request: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Streaming = isStreamingResponse(resp)

	// Read response
	if result.Streaming {
		result.BodySize, result.FirstEvent = handleStreamingResponse(resp.Body, opts.Quiet)
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			result.Err = fmt.Errorf("read response: %w", err)
			result.Duration = time.Since(start)
			return result
		}
		result.BodySize = int64(len(respBody))
		if !opts.Quiet {
			fmt.Println(string(respBody))
		}
	}

	result.Duration = time.Since(start)
	return result
}

// isStreamingResponse checks if the response is SSE streaming.
func isStreamingResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/event-stream")
}

// handleStreamingResponse reads SSE events, optionally printing them.
// Returns total bytes read and the first event type.
func handleStreamingResponse(body io.Reader, quiet bool) (int64, string) {
	scanner := bufio.NewScanner(body)
	// Increase buffer size for potentially large SSE data lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var totalBytes int64
	var firstEvent string

	for scanner.Scan() {
		line := scanner.Text()
		totalBytes += int64(len(line)) + 1 // +1 for newline

		if firstEvent == "" && strings.HasPrefix(line, "event: ") {
			firstEvent = strings.TrimPrefix(line, "event: ")
		}

		if !quiet {
			fmt.Println(line)
		}
	}

	return totalBytes, firstEvent
}

// ParseHeaders parses "Key:Value" strings into a map.
func ParseHeaders(headerFlags []string) map[string]string {
	headers := make(map[string]string)
	for _, h := range headerFlags {
		idx := strings.Index(h, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(h[:idx])
		val := strings.TrimSpace(h[idx+1:])
		headers[key] = val
	}
	return headers
}

// ParseSequences parses a comma-separated sequence string like "1,3,5" into a set.
func ParseSequences(s string) (map[int]bool, error) {
	if s == "" {
		return nil, nil
	}
	result := make(map[int]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		seq, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid sequence number: %q", part)
		}
		result[seq] = true
	}
	return result, nil
}
