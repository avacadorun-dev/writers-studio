package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
)

func TestAssistantExecutionManagerSerializesRunsAndCancels(t *testing.T) {
	t.Parallel()
	manager := newAssistantExecutionManager()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var calls atomic.Int32
	if !manager.start("run-1", func(ctx context.Context) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		close(cancelled)
	}) {
		t.Fatal("first execution did not start")
	}
	<-started
	if manager.start("run-1", func(context.Context) { calls.Add(1) }) {
		t.Fatal("duplicate execution started for the same run")
	}
	if !manager.stop("run-1") {
		t.Fatal("active execution was not stopped")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("execution context was not cancelled")
	}
	deadline := time.Now().Add(time.Second)
	for {
		manager.mu.Lock()
		active := len(manager.cancels)
		manager.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("finished execution remained registered")
		}
		time.Sleep(time.Millisecond)
	}
	if manager.stop("missing") || calls.Load() != 1 {
		t.Fatalf("unexpected execution manager state: calls=%d", calls.Load())
	}
}

func TestAssistantExecutionManagerAllowsIndependentRunsInParallel(t *testing.T) {
	t.Parallel()
	manager := newAssistantExecutionManager()
	started := make(chan string, 2)
	release := make(chan struct{})
	for _, runID := range []string{"run-1", "run-2"} {
		runID := runID
		if !manager.start(runID, func(ctx context.Context) {
			started <- runID
			select {
			case <-release:
			case <-ctx.Done():
			}
		}) {
			t.Fatalf("independent execution %s did not start", runID)
		}
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case runID := <-started:
			seen[runID] = true
		case <-time.After(time.Second):
			t.Fatal("independent runs did not execute concurrently")
		}
	}
	if !seen["run-1"] || !seen["run-2"] {
		t.Fatalf("unexpected parallel run set: %#v", seen)
	}
	close(release)
}

func TestAssistantThreadTitleUsesTheFirstInstruction(t *testing.T) {
	t.Parallel()
	if title := assistantThreadTitle("", "  Сделай   сцену тревожнее  ", assistant.InputChat); title != "Сделай сцену тревожнее" {
		t.Fatalf("thread title=%q", title)
	}
	long := strings.Repeat("а", 100)
	if title := assistantThreadTitle("", long, assistant.InputChat); len([]rune(title)) != 80 || !strings.HasSuffix(title, "…") {
		t.Fatalf("long title was not compacted: %q", title)
	}
	if title := assistantThreadTitle("", "", assistant.InputVoice); title != "Голосовое поручение" {
		t.Fatalf("voice title=%q", title)
	}
}

func TestSafeProviderHeadersRejectsCredentialAndRequestSmugglingHeaders(t *testing.T) {
	t.Parallel()
	valid, err := safeProviderHeaders(map[string]string{
		"X-Tenant": " writers ",
		"X-Trace":  "enabled",
	})
	if err != nil || valid["X-Tenant"] != "writers" {
		t.Fatalf("safe provider headers were rejected: %#v %v", valid, err)
	}
	invalid := []map[string]string{
		{"Authorization": "Bearer secret"},
		{"Cookie": "session=secret"},
		{"Host": "internal"},
		{"X-Test\r\nInjected": "value"},
		{"X-Test": "value\r\nInjected: true"},
		{"X:Bad": "value"},
		{"": "value"},
		{"X-Test": ""},
		{"X-Test": strings.Repeat("x", 2_001)},
	}
	for _, headers := range invalid {
		if _, err := safeProviderHeaders(headers); err == nil {
			t.Fatalf("unsafe headers accepted: %#v", headers)
		}
	}
	tooMany := make(map[string]string)
	for index := 0; index < 21; index++ {
		tooMany[string(rune('a'+index))] = "value"
	}
	if _, err := safeProviderHeaders(tooMany); err == nil {
		t.Fatal("provider header count limit was not enforced")
	}
}

func TestMemoryRequestMappingAndAssistantErrorResponses(t *testing.T) {
	t.Parallel()
	nodeID := "node"
	record := memoryFromRequest("project", "memory", memoryRequest{
		Kind: assistant.MemoryConstraint, Statement: "  Не менять финальную реплику. ",
		Scope: "node", ScopeNodeID: &nodeID, Tags: []string{"dialogue"},
		Active: true, AlwaysRelevant: true,
	})
	if record.ProjectID != "project" || record.ID != "memory" ||
		record.Statement != "Не менять финальную реплику." ||
		record.Source != "manual" || record.ScopeNodeID == nil {
		t.Fatalf("memory request was mapped incorrectly: %#v", record)
	}

	tests := []struct {
		err    error
		status int
		code   string
	}{
		{pgx.ErrNoRows, http.StatusNotFound, "not_found"},
		{assistant.ErrStaleChangeSet, http.StatusConflict, "stale_change_set"},
		{assistant.ErrRunBusy, http.StatusConflict, "run_busy"},
		{assistant.ErrInvalidRun, http.StatusUnprocessableEntity, "invalid_assistant_request"},
		{assistant.ErrInvalidTurn, http.StatusUnprocessableEntity, "invalid_assistant_request"},
		{assistant.ErrInvalidMemory, http.StatusUnprocessableEntity, "invalid_assistant_request"},
		{assistant.ErrInvalidChangeSet, http.StatusUnprocessableEntity, "invalid_assistant_request"},
		{errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handleAssistantError(response, test.err)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf(
				"error %v mapped to %d %s",
				test.err,
				response.Code,
				response.Body.String(),
			)
		}
	}
}

func TestAssistantEventCursorNeverReplaysBehindLastEventID(t *testing.T) {
	t.Parallel()
	if got := assistantEventCursor("", "12"); got != 12 {
		t.Fatalf("Last-Event-ID cursor = %d", got)
	}
	if got := assistantEventCursor("0", "12"); got != 12 {
		t.Fatalf("zero query replayed behind Last-Event-ID: %d", got)
	}
	if got := assistantEventCursor("25", "12"); got != 25 {
		t.Fatalf("explicit newer query cursor = %d", got)
	}
	if got := assistantEventCursor("invalid", "invalid"); got != 0 {
		t.Fatalf("invalid cursors were not ignored: %d", got)
	}
}

func TestAssistantVoiceStateTranscriptAndTimerLifecycle(t *testing.T) {
	t.Parallel()
	state := &assistantVoiceState{
		segments: []string{"  первая часть ", "вторая часть  "},
	}
	if state.transcript() != "первая часть  вторая часть" {
		t.Fatalf("segments were not combined: %q", state.transcript())
	}
	state.turn = &assistant.Turn{ID: "turn", Transcript: "  исправленный текст "}
	if state.transcript() != "исправленный текст" || voiceTurnID(state) != "turn" {
		t.Fatal("persisted turn transcript was not preferred")
	}
	state.silenceTimer = time.AfterFunc(time.Hour, func() {})
	state.stopTimer()
	if state.silenceTimer != nil {
		t.Fatal("silence timer was not cleared")
	}
	if voiceTurnID(&assistantVoiceState{}) != nil {
		t.Fatal("empty voice state exposed a turn")
	}
}
