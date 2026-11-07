package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	stdhtml "html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"writers.studio/internal/editor"
)

func fixtureSnapshot() Snapshot {
	return Snapshot{
		ProjectTitle:  "Дом у тихой воды",
		TitlePage:     true,
		ChapterTitles: true,
		Documents: []SnapshotDocument{
			{
				Title:     "Глава 1",
				PlainText: "Дом стоял у самой воды.",
				Content: json.RawMessage(`{
					"type":"doc",
					"content":[{"type":"paragraph","attrs":{"blockId":"p"},"content":[{"type":"text","text":"Дом стоял у самой воды."}]}]
				}`),
			},
		},
	}
}

func TestRenderPortableFormatsContainSnapshotContent(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"txt", "markdown"} {
		format := format
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			data, _, err := Render(format, fixtureSnapshot())
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			if !strings.Contains(content, "Дом у тихой воды") ||
				!strings.Contains(content, "Дом стоял у самой воды.") {
				t.Fatalf("export lost content: %q", content)
			}
		})
	}
}

func TestConcurrentRenderDoesNotMutateSnapshotContent(t *testing.T) {
	t.Parallel()

	legacy := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Run ` + "`go run`" + `."}]}]}`)
	original := bytes.Clone(legacy)
	snapshot := fixtureSnapshot()
	snapshot.Documents[0].NodeID = "chapter"
	snapshot.Documents[0].Content = legacy
	snapshot.TreeNodes = []SnapshotTreeNode{{
		NodeID: "chapter", Kind: "chapter", Title: "Chapter One", ContentBearing: true,
		Content: legacy, PlainText: "Run go run.",
	}}
	t.Cleanup(func() {
		if !bytes.Equal(snapshot.Documents[0].Content, original) {
			t.Error("rendering changed the input document content")
		}
		if !bytes.Equal(snapshot.TreeNodes[0].Content, original) {
			t.Error("rendering changed the input tree-node content")
		}
	})

	for _, format := range []string{"txt", "markdown", "markdown_tree", "docx", "epub"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			if _, _, err := Render(format, snapshot); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExportMigratesLegacyInlineCodeBeforeEveryPublicationRenderer(t *testing.T) {
	t.Parallel()
	legacy := json.RawMessage(`{
		"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"Команды: ` + "`go run`" + ` и ` + "``fmt `raw`  ``" + `."}
		]}]
	}`)
	snapshot := fixtureSnapshot()
	snapshot.Profile = NormalizeProfile(snapshot.Profile, snapshot.ProjectTitle, "")
	snapshot.Profile.TitlePage = false
	snapshot.Profile.ChapterTitles = false
	snapshot.Documents[0].Content = legacy
	snapshot.TreeNodes = []SnapshotTreeNode{{NodeID: "legacy", Content: legacy}}

	normalized := normalizeLegacyCodeFormatting(snapshot)
	for _, content := range []json.RawMessage{
		normalized.Documents[0].Content, normalized.TreeNodes[0].Content,
	} {
		var document editor.Node
		if err := json.Unmarshal(content, &document); err != nil {
			t.Fatal(err)
		}
		inline := document.Content[0].Content
		if len(inline) != 5 || inline[1].Text != "go run" || inline[3].Text != "fmt `raw`  " ||
			len(inline[1].Marks) != 1 || inline[1].Marks[0].Type != "code" ||
			len(inline[3].Marks) != 1 || inline[3].Marks[0].Type != "code" {
			t.Fatalf("legacy inline code was not canonicalized: %#v", inline)
		}
	}

	markdown, _, err := Render("markdown", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "Команды: `go run` и ``fmt `raw`  ``.") {
		t.Fatalf("Markdown lost inline code: %q", markdown)
	}
	var document editor.Node
	if err := json.Unmarshal(normalized.Documents[0].Content, &document); err != nil {
		t.Fatal(err)
	}
	html := renderPublicationNodes(document.Content, nil)
	if !strings.Contains(html, "Команды: <code>go run</code> и <code>fmt `raw`  </code>.") {
		t.Fatalf("publication HTML lost inline code: %s", html)
	}
}

func TestRenderDOCXProducesReadableOfficePackage(t *testing.T) {
	t.Parallel()

	data, contentType, err := Render("docx", fixtureSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	if outputPath := os.Getenv("WRITERS_TEST_DOCX_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{
		"word/document.xml": false, "word/styles.xml": false,
		"word/numbering.xml": false, "word/footer1.xml": false, "docProps/core.xml": false,
	}
	for _, file := range archive.File {
		if _, exists := required[file.Name]; exists {
			required[file.Name] = true
		}
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		document, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(document), "Дом стоял у самой воды.") {
			t.Fatalf("DOCX lost manuscript text: %s", document)
		}
	}
	for name, exists := range required {
		if !exists {
			t.Fatalf("%s is missing", name)
		}
	}
}

func TestRenderDOCXPreservesRichPublicationStructure(t *testing.T) {
	t.Parallel()

	const assetID = "11111111-1111-1111-1111-111111111111"
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "")
	snapshot.Media = []SnapshotMedia{{ID: assetID, MediaType: "image/png", Width: 1, Height: 1}}
	snapshot.Documents[0].Content = json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Подзаголовок"}]},
			{"type":"paragraph","content":[{"type":"text","text":"Жирная ссылка","marks":[{"type":"bold"},{"type":"link","attrs":{"href":"https://example.com/read"}}]}]},
			{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Пункт списка"}]}]}]},
			{"type":"callout","content":[{"type":"paragraph","content":[{"type":"text","text":"Важная заметка"}]}]},
			{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Колонка"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"Значение"}]}]}]}]},
			{"type":"imageBlock","attrs":{"assetId":"` + assetID + `","alt":"Иллюстрация","caption":"Подпись"}},
			{"type":"sceneBreak"},
			{"type":"codeBlock","content":[{"type":"text","text":"const x = 1"}]}
		]
	}`)
	data, _, err := RenderWithAssets(context.Background(), "docx", snapshot, map[string][]byte{assetID: png}, "")
	if err != nil {
		t.Fatal(err)
	}
	if outputPath := os.Getenv("WRITERS_TEST_RICH_DOCX_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	document := readZipFile(t, archive, "word/document.xml")
	for _, expected := range []string{
		`w:pStyle w:val="Heading2"`, `<w:b/>`, `w:numId w:val="2"`,
		`w:pStyle w:val="CalloutInfo"`, `<w:tbl>`, `r:embed="rIdImage1"`,
		`w:pStyle w:val="SceneBreak"`, `w:pStyle w:val="CodeBlock"`,
	} {
		if !strings.Contains(document, expected) {
			t.Fatalf("DOCX rich document is missing %q: %s", expected, document)
		}
	}
	relationships := readZipFile(t, archive, "word/_rels/document.xml.rels")
	if !strings.Contains(relationships, `Target="https://example.com/read" TargetMode="External"`) ||
		!strings.Contains(relationships, `Target="media/image-1.png"`) {
		t.Fatalf("DOCX rich relationships are incomplete: %s", relationships)
	}
	if image := readZipFileBytes(t, archive, "word/media/image-1.png"); !bytes.Equal(image, png) {
		t.Fatal("DOCX embedded image changed")
	}
}

func TestRenderEPUBProducesOrderedReflowablePublication(t *testing.T) {
	t.Parallel()
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.ExportID = "11111111-1111-1111-1111-111111111111"
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "Аннотация")
	snapshot.Documents = append(snapshot.Documents, SnapshotDocument{
		Title:     "Глава 2",
		PlainText: "Вторая глава.",
		Content:   json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Вторая глава."}]}]}`),
	})
	data, contentType, err := RenderWithAssets(context.Background(), "epub", snapshot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/epub+zip" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	repeated, _, err := RenderWithAssets(context.Background(), "epub", snapshot, nil, "")
	if err != nil || !bytes.Equal(data, repeated) {
		t.Fatalf("immutable EPUB snapshot is not repeatable: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) == 0 || archive.File[0].Name != "mimetype" || archive.File[0].Method != zip.Store {
		t.Fatal("EPUB mimetype must be the first uncompressed entry")
	}
	packageXML := readZipFile(t, archive, "OEBPS/package.opf")
	if strings.Index(packageXML, `idref="section-1"`) > strings.Index(packageXML, `idref="section-2"`) {
		t.Fatal("EPUB spine changed snapshot order")
	}
	if !strings.Contains(packageXML, `<itemref idref="nav"/>`) {
		t.Fatal("visible EPUB table of contents is missing from the spine")
	}
	for _, expected := range []string{"Оглавление", "Глава 1", "Глава 2"} {
		if !strings.Contains(readZipFile(t, archive, "OEBPS/nav.xhtml"), expected) {
			t.Fatalf("EPUB navigation lost %q", expected)
		}
	}
	if !strings.Contains(readZipFile(t, archive, "OEBPS/section-1.xhtml"), "Дом стоял у самой воды.") {
		t.Fatal("EPUB lost Cyrillic manuscript content")
	}
	if outputPath := os.Getenv("WRITERS_TEST_EPUB_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFrontMatterPagesStaySeparateWhenDocumentsAreContinuous(t *testing.T) {
	t.Parallel()

	const coverID = "33333333-3333-3333-3333-333333333333"
	cover, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.ExportID = "44444444-4444-4444-4444-444444444444"
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "")
	snapshot.Profile.ChapterPageBreak = false
	coverAssetID := coverID
	snapshot.Profile.Metadata.CoverAssetID = &coverAssetID
	snapshot.Media = []SnapshotMedia{{ID: coverID, MediaType: "image/png", Width: 1, Height: 1}}
	assets := map[string][]byte{coverID: cover}

	docx, _, err := RenderWithAssets(context.Background(), "docx", snapshot, assets, "")
	if err != nil {
		t.Fatal(err)
	}
	docxXML := string(readDOCXDocument(t, docx))
	if breaks := strings.Count(docxXML, `w:br w:type="page"`); breaks != 2 {
		t.Fatalf("DOCX front matter produced %d explicit page breaks, want title and TOC breaks after the cover section", breaks)
	}
	for _, expected := range []string{
		`<w:type w:val="nextPage"/>`,
		`<w:pgMar w:top="0" w:right="0" w:bottom="0" w:left="0"`,
		`<wp:positionH relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionH>`,
		`<wp:positionV relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionV>`,
	} {
		if !strings.Contains(docxXML, expected) {
			t.Fatalf("DOCX cover is missing full-page section markup %q", expected)
		}
	}

	epub, _, err := RenderWithAssets(context.Background(), "epub", snapshot, assets, "")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(epub), int64(len(epub)))
	if err != nil {
		t.Fatal(err)
	}
	packageXML := readZipFile(t, archive, "OEBPS/package.opf")
	spineOrder := []string{
		`<itemref idref="cover"/>`,
		`<itemref idref="title"/>`,
		`<itemref idref="nav"/>`,
		`<itemref idref="content"/>`,
	}
	previous := -1
	for _, item := range spineOrder {
		position := strings.Index(packageXML, item)
		if position <= previous {
			t.Fatalf("EPUB front matter is missing or out of order at %s: %s", item, packageXML)
		}
		previous = position
	}
	if !strings.Contains(readZipFile(t, archive, "OEBPS/cover.xhtml"), `class="cover"`) {
		t.Fatal("EPUB cover does not have its own document")
	}
	if strings.Contains(readZipFile(t, archive, "OEBPS/title.xhtml"), `class="cover"`) {
		t.Fatal("EPUB title page unexpectedly contains the cover")
	}
	if outputPath := os.Getenv("WRITERS_TEST_FRONT_MATTER_EPUB_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, epub, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	publication := buildPublication(snapshot, nil)
	printHTML := renderPrintHTML(snapshot, publication, func(string) string {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(cover)
	}, assets)
	for _, expected := range []string{
		`@page cover { size: A5; margin: 0; }`,
		`<body><section class="cover-page">`,
		`</section><main><section class="title-page">`,
		`.cover { position: absolute; inset: -.5mm; display: block;`,
		`width: calc(100% + 1mm); height: calc(100% + 1mm);`,
		`object-fit: cover;`,
	} {
		if !strings.Contains(printHTML, expected) {
			t.Fatalf("PDF cover is missing borderless markup %q", expected)
		}
	}
	coverPosition := strings.Index(printHTML, `alt="Обложка"`)
	titlePosition := strings.Index(printHTML, `<section class="title-page"><h1>`+snapshot.ProjectTitle)
	tocPosition := strings.Index(printHTML, `<nav class="toc">`)
	contentPosition := strings.Index(printHTML, "Дом стоял у самой воды.")
	if coverPosition < 0 || titlePosition <= coverPosition || tocPosition <= titlePosition || contentPosition <= tocPosition {
		t.Fatalf("PDF front matter is missing or out of order: %s", printHTML)
	}
}

func continuousDocumentsSnapshot() Snapshot {
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.ExportID = "22222222-2222-2222-2222-222222222222"
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "")
	snapshot.Profile.TableOfContents = false
	snapshot.Profile.ChapterTitles = false
	snapshot.Profile.ChapterPageBreak = false
	snapshot.Profile.Style.SceneSeparator = "dots"
	snapshot.Documents = append(snapshot.Documents, SnapshotDocument{
		Title:     "Сцена 2",
		PlainText: "Утро началось с дождя.",
		Content:   json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Утро началось с дождя."}]}]}`),
	})
	return snapshot
}

