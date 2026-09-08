package export

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"writers.studio/internal/editor"
)

type publication struct {
	Title      string
	Subtitle   string
	Author     string
	Annotation string
	Language   string
	Sections   []publicationSection
}

type publicationSection struct {
	ID    string
	Title string
	HTML  string
}

type imageResolver func(assetID string) string

func buildPublication(snapshot Snapshot, resolveImage imageResolver) publication {
	profile := snapshot.Profile
	result := publication{
		Title:      profile.Metadata.Title,
		Subtitle:   profile.Metadata.Subtitle,
		Author:     profile.Metadata.Author,
		Annotation: profile.Metadata.Annotation,
		Language:   snapshot.ProjectLanguage,
		Sections:   make([]publicationSection, 0, len(snapshot.Documents)),
	}
	if result.Language == "" {
		result.Language = "ru"
	}
	for index, document := range snapshot.Documents {
		var root editor.Node
		body := ""
		if json.Unmarshal(document.Content, &root) == nil {
			body = renderPublicationNodes(root.Content, resolveImage)
		}
		if strings.TrimSpace(body) == "" {
			body = "<p>" + html.EscapeString(document.PlainText) + "</p>"
		}
		result.Sections = append(result.Sections, publicationSection{
			ID:    fmt.Sprintf("section-%d", index+1),
			Title: document.Title,
			HTML:  body,
		})
	}
	return result
}

func renderPublicationNodes(nodes []editor.Node, resolveImage imageResolver) string {
	var output strings.Builder
	for _, node := range nodes {
		renderPublicationNode(&output, node, resolveImage)
	}
	return output.String()
}

