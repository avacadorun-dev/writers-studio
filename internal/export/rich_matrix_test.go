package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRichBlockExportMatrixPreservesSemanticsAndCodeBytes(t *testing.T) {
	code := "package main  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	snapshot, assets := richMatrixSnapshot(t, code)

	publication := buildPublication(snapshot, dataImageResolver(snapshot, assets))
	html := renderPrintHTML(snapshot, publication, dataImageResolver(snapshot, assets), assets)
	for _, expected := range []string{
		`class="callout callout-info"`, `class="callout callout-idea"`,
		`class="callout callout-warning"`, `class="callout callout-todo"`,
		`<table><thead>`, `class="image-block"`, `data-language="go"`,
		`mermaid-source`, `<ul>`, `<ol>`, `class="scene-break"`,
		`Команда <code>go run</code>`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("publication HTML is missing %q", expected)
		}
	}
	if got := extractHTMLCodeText(html); got != code {
		t.Fatalf("publication HTML changed code bytes: %q", got)
	}

	chromium := os.Getenv("WRITERS_TEST_CHROMIUM")
	docx, _, err := RenderWithAssets(context.Background(), "docx", snapshot, assets, chromium)
	if err != nil {
		t.Fatal(err)
	}
	documentXML := string(readDOCXDocument(t, docx))
	for _, expected := range []string{
		`w:pStyle w:val="CalloutInfo"`, `w:pStyle w:val="CalloutIdea"`,
		`w:pStyle w:val="CalloutWarning"`, `w:pStyle w:val="CalloutTodo"`,
		`<w:tbl>`, `w:tblHeader`, `w:pStyle w:val="TableBody"`,
		`w:pStyle w:val="CodeBlock"`, `w:numId w:val="1"`, `w:numId w:val="2"`,
		`w:ascii="IBM Plex Mono"`, `<w:t xml:space="preserve">go run</w:t>`,
	} {
		if !strings.Contains(documentXML, expected) {
			t.Fatalf("DOCX is missing %q", expected)
		}
	}
	if got := extractWordParagraphText(t, documentXML, "CodeBlock"); got != code {
		t.Fatalf("DOCX changed code bytes: %q", got)
	}

	epub, _, err := RenderWithAssets(context.Background(), "epub", snapshot, assets, chromium)
	if err != nil {
		t.Fatal(err)
	}
	epubArchive, err := zip.NewReader(bytes.NewReader(epub), int64(len(epub)))
	if err != nil {
		t.Fatal(err)
	}
	section := readZipFile(t, epubArchive, "OEBPS/section-1.xhtml")
	for _, expected := range []string{
		`callout-info`, `callout-idea`, `callout-warning`, `callout-todo`,
		`<table><thead>`, `data-language="go"`, `<ul>`, `<ol>`,
		`Команда <code>go run</code>`,
	} {
		if !strings.Contains(section, expected) {
			t.Fatalf("EPUB is missing %q: %s", expected, section)
		}
	}
	if got := extractHTMLCodeText(section); got != code {
		t.Fatalf("EPUB changed code bytes: %q", got)
	}

	outputDirectory := strings.TrimSpace(os.Getenv("WRITERS_TEST_RICH_OUTPUT_DIR"))
	if outputDirectory == "" {
		return
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRichArtifact(t, filepath.Join(outputDirectory, "rich-blocks.docx"), docx)
	writeRichArtifact(t, filepath.Join(outputDirectory, "rich-blocks.epub"), epub)
	writeRichArtifact(t, filepath.Join(outputDirectory, "rich-blocks.html"), []byte(html))
	if chromium != "" {
		pdf, _, renderErr := RenderWithAssets(
			context.Background(), "pdf", snapshot, assets, chromium,
		)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		writeRichArtifact(t, filepath.Join(outputDirectory, "rich-blocks.pdf"), pdf)
	}
}

