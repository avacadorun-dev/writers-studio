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
	case "horizontalRule":
		output.WriteString("<hr />")
	case "sceneBreak":
		output.WriteString(publicationSceneBreakHTML())
	case "codeBlock":
		language, _ := node.Attrs["language"].(string)
		output.WriteString(`<pre class="code-block" data-language="` + html.EscapeString(language) + `"><code>`)
		children()
		output.WriteString("</code></pre>")
	case "callout":
		kind, _ := node.Attrs["kind"].(string)
		output.WriteString(`<aside class="callout callout-` + html.EscapeString(kind) + `">`)
		children()
		output.WriteString("</aside>")
	case "table":
		output.WriteString("<table><tbody>")
		children()
		output.WriteString("</tbody></table>")
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
		output.WriteString(`<figure class="mermaid-source"><pre><code>` + html.EscapeString(code) + `</code></pre>`)
		if caption != "" {
			output.WriteString("<figcaption>" + html.EscapeString(caption) + "</figcaption>")
		}
		output.WriteString("</figure>")
	default:
		children()
	}
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
:root { color-scheme: light; }
html, body { margin: 0; padding: 0; }
body { color: #1f211f; background: #fff; font-family: %q, Georgia, serif; font-size: %.1fpt; line-height: %.2f; text-align: %s; hyphens: auto; }
main { max-width: 44em; margin: 0 auto; }
p { margin: %s; text-indent: %s; orphans: 2; widows: 2; }
h1, h2, h3 { text-align: left; line-height: 1.2; page-break-after: avoid; break-after: avoid; }
h1 { margin: 0 0 2em; font-size: 1.65em; }
h2 { margin: 1.6em 0 .7em; font-size: 1.35em; }
.title-page { min-height: 75vh; display: flex; flex-direction: column; justify-content: center; text-align: center; page-break-after: always; break-after: page; }
.title-page h1 { margin-bottom: .5em; text-align: center; font-size: 2.3em; }
.title-page .subtitle { font-size: 1.25em; }
.title-page .author { margin-top: 3em; }
.cover-page { min-height: 92vh; display: flex; align-items: center; justify-content: center; page-break-after: always; break-after: page; }
.cover { display: block; max-width: 100%%; max-height: 92vh; margin: 0 auto; object-fit: contain; }
.toc { page-break-after: always; break-after: page; }
.toc a { color: inherit; text-decoration: none; }
.chapter { %s }
.scene-break { margin: 1.5em 0; text-align: center; text-indent: 0; }
.scene-break span::before { content: %q; letter-spacing: .25em; }
blockquote { margin: 1.2em 2em; padding-left: 1em; border-left: .18em solid #a7aaa4; }
pre, code { font-family: "IBM Plex Mono", ui-monospace, monospace; }
pre { white-space: pre-wrap; padding: 1em; background: #f3f2ee; }
.callout { margin: 1.2em 0; padding: .8em 1em; border-left: .25em solid #627a70; background: #f3f6f4; }
table { width: 100%%; margin: 1em 0; border-collapse: collapse; font-size: .9em; }
th, td { padding: .4em .55em; border: 1px solid #aaa; vertical-align: top; }
figure { margin: 1.5em 0; text-align: center; }
figure img { max-width: 100%%; max-height: 80vh; object-fit: contain; }
figcaption { margin-top: .45em; font-size: .85em; color: #5b5e59; }
a { color: #265d4f; }
	`, page, style.MarginMM, publicationFont(style.FontFamily), float64(style.FontSizePT), style.LineHeight, style.TextAlign, paragraphMargin, firstLine, chapterBreakCSS(snapshot.Profile.ChapterPageBreak), separator)
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