func renderPublicationNode(output *strings.Builder, node editor.Node, resolveImage imageResolver) {
	children := func() { output.WriteString(renderPublicationNodes(node.Content, resolveImage)) }
	switch node.Type {
	case "text":
		value := html.EscapeString(node.Text)
		for _, mark := range node.Marks {
			switch mark.Type {
			case "bold":
				value = "<strong>" + value + "</strong>"
			case "italic":
				value = "<em>" + value + "</em>"
			case "underline":
				value = "<u>" + value + "</u>"
			case "strike":
				value = "<s>" + value + "</s>"
			case "code":
				value = "<code>" + value + "</code>"
			case "link":
				href, _ := mark.Attrs["href"].(string)
				if safePublicationURL(href) {
					value = `<a href="` + html.EscapeString(href) + `">` + value + `</a>`
				}
			}
		}
		output.WriteString(value)
	case "paragraph":
		output.WriteString("<p>")
		children()
		output.WriteString("</p>")
	case "heading":
		level := numericPublicationAttr(node.Attrs, "level", 2)
		if level < 1 || level > 3 {
			level = 2
		}
		fmt.Fprintf(output, "<h%d>", level)
		children()
		fmt.Fprintf(output, "</h%d>", level)
	case "blockquote":
		output.WriteString("<blockquote>")
		children()
		output.WriteString("</blockquote>")
	case "bulletList":
		output.WriteString("<ul>")
		children()
		output.WriteString("</ul>")
	case "orderedList":
		output.WriteString("<ol>")
		children()
		output.WriteString("</ol>")
	case "listItem":
		output.WriteString("<li>")
		children()
		output.WriteString("</li>")
	case "hardBreak":
		output.WriteString("<br />")
	case "inlineMath":
		latex, _ := node.Attrs["latex"].(string)
		description, _ := node.Attrs["description"].(string)
		label := description
		if strings.TrimSpace(label) == "" {
			label = "Формула: " + latex
		}
		output.WriteString(`<span class="inline-math-source" role="math" aria-label="` + html.EscapeString(label) + `">$` + html.EscapeString(latex) + `$</span>`)
	case "horizontalRule":
		output.WriteString("<hr />")
	case "sceneBreak":
		output.WriteString(publicationSceneBreakHTML())
	case "codeBlock":
		language, _ := node.Attrs["language"].(string)
		renderPublicationCodeListing(output, language, codeNodeText(node.Content), "")
	case "callout":
		kind := normalizedCalloutKind(node.Attrs["kind"])
		output.WriteString(`<aside class="callout callout-` + html.EscapeString(kind) + `">`)
		output.WriteString(`<div class="callout-label">` + html.EscapeString(calloutLabel(kind)) + `</div>`)
		children()
		output.WriteString("</aside>")
	case "table":
		renderPublicationTable(output, node, resolveImage)
	case "tableRow":
		output.WriteString("<tr>")
		children()
		output.WriteString("</tr>")
	case "tableHeader", "tableCell":
		tag := "td"
		if node.Type == "tableHeader" {
			tag = "th"
		}
		colspan := numericPublicationAttr(node.Attrs, "colspan", 1)
		rowspan := numericPublicationAttr(node.Attrs, "rowspan", 1)
		fmt.Fprintf(output, `<%s colspan="%d" rowspan="%d">`, tag, colspan, rowspan)
		children()
		fmt.Fprintf(output, "</%s>", tag)
	case "imageBlock":
		assetID, _ := node.Attrs["assetId"].(string)
		alt, _ := node.Attrs["alt"].(string)
		caption, _ := node.Attrs["caption"].(string)
		src := ""
		if resolveImage != nil {
			src = resolveImage(assetID)
		}
		if src != "" {
			output.WriteString(`<figure class="image-block"><img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `" />`)
			if caption != "" {
				output.WriteString("<figcaption>" + html.EscapeString(caption) + "</figcaption>")
			}
			output.WriteString("</figure>")
		}
	case "mermaidBlock":
		code, _ := node.Attrs["code"].(string)
		caption, _ := node.Attrs["caption"].(string)
		renderPublicationCodeListing(output, "mermaid", code, "mermaid-source")
		if caption != "" {
			output.WriteString(`<p class="diagram-caption">` + html.EscapeString(caption) + `</p>`)
		}
	case "mathBlock":
		latex, _ := node.Attrs["latex"].(string)
		description, _ := node.Attrs["description"].(string)
		label := description
		if strings.TrimSpace(label) == "" {
			label = "Формула: " + latex
		}
		output.WriteString(`<figure class="math-source" role="math" aria-label="` + html.EscapeString(label) + `"><pre>$$\n` + html.EscapeString(latex) + `\n$$</pre></figure>`)
	default:
		children()
	}
}

func renderPublicationCodeListing(output *strings.Builder, language, source, extraClass string) {
	class := "code-listing"
	if extraClass != "" {
		class += " " + extraClass
	}
	output.WriteString(`<figure class="` + html.EscapeString(class) + `"><figcaption class="code-language">`)
	output.WriteString(html.EscapeString(codeLanguageLabel(language)))
	output.WriteString(`</figcaption><pre class="code-block" data-language="`)
	output.WriteString(html.EscapeString(language))
	output.WriteString(`"><code>`)
	output.WriteString(highlightedCodeHTML(language, source))
	output.WriteString("</code></pre></figure>")
}

func calloutLabel(kind string) string {
	switch normalizedCalloutKind(kind) {
	case "idea":
		return "Идея"
	case "warning":
		return "Предупреждение"
	case "todo":
		return "TODO"
	default:
		return "Информация"
	}
}

func normalizedCalloutKind(value any) string {
	kind, _ := value.(string)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "idea":
		return "idea"
	case "warning":
		return "warning"
	case "todo":
		return "todo"
	default:
		return "info"
	}
}

func renderPublicationTable(output *strings.Builder, table editor.Node, resolveImage imageResolver) {
	rows := make([]editor.Node, 0, len(table.Content))
	for _, row := range table.Content {
		if row.Type == "tableRow" {
			rows = append(rows, row)
		}
	}
	output.WriteString("<table>")
	header := len(rows) > 0 && publicationHeaderRow(rows[0])
	if header {
		output.WriteString("<thead>")
		renderPublicationNode(output, rows[0], resolveImage)
		output.WriteString("</thead>")
		rows = rows[1:]
	}
	output.WriteString("<tbody>")
	for _, row := range rows {
		renderPublicationNode(output, row, resolveImage)
	}
	output.WriteString("</tbody></table>")
}

func publicationHeaderRow(row editor.Node) bool {
	seen := false
	for _, cell := range row.Content {
		if cell.Type != "tableHeader" && cell.Type != "tableCell" {
			continue
		}
		seen = true
		if cell.Type != "tableHeader" {
			return false
		}
	}
	return seen
}

