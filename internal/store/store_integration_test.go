//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
)

func TestPersistenceSafetyAndIdempotency(t *testing.T) {
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

	email := "integration-" + time.Now().Format("20060102150405.000000000") + "@example.test"
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
		Title:    "Integration manuscript",
		Kind:     "novel",
		Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	node := workspace.Nodes[0]
	part, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		nil,
		"part",
		"Part without manuscript content",
	)
	if err != nil {
		t.Fatal(err)
	}
	workspaceWithPart, err := dataStore.GetProject(ctx, user.ID, workspace.Project.ID)
	if err != nil {
		t.Fatalf("project with structural part must remain readable: %v", err)
	}
	if part.CurrentRevision != 0 || len(workspaceWithPart.Nodes) != 2 {
		t.Fatalf("unexpected structural part state: %#v %#v", part, workspaceWithPart.Nodes)
	}
	firstContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"first","sourceRefs":[]},"content":[{"type":"text","text":"Первая правка"}]}]}`,
	)
	revision, err := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: node.CurrentRevision,
		Content:      firstContent,
		PlainText:    "Первая правка",
		Source:       "manual",
	})
	if err != nil || revision.Revision != 2 {
		t.Fatalf("save revision: revision=%#v err=%v", revision, err)
	}
	child, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		&node.ID,
		"scene",
		"Nested scene",
	)
	if err != nil {
		t.Fatal(err)
	}
	childContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"child","sourceRefs":[]},"content":[{"type":"text","text":"Вложенная сцена"}]}]}`,
	)
	if _, err := dataStore.SaveRevision(ctx, user.ID, child.ID, SaveRevisionInput{
		BaseRevision: child.CurrentRevision,
		Content:      childContent,
		PlainText:    "Вложенная сцена",
		Source:       "manual",
	}); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.MoveDocumentNode(ctx, user.ID, child.ID, &node.ID, 9_000); err != nil {
		t.Fatal(err)
	}
	nextChapter, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		nil,
		"chapter",
		"Next chapter",
	)
	if err != nil {
		t.Fatal(err)
	}
	nextContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"next","sourceRefs":[]},"content":[{"type":"text","text":"Следующая глава"}]}]}`,
	)
	if _, err := dataStore.SaveRevision(ctx, user.ID, nextChapter.ID, SaveRevisionInput{
		BaseRevision: nextChapter.CurrentRevision,
		Content:      nextContent,
		PlainText:    "Следующая глава",
		Source:       "manual",
	}); err != nil {
		t.Fatal(err)
	}
	exportJob, err := dataStore.CreateExport(ctx, user.ID, workspace.Project.ID, CreateExportInput{
		Format:        "txt",
		NodeIDs:       []string{node.ID, child.ID, nextChapter.ID},
		ChapterTitles: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var exportSnapshot exportformat.Snapshot
	if err := json.Unmarshal(exportJob.Snapshot, &exportSnapshot); err != nil {
		t.Fatal(err)
	}
	if len(exportSnapshot.Documents) != 3 ||
		exportSnapshot.Documents[0].NodeID != node.ID ||
		exportSnapshot.Documents[1].NodeID != child.ID ||
		exportSnapshot.Documents[2].NodeID != nextChapter.ID {
		t.Fatalf("export did not preserve depth-first outline order: %#v", exportSnapshot.Documents)
	}
	_, conflictErr := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: 1,
		Content:      json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`),
		PlainText:    "Конфликт",
		Source:       "manual",
	})
	conflict, ok := AsConflict(conflictErr)
	if !ok || conflict.ConflictNodeID == "" || conflict.CurrentRevision != 2 {
		t.Fatalf("expected durable conflict copy, got %#v, %v", conflict, conflictErr)
	}
	conflictCopy, err := dataStore.GetDocumentNode(ctx, user.ID, conflict.ConflictNodeID)
	if err != nil {
		t.Fatalf("conflict copy was rolled back: %v", err)
	}
	if conflictCopy.PlainText != "Конфликт" {
		t.Fatalf("conflict copy lost local text: %#v", conflictCopy)
	}

	replacePreview, err := dataStore.PreviewProjectReplace(
		ctx,
		user.ID,
		workspace.Project.ID,
		"Первая",
		"Вторая",
		"all",
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacePreview.TotalCount != 1 || len(replacePreview.Documents) != 1 {
		t.Fatalf("unexpected replace preview: %#v", replacePreview)
	}
	replaceResult, err := dataStore.ApplyProjectReplace(
		ctx,
		user.ID,
		workspace.Project.ID,
		replacePreview.Search,
		replacePreview.Replace,
		replacePreview.Scope,
		replacePreview.Token,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replaceResult.Documents != 1 || replaceResult.Replacements != 1 ||
		replaceResult.OperationID == "" {
		t.Fatalf("unexpected replace result: %#v", replaceResult)
	}
	replacedNode, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacedNode.PlainText != "Вторая правка" || replacedNode.CurrentRevision != 3 {
		t.Fatalf("replace was not saved as a new revision: %#v", replacedNode)
	}
	storedExport, err := dataStore.GetExport(ctx, user.ID, exportJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var storedSnapshot exportformat.Snapshot
	if err := json.Unmarshal(storedExport.Snapshot, &storedSnapshot); err != nil {
		t.Fatal(err)
	}
	if storedSnapshot.Documents[0].Revision != 2 ||
		storedSnapshot.Documents[0].PlainText != "Первая правка" {
		t.Fatalf("later edits changed an existing export snapshot: %#v", storedSnapshot.Documents[0])
	}
	replaceRevisions, err := dataStore.ListRevisions(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replaceRevisions[0].Source != "project_replace" ||
		replaceRevisions[0].OperationID == nil ||
		*replaceRevisions[0].OperationID != replaceResult.OperationID {
		t.Fatalf("replace operation is not traceable: %#v", replaceRevisions[0])
	}
	undoResult, err := dataStore.UndoProjectReplace(
		ctx,
		user.ID,
		workspace.Project.ID,
		replaceResult.OperationID,
	)
	if err != nil || undoResult.Documents != 1 {
		t.Fatalf("undo project replace: %#v %v", undoResult, err)
	}
	restoredNode, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredNode.PlainText != "Первая правка" || restoredNode.CurrentRevision != 4 {
		t.Fatalf("undo did not preserve history: %#v", restoredNode)
	}
	if _, err := dataStore.ApplyProjectReplace(
		ctx,
		user.ID,
		workspace.Project.ID,
		replacePreview.Search,
		replacePreview.Replace,
		replacePreview.Scope,
		replacePreview.Token,
	); !errors.Is(err, ErrReplacePreviewStale) {
		t.Fatalf("stale preview must be rejected, got %v", err)
	}

	clientID := "integration-client-" + time.Now().Format("150405.000000000")
	session, err := dataStore.CreateDictationSession(
		ctx,
		user.ID,
		workspace.Project.ID,
		&node.ID,
		clientID,
		"ru",
	)
	if err != nil {
		t.Fatal(err)
	}
	duplicateSession, err := dataStore.CreateDictationSession(
		ctx,
		user.ID,
		workspace.Project.ID,
		&node.ID,
		clientID,
		"ru",
	)
	if err != nil || duplicateSession.ID != session.ID {
		t.Fatalf("client session creation is not idempotent: %#v %v", duplicateSession, err)
	}

	chunk := AudioChunk{
		SessionID:  session.ID,
		Sequence:   0,
		SHA256:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DurationMS: 1000,
		StorageKey: "integration/chunk",
		ByteSize:   5,
	}
	duplicate, err := dataStore.PutAudioChunk(ctx, user.ID, chunk)
	if err != nil || duplicate {
		t.Fatalf("first chunk insert failed: duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = dataStore.PutAudioChunk(ctx, user.ID, chunk)
	if err != nil || !duplicate {
		t.Fatalf("same checksum must be idempotent: duplicate=%v err=%v", duplicate, err)
	}
	conflictingChunk := chunk
	conflictingChunk.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := dataStore.PutAudioChunk(ctx, user.ID, conflictingChunk); err == nil {
		t.Fatal("different checksum for the same sequence must fail")
	}

	manifest := json.RawMessage(
		`{"format":"webm","chunks":[{"sequence":0,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
	)
	if _, err := dataStore.StopDictation(ctx, user.ID, session.ID, 1000, 1000, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.StopDictation(ctx, user.ID, session.ID, 1000, 1000, manifest); err != nil {
		t.Fatalf("double finish must be idempotent: %v", err)
	}
	if err := dataStore.MarkLiveTranscriptionStarted(ctx, user.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.ConfigureLiveCommands(ctx, user.ID, session.ID, true, "Редактор"); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.MarkLiveTranscriptionFinalized(ctx, user.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	workerMeta, err := dataStore.GetWorkerSessionMeta(ctx, session.ID)
	if err != nil || workerMeta.LiveFinalizedAt == nil {
		t.Fatalf("live finalization marker was not persisted: %#v %v", workerMeta, err)
	}
	if !workerMeta.CommandsEnabled || workerMeta.CommandPrefix != "Редактор" {
		t.Fatalf("live command configuration was not persisted: %#v", workerMeta)
	}
	if err := dataStore.AppendLiveSegment(
		ctx,
		user.ID,
		session.ID,
		"editable-one",
		"turn-one",
		"Дракон не чистил зубы.",
		0,
		2_000,
		nil,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.AppendLiveSegment(
		ctx,
		user.ID,
		session.ID,
		"editable-two",
		"turn-two",
		"Поэтому они заболели.",
		2_000,
		4_000,
		nil,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.ReplaceLiveSegments(
		ctx,
		user.ID,
		session.ID,
		[]string{"editable-one", "missing"},
		"Не должно сохраниться",
	); err == nil {
		t.Fatal("replacement with a missing segment must fail atomically")
	}
	liveSegments, err := dataStore.ListLiveSegments(ctx, session.ID)
	if err != nil ||
		len(liveSegments) != 2 ||
		liveSegments[0].Text != "Дракон не чистил зубы." {
		t.Fatalf("failed replacement changed live text: %#v %v", liveSegments, err)
	}
	if err := dataStore.ReplaceLiveSegments(
		ctx,
		user.ID,
		session.ID,
		[]string{"editable-one", "editable-two"},
		"Дракон ел много сладкого, поэтому зубы заболели.",
	); err != nil {
		t.Fatal(err)
	}
	liveSegments, err = dataStore.ListLiveSegments(ctx, session.ID)
	if err != nil ||
		len(liveSegments) != 1 ||
		liveSegments[0].ID != "editable-one" ||
		liveSegments[0].Text != "Дракон ел много сладкого, поэтому зубы заболели." ||
		liveSegments[0].StartMS != 0 ||
		liveSegments[0].EndMS != 4_000 {
		t.Fatalf("live paragraph replacement was not atomic: %#v %v", liveSegments, err)
	}
	var jobs int
	if err := dataStore.Pool.QueryRow(
		ctx,
		"SELECT count(*) FROM background_jobs WHERE idempotency_key=$1",
		"finalize:"+session.ID,
	).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("expected exactly one finalization job, count=%d err=%v", jobs, err)
	}

	liveArtifact, err := dataStore.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       session.ID,
		Kind:            "live_final",
		PipelineVersion: "live-v1",
		Provider:        "fixture",
		Model:           "live",
		Language:        "ru",
		Text:            "Живой текст",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondArtifact, err := dataStore.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       session.ID,
		Kind:            "live_final",
		PipelineVersion: "live-v2",
		Provider:        "fixture",
		Model:           "live",
		Language:        "ru",
		Text:            "Попытка перезаписи",
	})
	if err != nil || secondArtifact.ID != liveArtifact.ID {
		t.Fatalf("source artifact must be immutable: %#v %v", secondArtifact, err)
	}
	firstSuggestion, err := dataStore.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       session.ID,
		Kind:            "cleaned_suggestion",
		PipelineVersion: "cleaned-v1",
		Provider:        "fixture",
		Model:           "batch",
		Language:        "ru",
		Text:            "Предложение со словарём",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSuggestion, err := dataStore.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       session.ID,
		Kind:            "cleaned_suggestion",
		PipelineVersion: "cleaned-v2",
		Provider:        "fixture",
		Model:           "batch",
		Language:        "ru",
		Text:            "Новое предложение со словарём",
	})
	if err != nil || secondSuggestion.ID == firstSuggestion.ID {
		t.Fatalf("retranscription must create a new artifact: %#v %v", secondSuggestion, err)
	}
	if err := dataStore.SaveAudioAsset(ctx, AudioAsset{
		ID:         uuid.NewString(),
		SessionID:  session.ID,
		SHA256:     chunk.SHA256,
		StorageKey: "audio/" + session.ID + "/complete.webm",
		ByteSize:   chunk.ByteSize,
		DurationMS: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.FinalizeDictationState(ctx, session.ID, "review_ready"); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	if err := dataStore.QueueRetranscription(ctx, user.ID, session.ID, requestID); err != nil {
		t.Fatalf("queue retranscription: %v", err)
	}
	loadedSession, err := dataStore.GetDictationSession(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedSession.SuggestionArtifact == nil ||
		loadedSession.SuggestionArtifact.ID != secondSuggestion.ID {
		t.Fatalf("latest suggestion was not returned: %#v", loadedSession.SuggestionArtifact)
	}
	if loadedSession.RetranscriptionStatus != "queued" {
		t.Fatalf("retranscription status = %q, want queued", loadedSession.RetranscriptionStatus)
	}

	liveContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"live","sourceRefs":[]},"content":[{"type":"text","text":"Живой текст"}]}]}`,
	)
	if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:        session.ID,
		SourceArtifactID: &liveArtifact.ID,
		BaseSourceText:   "Живой текст",
		Content:          liveContent,
		PlainText:        "Живой текст",
		Source:           "live_source",
	}); err != nil {
		t.Fatal(err)
	}
	manualContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"manual","sourceRefs":[]},"content":[{"type":"text","text":"Авторский текст"}]}]}`,
	)
	if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:      session.ID,
		BaseSourceText: "Живой текст",
		Content:        manualContent,
		PlainText:      "Авторский текст",
		ManuallyEdited: true,
	}); err != nil {
		t.Fatal(err)
	}
	secondManualContent := json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"manual","sourceRefs":[]},"content":[{"type":"text","text":"Авторский текст после второй правки"}]}]}`,
	)
	if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:      session.ID,
		BaseSourceText: "Живой текст",
		Content:        secondManualContent,
		PlainText:      "Авторский текст после второй правки",
		ManuallyEdited: true,
		Source:         "Применена финальная расшифровка",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:      session.ID,
		BaseSourceText: "Поздний финальный текст",
		Content:        json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`),
		PlainText:      "Поздний финальный текст",
		Source:         "batch_source",
	}); err != nil {
		t.Fatal(err)
	}
	protected, err := dataStore.GetTranscriptDraft(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if protected.PlainText != "Авторский текст после второй правки" ||
		protected.BaseSourceText != "Живой текст" ||
		!protected.ManuallyEdited {
		t.Fatalf("late batch overwrote the author draft: %#v", protected)
	}
	draftRevisions, err := dataStore.ListTranscriptDraftRevisions(ctx, user.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(draftRevisions) != 3 ||
		draftRevisions[0].Source != "Применена финальная расшифровка" {
		t.Fatalf("draft revision history is incomplete: %#v", draftRevisions)
	}

	preferences, err := dataStore.GetUserPreferences(ctx, user.ID)
	if err != nil || preferences.AudioRetentionDays != 365 {
		t.Fatalf("default preferences: %#v %v", preferences, err)
	}
	preferences, err = dataStore.UpdateUserPreferences(ctx, user.ID, 90)
	if err != nil || preferences.AudioRetentionDays != 90 {
		t.Fatalf("update preferences: %#v %v", preferences, err)
	}
	backup, backupAudio, backupMedia, err := dataStore.BuildAccountBackup(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(backup, []byte("Integration manuscript")) || len(backupAudio) != 1 ||
		len(backupMedia) != 0 {
		t.Fatalf("account backup is incomplete: audio=%d data=%s", len(backupAudio), backup)
	}

	purgeWorkspace, err := dataStore.CreateProject(ctx, user.ID, CreateProjectInput{
		Title:    "Project to purge",
		Kind:     "story",
		Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.UpdateProjectStatus(
		ctx,
		user.ID,
		purgeWorkspace.Project.ID,
		"trash",
	); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.PurgeTrashedProject(
		ctx,
		user.ID,
		purgeWorkspace.Project.ID,
		0,
	); err != nil {
		t.Fatal(err)
	}
	var auditEvents int
	if err := dataStore.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM deletion_audit_events
		WHERE owner_user_id=$1 AND subject_id=$2 AND event_type='project_purged'
	`, user.ID, purgeWorkspace.Project.ID).Scan(&auditEvents); err != nil || auditEvents != 1 {
		t.Fatalf("purge audit event count=%d err=%v", auditEvents, err)
	}

	accountPlan, err := dataStore.GetAccountPurgePlan(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accountPlan.Email != email || len(accountPlan.ObjectKeys) == 0 {
		t.Fatalf("account purge plan is incomplete: %#v", accountPlan)
	}
	if err := dataStore.PurgeAccount(ctx, user.ID, len(accountPlan.ObjectKeys)); err != nil {
		t.Fatal(err)
	}
	var remainingUsers int
	if err := dataStore.Pool.QueryRow(
		ctx,
		"SELECT count(*) FROM users WHERE id=$1",
		user.ID,
	).Scan(&remainingUsers); err != nil || remainingUsers != 0 {
		t.Fatalf("account was not purged: count=%d err=%v", remainingUsers, err)
	}
	if err := dataStore.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM deletion_audit_events
		WHERE owner_user_id=$1 AND subject_id=$1 AND event_type='account_purged'
	`, user.ID).Scan(&auditEvents); err != nil || auditEvents != 1 {
		t.Fatalf("account purge audit event count=%d err=%v", auditEvents, err)
	}
	var orphanJobs int
	if err := dataStore.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM background_jobs
		WHERE payload->>'sessionId'=$1
	`, session.ID).Scan(&orphanJobs); err != nil || orphanJobs != 0 {
		t.Fatalf("account purge left background jobs: count=%d err=%v", orphanJobs, err)
	}
}

