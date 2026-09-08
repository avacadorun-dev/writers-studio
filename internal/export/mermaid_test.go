package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"writers.studio/internal/editor"
)

func TestMermaidTraversalReplacesOnlySuccessfullyRenderedBlocks(t *testing.T) {
	t.Parallel()
	snapshot := mermaidSnapshot()
	if count := countMermaidBlocks(snapshot); count != 1 {
		t.Fatalf("Mermaid block count = %d", count)
	}
	if count := countMermaidBlocks(Snapshot{Documents: []SnapshotDocument{{Content: json.RawMessage(`bad`)}}}); count != 0 {
		t.Fatalf("malformed document count = %d", count)
	}

	var root editor.Node
	if err := json.Unmarshal(snapshot.Documents[0].Content, &root); err != nil {
		t.Fatal(err)
	}
	failed := root
	if replaceMermaidNodes(&failed, func(string, string) (string, bool) { return "", false }) {
		t.Fatal("failed Mermaid rendering changed the source block")
	}
	if !replaceMermaidNodes(&root, func(code, caption string) (string, bool) {
		if code != "flowchart LR\n  A --> B" || caption != "Путь" {
			t.Fatalf("diagram metadata changed: %q %q", code, caption)
		}
		return "11111111-1111-1111-1111-111111111111", true
	}) {
		t.Fatal("successful Mermaid rendering did not change the block")
	}
	image := root.Content[0].Content[0]
	if image.Type != "imageBlock" || image.Attrs["caption"] != "Путь" ||
		image.Attrs["blockId"] != "diagram-block" {
		t.Fatalf("rendered Mermaid image is not canonical: %#v", image)
	}
}

func TestMermaidRendererEmbedsDiagramsAcrossDOCXAndEPUB(t *testing.T) {
	chromiumPath := os.Getenv("WRITERS_TEST_CHROMIUM")
	if chromiumPath == "" {
		for _, candidate := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				chromiumPath = candidate
				break
			}
		}
	}
	if chromiumPath == "" || resolveMermaidScriptPath() == "" {
		t.Skip("Chromium and Mermaid distribution are required for visual export")
	}

	snapshot := mermaidSnapshot()
	prepared, assets, prepareErr := prepareMermaidAssets(context.Background(), snapshot, nil, chromiumPath)
	if len(prepared.Media) != 1 || len(assets) != 1 || countMermaidBlocks(prepared) != 0 {
		t.Fatalf("Mermaid diagram was not prepared: media=%#v assets=%d err=%v", prepared.Media, len(assets), prepareErr)
	}
	png := assets[prepared.Media[0].ID]
	if len(png) < 8 || !bytes.Equal(png[:8], []byte("\x89PNG\r\n\x1a\n")) ||
		prepared.Media[0].Width < 100 || prepared.Media[0].Height < 50 {
		t.Fatalf("Mermaid PNG is invalid: bytes=%d size=%dx%d", len(png), prepared.Media[0].Width, prepared.Media[0].Height)
	}

	docx, _, err := RenderWithAssets(context.Background(), "docx", snapshot, nil, chromiumPath)
	if err != nil {
		t.Fatal(err)
	}
	docxArchive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipFile(t, docxArchive, "word/document.xml")
	if !strings.Contains(documentXML, `<w:drawing>`) || !strings.Contains(documentXML, "Путь") ||
		strings.Contains(documentXML, "flowchart LR") {
		t.Fatalf("DOCX did not embed the rendered Mermaid diagram: %s", documentXML)
	}
	if png := readZipFileBytes(t, docxArchive, "word/media/image-1.png"); len(png) < 8 {
		t.Fatal("DOCX Mermaid image is missing")
	}

	epub, _, err := RenderWithAssets(context.Background(), "epub", snapshot, nil, chromiumPath)
	if err != nil {
		t.Fatal(err)
	}
	epubArchive, err := zip.NewReader(bytes.NewReader(epub), int64(len(epub)))
	if err != nil {
		t.Fatal(err)
	}
	section := readZipFile(t, epubArchive, "OEBPS/section-1.xhtml")
	if !strings.Contains(section, `<img src="images/`) || !strings.Contains(section, "Путь") ||
		strings.Contains(section, "flowchart LR") {
		t.Fatalf("EPUB did not embed the rendered Mermaid diagram: %s", section)
	}

	invalidDiagram := mermaidSnapshot()
	invalidDiagram.Documents[0].Content = json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"mermaidBlock","attrs":{"code":"not mermaid {{{","caption":"Ошибка"}}
		]}`)
	prepared, retained, err := prepareMermaidAssets(
		context.Background(), invalidDiagram, map[string][]byte{"existing": {1}}, chromiumPath,
	)
	if err == nil || len(retained) != 1 || countMermaidBlocks(prepared) != 1 {
		t.Fatalf("invalid Mermaid source did not retain its fallback: assets=%d err=%v", len(retained), err)
	}

	brokenScript := filepath.Join(t.TempDir(), "broken-mermaid.js")
	if err := os.WriteFile(brokenScript, []byte(`throw new Error("broken renderer")`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MERMAID_JS_PATH", brokenScript)
	if _, _, err := prepareMermaidAssets(
		context.Background(), snapshot, nil, chromiumPath,
	); err == nil || !strings.Contains(err.Error(), "load Mermaid renderer") {
		t.Fatalf("broken Mermaid runtime error = %v", err)
	}
	emptyScript := filepath.Join(t.TempDir(), "empty-mermaid.js")
	if err := os.WriteFile(emptyScript, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MERMAID_JS_PATH", emptyScript)
	if _, _, err := prepareMermaidAssets(
		context.Background(), snapshot, nil, chromiumPath,
	); err == nil || !strings.Contains(err.Error(), "invalid size") {
		t.Fatalf("empty Mermaid runtime error = %v", err)
	}
}

func TestMermaidScriptPathHonorsExplicitMissingConfiguration(t *testing.T) {
	t.Setenv("MERMAID_JS_PATH", "/missing/mermaid.js")
	if path := resolveMermaidScriptPath(); path != "" {
		t.Fatalf("missing configured Mermaid script resolved to %q", path)
	}
}

func TestValidatedMermaidPNGRejectsNonImageBytes(t *testing.T) {
	t.Parallel()
	if _, _, _, err := validatedMermaidPNG([]byte("not png")); err == nil {
		t.Fatal("invalid Mermaid screenshot was accepted")
	}
	valid := matrixPNG(t)
	data, width, height, err := validatedMermaidPNG(valid)
	if err != nil || !bytes.Equal(data, valid) || width != 640 || height != 240 {
		t.Fatalf("valid Mermaid dimensions = %dx%d, %v", width, height, err)
	}
}

func mermaidSnapshot() Snapshot {
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.ExportID = "22222222-2222-2222-2222-222222222222"
	snapshot.ProjectLanguage = "ru"
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "")
	snapshot.Documents[0].Content = json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"callout","attrs":{"kind":"idea"},"content":[
				{"type":"mermaidBlock","attrs":{"blockId":"diagram-block","code":"flowchart LR\n  A --> B","caption":"Путь"}}
			]}
		]
	}`)
	return snapshot
}
