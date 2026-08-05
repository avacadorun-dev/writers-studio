package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
)

func TestValidateProjectTransferAcceptsPortableProject(t *testing.T) {
	transfer := validProjectTransferFixture()
	if err := ValidateProjectTransfer(transfer); err != nil {
		t.Fatalf("ValidateProjectTransfer() error = %v", err)
	}
}

func TestValidateProjectTransferRejectsInvalidAssistantStepLimit(t *testing.T) {
	transfer := validProjectTransferFixture()
	invalidLimit := 0
	transfer.Project.AssistantStepLimit = &invalidLimit
	if err := ValidateProjectTransfer(transfer); !errors.Is(err, ErrInvalidProjectTransfer) {
		t.Fatalf("ValidateProjectTransfer() error = %v, want ErrInvalidProjectTransfer", err)
	}
}

func TestValidateProjectTransferAcceptsLegacyV1WithoutAnnotations(t *testing.T) {
	transfer := validProjectTransferFixture()
	transfer.FormatVersion = 1
	transfer.DocumentAnnotations = nil
	if err := ValidateProjectTransfer(transfer); err != nil {
		t.Fatalf("legacy v1 transfer rejected: %v", err)
	}
}

func TestValidateProjectTransferV2Annotations(t *testing.T) {
	transfer := validProjectTransferFixture()
	transfer.DocumentRevisions[0].Content = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"block-1"},"content":[{"type":"text","text":"Лес"}]}]}`)
	transfer.DocumentRevisions[0].PlainText = "Лес"
	termID := "term-1"
	transfer.DictionaryTerms = []ProjectTransferDictionaryTerm{{
		ID: termID, Canonical: "Лес", Kind: "place", Language: "ru", Active: true,
		Revision: 1, Source: "manual", CreatedAt: transfer.ExportedAt, UpdatedAt: transfer.ExportedAt,
	}}
	transfer.DocumentAnnotations = []ProjectTransferDocumentAnnotation{
		{
			ID: "note-1", NodeID: "node-1", Kind: "note", Status: "open",
			Anchor: domain.AnnotationAnchor{BlockID: "block-1", Start: 0, End: 1, ExactText: "Л"},
			Note:   "Проверить", TargetSnapshot: json.RawMessage(`{}`), BaseRevision: 1,
			CreatedAt: transfer.ExportedAt, UpdatedAt: transfer.ExportedAt,
		},
		{
			ID: "link-1", NodeID: "node-1", Kind: "dictionary_link",
			DictionaryTermID: &termID, Status: "resolved",
			Anchor:         domain.AnnotationAnchor{BlockID: "block-1", Start: 0, End: 1, ExactText: "Л"},
			TargetSnapshot: json.RawMessage(`{"canonical":"Лес"}`), BaseRevision: 1,
			CreatedAt: transfer.ExportedAt, UpdatedAt: transfer.ExportedAt,
		},
	}
	if err := ValidateProjectTransfer(transfer); err != nil {
		t.Fatalf("valid v2 annotations rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ProjectTransfer)
	}{
		{"annotations in v1", func(value *ProjectTransfer) { value.FormatVersion = 1 }},
		{"missing node", func(value *ProjectTransfer) { value.DocumentAnnotations[0].NodeID = "missing" }},
		{"missing term", func(value *ProjectTransfer) {
			missing := "missing"
			value.DocumentAnnotations[1].DictionaryTermID = &missing
		}},
		{"empty note", func(value *ProjectTransfer) { value.DocumentAnnotations[0].Note = "" }},
		{"bad range", func(value *ProjectTransfer) { value.DocumentAnnotations[0].Anchor.End = 0 }},
		{"future revision", func(value *ProjectTransfer) { value.DocumentAnnotations[0].BaseRevision = 2 }},
		{"bad anchor status", func(value *ProjectTransfer) { value.DocumentAnnotations[0].AnchorStatus = "guessed" }},
		{"bad annotation revision", func(value *ProjectTransfer) { value.DocumentAnnotations[0].Revision = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := transfer
			candidate.DictionaryTerms = append([]ProjectTransferDictionaryTerm(nil), transfer.DictionaryTerms...)
			candidate.DocumentAnnotations = append([]ProjectTransferDocumentAnnotation(nil), transfer.DocumentAnnotations...)
			test.mutate(&candidate)
			if err := ValidateProjectTransfer(candidate); !errors.Is(err, ErrInvalidProjectTransfer) {
				t.Fatalf("ValidateProjectTransfer() error = %v", err)
			}
		})
	}
}

func TestValidateProjectTransferRejectsFutureVersionAndBrokenReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProjectTransfer)
	}{
		{
			name: "future version",
			mutate: func(transfer *ProjectTransfer) {
				transfer.FormatVersion++
			},
		},
		{
			name: "missing revision",
			mutate: func(transfer *ProjectTransfer) {
				transfer.DocumentRevisions = nil
			},
		},
		{
			name: "missing dictation destination",
			mutate: func(transfer *ProjectTransfer) {
				missing := "missing-node"
				transfer.DictationSessions[0].DestinationNodeID = &missing
			},
		},
		{
			name: "tree cycle",
			mutate: func(transfer *ProjectTransfer) {
				parent := transfer.Nodes[0].ID
				transfer.Nodes[0].ParentID = &parent
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transfer := validProjectTransferFixture()
			test.mutate(&transfer)
			err := ValidateProjectTransfer(transfer)
			if !errors.Is(err, ErrInvalidProjectTransfer) {
				t.Fatalf("ValidateProjectTransfer() error = %v, want ErrInvalidProjectTransfer", err)
			}
		})
	}
}

func TestProjectTransferV5CarriesProfileAndRemapsProjectImages(t *testing.T) {
	t.Parallel()

	assetID := "45b17482-b4a0-4624-a68c-d2f218eb4e20"
	transfer := validProjectTransferFixture()
	transfer.DocumentRevisions[0].SchemaVersion = 2
	transfer.DocumentRevisions[0].Content = json.RawMessage(
		`{"type":"doc","attrs":{"schemaVersion":2},"content":[{"type":"imageBlock","attrs":{"src":"/api/media/45b17482-b4a0-4624-a68c-d2f218eb4e20/content","assetId":"45b17482-b4a0-4624-a68c-d2f218eb4e20","alt":"Карта"}}]}`,
	)
	transfer.Media = []ProjectTransferMedia{{
		ID: assetID, Path: "media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png",
		SHA256:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ByteSize: 128, MediaType: "image/png", OriginalName: "map.png", Width: 640, Height: 480,
	}}
	transfer.Project.ExportProfile.Metadata.CoverAssetID = &assetID
	if err := ValidateProjectTransfer(transfer); err != nil {
		t.Fatalf("valid v3 project media rejected: %v", err)
	}

	legacyCandidate := transfer
	legacyCandidate.FormatVersion = 2
	if err := ValidateProjectTransfer(legacyCandidate); !errors.Is(err, ErrInvalidProjectTransfer) {
		t.Fatalf("media in v2 error = %v", err)
	}
	missingCandidate := transfer
	missingCandidate.Media = nil
	if err := ValidateProjectTransfer(missingCandidate); !errors.Is(err, ErrInvalidProjectTransfer) {
		t.Fatalf("missing media error = %v", err)
	}

	newID := "850b6344-8a17-4475-aed2-019eb99f96d4"
	remapped, err := remapDocumentMedia(
		transfer.DocumentRevisions[0].Content,
		map[string]string{assetID: newID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(remapped) == string(transfer.DocumentRevisions[0].Content) {
		t.Fatal("project media reference was not remapped")
	}
	if ids := documentMediaIDs(remapped); len(ids) != 1 || ids[0] != newID {
		t.Fatalf("remapped media IDs = %#v", ids)
	}
}

func TestProjectTransferV5CarriesMarkdownSourceMetadataAndRemapsInternalLinks(t *testing.T) {
	t.Parallel()
	transfer := validProjectTransferFixture()
	transfer.Nodes[0].SourceFrontmatter = "---\nraw:  value\n---\n"
	sourcePath := "assets/map.png"
	transfer.Media = []ProjectTransferMedia{{
		ID: "asset-1", Path: "media/asset-1.png", SourcePath: &sourcePath,
		SHA256:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ByteSize: 128, MediaType: "image/png", OriginalName: "map.png", Width: 1, Height: 1,
	}}
	if err := ValidateProjectTransfer(transfer); err != nil {
		t.Fatalf("valid v5 source metadata rejected: %v", err)
	}
	legacy := transfer
	legacy.FormatVersion = 4
	if err := ValidateProjectTransfer(legacy); !errors.Is(err, ErrInvalidProjectTransfer) {
		t.Fatalf("v4 unexpectedly accepted v5 metadata: %v", err)
	}

	raw := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"next","marks":[{"type":"link","attrs":{"href":"/projects/source-project/studio/source-node#heading"}}]}]}]}`)
	remapped, err := remapDocumentLinks(
		raw,
		"source-project",
		"target-project",
		map[string]string{"source-node": "target-node"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remapped), "/projects/target-project/studio/target-node#heading") {
		t.Fatalf("internal link was not remapped: %s", remapped)
	}
}

