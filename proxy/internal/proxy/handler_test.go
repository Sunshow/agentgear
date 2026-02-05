package proxy

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sunshow/agentgear/proxy/internal/transformer"
	"go.uber.org/zap"
)

func TestProcessSSEEvents_CreateDocumentsNoDuplicate(t *testing.T) {
	// Load test fixture
	fixtureData, err := os.ReadFile("../../testdata/fixtures/create_documents_duplicate/001_response.body")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	h := &Handler{
		logger:     logger,
		toolMapper: transformer.NewToolMapper(),
	}

	// Parse SSE events from fixture
	toolBlocks := make(map[int]*toolBlockState)
	var accumulator *toolBlockAccumulator
	var pendingMessageDelta *sseEvent
	nextOutputIndex := 0

	var outputEvents []sseEvent
	writeEvents := func(events []sseEvent) {
		outputEvents = append(outputEvents, events...)
	}

	// Tags for testing - simulate Droid + WARP scenario
	tags := []string{"$a_droid", "$u_warp"}

	// Create a mock request context for testing
	reqCtx := &requestContext{
		tags: tags,
	}

	// Count create_documents blocks in input
	inputCreateDocumentsCount := strings.Count(string(fixtureData), `"name":"create_documents"`)
	t.Logf("Input contains %d create_documents blocks", inputCreateDocumentsCount)

	reader := bufio.NewReader(strings.NewReader(string(fixtureData)))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSuffix(line, "\n")

		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")

			dataLine, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			dataLine = strings.TrimSuffix(dataLine, "\n")

			if strings.HasPrefix(dataLine, "data: ") {
				data := strings.TrimPrefix(dataLine, "data: ")
				events := h.processSSEEvent(eventType, data, toolBlocks, &nextOutputIndex, &accumulator, &pendingMessageDelta, writeEvents, tags, reqCtx)
				outputEvents = append(outputEvents, events...)
			}

			// Skip empty line
			_, _ = reader.ReadString('\n')
		}
	}

	// Find ExitSpecMode tool_use block in output
	var exitSpecModeEvents []sseEvent
	for _, evt := range outputEvents {
		if evt.eventType == "content_block_start" || evt.eventType == "content_block_delta" || evt.eventType == "content_block_stop" {
			if strings.Contains(evt.data, "ExitSpecMode") || strings.Contains(evt.data, `"plan"`) {
				exitSpecModeEvents = append(exitSpecModeEvents, evt)
			}
		}
	}

	t.Logf("Total output events: %d", len(outputEvents))
	t.Logf("ExitSpecMode events: %d", len(exitSpecModeEvents))

	// Log all ExitSpecMode related events for debugging
	for i, evt := range exitSpecModeEvents {
		t.Logf("ExitSpecMode event %d: type=%s, data=%s", i, evt.eventType, evt.data[:min(200, len(evt.data))])
	}

	if len(exitSpecModeEvents) == 0 {
		// Log some output events to debug
		for i, evt := range outputEvents {
			if i < 10 {
				t.Logf("Output event %d: type=%s, data=%s", i, evt.eventType, evt.data[:min(100, len(evt.data))])
			}
		}
		t.Fatal("no ExitSpecMode events found in output")
	}

	// Count how many content_block_start events have ExitSpecMode
	startCount := 0
	for _, evt := range outputEvents {
		if evt.eventType == "content_block_start" && strings.Contains(evt.data, "ExitSpecMode") {
			startCount++
		}
	}

	if startCount != 1 {
		t.Errorf("expected exactly 1 ExitSpecMode content_block_start, got %d", startCount)
	}

	// Extract the plan content and verify no duplication
	foundPlan := false
	for _, evt := range outputEvents {
		if evt.eventType == "content_block_delta" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(evt.data), &payload); err != nil {
				continue
			}
			delta, ok := payload["delta"].(map[string]interface{})
			if !ok {
				continue
			}
			partialJSON, ok := delta["partial_json"].(string)
			if !ok {
				continue
			}

			var input map[string]interface{}
			if err := json.Unmarshal([]byte(partialJSON), &input); err != nil {
				continue
			}

			plan, ok := input["plan"].(string)
			if !ok {
				continue
			}
			foundPlan = true

			// Check that title is not duplicated (should appear exactly once as the main title)
			// Note: "# 工具调用" may appear multiple times as section headers in the plan content, which is normal
			mainTitleCount := strings.Count(plan, "# 工具调用分开记录及配置控制")
			if mainTitleCount > 1 {
				t.Errorf("plan contains duplicated main title: found '# 工具调用分开记录及配置控制' %d times", mainTitleCount)
			}

			// Title should not be duplicated
			title, _ := input["title"].(string)
			if title != "" {
				titleCount := strings.Count(title, "工具调用分开记录")
				if titleCount > 1 {
					t.Errorf("title contains duplicated content: found '工具调用分开记录' %d times in: %s", titleCount, title)
				}
			}

			t.Logf("Plan length: %d characters", len(plan))
			t.Logf("Title: %s", title)
			t.Logf("Plan preview (first 200 chars): %s", plan[:min(200, len(plan))])
		}
	}

	if !foundPlan {
		t.Error("No plan content found in output events")
	}
}

func TestFlushAccumulator_MergesContent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := &Handler{
		logger:     logger,
		toolMapper: transformer.NewToolMapper(),
	}

	acc := &toolBlockAccumulator{
		toolName:     "create_documents",
		firstToolID:  "toolu_test",
		titleParts:   []string{"Part1", "Part2"},
		contentParts: []string{"Content1", "Content2"},
		blockCount:   2,
	}

	// Tags for testing - simulate Droid + WARP scenario
	tags := []string{"$a_droid", "$u_warp"}

	// Create a mock request context for testing
	reqCtx := &requestContext{
		tags: tags,
	}

	nextOutputIndex := 0
	events := h.flushAccumulator(acc, &nextOutputIndex, tags, reqCtx)

	if len(events) != 3 {
		t.Fatalf("expected 3 events (start, delta, stop), got %d", len(events))
	}

	// Verify start event
	if events[0].eventType != "content_block_start" {
		t.Errorf("expected content_block_start, got %s", events[0].eventType)
	}
	if !strings.Contains(events[0].data, "ExitSpecMode") {
		t.Errorf("expected ExitSpecMode in start event, got %s", events[0].data)
	}

	// Verify delta event contains merged content
	if events[1].eventType != "content_block_delta" {
		t.Errorf("expected content_block_delta, got %s", events[1].eventType)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(events[1].data), &payload); err != nil {
		t.Fatalf("failed to parse delta: %v", err)
	}

	delta := payload["delta"].(map[string]interface{})
	partialJSON := delta["partial_json"].(string)

	var input map[string]interface{}
	if err := json.Unmarshal([]byte(partialJSON), &input); err != nil {
		t.Fatalf("failed to parse partial_json: %v", err)
	}

	plan := input["plan"].(string)
	title := input["title"].(string)

	if plan != "Content1Content2" {
		t.Errorf("expected merged content 'Content1Content2', got '%s'", plan)
	}
	if title != "Part1Part2" {
		t.Errorf("expected merged title 'Part1Part2', got '%s'", title)
	}

	// Verify stop event
	if events[2].eventType != "content_block_stop" {
		t.Errorf("expected content_block_stop, got %s", events[2].eventType)
	}
}
