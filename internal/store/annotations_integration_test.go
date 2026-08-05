//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
)

func TestDocumentAnnotationsLifecycleAndRevisionSafety(t *testing.T) {
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
		ctx, "annotations-"+time.Now().Format("20060102150405.000000000")+"@example.test",
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
		Title: "Annotations", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	node := workspace.Nodes[0]
	revision2 := annotationDocument("Лиса 🦊 вошла в лес.")
	if _, err := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: 1, Content: revision2, PlainText: "Лиса 🦊 вошла в лес.", Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	term, err := dataStore.CreateDictionaryTerm(
		ctx, user.ID, workspace.Project.ID, "Тёмный лес", "place", "ru", []string{"лес"},
	)
	if err != nil {
		t.Fatal(err)
	}
	note, err := dataStore.CreateDocumentAnnotation(ctx, user.ID, node.ID, CreateAnnotationInput{
		Kind: "note", BaseRevision: 2, Note: "Уточнить движение",
		Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 7, End: 12, ExactText: "вошла",
			Prefix: "Лиса 🦊 ", Suffix: " в лес.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	link, err := dataStore.CreateDocumentAnnotation(ctx, user.ID, node.ID, CreateAnnotationInput{
		Kind: "dictionary_link", DictionaryTermID: &term.ID, BaseRevision: 2,
		Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 15, End: 18, ExactText: "лес",
			Prefix: "Лиса 🦊 вошла в ", Suffix: ".",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if link.DictionaryTerm == nil || link.DictionaryTerm.Canonical != "Тёмный лес" {
		t.Fatalf("dictionary snapshot/join missing: %#v", link)
	}

	items, err := dataStore.ListProjectAnnotations(ctx, user.ID, workspace.Project.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("project annotations = %#v, err=%v", items, err)
	}

	revision3 := annotationDocument("Вдруг Лиса 🦊 вошла в лес.")
	updates3 := []AnnotationAnchorUpdate{
		{ID: note.ID, Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 13, End: 18, ExactText: "вошла",
			Prefix: "Вдруг Лиса 🦊 ", Suffix: " в лес.",
		}},
		{ID: link.ID, Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 21, End: 24, ExactText: "лес",
			Prefix: "Вдруг Лиса 🦊 вошла в ", Suffix: ".",
		}},
	}
	if _, err := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: 2, Content: revision3, PlainText: "Вдруг Лиса 🦊 вошла в лес.",
		Source: "manual", AnnotationAnchors: updates3,
	}); err != nil {
		t.Fatal(err)
	}
	items, err = dataStore.ListDocumentAnnotations(ctx, user.ID, node.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("mapped annotations = %#v, err=%v", items, err)
	}
	for _, item := range items {
		if item.Status != "open" || item.AnchorStatus != "attached" || item.BaseRevision != 3 {
			t.Fatalf("mapped annotation not current: %#v", item)
		}
	}

	revision4 := annotationDocument("Вдруг Лиса 🦊 вошла в лес. Тихо.")
	if _, err := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: 3, Content: revision4, PlainText: "Вдруг Лиса 🦊 вошла в лес. Тихо.",
		Source: "legacy_client",
	}); err != nil {
		t.Fatal(err)
	}
	items, err = dataStore.ListDocumentAnnotations(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Status != "open" || item.AnchorStatus != "detached" || item.BaseRevision != 3 {
			t.Fatalf("legacy save must preserve and flag annotations: %#v", item)
		}
	}

	reopened, err := dataStore.UpdateDocumentAnnotation(ctx, user.ID, note.ID, UpdateAnnotationInput{
		Anchor: &updates3[0].Anchor,
	})
	if err != nil || reopened.Status != "open" || reopened.AnchorStatus != "attached" || reopened.BaseRevision != 4 {
		t.Fatalf("reattached note = %#v, err=%v", reopened, err)
	}
	changed := "Сократить и уточнить движение"
	resolved := "resolved"
	updated, err := dataStore.UpdateDocumentAnnotation(ctx, user.ID, note.ID, UpdateAnnotationInput{
		Note: &changed, Status: &resolved,
	})
	if err != nil || updated.Note != changed || updated.Status != "resolved" || updated.ResolvedAt == nil {
		t.Fatalf("updated note = %#v, err=%v", updated, err)
	}
	open := "open"
	updated, err = dataStore.UpdateDocumentAnnotation(ctx, user.ID, note.ID, UpdateAnnotationInput{Status: &open})
	if err != nil || updated.Status != "open" || updated.ResolvedAt != nil {
		t.Fatalf("reopened note = %#v, err=%v", updated, err)
	}

	if err := dataStore.DeleteDictionaryTerm(ctx, user.ID, term.ID); err != nil {
		t.Fatal(err)
	}
	items, err = dataStore.ListDocumentAnnotations(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	deletedTermLink := annotationByID(t, items, link.ID)
	if deletedTermLink.DictionaryTermID != nil || deletedTermLink.DictionaryTerm == nil ||
		deletedTermLink.DictionaryTerm.Canonical != "Тёмный лес" ||
		deletedTermLink.Status != "needs_attention" {
		t.Fatalf("deleted term reference lost its snapshot: %#v", deletedTermLink)
	}
	if _, err := dataStore.UpdateDocumentAnnotation(
		ctx, user.ID, link.ID, UpdateAnnotationInput{Status: &open},
	); !errors.Is(err, ErrInvalidAnnotation) {
		t.Fatalf("deleted dictionary target reopened: %v", err)
	}

	duplicate, err := dataStore.DuplicateDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	duplicatedAnnotations, err := dataStore.ListDocumentAnnotations(ctx, user.ID, duplicate.ID)
	if err != nil || len(duplicatedAnnotations) != 2 {
		t.Fatalf("duplicated annotations = %#v, err=%v", duplicatedAnnotations, err)
	}
	duplicatedNote := annotationByKind(t, duplicatedAnnotations, "note")
	if duplicatedNote.Status != "open" || duplicatedNote.BaseRevision != 1 {
		t.Fatalf("duplicate changed valid note state: %#v", duplicatedNote)
	}

	conflictContent := annotationDocument("Конфликт: Вдруг Лиса 🦊 вошла в лес. Тихо.")
	conflictUpdates := []AnnotationAnchorUpdate{
		{ID: note.ID, Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 23, End: 28, ExactText: "вошла",
		}},
		{ID: link.ID, Anchor: domain.AnnotationAnchor{
			BlockID: "block-1", Start: 31, End: 34, ExactText: "лес",
		}},
	}
	_, conflictErr := dataStore.SaveRevision(ctx, user.ID, node.ID, SaveRevisionInput{
		BaseRevision: 3, Content: conflictContent,
		PlainText: "Конфликт: Вдруг Лиса 🦊 вошла в лес. Тихо.", Source: "manual",
		AnnotationAnchors: conflictUpdates,
	})
	conflict, ok := AsConflict(conflictErr)
	if !ok || conflict.ConflictNodeID == "" {
		t.Fatalf("conflict result = %#v, %v", conflict, conflictErr)
	}
	conflictAnnotations, err := dataStore.ListDocumentAnnotations(ctx, user.ID, conflict.ConflictNodeID)
	if err != nil || len(conflictAnnotations) != 2 {
		t.Fatalf("conflict annotations = %#v, err=%v", conflictAnnotations, err)
	}
	if item := annotationByKind(t, conflictAnnotations, "note"); item.Status != "open" || item.BaseRevision != 1 {
		t.Fatalf("mapped conflict note = %#v", item)
	}

	if err := dataStore.DeleteDocumentAnnotation(ctx, user.ID, note.ID); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.DeleteDocumentAnnotation(ctx, user.ID, note.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second delete error = %v", err)
	}
}

