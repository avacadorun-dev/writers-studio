//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"writers.studio/internal/assistant"
	"writers.studio/internal/config"
	"writers.studio/internal/store"
)

func TestAssistantVoiceWebSocketKeepsTranscriptEditableUntilExplicitExecute(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dataStore, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, token, err := dataStore.Authenticate(
		ctx,
		"assistant-voice-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupContext, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
		Title: "Voice assistant", Kind: "free", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	node := workspace.Nodes[0]
	run, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID:         user.ID,
		ProjectID:           workspace.Project.ID,
		TargetKind:          assistant.TargetNode,
		TargetNodeID:        &node.ID,
		InputMode:           assistant.InputVoice,
		SilenceCheckpointMS: 3000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderSystem, Name: "fixture", Model: "fixture",
			Dialect: assistant.DialectChatCompletions,
			Capabilities: assistant.CapabilitySnapshot{
				Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
			},
		},
		ReadContext: map[string]any{"test": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(
		config.Config{
			LiveSTTAdapter: "fake",
			LLMAdapter:     "disabled",
			StaticDir:      ".",
		},
		dataStore,
		nil,
		nil,
		nil,
		logger,
	)
	// This test isolates the live transcript/checkpoint contract. Execution is
	// deliberately unavailable so the confirmed turn remains inspectable.
	server.assistantOrchestrator = nil
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	headers := http.Header{}
	headers.Set("Cookie", sessionCookieName+"="+token)
	socketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/assistant/runs/" + run.ID + "/voice"
	connection, _, err := websocket.Dial(ctx, socketURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")

	writeVoiceJSON(t, ctx, connection, map[string]any{
		"type": "fixture_segment", "text": "Напиши сцену о Лиоре",
		"startMs": 0, "endMs": 1400, "confidence": 0.98,
	})
	segment := readVoiceMessage(t, ctx, connection, "segment_final")
	if segment["transcript"] != "Напиши сцену о Лиоре" {
		t.Fatalf("stable transcript missing: %#v", segment)
	}
	checkpoint := readVoiceMessage(t, ctx, connection, "confirmation_required")
	if checkpoint["prompt"] != "Выполнить действие?" ||
		checkpoint["transcript"] != "Напиши сцену о Лиоре" {
		t.Fatalf("silence checkpoint is incomplete: %#v", checkpoint)
	}
	waitingTurns, err := dataStore.ListAssistantTurns(ctx, user.ID, run.ID)
	if err != nil || len(waitingTurns) != 1 ||
		waitingTurns[0].State != assistant.TurnAwaitingConfirmation {
		t.Fatalf("silence checkpoint was not durable: %#v %v", waitingTurns, err)
	}
	writeVoiceJSON(t, ctx, connection, map[string]any{
		"type": "edit_transcript", "text": "Напиши короткую сцену о Лиоре",
	})
	edited := readVoiceMessage(t, ctx, connection, "transcript_updated")
	if edited["transcript"] != "Напиши короткую сцену о Лиоре" {
		t.Fatalf("voice correction was not persisted: %#v", edited)
	}
	editedWaiting, err := dataStore.ListAssistantTurns(ctx, user.ID, run.ID)
	if err != nil || len(editedWaiting) != 1 ||
		editedWaiting[0].State != assistant.TurnAwaitingConfirmation ||
		editedWaiting[0].Transcript != "Напиши короткую сцену о Лиоре" {
		t.Fatalf("checkpoint correction was not durable: %#v %v", editedWaiting, err)
	}
	writeVoiceJSON(t, ctx, connection, map[string]any{"type": "confirm_execute"})
	queued := readVoiceMessage(t, ctx, connection, "turn_queued")
	if queued["started"] != false {
		t.Fatalf("isolated test unexpectedly started an LLM execution: %#v", queued)
	}

	turns, err := dataStore.ListAssistantTurns(ctx, user.ID, run.ID)
	if err != nil || len(turns) != 1 ||
		turns[0].State != assistant.TurnQueued ||
		turns[0].Instruction != "Напиши короткую сцену о Лиоре" {
		t.Fatalf("confirmed voice turn was not durable: %#v %v", turns, err)
	}
	unchanged, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil || unchanged.CurrentRevision != node.CurrentRevision {
		t.Fatalf("voice confirmation changed canonical content: %#v %v", unchanged, err)
	}

	draftRun, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID:         user.ID,
		ProjectID:           workspace.Project.ID,
		TargetKind:          assistant.TargetNode,
		TargetNodeID:        &node.ID,
		InputMode:           assistant.InputVoice,
		SilenceCheckpointMS: 0,
		Provider:            run.Provider,
		ReadContext:         map[string]any{"test": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDraftConnection := dialAssistantVoice(t, ctx, httpServer.URL, token, draftRun.ID)
	writeVoiceJSON(t, ctx, firstDraftConnection, map[string]any{
		"type": "fixture_segment", "text": "Первая часть",
		"startMs": 0, "endMs": 900, "confidence": 0.99,
	})
	_ = readVoiceMessage(t, ctx, firstDraftConnection, "segment_final")
	if err := firstDraftConnection.Close(websocket.StatusNormalClosure, "pause"); err != nil {
		t.Fatal(err)
	}

	restoredConnection := dialAssistantVoice(t, ctx, httpServer.URL, token, draftRun.ID)
	restored := readVoiceMessage(t, ctx, restoredConnection, "draft_restored")
	if restored["transcript"] != "Первая часть" {
		t.Fatalf("durable voice draft was not restored: %#v", restored)
	}
	writeVoiceJSON(t, ctx, restoredConnection, map[string]any{
		"type": "fixture_segment", "text": "вторая часть",
		"startMs": 900, "endMs": 1800, "confidence": 0.99,
	})
	continued := readVoiceMessage(t, ctx, restoredConnection, "segment_final")
	if continued["transcript"] != "Первая часть вторая часть" {
		t.Fatalf("restored voice draft did not continue: %#v", continued)
	}
	writeVoiceJSON(t, ctx, restoredConnection, map[string]any{"type": "discard"})
	_ = readVoiceMessage(t, ctx, restoredConnection, "draft_discarded")
	draftTurns, err := dataStore.ListAssistantTurns(ctx, user.ID, draftRun.ID)
	if err != nil || len(draftTurns) != 0 {
		t.Fatalf("discarded unconfirmed voice draft remained durable: %#v %v", draftTurns, err)
	}

	commandRun, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID: user.ID, ProjectID: workspace.Project.ID,
		TargetKind: assistant.TargetNode, TargetNodeID: &node.ID,
		InputMode: assistant.InputVoice, SilenceCheckpointMS: 0,
		Provider: run.Provider, ReadContext: map[string]any{"test": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	commandConnection := dialAssistantVoice(t, ctx, httpServer.URL, token, commandRun.ID)
	defer commandConnection.Close(websocket.StatusNormalClosure, "")
	writeVoiceJSON(t, ctx, commandConnection, map[string]any{
		"type": "edit_transcript", "text": "Сделай сцену короче",
	})
	manualVoiceDraft := readVoiceMessage(t, ctx, commandConnection, "transcript_updated")
	if manualVoiceDraft["transcript"] != "Сделай сцену короче" {
		t.Fatalf("typed correction did not create a durable voice draft: %#v", manualVoiceDraft)
	}
	writeVoiceJSON(t, ctx, commandConnection, map[string]any{
		"type": "fixture_segment", "text": "Команда, продолжить!",
		"startMs": 900, "endMs": 1300, "confidence": 0.99,
	})
	continuedCommand := readVoiceMessage(t, ctx, commandConnection, "command_recognized")
	if continuedCommand["command"] != "continue" {
		t.Fatalf("exact continue command was not recognized: %#v", continuedCommand)
	}
	_ = readVoiceMessage(t, ctx, commandConnection, "confirmation_dismissed")
	writeVoiceJSON(t, ctx, commandConnection, map[string]any{
		"type": "fixture_segment", "text": "и сохрани последнюю реплику",
		"startMs": 1300, "endMs": 2200, "confidence": 0.99,
	})
	combinedCommandDraft := readVoiceMessage(t, ctx, commandConnection, "segment_final")
	if combinedCommandDraft["transcript"] !=
		"Сделай сцену короче и сохрани последнюю реплику" {
		t.Fatalf("continue command leaked into the instruction: %#v", combinedCommandDraft)
	}
	writeVoiceJSON(t, ctx, commandConnection, map[string]any{
		"type": "fixture_segment", "text": "Команда, выполнить поручение.",
		"startMs": 2200, "endMs": 2800, "confidence": 0.99,
	})
	executeCommand := readVoiceMessage(t, ctx, commandConnection, "command_recognized")
	if executeCommand["command"] != "execute" {
		t.Fatalf("exact execute command was not recognized: %#v", executeCommand)
	}
	queuedCommand := readVoiceMessage(t, ctx, commandConnection, "turn_queued")
	if queuedCommand["started"] != false {
		t.Fatalf("isolated command unexpectedly started LLM: %#v", queuedCommand)
	}
	commandTurns, err := dataStore.ListAssistantTurns(ctx, user.ID, commandRun.ID)
	if err != nil || len(commandTurns) != 1 ||
		commandTurns[0].State != assistant.TurnQueued ||
		commandTurns[0].Instruction != "Сделай сцену короче и сохрани последнюю реплику" {
		t.Fatalf("exact voice commands corrupted the durable instruction: %#v %v", commandTurns, err)
	}
}

func dialAssistantVoice(
	t *testing.T,
	ctx context.Context,
	serverURL string,
	token string,
	runID string,
) *websocket.Conn {
	t.Helper()
	headers := http.Header{}
	headers.Set("Cookie", sessionCookieName+"="+token)
	socketURL := "ws" + strings.TrimPrefix(serverURL, "http") +
		"/api/assistant/runs/" + runID + "/voice"
	connection, _, err := websocket.Dial(ctx, socketURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func writeVoiceJSON(
	t *testing.T,
	ctx context.Context,
	connection *websocket.Conn,
	value any,
) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(ctx, websocket.MessageText, encoded); err != nil {
		t.Fatal(err)
	}
}

func readVoiceMessage(
	t *testing.T,
	ctx context.Context,
	connection *websocket.Conn,
	expectedType string,
) map[string]any {
	t.Helper()
	for {
		_, encoded, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if json.Unmarshal(encoded, &message) != nil {
			continue
		}
		if message["type"] == expectedType {
			return message
		}
	}
}
