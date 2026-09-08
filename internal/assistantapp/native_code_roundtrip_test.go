package assistantapp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"writers.studio/internal/editor"
	publicationexport "writers.studio/internal/export"
	"writers.studio/internal/markdowntree"
)

func TestNativeGoCodeAcceptanceRoundTrip(t *testing.T) {
	t.Parallel()
	original := "package main  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	created, err := normalizeAssistantContent(map[string]any{
		"blocks": []any{map[string]any{
			"type": "code", "language": "go", "text": original,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	readBlocks, err := assistantBlocksFromDocument(created)
	if err != nil {
		t.Fatal(err)
	}
	assertGoCodeBlock(t, readBlocks, original)
	var document editor.Node
	if err := json.Unmarshal(created, &document); err != nil {
		t.Fatal(err)
	}
	blockID, _ := document.Content[0].Attrs["blockId"].(string)
	edited, err := replaceDocumentRanges(created, map[string]any{
		"edits": []any{map[string]any{
			"blockId": blockID, "start": 8, "end": 12,
			"expectedText": "main", "replacement": "entry",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := "package entry  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"

	profile := publicationexport.DefaultProfile("Code round-trip", "")
	profile.TitlePage = false
	profile.ChapterTitles = false
	markdown, contentType, err := publicationexport.Render("markdown", publicationexport.Snapshot{
		ProjectTitle: "Code round-trip",
		Profile:      profile,
		Documents: []publicationexport.SnapshotDocument{{
			NodeID: "code", Title: "main.go", Revision: 2, Content: edited,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("Markdown content type = %q", contentType)
	}
	imported, err := markdowntree.ParseMarkdown("main.md", markdown, nil)
	if err != nil {
		t.Fatal(err)
	}
	importedBlocks, err := assistantBlocksFromDocument(imported.Content)
	if err != nil {
		t.Fatal(err)
	}
	assertGoCodeBlock(t, importedBlocks, expected)
}

func TestInlineCodeAcceptanceEditAndPublicationRoundTrip(t *testing.T) {
	t.Parallel()
	created, err := normalizeAssistantContent(map[string]any{
		"blocks": []any{map[string]any{
			"type": "paragraph",
			"text": "`go run` собирает, ``fmt `raw`  `` форматирует, `go test` проверяет.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document editor.Node
	if err := json.Unmarshal(created, &document); err != nil {
		t.Fatal(err)
	}
	inline := document.Content[0].Content
	if len(inline) != 6 || inline[0].Text != "go run" || inline[2].Text != "fmt `raw`  " ||
		inline[4].Text != "go test" {
		t.Fatalf("canonical inline split = %#v", inline)
	}
	for _, index := range []int{0, 2, 4} {
		if len(inline[index].Marks) != 1 || inline[index].Marks[0].Type != "code" {
			t.Fatalf("inline code marks = %#v", inline[index].Marks)
		}
	}
	blocks, err := assistantBlocksFromDocument(created)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Text !=
		"`go run` собирает, ``fmt `raw`  `` форматирует, `go test` проверяет." {
		t.Fatalf("MCP inline code read = %#v", blocks)
	}

	blockID, _ := document.Content[0].Attrs["blockId"].(string)
	edited, err := replaceDocumentRanges(created, map[string]any{
		"edits": []any{map[string]any{
			"blockId": blockID, "start": 3, "end": 6,
			"expectedText": "run", "replacement": "test",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	editedBlocks, err := assistantBlocksFromDocument(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(editedBlocks[0].Text, "`go test` собирает") {
		t.Fatalf("range edit lost inline code: %#v", editedBlocks)
	}

	profile := publicationexport.DefaultProfile("Inline code", "")
	profile.TitlePage = false
	profile.ChapterTitles = false
	snapshot := publicationexport.Snapshot{
		ProjectTitle: "Inline code", Profile: profile,
		Documents: []publicationexport.SnapshotDocument{{
			NodeID: "inline", Title: "Команды", Revision: 2, Content: edited,
		}},
	}
	markdown, _, err := publicationexport.Render("markdown", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := markdowntree.ParseMarkdown("inline.md", markdown, nil)
	if err != nil {
		t.Fatal(err)
	}
	importedBlocks, err := assistantBlocksFromDocument(imported.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(importedBlocks) != 1 || importedBlocks[0].Text != editedBlocks[0].Text {
		t.Fatalf("Markdown inline code round-trip = %#v, want %#v", importedBlocks, editedBlocks)
	}

	docx, _, err := publicationexport.Render("docx", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	documentXML := zipEntry(t, docx, "word/document.xml")
	if !strings.Contains(documentXML, `w:ascii="IBM Plex Mono"`) ||
		!strings.Contains(documentXML, `<w:t xml:space="preserve">go test</w:t>`) {
		t.Fatalf("DOCX inline code formatting missing: %s", documentXML)
	}
}

func zipEntry(t *testing.T, archive []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return string(data)
	}
	t.Fatalf("archive entry %q not found", name)
	return ""
}

func assertGoCodeBlock(t *testing.T, blocks []assistantContentBlock, expected string) {
	t.Helper()
	if len(blocks) != 1 || blocks[0].Type != "code" || blocks[0].Language == nil ||
		*blocks[0].Language != "go" || blocks[0].Text != expected {
		t.Fatalf("Go code changed: %#v", blocks)
	}
}
