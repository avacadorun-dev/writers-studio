package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"writers.studio/internal/store"
)

func TestProjectTransferArchiveRoundTrip(t *testing.T) {
	audio := []byte("portable audio")
	image, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	transfer := validHTTPProjectTransfer()
	transfer.Audio = []store.ProjectTransferAudio{{
		SessionID: "session-1", Path: "audio/session-1.webm", SHA256: sha256Hex(audio),
		ByteSize: int64(len(audio)), DurationMS: 1200, MediaType: "audio/webm",
	}}
	transfer.Media = []store.ProjectTransferMedia{{
		ID:   "45b17482-b4a0-4624-a68c-d2f218eb4e20",
		Path: "media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png", SHA256: sha256Hex(image),
		ByteSize: int64(len(image)), MediaType: "image/png", OriginalName: "map.png",
		Width: 1, Height: 1,
	}}
	archive, err := encodeProjectTransferArchive(
		transfer,
		map[string][]byte{
			"audio/session-1.webm":                           audio,
			"media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png": image,
		},
	)
	if err != nil {
		t.Fatalf("encodeProjectTransferArchive() error = %v", err)
	}
	decoded, decodedAudio, err := decodeProjectTransferArchive(archive)
	if err != nil {
		t.Fatalf("decodeProjectTransferArchive() error = %v", err)
	}
	if decoded.Project.Title != transfer.Project.Title {
		t.Fatalf("decoded title = %q", decoded.Project.Title)
	}
	if string(decodedAudio["audio/session-1.webm"]) != string(audio) {
		t.Fatalf("decoded audio = %q", decodedAudio["audio/session-1.webm"])
	}
	if string(decodedAudio["media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png"]) != string(image) {
		t.Fatalf("decoded image = %q", decodedAudio["media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png"])
	}
}

func TestProjectTransferArchiveRejectsChecksumMismatch(t *testing.T) {
	audio := []byte("portable audio")
	transfer := validHTTPProjectTransfer()
	transfer.Audio = []store.ProjectTransferAudio{{
		SessionID: "session-1", Path: "audio/session-1.webm", SHA256: strings.Repeat("0", 64),
		ByteSize: int64(len(audio)), DurationMS: 1200, MediaType: "audio/webm",
	}}
	archive, err := encodeProjectTransferArchive(
		transfer,
		map[string][]byte{"audio/session-1.webm": audio},
	)
	if err != nil {
		t.Fatalf("encodeProjectTransferArchive() error = %v", err)
	}
	if _, _, err := decodeProjectTransferArchive(archive); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("decodeProjectTransferArchive() error = %v, want checksum error", err)
	}
}

func validHTTPProjectTransfer() store.ProjectTransfer {
	now := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	document := json.RawMessage(`{"type":"doc","content":[]}`)
	structure := store.DefaultProjectStructure("novel", "ru")
	nodeID := "node-1"
	return store.ProjectTransfer{
		Format: store.ProjectTransferFormat, FormatVersion: store.ProjectTransferFormatVersion,
		ExportedAt: now,
		Project: store.ProjectTransferProject{
			ID: "project-1", Title: "Роман", Kind: "novel", Language: "ru",
			StructureRevision: 1, Structure: structure, CreatedAt: now, UpdatedAt: now,
		},
		Nodes: []store.ProjectTransferNode{{
			ID: nodeID, Kind: "chapter", Title: "Глава", Status: "draft", Position: 1000,
			CurrentRevision: 1, CreatedAt: now, UpdatedAt: now,
		}},
		DocumentRevisions: []store.ProjectTransferDocumentRevision{{
			ID: "revision-1", NodeID: nodeID, Revision: 1, SchemaVersion: 1,
			Content: document, Source: "project_create", CreatedAt: now,
		}},
		NamedVersions:   []store.ProjectTransferNamedVersion{},
		DictionaryTerms: []store.ProjectTransferDictionaryTerm{},
		DictationSessions: []store.ProjectTransferDictationSession{{
			ID: "session-1", DestinationNodeID: &nodeID, Language: "ru", Status: "review_ready",
			Manifest: json.RawMessage(`{"format":"webm","chunks":[]}`), CommandsEnabled: true,
			CommandPrefix: "Команда", CreatedAt: now, UpdatedAt: now,
		}},
		TranscriptArtifacts:      []store.ProjectTransferArtifact{},
		TranscriptDrafts:         []store.ProjectTransferDraft{},
		TranscriptDraftRevisions: []store.ProjectTransferDraftRevision{},
		ProjectMemory:            []store.ProjectTransferMemoryRecord{},
		ProjectMemoryRevisions:   []store.ProjectTransferMemoryRevision{},
	}
}
