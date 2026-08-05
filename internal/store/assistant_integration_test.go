//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
)

func TestAssistantPersistenceAndIsolation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dataStore, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	email := "assistant-" + uuid.NewString() + "@example.test"
	user, _, err := dataStore.Authenticate(ctx, email, "writers-studio", time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupContext, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, CreateProjectInput{
		Title: "Assistant manuscript", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	projectID := workspace.Project.ID
	nodeID := workspace.Nodes[0].ID

	connection, err := dataStore.CreateProviderConnection(ctx, ProviderConnectionRecord{
		OwnerUserID:       user.ID,
		Name:              "Compatible provider",
		BaseURL:           "https://provider.example/v1",
		Dialect:           assistant.DialectResponses,
		Model:             "writer",
		EncryptedAPIKey:   []byte("ciphertext"),
		APIKeyNonce:       []byte("123456789012"),
		APIKeyFingerprint: strings.Repeat("a", 64),
		APIKeyLast4:       "last",
		IsDefault:         true,
	})
	if err != nil || connection.Status != "unchecked" {
		t.Fatalf("create provider: %+v, %v", connection, err)
	}
	connections, err := dataStore.ListProviderConnections(ctx, user.ID)
	if err != nil || len(connections) != 1 || string(connections[0].EncryptedAPIKey) == "" {
		t.Fatalf("list provider: %+v, %v", connections, err)
	}
	connection, err = dataStore.UpdateProviderProbe(
		ctx,
		user.ID,
		connection.ID,
		"limited",
		"streaming unavailable",
		assistant.CapabilitySnapshot{
			Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
		},
	)
	if err != nil || !connection.Capabilities.CanRunAssistant() {
		t.Fatalf("update provider probe: %+v, %v", connection, err)
	}

	guide, err := dataStore.SaveAssistantGuide(
		ctx,
		user.ID,
		projectID,
		"# Стиль\n\nКороткие фразы.",
		"manual",
		nil,
		nil,
		nil,
	)
	if err != nil || guide.Revision != 1 || len(guide.History) != 1 {
		t.Fatalf("save guide: %+v, %v", guide, err)
	}
	guide, err = dataStore.SaveAssistantGuide(
		ctx,
		user.ID,
		projectID,
		"# Стиль\n\nТочные короткие фразы.",
		"manual",
		nil,
		nil,
		nil,
	)
	if err != nil || guide.Revision != 2 {
		t.Fatalf("update guide: %+v, %v", guide, err)
	}
	guide, err = dataStore.RestoreAssistantGuide(ctx, user.ID, projectID, 1)
	if err != nil || guide.Revision != 3 || guide.History[0].RestoredFromRevision == nil {
		t.Fatalf("restore guide as new revision: %+v, %v", guide, err)
	}

	memory, err := dataStore.SaveProjectMemory(ctx, user.ID, assistant.MemoryRecord{
		ProjectID: projectID,
		Kind:      assistant.MemoryCharacter,
		Statement: "Марина никогда не лжёт.",
		Scope:     "project",
		Tags:      []string{"Марина"},
		Active:    true,
		Source:    "manual",
	}, nil)
	if err != nil || memory.Revision != 1 {
		t.Fatalf("create memory: %+v, %v", memory, err)
	}
	baseRevision := memory.Revision
	memory.Statement = "Марина никогда не лжёт напрямую."
	memory, err = dataStore.SaveProjectMemory(ctx, user.ID, memory, &baseRevision)
	if err != nil || memory.Revision != 2 {
		t.Fatalf("update memory: %+v, %v", memory, err)
	}
	if _, err := dataStore.SaveProjectMemory(ctx, user.ID, memory, &baseRevision); err == nil {
		t.Fatal("stale memory update succeeded")
	}
	memories, err := dataStore.ListProjectMemory(ctx, user.ID, projectID, false)
	if err != nil || len(memories) != 1 {
		t.Fatalf("list memory: %+v, %v", memories, err)
	}
	memoryRevisions, err := dataStore.ListProjectMemoryRevisions(
		ctx, user.ID, projectID, memory.ID,
	)
	if err != nil || len(memoryRevisions) != 2 || memoryRevisions[0].Revision != 2 ||
		memoryRevisions[1].Revision != 1 ||
		memoryRevisions[0].Snapshot.Statement != "Марина никогда не лжёт напрямую." {
		t.Fatalf("memory revision history is incomplete: %+v, %v", memoryRevisions, err)
	}
	foreignMemoryRevisions, err := dataStore.ListProjectMemoryRevisions(
		ctx, uuid.NewString(), projectID, memory.ID,
	)
	if err != nil || len(foreignMemoryRevisions) != 0 {
		t.Fatalf("memory revision history escaped ownership: %+v, %v", foreignMemoryRevisions, err)
	}
	restoredMemory, err := dataStore.RestoreProjectMemory(
		ctx, user.ID, projectID, memory.ID, 1,
	)
	if err != nil || restoredMemory.Revision != 3 ||
		restoredMemory.Statement != "Марина никогда не лжёт." ||
		restoredMemory.Source != "manual" {
		t.Fatalf("restore memory revision: %+v, %v", restoredMemory, err)
	}
	memoryRevisions, err = dataStore.ListProjectMemoryRevisions(
		ctx, user.ID, projectID, memory.ID,
	)
	if err != nil || len(memoryRevisions) != 3 || memoryRevisions[0].Revision != 3 ||
		memoryRevisions[0].Snapshot.Statement != "Марина никогда не лжёт." {
		t.Fatalf("restored memory did not create a new revision: %+v, %v", memoryRevisions, err)
	}
	if _, err := dataStore.RestoreProjectMemory(
		ctx, uuid.NewString(), projectID, memory.ID, 1,
	); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign memory restore returned %v", err)
	}

	connectionID := connection.ID
	initialGuideRevision := guide.Revision
	run, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID:         user.ID,
		ProjectID:           projectID,
		TargetKind:          assistant.TargetNode,
		TargetNodeID:        &nodeID,
		InputMode:           assistant.InputVoice,
		Status:              assistant.RunActive,
		SilenceCheckpointMS: 5000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderConnection, ConnectionID: &connectionID,
			Name: connection.Name, Model: connection.Model, Dialect: connection.Dialect,
			Capabilities: connection.Capabilities,
		},
		GuideRevision: &initialGuideRevision,
		ReadContext:   map[string]any{"nodeIds": []string{nodeID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := &assistant.SelectionAttachment{
		NodeID: nodeID, BaseRevision: 1, Quote: "фрагмент",
		Ranges: []assistant.SelectionRange{{
			BlockID: "selection-block", Start: 0, End: 8, ExpectedText: "фрагмент",
		}},
	}
	turn, err := dataStore.CreateAssistantTurnWithSelection(
		ctx,
		user.ID,
		run.ID,
		assistant.InputVoice,
		"Допиши сцену",
		assistant.TurnAwaitingConfirmation,
		selection,
	)
	if err != nil || turn.Sequence != 1 || turn.GuideRevision == nil {
		t.Fatalf("create turn: %+v, %v", turn, err)
	}
	if *turn.GuideRevision != guide.Revision || run.GuideRevision == nil ||
		*run.GuideRevision != guide.Revision {
		t.Fatalf("initial guide revision was not captured: run=%+v turn=%+v", run, turn)
	}
	turnsWithSelection, err := dataStore.ListAssistantTurns(ctx, user.ID, run.ID)
	if err != nil || len(turnsWithSelection) != 1 ||
		turnsWithSelection[0].SelectionAttachment == nil ||
		turnsWithSelection[0].SelectionAttachment.Quote != selection.Quote {
		t.Fatalf("selection attachment was not persisted: %+v, %v", turnsWithSelection, err)
	}
	guideBaseRevision := guide.Revision
	guide, err = dataStore.SaveAssistantGuide(
		ctx,
		user.ID,
		projectID,
		"# Стиль\n\nНовая редакция действует со следующего подтверждённого поручения.",
		"manual",
		nil,
		nil,
		&guideBaseRevision,
	)
	if err != nil {
		t.Fatal(err)
	}
	pinnedGuideRevision, err := dataStore.PinAssistantTurnGuideRevision(
		ctx, user.ID, run.ID, turn.ID,
	)
	if err != nil || pinnedGuideRevision == nil || *pinnedGuideRevision != guide.Revision ||
		*run.GuideRevision == *pinnedGuideRevision {
		t.Fatalf(
			"turn confirmation did not pin the latest guide independently of the run: run=%v pinned=%v guide=%+v err=%v",
			run.GuideRevision,
			pinnedGuideRevision,
			guide,
			err,
		)
	}
	turn.GuideRevision = pinnedGuideRevision
	eventData, _ := assistant.NewEventData(map[string]string{"instruction": turn.Instruction})
	event, err := dataStore.AppendAssistantEvent(ctx, assistant.Event{
		RunID: run.ID, TurnID: &turn.ID, Type: assistant.EventTurnCheckpoint, Data: eventData,
	})
	if err != nil || event.Sequence != 1 {
		t.Fatalf("append event: %+v, %v", event, err)
	}
	events, err := dataStore.ListAssistantEvents(ctx, user.ID, run.ID, 0, 20)
	if err != nil || len(events) != 1 {
		t.Fatalf("list events: %+v, %v", events, err)
	}
	fingerprint, err := dataStore.ProjectFingerprint(ctx, user.ID, projectID)
	if err != nil || len(fingerprint) != 64 {
		t.Fatalf("project fingerprint: %q, %v", fingerprint, err)
	}
	beforeNode, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	beforeNodeSnapshot, _ := json.Marshal(beforeNode)
	baseDocumentRevision := beforeNode.CurrentRevision
	changeSet, err := dataStore.StageAssistantChangeSet(ctx, assistant.ChangeSet{
		RunID: run.ID, TurnID: turn.ID, Title: "Дополнить сцену",
		Summary: "Один абзац", RiskLevel: assistant.RiskLow,
		BaseFingerprint: fingerprint,
		Operations: []assistant.ChangeOperation{{
			GroupKey: "document", Selected: true,
			ToolName: "document.append", TargetKind: "document", TargetID: &nodeID,
			BaseRevision:   &baseDocumentRevision,
			Arguments:      json.RawMessage(`{"nodeId":"` + nodeID + `"}`),
			BeforeSnapshot: beforeNodeSnapshot,
			ProposedSnapshot: json.RawMessage(
				`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"assistant-block","sourceRefs":[]},"content":[{"type":"text","text":"Новый абзац"}]}]}`,
			),
			RiskLevel: assistant.RiskLow,
		}},
	})
	if err != nil || len(changeSet.Operations) != 1 {
		t.Fatalf("stage change set: %+v, %v", changeSet, err)
	}
	if changeSet.Operations[0].Dependencies == nil {
		t.Fatal("empty operation dependencies must serialize as [] rather than null")
	}
	latest, err := dataStore.GetLatestAssistantChangeSet(ctx, user.ID, run.ID)
	if err != nil || latest.ID != changeSet.ID || len(latest.Operations) != 1 {
		t.Fatalf("latest native change context missing: %+v, %v", latest, err)
	}
	if _, err := dataStore.GetLatestAssistantChangeSet(ctx, uuid.NewString(), run.ID); err == nil {
		t.Fatal("latest change context escaped run ownership")
	}
	run, err = dataStore.GetAssistantRun(ctx, user.ID, run.ID)
	if err != nil || run.LatestTurn == nil || run.PendingChangeSet == nil ||
		!run.Capabilities.ChangeReviewRequired {
		t.Fatalf("get durable run: %+v, %v", run, err)
	}
	applied, err := dataStore.ApplyAssistantChangeSet(
		ctx,
		user.ID,
		run.ID,
		changeSet.ID,
		[]string{"document"},
	)
	if err != nil || applied.State != assistant.ChangeApplied ||
		applied.OperationID == nil || !applied.Operations[0].Applied {
		t.Fatalf("apply change set: %+v, %v", applied, err)
	}
	afterApply, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil || afterApply.PlainText != "Новый абзац" ||
		afterApply.CurrentRevision != beforeNode.CurrentRevision+1 {
		t.Fatalf("assistant document revision missing: %+v, %v", afterApply, err)
	}
	revisions, err := dataStore.ListRevisions(ctx, user.ID, nodeID)
	if err != nil || revisions[0].Source != "assistant" ||
		revisions[0].OperationID == nil {
		t.Fatalf("assistant provenance missing: %+v, %v", revisions, err)
	}
	undone, err := dataStore.UndoAssistantChangeSet(
		ctx,
		user.ID,
		run.ID,
		changeSet.ID,
	)
	if err != nil || undone.State != assistant.ChangeUndone {
		t.Fatalf("undo change set: %+v, %v", undone, err)
	}
	afterUndo, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil || string(afterUndo.Content) != string(beforeNode.Content) {
		t.Fatalf("grouped undo did not restore document: %+v, %v", afterUndo, err)
	}
	runs, err := dataStore.ListAssistantRuns(ctx, user.ID, projectID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %+v, %v", runs, err)
	}
	parallelRun, parallelTurn, err := dataStore.CreateAssistantRunWithFirstTurn(
		ctx,
		assistant.Run{
			OwnerUserID: user.ID, ProjectID: projectID, Title: "Проверить параллельную сцену",
			TargetKind: assistant.TargetNode, TargetNodeID: &nodeID,
			InputMode: assistant.InputChat, Status: assistant.RunActive,
			SilenceCheckpointMS: 5000,
			Provider: assistant.ProviderRef{
				Kind: assistant.ProviderConnection, ConnectionID: &connectionID,
				Name: connection.Name, Model: connection.Model, Dialect: connection.Dialect,
				Capabilities: connection.Capabilities,
			},
			GuideRevision: &guide.Revision,
		},
		assistant.InputChat,
		"Проверь параллельную сцену",
		nil,
	)
	if err != nil || parallelTurn.Sequence != 1 || parallelRun.LatestTurn == nil ||
		parallelRun.Title != "Проверить параллельную сцену" {
		t.Fatalf("atomic first chat turn: run=%+v turn=%+v err=%v", parallelRun, parallelTurn, err)
	}
	parallelRun, err = dataStore.MarkAssistantRunViewed(ctx, user.ID, parallelRun.ID)
	if err != nil {
		t.Fatalf("mark active chat viewed: %+v, %v", parallelRun, err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, parallelRun.ID, assistant.RunWorking, nil,
	); err != nil {
		t.Fatal(err)
	}
	parallelRun, err = dataStore.GetAssistantRun(ctx, user.ID, parallelRun.ID)
	if err != nil || parallelRun.Unread {
		t.Fatalf("working chat without an error code must remain readable: %+v, %v", parallelRun, err)
	}
	failedCode := "provider_request_failed"
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, parallelRun.ID, assistant.RunFailed, &failedCode,
	); err != nil {
		t.Fatal(err)
	}
	parallelRun, err = dataStore.GetAssistantRun(ctx, user.ID, parallelRun.ID)
	if err != nil || !parallelRun.Unread {
		t.Fatalf("finished background chat was not unread: %+v, %v", parallelRun, err)
	}
	parallelRun, err = dataStore.MarkAssistantRunViewed(ctx, user.ID, parallelRun.ID)
	if err != nil || parallelRun.Unread {
		t.Fatalf("view marker was not durable: %+v, %v", parallelRun, err)
	}
	if err := dataStore.DeleteAssistantRunHistory(ctx, user.ID, parallelRun.ID); err != nil {
		t.Fatal(err)
	}
	historyRun, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID: user.ID, ProjectID: projectID,
		TargetKind: assistant.TargetNode, TargetNodeID: &nodeID,
		InputMode: assistant.InputChat, Status: assistant.RunActive,
		SilenceCheckpointMS: 5000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderConnection, ConnectionID: &connectionID,
			Name: connection.Name, Model: connection.Model, Dialect: connection.Dialect,
			Capabilities: connection.Capabilities,
		},
		GuideRevision: &guide.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	historyTurn, err := dataStore.CreateAssistantTurn(
		ctx,
		user.ID,
		historyRun.ID,
		assistant.InputChat,
		"Замени текст и сохрани результат",
		assistant.TurnQueued,
	)
	if err != nil {
		t.Fatal(err)
	}
	beforeHistoryDelete, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	beforeHistorySnapshot, _ := json.Marshal(beforeHistoryDelete)
	historyBaseRevision := beforeHistoryDelete.CurrentRevision
	historyFingerprint, err := dataStore.ProjectFingerprint(ctx, user.ID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	historySet, err := dataStore.StageAssistantChangeSet(ctx, assistant.ChangeSet{
		RunID: historyRun.ID, TurnID: historyTurn.ID, Title: "Правка перед удалением истории",
		Summary: "Проверка сохранности", RiskLevel: assistant.RiskLow,
		BaseFingerprint: historyFingerprint,
		Operations: []assistant.ChangeOperation{{
			GroupKey: "document", Selected: true,
			ToolName: "document.append", TargetKind: "document", TargetID: &nodeID,
			BaseRevision: &historyBaseRevision, BeforeSnapshot: beforeHistorySnapshot,
			Arguments: json.RawMessage(`{"nodeId":"` + nodeID + `"}`),
			ProposedSnapshot: json.RawMessage(
				`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"history-survives","sourceRefs":[]},"content":[{"type":"text","text":"Применённый текст пережил удаление истории"}]}]}`,
			),
			RiskLevel: assistant.RiskLow,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.ApplyAssistantChangeSet(
		ctx,
		user.ID,
		historyRun.ID,
		historySet.ID,
		[]string{"document"},
	); err != nil {
		t.Fatal(err)
	}
	appliedBeforeHistoryDelete, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx,
		user.ID,
		historyRun.ID,
		assistant.RunWorking,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.DeleteAssistantRunHistory(
		ctx,
		user.ID,
		historyRun.ID,
	); !errors.Is(err, assistant.ErrRunBusy) {
		t.Fatalf("working run history deletion was not blocked: %v", err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx,
		user.ID,
		historyRun.ID,
		assistant.RunActive,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.DeleteAssistantRunHistory(
		ctx,
		uuid.NewString(),
		historyRun.ID,
	); err == nil {
		t.Fatal("run history deletion escaped ownership")
	}
	if err := dataStore.DeleteAssistantRunHistory(ctx, user.ID, historyRun.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.GetAssistantRun(ctx, user.ID, historyRun.ID); err == nil {
		t.Fatal("assistant run history remained after deletion")
	}
	afterHistoryDelete, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil ||
		afterHistoryDelete.CurrentRevision != appliedBeforeHistoryDelete.CurrentRevision ||
		string(afterHistoryDelete.Content) != string(appliedBeforeHistoryDelete.Content) {
		t.Fatalf(
			"deleting run history changed applied project content: before=%+v after=%+v err=%v",
			appliedBeforeHistoryDelete,
			afterHistoryDelete,
			err,
		)
	}
	if err := dataStore.DisableProviderConnection(ctx, user.ID, connection.ID); err != nil {
		t.Fatal(err)
	}
	connections, err = dataStore.ListProviderConnections(ctx, user.ID)
	if err != nil || len(connections) != 0 {
		t.Fatalf("disabled provider leaked into list: %+v, %v", connections, err)
	}
}

func TestRecoverInterruptedAssistantRuns(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dataStore, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, _, err := dataStore.Authenticate(
		ctx,
		"assistant-recovery-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupContext, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, CreateProjectInput{
		Title: "Interrupted Assistant", Kind: "story", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID: user.ID, ProjectID: workspace.Project.ID,
		TargetKind: assistant.TargetProject, InputMode: assistant.InputChat,
		Status: assistant.RunActive, SilenceCheckpointMS: 5000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderSystem, Name: "test", Model: "test",
			Dialect: assistant.DialectChatCompletions,
			Capabilities: assistant.CapabilitySnapshot{
				Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := dataStore.CreateAssistantTurn(
		ctx, user.ID, run.ID, assistant.InputChat, "Большое поручение", assistant.TurnQueued,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	turn.State = assistant.TurnWorking
	turn.StartedAt = &now
	if err := dataStore.UpdateAssistantTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, run.ID, assistant.RunWorking, nil,
	); err != nil {
		t.Fatal(err)
	}
	eventData, _ := assistant.NewEventData(map[string]any{"step": 1})
	if _, err := dataStore.AppendAssistantEvent(ctx, assistant.Event{
		RunID: run.ID, TurnID: &turn.ID, Type: assistant.EventStepStarted, Data: eventData,
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := dataStore.RecoverInterruptedAssistantRuns(ctx)
	if err != nil || recovered < 1 {
		t.Fatalf("interrupted run was not recovered: count=%d error=%v", recovered, err)
	}
	run, err = dataStore.GetAssistantRun(ctx, user.ID, run.ID)
	if err != nil || run.Status != assistant.RunActive || run.LatestTurn == nil ||
		run.LatestTurn.State != assistant.TurnFailed || run.LatestTurn.ErrorCode == nil ||
		*run.LatestTurn.ErrorCode != "execution_interrupted" {
		t.Fatalf("interrupted run remained busy: %#v error=%v", run, err)
	}
	events, err := dataStore.ListAssistantEvents(ctx, user.ID, run.ID, 0, 20)
	if err != nil || len(events) != 2 || events[1].Type != assistant.EventRunError {
		t.Fatalf("interruption event was not durable: %#v error=%v", events, err)
	}
	secondPass, err := dataStore.RecoverInterruptedAssistantRuns(ctx)
	if err != nil || secondPass != 0 {
		t.Fatalf("recovery was not idempotent: count=%d error=%v", secondPass, err)
	}

	stoppedTurn, err := dataStore.CreateAssistantTurn(
		ctx, user.ID, run.ID, assistant.InputChat, "Остановить этот turn", assistant.TurnQueued,
	)
	if err != nil {
		t.Fatal(err)
	}
	stoppedTurn.State = assistant.TurnWorking
	stoppedTurn.StartedAt = &now
	if err := dataStore.UpdateAssistantTurn(ctx, stoppedTurn); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, run.ID, assistant.RunWorking, nil,
	); err != nil {
		t.Fatal(err)
	}
	cancelledCode := "cancelled"
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, run.ID, assistant.RunCancelled, &cancelledCode,
	); err != nil {
		t.Fatal(err)
	}
	run, err = dataStore.GetAssistantRun(ctx, user.ID, run.ID)
	if err != nil || run.LatestTurn == nil ||
		run.LatestTurn.State != assistant.TurnCancelled ||
		run.LatestTurn.CompletedAt == nil {
		t.Fatalf("terminal run left a working turn behind: run=%#v error=%v", run, err)
	}
}

func TestDynamicProjectStructureTemplatesAndNesting(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dataStore, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, _, err := dataStore.Authenticate(
		ctx,
		"structure-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupContext, "DELETE FROM users WHERE id=$1", user.ID)
	})

	tests := []struct {
		kind        string
		template    string
		initialType string
		build       func(t *testing.T, workspace ProjectWorkspace)
	}{
		{
			kind: "novel", template: "manuscript", initialType: "chapter",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				scene, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &workspace.Nodes[0].ID,
					"scene", "Вложенная сцена",
				)
				if err != nil || scene.Kind != "scene" {
					t.Fatalf("create manuscript scene: %+v, %v", scene, err)
				}
				if _, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &scene.ID, "chapter", "Нельзя",
				); !errors.Is(err, ErrInvalidParentType) {
					t.Fatalf("invalid manuscript nesting accepted: %v", err)
				}
			},
		},
		{
			kind: "story", template: "short_story", initialType: "scene",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				beat, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &workspace.Nodes[0].ID,
					"beat", "Поворот",
				)
				if err != nil || beat.CurrentRevision != 1 {
					t.Fatalf("create story beat: %+v, %v", beat, err)
				}
				if _, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &beat.ID, "note", "Мотив",
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			kind: "screenplay", template: "screenplay", initialType: "scene",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				act, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, nil, "act", "Акт I",
				)
				if err != nil || act.CurrentRevision != 0 {
					t.Fatalf("create structural act: %+v, %v", act, err)
				}
				sequence, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &act.ID, "sequence", "Погоня",
				)
				if err != nil {
					t.Fatal(err)
				}
				scene, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &sequence.ID, "scene", "Крыша",
				)
				if err != nil || scene.CurrentRevision != 1 {
					t.Fatalf("create screenplay scene: %+v, %v", scene, err)
				}
			},
		},
		{
			kind: "nonfiction", template: "nonfiction", initialType: "chapter",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				section, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &workspace.Nodes[0].ID,
					"section", "Аргумент",
				)
				if err != nil {
					t.Fatal(err)
				}
				research, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &section.ID,
					"research", "Источники",
				)
				if err != nil || research.CurrentRevision != 1 {
					t.Fatalf("create nonfiction research: %+v, %v", research, err)
				}
			},
		},
		{
			kind: "game_design", template: "game_design", initialType: "section",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				system, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &workspace.Nodes[0].ID,
					"system", "Боевая система",
				)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &system.ID, "feature", "Парирование",
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			kind: "free", template: "free", initialType: "document",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				folder, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, nil, "folder", "Исследования",
				)
				if err != nil || folder.CurrentRevision != 0 {
					t.Fatalf("create free folder: %+v, %v", folder, err)
				}
				if _, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &folder.ID, "document", "Гипотеза",
				); err != nil {
					t.Fatal(err)
				}
				if err := dataStore.MoveDocumentNode(
					ctx, user.ID, folder.ID, &workspace.Nodes[0].ID, 1000,
				); !errors.Is(err, ErrInvalidDocumentMove) {
					t.Fatalf("folder moved under document: %v", err)
				}
			},
		},
		{
			kind: "custom", template: "custom", initialType: "document",
			build: func(t *testing.T, workspace ProjectWorkspace) {
				group, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, nil, "group", "Контур",
				)
				if err != nil || group.CurrentRevision != 0 {
					t.Fatalf("create custom group: %+v, %v", group, err)
				}
				nested, err := dataStore.CreateDocumentNode(
					ctx, user.ID, workspace.Project.ID, &group.ID, "group", "Подконтур",
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := dataStore.MoveDocumentNode(
					ctx, user.ID, workspace.Nodes[0].ID, &nested.ID, 1000,
				); err != nil {
					t.Fatalf("move custom document into nested group: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.kind, func(t *testing.T) {
			workspace, err := dataStore.CreateProject(ctx, user.ID, CreateProjectInput{
				Title: test.kind, Kind: test.kind, Language: "ru",
			})
			if err != nil {
				t.Fatal(err)
			}
			if workspace.Project.Structure.Template != test.template ||
				workspace.Project.StructureRevision != 1 ||
				workspace.Nodes[0].Kind != test.initialType {
				t.Fatalf("unexpected project template: %+v", workspace)
			}
			structure, revision, err := dataStore.GetProjectStructure(
				ctx, user.ID, workspace.Project.ID,
			)
			if err != nil || revision != 1 || structure.Template != test.template {
				t.Fatalf("read structure: %+v, %d, %v", structure, revision, err)
			}
			test.build(t, workspace)
			incompatible := structure
			incompatible.NodeTypes = append(
				[]domain.ProjectNodeType(nil),
				structure.NodeTypes...,
			)
			for index := range incompatible.NodeTypes {
				incompatible.NodeTypes[index].AllowedChildren = nil
			}
			if _, _, err := dataStore.SaveProjectStructure(
				ctx, user.ID, workspace.Project.ID, revision, incompatible,
			); err == nil {
				t.Fatal("structure update that invalidates nesting succeeded")
			}
			structure.RootLabel += " · настроено"
			_, nextRevision, err := dataStore.SaveProjectStructure(
				ctx, user.ID, workspace.Project.ID, revision, structure,
			)
			if err != nil || nextRevision != 2 {
				t.Fatalf("save compatible structure: %d, %v", nextRevision, err)
			}
			if _, _, err := dataStore.SaveProjectStructure(
				ctx, user.ID, workspace.Project.ID, revision, structure,
			); err == nil {
				t.Fatal("stale structure update succeeded")
			}
		})
	}
}