func publicationCSS(snapshot Snapshot, assets map[string][]byte, fontTarget string) string {
	style := snapshot.Profile.Style
	page := strings.ToUpper(style.PageSize)
	if style.PageSize == "letter" {
		page = "Letter"
	}
	firstLine := fmt.Sprintf("%.2fem", style.FirstLineIndent)
	paragraphMargin := "0"
	if style.ParagraphMode == "spaced" {
		firstLine = "0"
		paragraphMargin = fmt.Sprintf("0 0 %.2fem", style.ParagraphSpace)
	}
	separator := sceneSeparatorText(style.SceneSeparator)
	return embeddedFontCSS(snapshot.Profile.Style.FontFamily, assets, fontTarget) + fmt.Sprintf(`
@page { size: %s; margin: %dmm; }
@page cover { size: %s; margin: 0; }
:root { color-scheme: light; }
html, body { margin: 0; padding: 0; }
body { color: #1f211f; background: #fff; font-family: %q, Georgia, serif; font-size: %.1fpt; line-height: %.2f; text-align: %s; hyphens: auto; }
main { position: relative; z-index: 1; max-width: 44em; margin: 0 auto; background: #fff; }
.cover-page + main::before { content: ""; position: absolute; z-index: -1; top: -1mm; left: 50%%; width: 100vw; height: 2mm; transform: translateX(-50%%); background: #fff; }
p { margin: %s; text-indent: %s; orphans: 2; widows: 2; }
h1, h2, h3 { text-align: left; line-height: 1.2; page-break-after: avoid; break-after: avoid; }
h1 { margin: 0 0 2em; font-size: 1.65em; }
h2 { margin: 1.6em 0 .7em; font-size: 1.35em; }
.title-page { min-height: 75vh; display: flex; flex-direction: column; justify-content: center; text-align: center; page-break-after: always; break-after: page; }
.title-page h1 { margin-bottom: .5em; text-align: center; font-size: 2.3em; }
.title-page .subtitle { font-size: 1.25em; }
.title-page .author { margin-top: 3em; }
.cover-page { page: cover; position: relative; width: 100%%; height: 100vh; margin: 0; padding: 0; page-break-after: always; break-after: page; }
.cover { position: absolute; inset: -.5mm; display: block; width: calc(100%% + 1mm); height: calc(100%% + 1mm); margin: 0; object-fit: cover; }
.toc { page-break-after: always; break-after: page; }
.toc a { color: inherit; text-decoration: none; }
.chapter { %s }
.scene-break { margin: 1.5em 0; text-align: center; text-indent: 0; }
.scene-break span::before { content: %q; letter-spacing: .25em; }
blockquote { margin: 1.2em 2em; padding-left: 1em; border-left: .18em solid #a7aaa4; }
pre, code { font-family: "IBM Plex Mono", ui-monospace, monospace; }
`+inlineCodePublicationCSS()+`
pre { white-space: pre-wrap; }
figure.code-listing { margin: 1.15em 0; border: 1px solid #d8d6cf; border-radius: .38em; overflow: hidden; background: #f6f5f2; text-align: left; break-inside: auto; page-break-inside: auto; }
.code-language { margin: 0; padding: .42em 1em; border-bottom: 1px solid #d8d6cf; background: #e9e8e3; color: #545852; font-family: "IBM Plex Mono", ui-monospace, monospace; font-size: 7.5pt; font-weight: 600; letter-spacing: .075em; line-height: 1.25; text-transform: uppercase; break-after: avoid; page-break-after: avoid; }
pre.code-block { margin: 0; padding: .82em 1em .9em; border: 0; border-radius: 0; background: #f6f5f2; color: #24292f; font-size: 8.5pt; line-height: 1.42; tab-size: 4; overflow-wrap: anywhere; word-break: normal; orphans: 3; widows: 3; break-inside: auto; page-break-inside: auto; }
pre.code-block code { font: inherit; white-space: inherit; }
.diagram-caption { margin: -.7em 0 1.1em; text-align: center; text-indent: 0; font-size: .82em; font-style: italic; color: #5b5e59; }
.callout { margin: 1.2em 0; padding: .72em .95em .78em; border: 1px solid #c8d5cf; border-left: .28em solid #497568; border-radius: .28em; background: #f2f7f5; break-inside: avoid; page-break-inside: avoid; }
.callout-label { margin: 0 0 .38em; color: #315d51; font-family: "IBM Plex Mono", ui-monospace, monospace; font-size: .72em; font-weight: 600; letter-spacing: .055em; line-height: 1.2; text-transform: uppercase; }
.callout p { margin: 0; text-indent: 0; }
.callout-idea { border-color: #d9cda4; border-left-color: #9a7720; background: #fbf8eb; }
.callout-idea .callout-label { color: #765b13; }
.callout-warning { border-color: #e4bbb6; border-left-color: #b74c42; background: #fdf3f1; }
.callout-warning .callout-label { color: #913b33; }
.callout-todo { border-color: #c9c2df; border-left-color: #735da8; background: #f7f4fc; }
.callout-todo .callout-label { color: #5b458e; }
table { width: 100%%; margin: 1.1em 0; border: 1px solid #9fa49e; border-collapse: collapse; border-spacing: 0; font-size: .84em; line-height: 1.35; table-layout: auto; break-inside: auto; page-break-inside: auto; }
thead { display: table-header-group; }
tr { break-inside: avoid; page-break-inside: avoid; }
th, td { padding: .48em .62em; border: 1px solid #b7bbb5; vertical-align: top; overflow-wrap: anywhere; }
th { background: #e8ece9; color: #2f3833; font-weight: 600; text-align: left; }
tbody tr:nth-child(even) td { background: #f7f8f6; }
th p, td p { margin: 0; text-indent: 0; orphans: 2; widows: 2; }
figure { margin: 1.5em 0; text-align: center; }
figure img { max-width: 100%%; max-height: 80vh; object-fit: contain; }
figcaption { margin-top: .45em; font-size: .85em; color: #5b5e59; }
a { color: #265d4f; }
	`, page, style.MarginMM, page, publicationFont(style.FontFamily), float64(style.FontSizePT), style.LineHeight, style.TextAlign, paragraphMargin, firstLine, chapterBreakCSS(snapshot.Profile.ChapterPageBreak), separator)
}

