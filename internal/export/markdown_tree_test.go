package export

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestRenderMarkdownTreePreservesStructureFrontmatterLinksAndMedia(t *testing.T) {
	t.Parallel()
	const (
		projectID = "00000000-0000-0000-0000-000000000001"
		chapterID = "00000000-0000-0000-0000-000000000002"
		childID   = "00000000-0000-0000-0000-000000000003"
		folderID  = "00000000-0000-0000-0000-000000000004"
		mediaID   = "00000000-0000-0000-0000-000000000005"
	)
	childParent := chapterID
	image := []byte("exact-binary-image")
	checksum := sha256.Sum256(image)
	chapterContent := json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":3},"content":[
			{"type":"heading","attrs":{"level":4},"content":[{"type":"text","text":"Chapter"}]},
			{"type":"paragraph","content":[{"type":"text","text":"child","marks":[{"type":"link","attrs":{"href":"/projects/` + projectID + `/studio/` + childID + `#part"}}]}]},
			{"type":"imageBlock","attrs":{"assetId":"` + mediaID + `","src":"/api/media/` + mediaID + `/content","alt":"pixel"}}
		]
	}`)
	snapshot := Snapshot{
		SchemaVersion: 2, ProjectID: projectID, ProjectTitle: "Vault: Test",
		TreeNodes: []SnapshotTreeNode{
			{NodeID: chapterID, Kind: "document", Title: "Chapter", Position: 1000, ContentBearing: true, Revision: 1, Content: chapterContent, Frontmatter: "---\nraw:  value\n---\n"},
			{NodeID: childID, ParentID: &childParent, Kind: "document", Title: "Child", Position: 1000, ContentBearing: true, Revision: 1, Content: json.RawMessage(`{"type":"doc","attrs":{"schemaVersion":3},"content":[{"type":"paragraph","content":[{"type":"text","text":"deep marker"}]}]}`)},
			{NodeID: folderID, Kind: "folder", Title: "Empty", Position: 2000, ContentBearing: false, Revision: 1},
			{NodeID: "00000000-0000-0000-0000-000000000006", Kind: "document", Title: "Note", Position: 3000, ContentBearing: true, Content: json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)},
			{NodeID: "00000000-0000-0000-0000-000000000007", Kind: "document", Title: "note", Position: 4000, ContentBearing: true, Content: json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)},
		},
		Media: []SnapshotMedia{{ID: mediaID, MediaType: "image/png", OriginalName: "pixel.png", SourcePath: stringPointer("assets/pixel.png"), SHA256: hex.EncodeToString(checksum[:])}},
	}
	data, contentType, err := RenderWithAssets(nil, "markdown_tree", snapshot, map[string][]byte{mediaID: image}, "")
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/zip" {
		t.Fatalf("unexpected MIME type %q", contentType)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, item := range archive.File {
		reader, openErr := item.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[item.Name] = content
	}
	for _, expected := range []string{
		"Vault- Test/", "Vault- Test/Empty/", "Vault- Test/Chapter.md",
		"Vault- Test/Chapter/", "Vault- Test/Chapter/Child.md",
		"Vault- Test/Note.md", "Vault- Test/note (2).md", "Vault- Test/assets/pixel.png",
	} {
		if _, exists := entries[expected]; !exists {
			t.Fatalf("ZIP entry %q is missing; got %v", expected, sortedEntryNames(entries))
		}
	}
	chapter := string(entries["Vault- Test/Chapter.md"])
	if !strings.HasPrefix(chapter, "---\nraw:  value\n---\n") ||
		!strings.Contains(chapter, "#### Chapter") ||
		!strings.Contains(chapter, "Chapter/Child.md#part") ||
		!strings.Contains(chapter, "assets/pixel.png") {
		t.Fatalf("portable Markdown is incomplete: %s", chapter)
	}
	if !bytes.Equal(entries["Vault- Test/assets/pixel.png"], image) {
		t.Fatal("image bytes changed")
	}
}

func stringPointer(value string) *string { return &value }

func sortedEntryNames(entries map[string][]byte) []string {
	result := make([]string, 0, len(entries))
	for name := range entries {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}