func TestImportedDictationStatusIsTerminal(t *testing.T) {
	if got := normalizedImportedDictationStatus("recording_online", true); got != "review_ready" {
		t.Fatalf("status with draft = %q", got)
	}
	if got := normalizedImportedDictationStatus("recording_online", false); got != "finalization_failed" {
		t.Fatalf("status without draft = %q", got)
	}
	if got := normalizedImportedDictationStatus("trashed", true); got != "trashed" {
		t.Fatalf("trashed status = %q", got)
	}
}

func TestRemapSourceReferencesPreservesEmptyArray(t *testing.T) {
	references := remapSourceReferences([]assistant.SourceReference{}, map[string]string{})
	if references == nil {
		t.Fatal("empty source references became nil")
	}
	encoded, err := json.Marshal(references)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("encoded source references = %s, want []", encoded)
	}
}

func validProjectTransferFixture() ProjectTransfer {
	now := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	document := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"blockId":"block-1","sourceRefs":[]}}]}`)
	structure := DefaultProjectStructure("novel", "ru")
	nodeID := "node-1"
	sessionID := "session-1"
	artifactID := "artifact-1"
	return ProjectTransfer{
		Format:        ProjectTransferFormat,
		FormatVersion: ProjectTransferFormatVersion,
		ExportedAt:    now,
		Project: ProjectTransferProject{
			ID: "project-1", Title: "Роман", Kind: "novel", Language: "ru",
			StructureRevision: 1, Structure: structure,
			ExportProfile: exportformat.DefaultProfile("Роман", ""), ExportProfileRevision: 1,
			CreatedAt: now, UpdatedAt: now,
		},
		Nodes: []ProjectTransferNode{{
			ID: nodeID, Kind: "chapter", Title: "Глава 1", Status: "draft",
			Position: 1000, CurrentRevision: 1, CreatedAt: now, UpdatedAt: now,
		}},
		DocumentRevisions: []ProjectTransferDocumentRevision{{
			ID: "revision-1", NodeID: nodeID, Revision: 1, BaseRevision: 0,
			SchemaVersion: 1, Content: document, PlainText: "", Source: "project_create",
			CreatedAt: now,
		}},
		NamedVersions:   []ProjectTransferNamedVersion{},
		DictionaryTerms: []ProjectTransferDictionaryTerm{},
		DictationSessions: []ProjectTransferDictationSession{{
			ID: sessionID, DestinationNodeID: &nodeID, Language: "ru", Status: "review_ready",
			Manifest:        json.RawMessage(`{"format":"webm","chunks":[]}`),
			CommandsEnabled: true, CommandPrefix: "Команда", CreatedAt: now, UpdatedAt: now,
		}},
		TranscriptArtifacts: []ProjectTransferArtifact{{
			ID: artifactID, SessionID: sessionID, Kind: "live_final", PipelineVersion: "1",
			Provider: "test", Model: "test", Language: "ru", Segments: json.RawMessage(`[]`),
			CreatedAt: now,
		}},
		TranscriptDrafts: []ProjectTransferDraft{{
			ID: "draft-1", SessionID: sessionID, SourceArtifactID: &artifactID, Revision: 1,
			Content: document, CreatedAt: now, UpdatedAt: now,
		}},
		TranscriptDraftRevisions: []ProjectTransferDraftRevision{{
			ID: "draft-revision-1", SessionID: sessionID, Revision: 1, Content: document,
			Source: "imported", CreatedAt: now,
		}},
		Audio:                  []ProjectTransferAudio{},
		ProjectMemory:          []ProjectTransferMemoryRecord{},
		ProjectMemoryRevisions: []ProjectTransferMemoryRevision{},
	}
}
