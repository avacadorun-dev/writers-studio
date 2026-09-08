package export

import (
	"strings"
	"testing"

	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

func TestFormulaExportUsesSafeSourceFallback(t *testing.T) {
	t.Parallel()
	nodes := []editor.Node{
		{
			Type: "paragraph",
			Content: []editor.Node{
				{Type: "text", Text: "Энергия "},
				{Type: "inlineMath", Attrs: map[string]any{"latex": "E < mc^2", "description": "Энергия"}},
			},
		},
		{Type: "mathBlock", Attrs: map[string]any{"latex": `\\frac{a}{b} & c`, "description": "Дробь"}},
	}

	html := renderPublicationNodes(nodes, nil)
	for _, expected := range []string{"$E &lt; mc^2$", `$$\n\\frac{a}{b} &amp; c\n$$`, `aria-label="Дробь"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("publication formula fallback missing %q: %s", expected, html)
		}
	}
	if strings.Contains(html, "< mc") || strings.Contains(html, "& c") {
		t.Fatalf("publication formula source was not escaped: %s", html)
	}

	word := renderWordNodes(nodes, domain.ExportProfile{}, wordResources{})
	for _, expected := range []string{"[Формула: E &lt; mc^2]", `Формула: \\frac{a}{b} &amp; c`} {
		if !strings.Contains(word, expected) {
			t.Fatalf("DOCX formula fallback missing %q: %s", expected, word)
		}
	}
}
