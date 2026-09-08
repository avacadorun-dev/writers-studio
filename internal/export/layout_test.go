package export

import (
	"strings"
	"testing"

	"writers.studio/internal/editor"
)

func TestPublicationTablesUseSemanticHeadersAndCompactPaginationCSS(t *testing.T) {
	t.Parallel()
	table := publicationLayoutTable()
	output := renderPublicationNodes([]editor.Node{table}, nil)
	for _, expected := range []string{
		"<table><thead><tr>", "</thead><tbody><tr>", "<th", "<td", "</tbody></table>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("publication table is missing %q: %s", expected, output)
		}
	}

	mixed := table
	mixed.Content[1].Content[1].Type = "tableCell"
	if output := renderPublicationNodes([]editor.Node{mixed}, nil); strings.Contains(output, "<thead>") {
		t.Fatalf("mixed first row was incorrectly promoted to a header: %s", output)
	}
	empty := renderPublicationNodes([]editor.Node{{Type: "table"}}, nil)
	if empty != "<table><tbody></tbody></table>" {
		t.Fatalf("empty table changed: %s", empty)
	}
	if publicationHeaderRow(editor.Node{Type: "tableRow", Content: []editor.Node{{Type: "ignored"}}}) {
		t.Fatal("a row without cells was promoted to a header")
	}

	snapshot := fixtureSnapshot()
	snapshot.Profile = NormalizeProfile(snapshot.Profile, snapshot.ProjectTitle, "")
	css := publicationCSS(snapshot, nil, "epub")
	for _, expected := range []string{
		"thead { display: table-header-group; }",
		"tr { break-inside: avoid;",
		"pre.code-block {",
		"p code, li code, blockquote code, td code, th code, .callout code",
		"font-size: 8.5pt",
		"page-break-inside: auto",
		"th p, td p { margin: 0; text-indent: 0;",
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("publication CSS is missing %q", expected)
		}
	}
}