func TestContinuousDocumentsUseSceneSeparatorWithoutForcedPageBreak(t *testing.T) {
	t.Parallel()
	snapshot := continuousDocumentsSnapshot()

	docx, _, err := RenderWithAssets(context.Background(), "docx", snapshot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	docxXML := string(readDOCXDocument(t, docx))
	if strings.Count(docxXML, `w:br w:type="page"`) != 1 {
		t.Fatalf("DOCX must only break after the title page: %s", docxXML)
	}
	if !strings.Contains(docxXML, `w:pStyle w:val="SceneBreak"`) ||
		!strings.Contains(docxXML, "· · ·") {
		t.Fatalf("DOCX omitted the selected inter-document separator: %s", docxXML)
	}
	if outputPath := os.Getenv("WRITERS_TEST_CONTINUOUS_DOCX_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, docx, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	epub, _, err := RenderWithAssets(context.Background(), "epub", snapshot, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(epub), int64(len(epub)))
	if err != nil {
		t.Fatal(err)
	}
	content := readZipFile(t, archive, "OEBPS/content.xhtml")
	if !strings.Contains(content, "Дом стоял у самой воды.") ||
		!strings.Contains(content, "Утро началось с дождя.") ||
		!strings.Contains(content, `class="scene-break"`) {
		t.Fatalf("continuous EPUB content is incomplete: %s", content)
	}
	packageXML := readZipFile(t, archive, "OEBPS/package.opf")
	if !strings.Contains(packageXML, `idref="content"`) ||
		strings.Contains(packageXML, `idref="section-2"`) {
		t.Fatalf("continuous EPUB must use one spine item: %s", packageXML)
	}
	navigation := readZipFile(t, archive, "OEBPS/nav.xhtml")
	if !strings.Contains(navigation, `content.xhtml#section-2`) {
		t.Fatalf("continuous EPUB navigation lost the second scene anchor: %s", navigation)
	}
	if outputPath := os.Getenv("WRITERS_TEST_CONTINUOUS_EPUB_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, epub, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	publication := buildPublication(snapshot, nil)
	printHTML := renderPrintHTML(snapshot, publication, nil, nil)
	first := strings.Index(printHTML, "Дом стоял у самой воды.")
	separator := strings.Index(printHTML, `class="scene-break"`)
	second := strings.Index(printHTML, "Утро началось с дождя.")
	if first < 0 || separator <= first || second <= separator {
		t.Fatalf("PDF HTML omitted or misplaced the inter-document separator: %s", printHTML)
	}
}

func TestRenderContinuousPDFWithChromium(t *testing.T) {
	chromiumPath := os.Getenv("WRITERS_TEST_CHROMIUM")
	if chromiumPath == "" {
		t.Skip("WRITERS_TEST_CHROMIUM is not configured")
	}
	data, _, err := RenderWithAssets(
		context.Background(),
		"pdf",
		continuousDocumentsSnapshot(),
		nil,
		chromiumPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	info, err := api.PDFInfo(bytes.NewReader(data), "continuous-publication.pdf", nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.PageCount != 2 {
		t.Fatalf("title plus two short continuous scenes produced %d pages, want 2", info.PageCount)
	}
	if outputPath := os.Getenv("WRITERS_TEST_CONTINUOUS_PDF_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPDFReportsUnavailableRendererSeparately(t *testing.T) {
	t.Parallel()
	_, _, err := RenderWithAssets(context.Background(), "pdf", fixtureSnapshot(), nil, "/missing/chromium")
	if !errors.Is(err, ErrPDFRendererUnavailable) {
		t.Fatalf("error = %v, want ErrPDFRendererUnavailable", err)
	}
}

func TestRenderPDFWithChromium(t *testing.T) {
	chromiumPath := os.Getenv("WRITERS_TEST_CHROMIUM")
	if chromiumPath == "" {
		t.Skip("WRITERS_TEST_CHROMIUM is not configured")
	}
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "Короткая аннотация")
	snapshot.Profile.Metadata.Author = "Автор Кириллицей"
	data, contentType, err := RenderWithAssets(context.Background(), "pdf", snapshot, nil, chromiumPath)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/pdf" || len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("invalid PDF output (%s, %d bytes)", contentType, len(data))
	}
	info, err := api.PDFInfo(bytes.NewReader(data), "publication.pdf", nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != snapshot.ProjectTitle || info.Author != "Автор Кириллицей" ||
		info.Subject != "Короткая аннотация" || info.Creator != "Writers Studio" {
		t.Fatalf("PDF metadata is incomplete: %#v", info)
	}
	if !info.Tagged || !info.Outlines {
		t.Fatalf("PDF accessibility structure is incomplete: tagged=%t outlines=%t", info.Tagged, info.Outlines)
	}
	if outputPath := os.Getenv("WRITERS_TEST_PDF_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderBorderlessCoverArtifacts(t *testing.T) {
	chromiumPath := os.Getenv("WRITERS_TEST_CHROMIUM")
	coverPath := os.Getenv("WRITERS_TEST_COVER_INPUT")
	outputDirectory := os.Getenv("WRITERS_TEST_COVER_OUTPUT_DIR")
	if chromiumPath == "" || coverPath == "" || outputDirectory == "" {
		t.Skip("borderless cover visual QA is not configured")
	}
	cover, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	mediaType := map[string]string{"jpeg": "image/jpeg", "png": "image/png"}[format]
	if mediaType == "" {
		t.Fatalf("unsupported cover format %q", format)
	}

	const coverID = "55555555-5555-5555-5555-555555555555"
	snapshot := fixtureSnapshot()
	snapshot.SchemaVersion = 2
	snapshot.Profile = DefaultProfile(snapshot.ProjectTitle, "")
	snapshot.Profile.Style.PageSize = "a4"
	snapshot.Profile.Style.MarginMM = 25
	snapshot.Profile.TitlePage = false
	snapshot.Profile.TableOfContents = false
	snapshot.Profile.ChapterTitles = false
	snapshot.Profile.Metadata.CoverAssetID = pointerTo(coverID)
	snapshot.Media = []SnapshotMedia{{
		ID: coverID, MediaType: mediaType, Width: configuration.Width, Height: configuration.Height,
	}}
	assets := map[string][]byte{coverID: cover}

	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, exportFormat := range []string{"pdf", "docx", "epub"} {
		data, _, renderErr := RenderWithAssets(context.Background(), exportFormat, snapshot, assets, chromiumPath)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if writeErr := os.WriteFile(filepath.Join(outputDirectory, "borderless-cover."+exportFormat), data, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		if exportFormat == "pdf" {
			info, infoErr := api.PDFInfo(bytes.NewReader(data), "borderless-cover.pdf", nil, false, nil)
			if infoErr != nil {
				t.Fatal(infoErr)
			}
			if info.PageCount != 2 {
				t.Fatalf("cover plus manuscript produced %d PDF pages, want 2", info.PageCount)
			}
		}
	}
}

func pointerTo[T any](value T) *T {
	return &value
}

func TestRenderPreservesRussianTypographyAcrossPortableFormats(t *testing.T) {
	t.Parallel()

	const typography = "«Князь» — сказал\u00a0он."
	snapshot := Snapshot{
		ProjectTitle: "Типографика",
		Documents: []SnapshotDocument{{
			Title:     "Типографика",
			PlainText: typography,
			Content: json.RawMessage(`{
				"type":"doc",
				"attrs":{"schemaVersion":1},
				"content":[{"type":"paragraph","attrs":{"blockId":"unicode"},"content":[
					{"type":"text","text":"«Князь» — сказал он."}
				]}]
			}`),
		}},
	}
	for _, format := range []string{"txt", "markdown", "docx"} {
		format := format
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			data, _, err := Render(format, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if format == "docx" {
				data = readDOCXDocument(t, data)
			}
			if !bytes.Contains(data, []byte(typography)) {
				t.Fatalf("%s export changed Russian typography: %q", format, data)
			}
		})
	}
}

func TestCodeListingsPreserveWhitespaceLanguageAndFormattingAcrossExports(t *testing.T) {
	t.Parallel()
	code := "package main  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	content, err := json.Marshal(map[string]any{
		"type": "doc", "attrs": map[string]any{"schemaVersion": 2},
		"content": []any{map[string]any{
			"type": "codeBlock", "attrs": map[string]any{"language": "go"},
			"content": []any{map[string]any{"type": "text", "text": code}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fixtureSnapshot()
	snapshot.Documents[0].Content = content
	snapshot.Documents[0].PlainText = code
	snapshot.Profile = NormalizeProfile(snapshot.Profile, snapshot.ProjectTitle, "")

	publication := buildPublication(snapshot, nil)
	htmlDocument := renderPrintHTML(snapshot, publication, nil, nil)
	if !strings.Contains(htmlDocument, `class="code-block" data-language="go"`) ||
		!strings.Contains(htmlDocument, `class="code-listing"`) ||
		!strings.Contains(htmlDocument, `style="color:#`) ||
		extractHTMLCodeText(htmlDocument) != code {
		t.Fatalf("PDF publication HTML lost code listing: %q", htmlDocument)
	}

	docx, _, err := Render("docx", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	documentXML := string(readDOCXDocument(t, docx))
	for _, expected := range []string{
		`w:pStyle w:val="CodeLanguage"`, `>go</w:t>`,
		`w:pStyle w:val="CodeBlock"`, `w:color w:val="CF222E"`, "<w:br/>", "<w:tab/>",
	} {
		if !strings.Contains(documentXML, expected) {
			t.Fatalf("DOCX code listing missing %q: %s", expected, documentXML)
		}
	}
	if rendered := extractWordParagraphText(t, documentXML, "CodeBlock"); rendered != code {
		t.Fatalf("DOCX code listing changed whitespace: %q", rendered)
	}

	epub, _, err := Render("epub", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(epub), int64(len(epub)))
	if err != nil {
		t.Fatal(err)
	}
	var xhtml strings.Builder
	for _, file := range archive.File {
		if strings.HasSuffix(file.Name, ".xhtml") {
			xhtml.WriteString(readZipFile(t, archive, file.Name))
		}
	}
	if !strings.Contains(xhtml.String(), `class="code-block" data-language="go"`) ||
		!strings.Contains(xhtml.String(), `style="color:#`) ||
		extractHTMLCodeText(xhtml.String()) != code {
		t.Fatalf("EPUB lost code listing: %s", xhtml.String())
	}
}

func extractHTMLCodeText(document string) string {
	start := strings.Index(document, "<code>")
	if start < 0 {
		return ""
	}
	start += len("<code>")
	end := strings.Index(document[start:], "</code>")
	if end < 0 {
		return ""
	}
	return stdhtml.UnescapeString(stripSyntaxSpans(document[start : start+end]))
}

func extractWordParagraphText(t *testing.T, document, style string) string {
	t.Helper()
	marker := `w:pStyle w:val="` + style + `"`
	markerIndex := strings.Index(document, marker)
	if markerIndex < 0 {
		t.Fatalf("Word style %s is missing", style)
	}
	start := strings.LastIndex(document[:markerIndex], "<w:p>")
	end := strings.Index(document[markerIndex:], "</w:p>")
	if start < 0 || end < 0 {
		t.Fatalf("Word paragraph %s is malformed", style)
	}
	fragment := document[start : markerIndex+end+len("</w:p>")]
	decoder := xml.NewDecoder(strings.NewReader(`<root xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` + fragment + `</root>`))
	var output strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "t":
				inText = true
			case "br":
				output.WriteByte('\n')
			case "tab":
				output.WriteByte('\t')
			}
		case xml.EndElement:
			if value.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				output.Write(value)
			}
		}
	}
	return output.String()
}

func readDOCXDocument(t *testing.T, data []byte) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		document, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return document
	}
	t.Fatal("word/document.xml is missing")
	return nil
}

func readZipFile(t *testing.T, archive *zip.Reader, name string) string {
	t.Helper()
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatalf("%s is missing", name)
	return ""
}

func readZipFileBytes(t *testing.T, archive *zip.Reader, name string) []byte {
	t.Helper()
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("%s is missing", name)
	return nil
}