func inlineCodePublicationCSS() string {
	return `p code, li code, blockquote code, td code, th code, .callout code { padding: .08em .3em; border-radius: .22em; background: #efeee9; color: #263c36; font-size: .88em; white-space: pre-wrap; word-spacing: normal; letter-spacing: normal; hyphens: none; }`
}

func embeddedFontCSS(fontFamily string, assets map[string][]byte, target string) string {
	var result strings.Builder
	for _, filename := range exportFontFilenames(fontFamily) {
		data := assets[fontAssetKey(filename)]
		if len(data) == 0 {
			continue
		}
		source := "fonts/" + filename
		if target == "data" {
			source = "data:font/woff2;base64," + base64.StdEncoding.EncodeToString(data)
		}
		fmt.Fprintf(&result, `@font-face { font-family: %q; font-style: normal; font-weight: 400; font-display: swap; src: url(%q) format("woff2"); }`, publicationFont(fontFamily), source)
	}
	return result.String()
}

func exportFontFilenames(fontFamily string) []string {
	prefix := map[string]string{
		"literata":      "literata",
		"source-serif":  "source-serif-4",
		"inter":         "inter",
		"ibm-plex-mono": "ibm-plex-mono",
	}[fontFamily]
	if prefix == "" {
		prefix = "literata"
	}
	return []string{
		prefix + "-cyrillic-400-normal.woff2",
		prefix + "-latin-400-normal.woff2",
	}
}

func fontAssetKey(filename string) string { return "__export_font__" + filename }

func publicationFont(value string) string {
	switch value {
	case "source-serif":
		return "Source Serif 4"
	case "inter":
		return "Inter"
	case "ibm-plex-mono":
		return "IBM Plex Mono"
	default:
		return "Literata"
	}
}

func chapterBreakCSS(enabled bool) string {
	if enabled {
		return "page-break-before: always; break-before: page;"
	}
	return ""
}

func publicationSceneBreakHTML() string {
	return `<div class="scene-break" role="separator"><span></span></div>`
}

func sceneSeparatorText(value string) string {
	switch value {
	case "dots":
		return "· · ·"
	case "line":
		return "—"
	default:
		return "* * *"
	}
}

func dataImageResolver(snapshot Snapshot, assets map[string][]byte) imageResolver {
	media := make(map[string]SnapshotMedia, len(snapshot.Media))
	for _, item := range snapshot.Media {
		media[item.ID] = item
	}
	return func(assetID string) string {
		item, exists := media[assetID]
		data := assets[assetID]
		if !exists || len(data) == 0 || !strings.HasPrefix(item.MediaType, "image/") {
			return ""
		}
		return "data:" + item.MediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
}

func mediaExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func safePublicationURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "" || parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "mailto"
}

func numericPublicationAttr(attrs map[string]any, key string, fallback int) int {
	value, exists := attrs[key]
	if !exists {
		return fallback
	}
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func safePublicationFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(character rune) rune {
		if strings.ContainsRune(`<>:"/\\|?*`, character) || character < 32 {
			return '-'
		}
		return character
	}, value)
	value = strings.Trim(value, " .-")
	if value == "" {
		return "publication"
	}
	return value
}

// SafeFilename returns a filesystem-safe Unicode publication basename.
func SafeFilename(value string) string { return safePublicationFilename(value) }
