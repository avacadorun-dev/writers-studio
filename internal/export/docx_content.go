package export

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

type wordLinkResource struct {
	href           string
	relationshipID string
}

type wordImageResource struct {
	assetID        string
	filename       string
	relationshipID string
	media          SnapshotMedia
	objectID       int
}

type wordResources struct {
	links      []wordLinkResource
	images     []wordImageResource
	linkByHref map[string]wordLinkResource
	imageByID  map[string]wordImageResource
}

func collectWordResources(snapshot Snapshot, assets map[string][]byte) wordResources {
	result := wordResources{
		linkByHref: make(map[string]wordLinkResource),
		imageByID:  make(map[string]wordImageResource),
	}
	mediaByID := make(map[string]SnapshotMedia, len(snapshot.Media))
	for _, item := range snapshot.Media {
		mediaByID[item.ID] = item
	}
	var visit func(editor.Node)
	visit = func(node editor.Node) {
		for _, mark := range node.Marks {
			if mark.Type != "link" {
				continue
			}
			href, _ := mark.Attrs["href"].(string)
			href = strings.TrimSpace(href)
			if !safePublicationURL(href) || href == "" {
				continue
			}
			if _, exists := result.linkByHref[href]; !exists {
				resource := wordLinkResource{
					href:           href,
					relationshipID: fmt.Sprintf("rIdLink%d", len(result.links)+1),
				}
				result.links = append(result.links, resource)
				result.linkByHref[href] = resource
			}
		}
		if node.Type == "imageBlock" {
			assetID, _ := node.Attrs["assetId"].(string)
			media, exists := mediaByID[assetID]
			if exists && len(assets[assetID]) > 0 && strings.HasPrefix(media.MediaType, "image/") {
				if _, added := result.imageByID[assetID]; !added {
					index := len(result.images) + 1
					resource := wordImageResource{
						assetID:        assetID,
						filename:       fmt.Sprintf("image-%d%s", index, mediaExtension(media.MediaType)),
						relationshipID: fmt.Sprintf("rIdImage%d", index),
						media:          media,
						objectID:       index + 1,
					}
					result.images = append(result.images, resource)
					result.imageByID[assetID] = resource
				}
			}
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	for _, document := range snapshot.Documents {
		var root editor.Node
		if json.Unmarshal(document.Content, &root) != nil {
			continue
		}
		visit(root)
	}
	return result
}

type wordNodeRenderer struct {
	profile   domain.ExportProfile
	resources wordResources
}

func renderWordNodes(nodes []editor.Node, profile domain.ExportProfile, resources wordResources) string {
	renderer := wordNodeRenderer{profile: profile, resources: resources}
	var result strings.Builder
	renderer.renderBlocks(&result, nodes, 0, 0)
	return result.String()
}

// renderBlocks turns the same editor tree used by the HTML publication into
// semantic Word paragraphs, lists, tables, callouts, links and drawings.
func (renderer wordNodeRenderer) renderBlocks(output *strings.Builder, nodes []editor.Node, listNumberID, listLevel int) {
	for _, node := range nodes {
		switch node.Type {
		case "paragraph":
			renderer.renderParagraph(output, node.Content, "", listNumberID, listLevel)
		case "heading":
			level := numericPublicationAttr(node.Attrs, "level", 2)
			if level < 1 || level > 3 {
				level = 2
			}
			renderer.renderParagraph(output, node.Content, fmt.Sprintf("Heading%d", level), 0, 0)
		case "blockquote":
			renderer.renderStyledChildren(output, node.Content, "Quote")
		case "bulletList":
			renderer.renderList(output, node.Content, 2, listLevel)
		case "orderedList":
			renderer.renderList(output, node.Content, 1, listLevel)
		case "listItem":
			renderer.renderListItem(output, node, listNumberID, listLevel)
		case "horizontalRule":
			output.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="A7AAA4"/></w:pBdr></w:pPr></w:p>`)
		case "sceneBreak":
			renderer.renderParagraph(output, []editor.Node{{Type: "text", Text: renderer.sceneSeparator()}}, "SceneBreak", 0, 0)
		case "codeBlock":
			renderer.renderCodeBlock(output, node)
		case "callout":
			renderer.renderCallout(output, node)
		case "table":
			renderer.renderTable(output, node)
		case "imageBlock":
			renderer.renderImage(output, node)
		case "mermaidBlock":
			code, _ := node.Attrs["code"].(string)
			caption, _ := node.Attrs["caption"].(string)
			renderer.renderCodeBlock(output, editor.Node{
				Type: "codeBlock", Attrs: map[string]any{"language": "mermaid"},
				Content: []editor.Node{{Type: "text", Text: code}},
			})
			if caption != "" {
				renderer.renderParagraph(output, []editor.Node{{Type: "text", Text: caption}}, "Caption", 0, 0)
			}
		case "mathBlock":
			latex, _ := node.Attrs["latex"].(string)
			renderer.renderParagraph(
				output,
				[]editor.Node{{Type: "text", Text: "Формула: " + latex}},
				"Quote",
				0,
				0,
			)
		case "text", "hardBreak":
			renderer.renderParagraph(output, []editor.Node{node}, "", listNumberID, listLevel)
		default:
			renderer.renderBlocks(output, node.Content, listNumberID, listLevel)
		}
	}
}

func (renderer wordNodeRenderer) renderCallout(output *strings.Builder, node editor.Node) {
	kind := normalizedCalloutKind(node.Attrs["kind"])
	style := map[string]string{
		"idea": "CalloutIdea", "warning": "CalloutWarning", "todo": "CalloutTodo",
	}[kind]
	if style == "" {
		style = "CalloutInfo"
	}
	renderer.renderParagraph(
		output, []editor.Node{{Type: "text", Text: calloutLabel(kind)}}, style+"Label", 0, 0,
	)
	renderer.renderStyledChildren(output, node.Content, style)
}

func (renderer wordNodeRenderer) renderCodeBlock(output *strings.Builder, node editor.Node) {
	language, _ := node.Attrs["language"].(string)
	renderer.renderParagraph(
		output,
		[]editor.Node{{Type: "text", Text: codeLanguageLabel(language)}},
		"CodeLanguage",
		0,
		0,
	)
	output.WriteString(`<w:p><w:pPr><w:pStyle w:val="CodeBlock"/></w:pPr>`)
	for _, token := range highlightCode(language, codeNodeText(node.Content)) {
		writeWordTextRun(output, token.Text, wordSyntaxProperties(token))
	}
	output.WriteString(`</w:p>`)
}

func wordSyntaxProperties(token syntaxToken) string {
	properties := `<w:rFonts w:ascii="IBM Plex Mono" w:hAnsi="IBM Plex Mono"/>`
	if token.Colour != "" {
		properties += `<w:color w:val="` + strings.ToUpper(strings.TrimPrefix(token.Colour, "#")) + `"/>`
	}
	if token.Bold {
		properties += `<w:b/>`
	}
	if token.Italic {
		properties += `<w:i/>`
	}
	if token.Underline {
		properties += `<w:u w:val="single"/>`
	}
	return properties
}

func (renderer wordNodeRenderer) renderList(output *strings.Builder, items []editor.Node, numberID, level int) {
	for _, item := range items {
		if item.Type == "listItem" {
			renderer.renderListItem(output, item, numberID, level)
		}
	}
}

func (renderer wordNodeRenderer) renderListItem(output *strings.Builder, item editor.Node, numberID, level int) {
	wroteParagraph := false
	for _, child := range item.Content {
		switch child.Type {
		case "paragraph":
			currentNumberID := 0
			if !wroteParagraph {
				currentNumberID = numberID
			}
			renderer.renderParagraph(output, child.Content, "", currentNumberID, level)
			wroteParagraph = true
		case "bulletList":
			renderer.renderList(output, child.Content, 2, minWordListLevel(level+1))
		case "orderedList":
			renderer.renderList(output, child.Content, 1, minWordListLevel(level+1))
		default:
			if !wroteParagraph {
				renderer.renderParagraph(output, child.Content, "", numberID, level)
				wroteParagraph = true
			} else {
				renderer.renderBlocks(output, []editor.Node{child}, 0, level)
			}
		}
	}
	if !wroteParagraph {
		renderer.renderParagraph(output, nil, "", numberID, level)
	}
}

func minWordListLevel(level int) int {
	if level > 8 {
		return 8
	}
	return level
}

func (renderer wordNodeRenderer) renderStyledChildren(output *strings.Builder, children []editor.Node, style string) {
	for _, child := range children {
		if child.Type == "paragraph" {
			renderer.renderParagraph(output, child.Content, style, 0, 0)
			continue
		}
		if child.Type == "bulletList" {
			renderer.renderList(output, child.Content, 2, 0)
			continue
		}
		if child.Type == "orderedList" {
			renderer.renderList(output, child.Content, 1, 0)
			continue
		}
		renderer.renderBlocks(output, []editor.Node{child}, 0, 0)
	}
}

func (renderer wordNodeRenderer) renderParagraph(output *strings.Builder, inline []editor.Node, style string, numberID, level int) {
	output.WriteString("<w:p>")
	if style != "" || numberID != 0 {
		output.WriteString("<w:pPr>")
		if style != "" {
			output.WriteString(`<w:pStyle w:val="` + style + `"/>`)
		}
		if numberID != 0 {
			fmt.Fprintf(output, `<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, level, numberID)
		}
		output.WriteString("</w:pPr>")
	}
	for _, child := range inline {
		renderer.renderInline(output, child)
	}
	output.WriteString("</w:p>")
}

func (renderer wordNodeRenderer) renderInline(output *strings.Builder, node editor.Node) {
	switch node.Type {
	case "text":
		properties := ""
		linkID := ""
		for _, mark := range node.Marks {
			switch mark.Type {
			case "bold":
				properties += "<w:b/>"
			case "italic":
				properties += "<w:i/>"
			case "underline":
				properties += `<w:u w:val="single"/>`
			case "strike":
				properties += "<w:strike/>"
			case "code":
				properties += `<w:rFonts w:ascii="IBM Plex Mono" w:hAnsi="IBM Plex Mono"/><w:shd w:fill="F3F2EE"/>`
			case "link":
				href, _ := mark.Attrs["href"].(string)
				if resource, exists := renderer.resources.linkByHref[strings.TrimSpace(href)]; exists {
					linkID = resource.relationshipID
					properties += `<w:rStyle w:val="Hyperlink"/>`
				}
			}
		}
		if linkID != "" {
			output.WriteString(`<w:hyperlink r:id="` + linkID + `" w:history="1">`)
		}
		writeWordTextRun(output, node.Text, properties)
		if linkID != "" {
			output.WriteString("</w:hyperlink>")
		}
	case "hardBreak":
		output.WriteString("<w:r><w:br/></w:r>")
	case "inlineMath":
		latex, _ := node.Attrs["latex"].(string)
		writeWordTextRun(
			output,
			"[Формула: "+latex+"]",
			`<w:rFonts w:ascii="IBM Plex Mono" w:hAnsi="IBM Plex Mono"/>`,
		)
	default:
		for _, child := range node.Content {
			renderer.renderInline(output, child)
		}
	}
}

func writeWordTextRun(output *strings.Builder, text, properties string) {
	output.WriteString("<w:r>")
	if properties != "" {
		output.WriteString("<w:rPr>" + properties + "</w:rPr>")
	}
	for lineIndex, line := range strings.Split(text, "\n") {
		if lineIndex > 0 {
			output.WriteString("<w:br/>")
		}
		for tabIndex, part := range strings.Split(line, "\t") {
			if tabIndex > 0 {
				output.WriteString("<w:tab/>")
			}
			if part == "" {
				continue
			}
			output.WriteString(`<w:t xml:space="preserve">`)
			_ = xml.EscapeText(output, []byte(part))
			output.WriteString("</w:t>")
		}
	}
	output.WriteString("</w:r>")
}

func (renderer wordNodeRenderer) renderTable(output *strings.Builder, table editor.Node) {
	widths := renderer.tableColumnWidths(table)
	tableWidth := 0
	for _, width := range widths {
		tableWidth += width
	}
	fmt.Fprintf(output, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblCellMar><w:top w:w="90" w:type="dxa"/><w:left w:w="110" w:type="dxa"/><w:bottom w:w="90" w:type="dxa"/><w:right w:w="110" w:type="dxa"/></w:tblCellMar><w:tblBorders><w:top w:val="single" w:sz="5" w:color="9FA49E"/><w:left w:val="single" w:sz="5" w:color="9FA49E"/><w:bottom w:val="single" w:sz="5" w:color="9FA49E"/><w:right w:val="single" w:sz="5" w:color="9FA49E"/><w:insideH w:val="single" w:sz="4" w:color="B7BBB5"/><w:insideV w:val="single" w:sz="4" w:color="B7BBB5"/></w:tblBorders></w:tblPr><w:tblGrid>`, tableWidth)
	for _, width := range widths {
		fmt.Fprintf(output, `<w:gridCol w:w="%d"/>`, width)
	}
	output.WriteString(`</w:tblGrid>`)
	rowIndex := 0
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		headerRow := rowIndex == 0 && publicationHeaderRow(row)
		output.WriteString("<w:tr><w:trPr><w:cantSplit/>")
		if headerRow {
			output.WriteString(`<w:tblHeader/>`)
		}
		output.WriteString("</w:trPr>")
		column := 0
		for _, cell := range row.Content {
			if cell.Type != "tableCell" && cell.Type != "tableHeader" {
				continue
			}
			colspan := numericPublicationAttr(cell.Attrs, "colspan", 1)
			if colspan < 1 {
				colspan = 1
			}
			cellWidth := 0
			for index := column; index < column+colspan && index < len(widths); index++ {
				cellWidth += widths[index]
			}
			output.WriteString("<w:tc><w:tcPr>")
			fmt.Fprintf(output, `<w:tcW w:w="%d" w:type="dxa"/><w:vAlign w:val="center"/>`, cellWidth)
			if colspan > 1 {
				fmt.Fprintf(output, `<w:gridSpan w:val="%d"/>`, colspan)
			}
			if cell.Type == "tableHeader" {
				output.WriteString(`<w:shd w:fill="E8ECE9"/>`)
			} else if rowIndex%2 == 0 {
				output.WriteString(`<w:shd w:fill="F7F8F6"/>`)
			}
			output.WriteString("</w:tcPr>")
			if len(cell.Content) == 0 {
				output.WriteString(`<w:p><w:pPr><w:pStyle w:val="TableBody"/></w:pPr></w:p>`)
			} else {
				for _, child := range cell.Content {
					if child.Type == "paragraph" {
						style := "TableBody"
						if cell.Type == "tableHeader" {
							style = "TableHeader"
						}
						renderer.renderParagraph(output, child.Content, style, 0, 0)
					} else {
						renderer.renderBlocks(output, []editor.Node{child}, 0, 0)
					}
				}
			}
			output.WriteString("</w:tc>")
			column += colspan
		}
		output.WriteString("</w:tr>")
		rowIndex++
	}
	output.WriteString("</w:tbl>")
}

func (renderer wordNodeRenderer) tableColumnWidths(table editor.Node) []int {
	columnCount := 0
	weights := []int{}
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		column := 0
		for _, cell := range row.Content {
			if cell.Type != "tableCell" && cell.Type != "tableHeader" {
				continue
			}
			span := numericPublicationAttr(cell.Attrs, "colspan", 1)
			if span < 1 {
				span = 1
			}
			for len(weights) < column+span {
				weights = append(weights, 4)
			}
			weight := len([]rune(strings.TrimSpace(codeNodeText(cell.Content)))) / span
			if weight < 4 {
				weight = 4
			}
			if weight > 30 {
				weight = 30
			}
			for index := column; index < column+span; index++ {
				if weight > weights[index] {
					weights[index] = weight
				}
			}
			column += span
		}
		if column > columnCount {
			columnCount = column
		}
	}
	if columnCount == 0 {
		return nil
	}
	weights = weights[:columnCount]
	pageWidth, _ := wordPageSize(renderer.profile.Style.PageSize)
	available := pageWidth - 2*renderer.profile.Style.MarginMM*57
	if available < columnCount*720 {
		available = columnCount * 720
	}
	base := 540
	remaining := available - base*columnCount
	totalWeight := 0
	for _, weight := range weights {
		totalWeight += weight
	}
	widths := make([]int, columnCount)
	used := 0
	for index, weight := range weights {
		widths[index] = base + remaining*weight/totalWeight
		used += widths[index]
	}
	widths[len(widths)-1] += available - used
	return widths
}

func (renderer wordNodeRenderer) renderImage(output *strings.Builder, node editor.Node) {
	assetID, _ := node.Attrs["assetId"].(string)
	resource, exists := renderer.resources.imageByID[assetID]
	if !exists {
		return
	}
	alt, _ := node.Attrs["alt"].(string)
	caption, _ := node.Attrs["caption"].(string)
	if alt == "" {
		alt = caption
	}
	output.WriteString(renderWordImage(resource.relationshipID, alt, resource.media.Width, resource.media.Height, resource.objectID))
	if caption != "" {
		renderer.renderParagraph(output, []editor.Node{{Type: "text", Text: caption}}, "Caption", 0, 0)
	}
}

func (renderer wordNodeRenderer) sceneSeparator() string {
	return sceneSeparatorText(renderer.profile.Style.SceneSeparator)
}