func richMatrixSnapshot(t *testing.T, code string) (Snapshot, map[string][]byte) {
	t.Helper()
	const assetID = "11111111-1111-1111-1111-111111111111"
	content, err := json.Marshal(map[string]any{
		"type": "doc", "attrs": map[string]any{"schemaVersion": 2},
		"content": []any{
			map[string]any{"type": "heading", "attrs": map[string]any{"level": 2}, "content": []any{map[string]any{"type": "text", "text": "Матрица блоков"}}},
			map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Текст до листингов и схем."}}},
			matrixCallout("info", "Справочный контекст"),
			matrixCallout("idea", "Идея для следующей главы"),
			matrixCallout("warning", "Проверить риск"),
			matrixCallout("todo", "Сверить факты"),
			map[string]any{"type": "codeBlock", "attrs": map[string]any{"language": "go"}, "content": []any{map[string]any{"type": "text", "text": code}}},
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": "Команда "},
				map[string]any{"type": "text", "text": "go run", "marks": []any{map[string]any{"type": "code"}}},
				map[string]any{"type": "text", "text": " собирает программу."},
			}},
			map[string]any{"type": "sceneBreak"},
			map[string]any{"type": "table", "content": []any{
				matrixTableRow("tableHeader", "Сущность", "Состояние", "Комментарий"),
				matrixTableRow("tableCell", "Герой", "Готов", "Длинное значение переносится внутри ячейки, а строка остаётся цельной."),
				matrixTableRow("tableCell", "Антагонист", "Черновик", "Нужна мотивация"),
			}},
			map[string]any{"type": "horizontalRule"},
			map[string]any{"type": "bulletList", "content": []any{matrixListItem("Первый пункт"), matrixListItem("Второй пункт")}},
			map[string]any{"type": "orderedList", "content": []any{matrixListItem("Первый шаг"), matrixListItem("Второй шаг")}},
			map[string]any{"type": "imageBlock", "attrs": map[string]any{"assetId": assetID, "src": "/api/media/" + assetID + "/content", "alt": "Тестовая иллюстрация", "caption": "Рисунок 1. Иллюстрация"}},
			map[string]any{"type": "mermaidBlock", "attrs": map[string]any{"code": "flowchart LR\n  Draft --> Review\n  Review --> Publish", "caption": "Процесс публикации"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	imageBytes := matrixPNG(t)
	profile := DefaultProfile("Rich blocks", "Проверка всех канонических блоков")
	profile.TitlePage = false
	profile.TableOfContents = false
	profile.ChapterTitles = true
	profile.Metadata.Author = "Writer's Studio QA"
	return Snapshot{
		SchemaVersion: 2, ExportID: "22222222-2222-2222-2222-222222222222",
		ProjectID: "rich-project", ProjectTitle: "Rich blocks", ProjectLanguage: "ru",
		Profile: profile,
		Documents: []SnapshotDocument{{
			NodeID: "rich-document", Kind: "chapter", Title: "Все блоки",
			Revision: 1, Content: content, PlainText: "Матрица блоков",
		}},
		Media: []SnapshotMedia{{
			ID: assetID, MediaType: "image/png", OriginalName: "illustration.png",
			Width: 640, Height: 240,
		}},
	}, map[string][]byte{assetID: imageBytes}
}

func matrixCallout(kind, text string) map[string]any {
	return map[string]any{
		"type": "callout", "attrs": map[string]any{"kind": kind},
		"content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}},
	}
}

func matrixTableRow(cellType, first, second, third string) map[string]any {
	cell := func(text string) map[string]any {
		return map[string]any{
			"type": cellType, "attrs": map[string]any{"colspan": 1, "rowspan": 1},
			"content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}},
		}
	}
	return map[string]any{"type": "tableRow", "content": []any{cell(first), cell(second), cell(third)}}
}

func matrixListItem(text string) map[string]any {
	return map[string]any{
		"type":    "listItem",
		"content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}},
	}
}

func matrixPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 640, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 640; x++ {
			shade := uint8(35 + (x*100)/640)
			picture.SetRGBA(x, y, color.RGBA{R: shade, G: uint8(90 + y/3), B: 150, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, picture); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeRichArtifact(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
