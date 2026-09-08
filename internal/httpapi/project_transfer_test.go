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
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	mp3 := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x0f"), []byte("canvas-demo-mp3")...)
	mp4 := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x02\x00isommp42")
	transfer := validHTTPProjectTransfer()
	transfer.Audio = []store.ProjectTransferAudio{{
		SessionID: "session-1", Path: "audio/session-1.webm", SHA256: sha256Hex(audio),
		ByteSize: int64(len(audio)), DurationMS: 1200, MediaType: "audio/webm",
	}}
	transfer.Media = []store.ProjectTransferMedia{
		{
			ID:   "45b17482-b4a0-4624-a68c-d2f218eb4e20",
			Path: "media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png", SHA256: sha256Hex(image),
			ByteSize: int64(len(image)), MediaType: "image/png", OriginalName: "map.png",
			Width: intPointer(1), Height: intPointer(1),
		},
		{
			ID: "pdf-1", Path: "media/pdf-1.pdf", SHA256: sha256Hex(pdf),
			ByteSize: int64(len(pdf)), MediaType: "application/pdf", OriginalName: "rules.pdf",
		},
		{
			ID: "audio-1", Path: "media/audio-1.mp3", SHA256: sha256Hex(mp3),
			ByteSize: int64(len(mp3)), MediaType: "audio/mpeg", OriginalName: "signal.mp3",
			DurationMS: int64Pointer(4000),
		},
		{
			ID: "video-1", Path: "media/video-1.mp4", SHA256: sha256Hex(mp4),
			ByteSize: int64(len(mp4)), MediaType: "video/mp4", OriginalName: "field-log.mp4",
			DurationMS: int64Pointer(4000),
		},
	}
	archive, err := encodeProjectTransferArchive(
		transfer,
		map[string][]byte{
			"audio/session-1.webm":                           audio,
			"media/45b17482-b4a0-4624-a68c-d2f218eb4e20.png": image,
			"media/pdf-1.pdf":                                pdf,
			"media/audio-1.mp3":                              mp3,
			"media/video-1.mp4":                              mp4,
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
	if string(decodedAudio["media/pdf-1.pdf"]) != string(pdf) {
		t.Fatalf("decoded PDF = %q", decodedAudio["media/pdf-1.pdf"])
	}
	if string(decodedAudio["media/audio-1.mp3"]) != string(mp3) {
		t.Fatalf("decoded MP3 = %q", decodedAudio["media/audio-1.mp3"])
	}
	if string(decodedAudio["media/video-1.mp4"]) != string(mp4) {
		t.Fatalf("decoded MP4 = %q", decodedAudio["media/video-1.mp4"])
	}
}

func int64Pointer(value int64) *int64 {
	return &value
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