func TestDocumentMoveValidation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dataStore, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	email := "move-validation-" + time.Now().Format("20060102150405.000000000") + "@example.test"
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
		Title:    "Move validation",
		Kind:     "novel",
		Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	part, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		nil,
		"part",
		"Part",
	)
	if err != nil {
		t.Fatal(err)
	}
	chapter, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		&part.ID,
		"chapter",
		"Nested chapter",
	)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := dataStore.CreateDocumentNode(
		ctx,
		user.ID,
		workspace.Project.ID,
		nil,
		"scene",
		"Scene",
	)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name     string
		nodeID   string
		parentID string
	}{
		{name: "self parent", nodeID: chapter.ID, parentID: chapter.ID},
		{name: "cycle", nodeID: part.ID, parentID: chapter.ID},
		{name: "content node as parent", nodeID: part.ID, parentID: scene.ID},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := dataStore.MoveDocumentNode(
				ctx,
				user.ID,
				testCase.nodeID,
				&testCase.parentID,
				10_000,
			)
			if !errors.Is(err, ErrInvalidDocumentMove) {
				t.Fatalf("expected ErrInvalidDocumentMove, got %v", err)
			}
		})
	}

	if err := dataStore.MoveDocumentNode(ctx, user.ID, scene.ID, &chapter.ID, 10_000); err != nil {
		t.Fatalf("valid move failed: %v", err)
	}
	movedScene, err := dataStore.GetDocumentNode(ctx, user.ID, scene.ID)
	if err != nil {
		t.Fatal(err)
	}
	if movedScene.ParentID == nil || *movedScene.ParentID != chapter.ID {
		t.Fatalf("move was not persisted: %#v", movedScene.ParentID)
	}
}