func TestWordTablesHaveStableGridMarginsRepeatingHeaderAndCompactStyles(t *testing.T) {
	t.Parallel()
	profile := NormalizeProfile(fixtureSnapshot().Profile, "Layout", "")
	output := renderWordNodes([]editor.Node{publicationLayoutTable()}, profile, wordResources{})
	for _, expected := range []string{
		`w:tblLayout w:type="fixed"`,
		`<w:tblCellMar>`,
		`<w:tblHeader/>`,
		`<w:cantSplit/>`,
		`w:pStyle w:val="TableHeader"`,
		`w:pStyle w:val="TableBody"`,
		`w:gridSpan w:val="2"`,
		`w:shd w:fill="F7F8F6"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("Word table is missing %q: %s", expected, output)
		}
	}
	if count := strings.Count(output, "<w:gridCol"); count != 3 {
		t.Fatalf("Word grid has %d columns: %s", count, output)
	}

	renderer := wordNodeRenderer{profile: profile}
	widths := renderer.tableColumnWidths(publicationLayoutTable())
	pageWidth, _ := wordPageSize(profile.Style.PageSize)
	wantTotal := pageWidth - 2*profile.Style.MarginMM*57
	total := 0
	for _, width := range widths {
		total += width
	}
	if len(widths) != 3 || total != wantTotal || widths[0] <= widths[1] {
		t.Fatalf("unexpected weighted grid: widths=%v total=%d want=%d", widths, total, wantTotal)
	}
	profile.Style.MarginMM = 100
	minimum := (wordNodeRenderer{profile: profile}).tableColumnWidths(publicationLayoutTable())
	if minimum[0]+minimum[1]+minimum[2] != 3*720 {
		t.Fatalf("narrow page did not retain minimum columns: %v", minimum)
	}
	if widths := renderer.tableColumnWidths(editor.Node{Type: "table"}); widths != nil {
		t.Fatalf("empty table received a grid: %v", widths)
	}

	styles := renderWordStyles(profile)
	for _, expected := range []string{`w:styleId="CodeBlock"`, `w:sz w:val="17"`, `w:jc w:val="left"`, `w:styleId="TableBody"`} {
		if !strings.Contains(styles, expected) {
			t.Fatalf("Word styles are missing %q", expected)
		}
	}
	properties := wordSyntaxProperties(syntaxToken{
		Colour: "#aabbcc", Bold: true, Italic: true, Underline: true,
	})
	for _, expected := range []string{`w:val="AABBCC"`, `<w:b/>`, `<w:i/>`, `<w:u w:val="single"/>`} {
		if !strings.Contains(properties, expected) {
			t.Fatalf("Word syntax properties are missing %q: %s", expected, properties)
		}
	}
}

func TestWordCoverCropFillsPageWithoutStretching(t *testing.T) {
	t.Parallel()

	pageWidth, pageHeight := wordPageSize("a4")
	if left, top, right, bottom := wordCoverCrop(2480, 3508, pageWidth, pageHeight); left != 0 || right != 0 || top != bottom || top > 20 {
		t.Fatalf("A4 cover was unexpectedly cropped: %d %d %d %d", left, top, right, bottom)
	}
	left, top, right, bottom := wordCoverCrop(1600, 900, pageWidth, pageHeight)
	if left <= 0 || left != right || top != 0 || bottom != 0 {
		t.Fatalf("wide cover crop is not centered: %d %d %d %d", left, top, right, bottom)
	}
	left, top, right, bottom = wordCoverCrop(900, 1600, pageWidth, pageHeight)
	if top <= 0 || top != bottom || left != 0 || right != 0 {
		t.Fatalf("tall cover crop is not centered: %d %d %d %d", left, top, right, bottom)
	}
	if left, top, right, bottom := wordCoverCrop(0, 0, pageWidth, pageHeight); left != 0 || top != 0 || right != 0 || bottom != 0 {
		t.Fatal("unknown cover dimensions must not invent a crop")
	}
}

func TestCalloutVariantsStaySemanticInHTMLAndWord(t *testing.T) {
	t.Parallel()
	profile := NormalizeProfile(fixtureSnapshot().Profile, "Callouts", "")
	for _, test := range []struct {
		kind, normalized, label, wordStyle string
	}{
		{kind: "info", normalized: "info", label: "Информация", wordStyle: "CalloutInfo"},
		{kind: "idea", normalized: "idea", label: "Идея", wordStyle: "CalloutIdea"},
		{kind: "warning", normalized: "warning", label: "Предупреждение", wordStyle: "CalloutWarning"},
		{kind: "todo", normalized: "todo", label: "TODO", wordStyle: "CalloutTodo"},
		{kind: " legacy ", normalized: "info", label: "Информация", wordStyle: "CalloutInfo"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			node := editor.Node{
				Type: "callout", Attrs: map[string]any{"kind": test.kind},
				Content: []editor.Node{{Type: "paragraph", Content: []editor.Node{{Type: "text", Text: "Текст"}}}},
			}
			html := renderPublicationNodes([]editor.Node{node}, nil)
			if !strings.Contains(html, `class="callout callout-`+test.normalized+`"`) ||
				!strings.Contains(html, `class="callout-label">`+test.label) {
				t.Fatalf("HTML callout changed: %s", html)
			}
			word := renderWordNodes([]editor.Node{node}, profile, wordResources{})
			if !strings.Contains(word, `w:pStyle w:val="`+test.wordStyle+`Label"`) ||
				!strings.Contains(word, `w:pStyle w:val="`+test.wordStyle+`"`) ||
				!strings.Contains(word, test.label) {
				t.Fatalf("Word callout changed: %s", word)
			}
		})
	}
}

func publicationLayoutTable() editor.Node {
	paragraph := func(text string) editor.Node {
		return editor.Node{Type: "paragraph", Content: []editor.Node{{Type: "text", Text: text}}}
	}
	long := strings.Repeat("длинное значение ", 4)
	return editor.Node{Type: "table", Content: []editor.Node{
		{Type: "ignored"},
		{Type: "tableRow", Content: []editor.Node{
			{Type: "tableHeader", Content: []editor.Node{paragraph("Очень длинный заголовок колонки")}},
			{Type: "tableHeader", Content: []editor.Node{paragraph("Тип")}},
			{Type: "tableHeader", Content: []editor.Node{paragraph("Итог")}},
		}},
		{Type: "tableRow", Content: []editor.Node{
			{Type: "tableCell", Content: []editor.Node{paragraph(long)}},
			{Type: "tableCell", Attrs: map[string]any{"colspan": 2.0}, Content: []editor.Node{paragraph("Объединено")}},
		}},
		{Type: "tableRow", Content: []editor.Node{
			{Type: "tableCell", Attrs: map[string]any{"colspan": 0.0}},
			{Type: "tableCell", Content: []editor.Node{{Type: "heading", Attrs: map[string]any{"level": 3.0}, Content: []editor.Node{{Type: "text", Text: "Вложено"}}}}},
			{Type: "tableCell", Content: []editor.Node{paragraph("Да")}},
		}},
	}}
}
