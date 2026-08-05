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
			renderer.renderParagraph(output, flattenWordText(node.Content), "CodeBlock", 0, 0)
		case "callout":
			renderer.renderStyledChildren(output, node.Content, "Callout")
		case "table":
			renderer.renderTable(output, node)
		case "imageBlock":
			renderer.renderImage(output, node)
		case "mermaidBlock":
			code, _ := node.Attrs["code"].(string)
			caption, _ := node.Attrs["caption"].(string)
			renderer.renderParagraph(output, []editor.Node{{Type: "text", Text: code}}, "CodeBlock", 0, 0)
			if caption != "" {
				renderer.renderParagraph(output, []editor.Node{{Type: "text", Text: caption}}, "Caption", 0, 0)
			}
		case "text", "hardBreak":
			renderer.renderParagraph(output, []editor.Node{node}, "", listNumberID, listLevel)
		default:
			renderer.renderBlocks(output, node.Content, listNumberID, listLevel)
		}
	}
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
		output.WriteString("<w:r>")
		if properties != "" {
			output.WriteString("<w:rPr>" + properties + "</w:rPr>")
		}
		output.WriteString(`<w:t xml:space="preserve">`)
		_ = xml.EscapeText(output, []byte(node.Text))
		output.WriteString("</w:t></w:r>")
		if linkID != "" {
			output.WriteString("</w:hyperlink>")
		}
	case "hardBreak":
		output.WriteString("<w:r><w:br/></w:r>")
	default:
		for _, child := range node.Content {
			renderer.renderInline(output, child)
		}
	}
}

func (renderer wordNodeRenderer) renderTable(output *strings.Builder, table editor.Node) {
	output.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="A7AAA4"/><w:left w:val="single" w:sz="4" w:color="A7AAA4"/><w:bottom w:val="single" w:sz="4" w:color="A7AAA4"/><w:right w:val="single" w:sz="4" w:color="A7AAA4"/><w:insideH w:val="single" w:sz="4" w:color="A7AAA4"/><w:insideV w:val="single" w:sz="4" w:color="A7AAA4"/></w:tblBorders></w:tblPr>`)
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		output.WriteString("<w:tr>")
		for _, cell := range row.Content {
			if cell.Type != "tableCell" && cell.Type != "tableHeader" {
				continue
			}
			output.WriteString("<w:tc><w:tcPr>")
			if colspan := numericPublicationAttr(cell.Attrs, "colspan", 1); colspan > 1 {
				fmt.Fprintf(output, `<w:gridSpan w:val="%d"/>`, colspan)
			}
			if cell.Type == "tableHeader" {
				output.WriteString(`<w:shd w:fill="E8ECE9"/>`)
			}
			output.WriteString("</w:tcPr>")
			if len(cell.Content) == 0 {
				output.WriteString("<w:p/>")
			} else {
				for _, child := range cell.Content {
					if child.Type == "paragraph" {
						style := ""
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
		}
		output.WriteString("</w:tr>")
	}
	output.WriteString("</w:tbl>")
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

func flattenWordText(nodes []editor.Node) []editor.Node {
	var result strings.Builder
	var visit func(editor.Node)
	visit = func(node editor.Node) {
		if node.Type == "text" {
			result.WriteString(node.Text)
		}
		if node.Type == "hardBreak" {
			result.WriteByte('\n')
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	parts := strings.Split(result.String(), "\n")
	inline := make([]editor.Node, 0, len(parts)*2)
	for index, part := range parts {
		if index > 0 {
			inline = append(inline, editor.Node{Type: "hardBreak"})
		}
		inline = append(inline, editor.Node{Type: "text", Text: part})
	}
	return inline
}
