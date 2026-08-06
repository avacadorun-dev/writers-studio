//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
)

func TestCanvasPersistenceOwnershipConflictsAndResources(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dataStore, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().Format("20060102150405.000000000")
	owner, _, err := dataStore.Authenticate(
		ctx, "canvas-owner-"+suffix+"@example.test", "writers-studio", time.Hour, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := dataStore.Authenticate(
		ctx, "canvas-other-"+suffix+"@example.test", "writers-studio", time.Hour, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=ANY($1::uuid[])", []string{owner.ID, other.ID})
	})

	workspace, err := dataStore.CreateProject(ctx, owner.ID, CreateProjectInput{
		Title: "Canvas project", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignWorkspace, err := dataStore.CreateProject(ctx, owner.ID, CreateProjectInput{
		Title: "Foreign resources", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := dataStore.CreateProject(ctx, other.ID, CreateProjectInput{
		Title: "Other owner", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	media, err := dataStore.CreateProjectMedia(ctx, owner.ID, CreateProjectMediaInput{
		ProjectID: workspace.Project.ID, MediaType: "image/png", OriginalName: "board.png",
		SHA256: "canvas-local", ByteSize: 128, Width: 640, Height: 480, StorageKey: "canvas/local.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignMedia, err := dataStore.CreateProjectMedia(ctx, owner.ID, CreateProjectMediaInput{
		ProjectID: foreignWorkspace.Project.ID, MediaType: "image/png", OriginalName: "foreign.png",
		SHA256: "canvas-foreign", ByteSize: 128, Width: 640, Height: 480, StorageKey: "canvas/foreign.png",
	})
	if err != nil {
		t.Fatal(err)
	}

	board, err := dataStore.CreateCanvas(ctx, owner.ID, workspace.Project.ID, "Сюжет")
	if err != nil || board.CurrentRevision != 1 {
		t.Fatalf("create canvas = %#v, err = %v", board, err)
	}
	content := json.RawMessage(fmt.Sprintf(`{
		"nodes":[
			{"id":"note","type":"text","x":0,"y":0,"width":260,"height":180,"text":"Завязка"},
			{"id":"doc","type":"file","x":320,"y":0,"width":280,"height":180,"file":"writers-studio://documents/%[1]s","x-writers-studio":{"resource":{"kind":"document","id":"%[1]s"}}},
			{"id":"image","type":"file","x":640,"y":0,"width":300,"height":220,"file":"writers-studio://media/%[2]s","x-writers-studio":{"resource":{"kind":"media","id":"%[2]s"}}}
		],
		"edges":[{"id":"link","fromNode":"note","toNode":"doc","toEnd":"arrow"}]
	}`, workspace.Nodes[0].ID, media.ID))
	references, err := canvasdocument.Validate(content)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := dataStore.SaveCanvasRevision(ctx, owner.ID, board.ID, SaveCanvasRevisionInput{
		BaseRevision: 1, SchemaVersion: canvasdocument.SchemaVersion,
		Content: content, Source: "integration", References: references,
	})
	if err != nil || revision.Revision != 2 || revision.BaseRevision != 1 {
		t.Fatalf("save canvas revision = %#v, err = %v", revision, err)
	}
	loaded, err := dataStore.GetCanvasWorkspace(ctx, owner.ID, board.ID)
	if err != nil || len(loaded.Documents) != 1 || len(loaded.Media) != 1 ||
		loaded.Documents[0].ID != workspace.Nodes[0].ID || loaded.Media[0].ID != media.ID {
		t.Fatalf("resolved canvas resources = %#v, err = %v", loaded, err)
	}

	title := "Новая схема"
	position := 42.5
	updated, err := dataStore.UpdateCanvas(ctx, owner.ID, board.ID, UpdateCanvasInput{
		Title: &title, Position: &position,
	})
	if err != nil || updated.Title != title || updated.Position != position {
		t.Fatalf("update canvas = %#v, err = %v", updated, err)
	}
	duplicate, err := dataStore.DuplicateCanvas(ctx, owner.ID, board.ID)
	if err != nil || duplicate.ID == board.ID || duplicate.CurrentRevision != 1 {
		t.Fatalf("duplicate canvas = %#v, err = %v", duplicate, err)
	}
	duplicateWorkspace, err := dataStore.GetCanvasWorkspace(ctx, owner.ID, duplicate.ID)
	if err != nil || !canvasJSONEqual(duplicateWorkspace.Revision.Content, content) {
		t.Fatalf("duplicate content = %s, err = %v", duplicateWorkspace.Revision.Content, err)
	}

	conflictContent := json.RawMessage(`{"nodes":[{"id":"local","type":"text","x":0,"y":0,"width":260,"height":180,"text":"Локальная версия"}],"edges":[]}`)
	conflictReferences, err := canvasdocument.Validate(conflictContent)
	if err != nil {
		t.Fatal(err)
	}
	_, conflictErr := dataStore.SaveCanvasRevision(ctx, owner.ID, board.ID, SaveCanvasRevisionInput{
		BaseRevision: 1, SchemaVersion: canvasdocument.SchemaVersion,
		Content: conflictContent, Source: "offline", References: conflictReferences,
	})
	conflict, ok := AsCanvasConflict(conflictErr)
	if !ok || conflict.CurrentRevision != 2 || conflict.ConflictCanvasID == "" {
		t.Fatalf("expected durable canvas conflict, got %#v, err = %v", conflict, conflictErr)
	}
	conflictWorkspace, err := dataStore.GetCanvasWorkspace(ctx, owner.ID, conflict.ConflictCanvasID)
	if err != nil || !canvasJSONEqual(conflictWorkspace.Revision.Content, conflictContent) {
		t.Fatalf("conflict copy content = %s, err = %v", conflictWorkspace.Revision.Content, err)
	}

	foreignContent := json.RawMessage(fmt.Sprintf(`{
		"nodes":[
			{"id":"foreign-doc","type":"file","x":0,"y":0,"width":280,"height":180,"file":"writers-studio://documents/%[1]s","x-writers-studio":{"resource":{"kind":"document","id":"%[1]s"}}},
			{"id":"foreign-image","type":"file","x":320,"y":0,"width":300,"height":220,"file":"writers-studio://media/%[2]s","x-writers-studio":{"resource":{"kind":"media","id":"%[2]s"}}}
		],"edges":[]
	}`, foreignWorkspace.Nodes[0].ID, foreignMedia.ID))
	foreignReferences, err := canvasdocument.Validate(foreignContent)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dataStore.SaveCanvasRevision(ctx, owner.ID, board.ID, SaveCanvasRevisionInput{
		BaseRevision: 2, SchemaVersion: canvasdocument.SchemaVersion,
		Content: foreignContent, Source: "integration", References: foreignReferences,
	})
	if !errors.Is(err, canvasdocument.ErrInvalidDocument) {
		t.Fatalf("foreign project resource must be rejected, got %v", err)
	}
	loaded, err = dataStore.GetCanvasWorkspace(ctx, owner.ID, board.ID)
	if err != nil || loaded.Canvas.CurrentRevision != 2 {
		t.Fatalf("rejected revision changed the canvas: %#v, err = %v", loaded.Canvas, err)
	}
	var revisionCount int
	if err := dataStore.Pool.QueryRow(
		ctx, "SELECT COUNT(*) FROM canvas_revisions WHERE canvas_id=$1", board.ID,
	).Scan(&revisionCount); err != nil || revisionCount != 2 {
		t.Fatalf("canvas revision count = %d, err = %v", revisionCount, err)
	}

	trashed, err := dataStore.TrashCanvas(ctx, owner.ID, board.ID)
	if err != nil || trashed.TrashedAt == nil {
		t.Fatalf("trash canvas = %#v, err = %v", trashed, err)
	}
	active, err := dataStore.ListCanvases(ctx, owner.ID, workspace.Project.ID, false)
	if err != nil || containsCanvas(active, board.ID) {
		t.Fatalf("trashed canvas leaked into active list: %#v, err = %v", active, err)
	}
	all, err := dataStore.ListCanvases(ctx, owner.ID, workspace.Project.ID, true)
	if err != nil || !containsCanvas(all, board.ID) {
		t.Fatalf("trashed canvas missing from full list: %#v, err = %v", all, err)
	}
	restored, err := dataStore.RestoreCanvas(ctx, owner.ID, board.ID)
	if err != nil || restored.TrashedAt != nil {
		t.Fatalf("restore canvas = %#v, err = %v", restored, err)
	}

	if _, err := dataStore.GetCanvasWorkspace(ctx, other.ID, board.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other owner read canvas, err = %v", err)
	}
	if listed, err := dataStore.ListCanvases(ctx, other.ID, workspace.Project.ID, true); err != nil || len(listed) != 0 {
		t.Fatalf("other owner listed canvases: %#v, err = %v", listed, err)
	}
	if _, err := dataStore.CreateCanvas(ctx, owner.ID, otherWorkspace.Project.ID, "Чужая"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other owner's project accepted a canvas, err = %v", err)
	}
}

func containsCanvas(items []domain.CanvasSummary, canvasID string) bool {
	for _, item := range items {
		if item.ID == canvasID {
			return true
		}
	}
	return false
}

func canvasJSONEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
