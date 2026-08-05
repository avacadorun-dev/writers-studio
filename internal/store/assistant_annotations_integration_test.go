//go:build integration

package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
)

func TestAssistantAnnotationFixApplyUndoAndDurableHistory(t *testing.T) {
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
		ctx, "assistant-annotation-"+uuid.NewString()+"@example.test",
		"writers-studio", time.Hour, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, CreateProjectInput{
		Title: "Assistant annotations", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := workspace.Nodes[0].ID
	beforeContent := annotationDocument("Старый фрагмент рядом")
	if _, err := dataStore.SaveRevision(ctx, user.ID, nodeID, SaveRevisionInput{
		BaseRevision: 1, Content: beforeContent,
		PlainText: "Старый фрагмент рядом", Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	note, err := dataStore.CreateDocumentAnnotation(ctx, user.ID, nodeID, CreateAnnotationInput{
		Kind: "note", Note: "Переписать точнее", BaseRevision: 2,
		Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 0, End: 6, ExactText: "Старый",
			Suffix: " фрагмент рядом",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID: user.ID, ProjectID: workspace.Project.ID,
		TargetKind: assistant.TargetNode, TargetNodeID: &nodeID,
		InputMode: assistant.InputChat, Status: assistant.RunActive,
		SilenceCheckpointMS: 5000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderSystem, Name: "integration", Model: "fixture",
			Dialect: assistant.DialectResponses,
		},
		ReadContext: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := dataStore.CreateAssistantTurn(
		ctx, user.ID, run.ID, assistant.InputChat,
		"Исправь заметку", assistant.TurnQueued,
	)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := dataStore.ProjectFingerprint(ctx, user.ID, workspace.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := dataStore.GetDocumentNode(ctx, user.ID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, _ := json.Marshal(node)
	proposed := annotationDocument("Новый фрагмент рядом")
	baseRevision := node.CurrentRevision
	arguments, _ := json.Marshal(map[string]any{
		"nodeId": nodeID, "baseRevision": baseRevision,
		"edits": []map[string]any{{
			"blockId": "block-1", "start": 0, "end": 6,
			"expectedText": "Старый", "replacement": "Новый",
		}},
		"annotationFixes": []map[string]any{{
			"annotationId": note.ID, "annotationRevision": note.Revision,
			"originalText": "Старый", "replacement": "Новый", "resolve": true,
		}},
	})
	changeSet, err := dataStore.StageAssistantChangeSet(ctx, assistant.ChangeSet{
		RunID: run.ID, TurnID: turn.ID, Title: "Исправить заметку",
		Summary: "Точная правка", RiskLevel: assistant.RiskLow,
		BaseFingerprint: fingerprint,
		Operations: []assistant.ChangeOperation{{
			GroupKey: "annotation:" + nodeID, Selected: true,
			ToolName: "annotation.propose_fixes", TargetKind: "document",
			TargetID: &nodeID, BaseRevision: &baseRevision, Arguments: arguments,
			BeforeSnapshot: beforeSnapshot, ProposedSnapshot: proposed,
			RiskLevel: assistant.RiskLow,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.ApplyAssistantChangeSet(
		ctx, user.ID, run.ID, changeSet.ID, []string{"annotation:" + nodeID},
	); err != nil {
		t.Fatal(err)
	}
	afterApply, err := dataStore.GetDocumentAnnotation(ctx, user.ID, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterApply.Anchor.ExactText != "Новый" || afterApply.Anchor.End != 5 ||
		afterApply.Status != "resolved" || afterApply.AnchorStatus != "attached" ||
		afterApply.BaseRevision != 3 || afterApply.Revision != 2 {
		t.Fatalf("applied annotation = %#v", afterApply)
	}
	history, err := dataStore.ListDocumentAnnotationHistory(ctx, user.ID, note.ID)
	if err != nil || len(history) != 1 || history[0].Before.Anchor.ExactText != "Старый" ||
		history[0].After.Anchor.ExactText != "Новый" || history[0].EventKind != "assistant_resolve" {
		t.Fatalf("apply history = %#v, err=%v", history, err)
	}
	if _, err := dataStore.UndoAssistantChangeSet(ctx, user.ID, run.ID, changeSet.ID); err != nil {
		t.Fatal(err)
	}
	afterUndo, err := dataStore.GetDocumentAnnotation(ctx, user.ID, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUndo.Anchor.ExactText != "Старый" || afterUndo.Status != "open" ||
		afterUndo.AnchorStatus != "attached" || afterUndo.BaseRevision != 4 ||
		afterUndo.Revision != 3 {
		t.Fatalf("undone annotation = %#v", afterUndo)
	}
	history, err = dataStore.ListDocumentAnnotationHistory(ctx, user.ID, note.ID)
	if err != nil || len(history) != 2 || history[0].EventKind != "assistant_undo" {
		t.Fatalf("undo history = %#v, err=%v", history, err)
	}
	if err := dataStore.UpdateAssistantRunStatus(
		ctx, user.ID, run.ID, assistant.RunCompleted, nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.DeleteAssistantRunHistory(ctx, user.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	history, err = dataStore.ListDocumentAnnotationHistory(ctx, user.ID, note.ID)
	if err != nil || len(history) != 2 || history[0].AssistantRunID != nil ||
		history[0].AssistantChangeSetID != nil || history[0].ChangeOperationID != nil {
		t.Fatalf("durable annotation audit = %#v, err=%v", history, err)
	}
}
