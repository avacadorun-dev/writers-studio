//go:build integration

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"writers.studio/internal/assistant"
	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
)

func TestProjectTransferDatabaseRoundTrip(t *testing.T) {
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
		"transfer-"+time.Now().Format("20060102150405.000000000")+"@example.test",
		"writers-studio",
		time.Hour,
		true,
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
		Title: "Portable manuscript", Kind: "novel", Language: "ru", Synopsis: "Synopsis",
	})
	if err != nil {
		t.Fatal(err)
	}
	stepLimit := 40
	if _, err := dataStore.UpdateProjectAssistantSettings(
		ctx,
		user.ID,
		workspace.Project.ID,
		ProjectAssistantSettings{StepLimit: &stepLimit},
	); err != nil {
		t.Fatal(err)
	}
	sourceNode := workspace.Nodes[0]
	content := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"portable","sourceRefs":[]},"content":[{"type":"text","text":"Переносимый текст"}]}]}`)
	if _, err := dataStore.SaveRevision(ctx, user.ID, sourceNode.ID, SaveRevisionInput{
		BaseRevision: 1, Content: content, PlainText: "Переносимый текст", Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.CreateNamedVersion(ctx, user.ID, sourceNode.ID, "Перед переносом", "Контрольная версия"); err != nil {
		t.Fatal(err)
	}
	canvas, err := dataStore.CreateCanvas(ctx, user.ID, workspace.Project.ID, "Сюжет")
	if err != nil {
		t.Fatal(err)
	}
	canvasContent := json.RawMessage(fmt.Sprintf(
		`{"nodes":[{"id":"document-card","type":"file","x":0,"y":0,"width":280,"height":180,"file":"writers-studio://documents/%s","x-writers-studio":{"resource":{"kind":"document","id":"%s"}}}],"edges":[]}`,
		sourceNode.ID, sourceNode.ID,
	))
	references, err := canvasdocument.Validate(canvasContent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.SaveCanvasRevision(ctx, user.ID, canvas.ID, SaveCanvasRevisionInput{
		BaseRevision: 1, SchemaVersion: canvasdocument.SchemaVersion,
		Content: canvasContent, Source: "manual", References: references,
	}); err != nil {
		t.Fatal(err)
	}
	term, err := dataStore.CreateDictionaryTerm(ctx, user.ID, workspace.Project.ID, "Элиас", "character", "ru", []string{"Эл"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.CreateDocumentAnnotation(ctx, user.ID, sourceNode.ID, CreateAnnotationInput{
		Kind: "dictionary_link", DictionaryTermID: &term.ID, BaseRevision: 2,
		Anchor: domain.AnnotationAnchor{BlockID: "portable", Start: 0, End: 11, ExactText: "Переносимый"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.CreateDocumentAnnotation(ctx, user.ID, sourceNode.ID, CreateAnnotationInput{
		Kind: "note", Note: "Проверить перенос", BaseRevision: 2,
		Anchor: domain.AnnotationAnchor{BlockID: "portable", Start: 12, End: 17, ExactText: "текст"},
	}); err != nil {
		t.Fatal(err)
	}

	session, err := dataStore.CreateDictationSession(ctx, user.ID, workspace.Project.ID, &sourceNode.ID, "transfer-session", "ru")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := dataStore.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID: session.ID, Kind: "live_final", PipelineVersion: "1", Provider: "test",
		Model: "test", Language: "ru", Text: "Голосовой текст", Segments: []domain.TranscriptSegment{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID: session.ID, SourceArtifactID: &artifact.ID, BaseSourceText: artifact.Text,
		Content: content, PlainText: artifact.Text, Source: "live_source",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.SaveAssistantGuide(ctx, user.ID, workspace.Project.ID, "Пиши точно.", "manual", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.SaveProjectMemory(ctx, user.ID, assistant.MemoryRecord{
		ProjectID: workspace.Project.ID, Kind: assistant.MemoryFact, Statement: "Герой боится высоты",
		Scope: "node", ScopeNodeID: &sourceNode.ID, SourceReferences: []assistant.SourceReference{},
		Tags: []string{}, Active: true, Source: "manual",
	}, nil); err != nil {
		t.Fatal(err)
	}

	transfer, err := dataStore.BuildProjectTransfer(ctx, user.ID, workspace.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transfer.Project.AssistantStepLimit == nil || *transfer.Project.AssistantStepLimit != stepLimit {
		t.Fatalf("exported assistant step limit = %#v", transfer.Project.AssistantStepLimit)
	}
	result, err := dataStore.ImportProjectTransfer(ctx, user.ID, transfer)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectID == workspace.Project.ID || result.FirstNodeID == sourceNode.ID {
		t.Fatalf("ids were not remapped: %#v", result)
	}
	imported, err := dataStore.GetProject(ctx, user.ID, result.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Project.Status != "active" || imported.Project.Title != workspace.Project.Title ||
		len(imported.Nodes) != 1 || imported.Nodes[0].PlainText != "Переносимый текст" {
		t.Fatalf("imported workspace mismatch: %#v", imported)
	}
	importedCanvases, err := dataStore.ListCanvases(ctx, user.ID, result.ProjectID, true)
	if err != nil || len(importedCanvases) != 1 || importedCanvases[0].ID == canvas.ID ||
		importedCanvases[0].CurrentRevision != 2 {
		t.Fatalf("imported canvases = %#v, err = %v", importedCanvases, err)
	}
	importedCanvas, err := dataStore.GetCanvasWorkspace(ctx, user.ID, importedCanvases[0].ID)
	if err != nil || len(importedCanvas.Documents) != 1 ||
		importedCanvas.Documents[0].ID != imported.Nodes[0].ID {
		t.Fatalf("imported canvas workspace = %#v, err = %v", importedCanvas, err)
	}
	importedAssistantSettings, err := dataStore.GetProjectAssistantSettings(
		ctx,
		user.ID,
		result.ProjectID,
	)
	if err != nil || importedAssistantSettings.StepLimit == nil ||
		*importedAssistantSettings.StepLimit != stepLimit {
		t.Fatalf("imported assistant settings = %#v, err = %v", importedAssistantSettings, err)
	}
	versions, err := dataStore.ListNamedVersions(ctx, user.ID, imported.Nodes[0].ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions = %#v, err = %v", versions, err)
	}
	terms, err := dataStore.ListDictionaryTerms(ctx, user.ID, result.ProjectID)
	if err != nil || len(terms) != 1 {
		t.Fatalf("terms = %#v, err = %v", terms, err)
	}
	importedAnnotations, err := dataStore.ListDocumentAnnotations(ctx, user.ID, imported.Nodes[0].ID)
	if err != nil || len(importedAnnotations) != 2 {
		t.Fatalf("annotations = %#v, err = %v", importedAnnotations, err)
	}
	for _, annotation := range importedAnnotations {
		if annotation.NodeID != imported.Nodes[0].ID || annotation.ProjectID != result.ProjectID {
			t.Fatalf("annotation IDs were not remapped: %#v", annotation)
		}
		if annotation.Kind == "dictionary_link" &&
			(annotation.DictionaryTermID == nil || *annotation.DictionaryTermID != terms[0].ID) {
			t.Fatalf("dictionary annotation target was not remapped: %#v", annotation)
		}
	}
	guide, err := dataStore.GetAssistantGuide(ctx, user.ID, result.ProjectID, true)
	if err != nil || guide.Content != "Пиши точно." {
		t.Fatalf("guide = %#v, err = %v", guide, err)
	}
	memory, err := dataStore.ListProjectMemory(ctx, user.ID, result.ProjectID, true)
	if err != nil || len(memory) != 1 || memory[0].ScopeNodeID == nil ||
		*memory[0].ScopeNodeID != imported.Nodes[0].ID || memory[0].SourceReferences == nil ||
		memory[0].Tags == nil {
		t.Fatalf("memory = %#v, err = %v", memory, err)
	}
	importedSession, err := dataStore.GetDictationSession(ctx, user.ID, sessionIDForProject(t, ctx, dataStore, result.ProjectID))
	if err != nil || importedSession.Status != "review_ready" {
		t.Fatalf("session = %#v, err = %v", importedSession, err)
	}
}

func sessionIDForProject(t *testing.T, ctx context.Context, dataStore *Store, projectID string) string {
	t.Helper()
	var sessionID string
	if err := dataStore.Pool.QueryRow(ctx, "SELECT id::text FROM dictation_sessions WHERE project_id=$1", projectID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	return sessionID
}