func TestDocumentAnnotationValidationAndOwnership(t *testing.T) {
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
	stamp := time.Now().Format("20060102150405.000000000")
	owner, _, err := dataStore.Authenticate(ctx, "annotation-owner-"+stamp+"@example.test", "writers-studio", time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := dataStore.Authenticate(ctx, "annotation-other-"+stamp+"@example.test", "writers-studio", time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=ANY($1::uuid[])", []string{owner.ID, other.ID})
	})
	workspace, err := dataStore.CreateProject(ctx, owner.ID, CreateProjectInput{Title: "Private", Kind: "novel", Language: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	node := workspace.Nodes[0]
	content := annotationDocument("Секрет")
	if _, err := dataStore.SaveRevision(ctx, owner.ID, node.ID, SaveRevisionInput{
		BaseRevision: 1, Content: content, PlainText: "Секрет", Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	valid := CreateAnnotationInput{
		Kind: "note", Note: "Проверить", BaseRevision: 2,
		Anchor: domain.AnnotationAnchor{BlockID: "block-1", Start: 0, End: 6, ExactText: "Секрет"},
	}
	if _, err := dataStore.CreateDocumentAnnotation(ctx, other.ID, node.ID, valid); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-owner create error = %v", err)
	}
	invalidCases := []CreateAnnotationInput{
		{Kind: "unknown", Note: "x", BaseRevision: 2, Anchor: valid.Anchor},
		{Kind: "note", Note: "", BaseRevision: 2, Anchor: valid.Anchor},
		{Kind: "note", Note: "x", BaseRevision: 2, Anchor: domain.AnnotationAnchor{BlockID: "block-1", Start: 0, End: 6, ExactText: "Другое"}},
	}
	for _, input := range invalidCases {
		if _, err := dataStore.CreateDocumentAnnotation(ctx, owner.ID, node.ID, input); !errors.Is(err, ErrInvalidAnnotation) {
			t.Fatalf("invalid annotation error = %v for %#v", err, input)
		}
	}
	valid.BaseRevision = 1
	if _, err := dataStore.CreateDocumentAnnotation(ctx, owner.ID, node.ID, valid); err == nil {
		t.Fatal("stale annotation base revision accepted")
	}
}

func annotationDocument(text string) json.RawMessage {
	encoded, _ := json.Marshal(map[string]any{
		"type": "doc",
		"content": []any{map[string]any{
			"type": "paragraph", "attrs": map[string]any{"blockId": "block-1", "sourceRefs": []string{}},
			"content": []any{map[string]any{"type": "text", "text": text}},
		}},
	})
	return encoded
}

func annotationByID(t *testing.T, items []domain.DocumentAnnotation, id string) domain.DocumentAnnotation {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("annotation %s not found in %#v", id, items)
	return domain.DocumentAnnotation{}
}

func annotationByKind(t *testing.T, items []domain.DocumentAnnotation, kind string) domain.DocumentAnnotation {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind {
			return item
		}
	}
	t.Fatalf("annotation kind %s not found in %#v", kind, items)
	return domain.DocumentAnnotation{}
}
