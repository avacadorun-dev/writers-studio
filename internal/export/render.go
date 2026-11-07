package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

type Snapshot struct {
	SchemaVersion   int                  `json:"schemaVersion,omitempty"`
	ExportID        string               `json:"exportId,omitempty"`
	ProjectID       string               `json:"projectId"`
	ProjectTitle    string               `json:"projectTitle"`
	ProjectLanguage string               `json:"projectLanguage,omitempty"`
	TitlePage       bool                 `json:"titlePage"`
	ChapterTitles   bool                 `json:"chapterTitles"`
	IncludeNotes    bool                 `json:"includeNotes"`
	Profile         domain.ExportProfile `json:"profile,omitempty"`
	Documents       []SnapshotDocument   `json:"documents"`
	TreeNodes       []SnapshotTreeNode   `json:"treeNodes,omitempty"`
	Media           []SnapshotMedia      `json:"media,omitempty"`
}

type SnapshotMedia struct {
	ID           string  `json:"id"`
	MediaType    string  `json:"mediaType"`
	StorageKey   string  `json:"storageKey"`
	OriginalName string  `json:"originalName,omitempty"`
	SourcePath   *string `json:"sourcePath,omitempty"`
	SHA256       string  `json:"sha256,omitempty"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
}

type SnapshotTreeNode struct {
	NodeID         string          `json:"nodeId"`
	ParentID       *string         `json:"parentId,omitempty"`
	Kind           string          `json:"kind"`
	Title          string          `json:"title"`
	Position       float64         `json:"position"`
	ContentBearing bool            `json:"contentBearing"`
	Revision       int             `json:"revision"`
	Content        json.RawMessage `json:"content,omitempty"`
	PlainText      string          `json:"plainText,omitempty"`
	Frontmatter    string          `json:"frontmatter,omitempty"`
}

type SnapshotDocument struct {
	NodeID    string          `json:"nodeId"`
	Kind      string          `json:"kind"`
	Title     string          `json:"title"`
	Revision  int             `json:"revision"`
	Content   json.RawMessage `json:"content"`
	PlainText string          `json:"plainText"`
}

func Render(format string, snapshot Snapshot) ([]byte, string, error) {
	return RenderWithAssets(context.Background(), format, snapshot, nil, "")
}

// RenderWithAssets renders an immutable export snapshot. Assets are keyed by
// project-media ID; renderers never fetch network resources while building a
// publication.
func RenderWithAssets(
	ctx context.Context,
	format string,
	snapshot Snapshot,
	assets map[string][]byte,
	chromiumPath string,
) ([]byte, string, error) {
	snapshot.Profile = NormalizeProfile(snapshot.Profile, snapshot.ProjectTitle, "")
	// Snapshots produced before the publication profile was introduced keep
	// their original title/chapter switches as compatibility defaults.
	if snapshot.SchemaVersion < 2 {
		snapshot.Profile.TitlePage = snapshot.TitlePage
		snapshot.Profile.ChapterTitles = snapshot.ChapterTitles
	}
	if err := ValidateProfile(snapshot.Profile); err != nil {
		return nil, "", err
	}
	snapshot.TitlePage = snapshot.Profile.TitlePage
	snapshot.ChapterTitles = snapshot.Profile.ChapterTitles
	snapshot.IncludeNotes = snapshot.Profile.IncludeNotes
	snapshot = normalizeLegacyCodeFormatting(snapshot)
	if format == "docx" || format == "epub" || format == "pdf" {
		preparedSnapshot, preparedAssets, _ := prepareMermaidAssets(ctx, snapshot, assets, chromiumPath)
		snapshot, assets = preparedSnapshot, preparedAssets
	}
	switch format {
	case "txt":
		return renderText(snapshot), "text/plain; charset=utf-8", nil
	case "markdown":
		return renderMarkdown(snapshot), "text/markdown; charset=utf-8", nil
	case "markdown_tree":
		data, err := renderMarkdownTree(snapshot, assets)
		return data, "application/zip", err
	case "docx":
		data, err := renderDOCX(snapshot, assets)
		return data, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", err
	case "epub":
		data, err := renderEPUB(snapshot, assets)
		return data, "application/epub+zip", err
	case "pdf":
		data, err := renderPDF(ctx, snapshot, assets, chromiumPath)
		return data, "application/pdf", err
	default:
		return nil, "", fmt.Errorf("unsupported export format %q", format)
	}
}

func normalizeLegacyCodeFormatting(snapshot Snapshot) Snapshot {
	// Concurrent renders can share the input slices. Own their elements before
	// replacing content so normalization leaves the caller's snapshot unchanged.
	snapshot.Documents = append([]SnapshotDocument(nil), snapshot.Documents...)
	snapshot.TreeNodes = append([]SnapshotTreeNode(nil), snapshot.TreeNodes...)
	for index := range snapshot.Documents {
		content := snapshot.Documents[index].Content
		if normalized, err := editor.NormalizeFencedCodeBlocks(content); err == nil {
			content = normalized
		}
		if normalized, err := editor.NormalizeInlineCodeMarks(content); err == nil {
			content = normalized
		}
		snapshot.Documents[index].Content = content
	}
	for index := range snapshot.TreeNodes {
		if len(snapshot.TreeNodes[index].Content) == 0 {
			continue
		}
		content := snapshot.TreeNodes[index].Content
		if normalized, err := editor.NormalizeFencedCodeBlocks(content); err == nil {
			content = normalized
		}
		if normalized, err := editor.NormalizeInlineCodeMarks(content); err == nil {
			content = normalized
		}
		snapshot.TreeNodes[index].Content = content
	}
	return snapshot
}

func renderText(snapshot Snapshot) []byte {
	var builder strings.Builder
	if snapshot.TitlePage {
		builder.WriteString(snapshot.ProjectTitle)
		builder.WriteString("\n\n")
	}
	for index, document := range snapshot.Documents {
		if snapshot.ChapterTitles {
			builder.WriteString(document.Title)
			builder.WriteString("\n\n")
		}
		builder.WriteString(strings.TrimSpace(document.PlainText))
		if index < len(snapshot.Documents)-1 {
			builder.WriteString("\n\n")
		}
	}
	return []byte(builder.String())
}

func renderMarkdown(snapshot Snapshot) []byte {
	var builder strings.Builder
	if snapshot.TitlePage {
		builder.WriteString("# ")
		builder.WriteString(snapshot.ProjectTitle)
		builder.WriteString("\n\n")
	}
	for index, document := range snapshot.Documents {
		if snapshot.ChapterTitles {
			builder.WriteString("## ")
			builder.WriteString(document.Title)
			builder.WriteString("\n\n")
		}
		markdown, err := editor.Markdown(document.Content)
		if err != nil {
			markdown = document.PlainText
		}
		builder.WriteString(strings.TrimSpace(markdown))
		if index < len(snapshot.Documents)-1 {
			builder.WriteString("\n\n")
		}
	}
	return []byte(builder.String())
}

func renderDOCX(snapshot Snapshot, assets map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	var cover *SnapshotMedia
	if snapshot.Profile.Metadata.CoverAssetID != nil && len(assets[*snapshot.Profile.Metadata.CoverAssetID]) > 0 {
		for index := range snapshot.Media {
			if snapshot.Media[index].ID == *snapshot.Profile.Metadata.CoverAssetID && strings.HasPrefix(snapshot.Media[index].MediaType, "image/") {
				cover = &snapshot.Media[index]
				break
			}
		}
	}
	resources := collectWordResources(snapshot, assets)

	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
	<Default Extension="png" ContentType="image/png"/>
	<Default Extension="jpg" ContentType="image/jpeg"/>
	<Default Extension="jpeg" ContentType="image/jpeg"/>
	<Default Extension="gif" ContentType="image/gif"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
	<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
	<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
	<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
	<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
	<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
	<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
	</Types>`),
		"_rels/.rels": []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
	<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
	<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
		</Relationships>`),
		"word/_rels/document.xml.rels": []byte(renderWordRelationships(cover, resources)),
		"word/document.xml":            []byte(renderWordDocument(snapshot, cover, resources)),
		"word/styles.xml":              []byte(renderWordStyles(snapshot.Profile)),
		"word/settings.xml":            []byte(renderWordSettings()),
		"word/numbering.xml":           []byte(renderWordNumbering()),
		"word/footer1.xml":             []byte(renderWordFooter()),
		"docProps/core.xml":            []byte(renderWordCoreProperties(snapshot)),
		"docProps/app.xml": []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
			<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>Writers Studio</Application></Properties>`),
	}
	if cover != nil {
		files["word/media/cover"+mediaExtension(cover.MediaType)] = assets[cover.ID]
	}
	for _, image := range resources.images {
		files["word/media/"+image.filename] = assets[image.assetID]
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := files[name]
		file, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(content); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderWordDocument(snapshot Snapshot, cover *SnapshotMedia, resources wordResources) string {
	var body strings.Builder
	addParagraph := func(text, style string) {
		body.WriteString("<w:p>")
		if style != "" {
			body.WriteString(`<w:pPr><w:pStyle w:val="`)
			body.WriteString(style)
			body.WriteString(`"/></w:pPr>`)
		}
		body.WriteString(`<w:r><w:t xml:space="preserve">`)
		_ = xml.EscapeText(&body, []byte(text))
		body.WriteString("</w:t></w:r></w:p>")
	}

	profile := snapshot.Profile
	if cover != nil {
		pageWidth, pageHeight := wordPageSize(profile.Style.PageSize)
		body.WriteString(renderWordCoverImage("rIdCover", "Обложка", cover.Width, cover.Height, pageWidth, pageHeight, 1))
	}
	if profile.TitlePage {
		addParagraph(profile.Metadata.Title, "Title")
		if profile.Metadata.Subtitle != "" {
			addParagraph(profile.Metadata.Subtitle, "Subtitle")
		}
		if profile.Metadata.Author != "" {
			addParagraph(profile.Metadata.Author, "Author")
		}
		body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
	}
	if profile.TableOfContents {
		addParagraph("Оглавление", "TOCHeading")
		for index, document := range snapshot.Documents {
			body.WriteString(`<w:p><w:pPr><w:pStyle w:val="TOC1"/></w:pPr><w:hyperlink w:anchor="chapter-`)
			body.WriteString(fmt.Sprintf("%d", index+1))
			body.WriteString(`" w:history="1"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/></w:rPr><w:t>`)
			_ = xml.EscapeText(&body, []byte(document.Title))
			body.WriteString(`</w:t></w:r></w:hyperlink></w:p>`)
		}
		body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
	}
	for index, document := range snapshot.Documents {
		if index > 0 {
			if profile.ChapterPageBreak {
				body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
			} else {
				addParagraph(sceneSeparatorText(profile.Style.SceneSeparator), "SceneBreak")
			}
		}
		if profile.ChapterTitles {
			body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:bookmarkStart w:id="`)
			body.WriteString(fmt.Sprintf("%d", index+1))
			body.WriteString(`" w:name="chapter-`)
			body.WriteString(fmt.Sprintf("%d", index+1))
			body.WriteString(`"/><w:r><w:t>`)
			_ = xml.EscapeText(&body, []byte(document.Title))
			body.WriteString(`</w:t></w:r><w:bookmarkEnd w:id="`)
			body.WriteString(fmt.Sprintf("%d", index+1))
			body.WriteString(`"/></w:p>`)
		}
		var root editor.Node
		if json.Unmarshal(document.Content, &root) == nil && len(root.Content) > 0 {
			body.WriteString(renderWordNodes(root.Content, snapshot.Profile, resources))
			continue
		}
		for _, paragraph := range strings.Split(document.PlainText, "\n\n") {
			if paragraph != "" {
				addParagraph(paragraph, "")
			}
		}
	}
	pageWidth, pageHeight := wordPageSize(profile.Style.PageSize)
	margin := profile.Style.MarginMM * 57
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<w:body>` + body.String() + fmt.Sprintf(`<w:sectPr><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d"/><w:pgNumType w:start="1"/></w:sectPr>`, pageWidth, pageHeight, margin, margin, margin, margin) + `</w:body></w:document>`
}

func renderWordRelationships(media *SnapshotMedia, resources wordResources) string {
	cover := ""
	if media != nil {
		cover = `<Relationship Id="rIdCover" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/cover` + mediaExtension(media.MediaType) + `"/>`
	}
	var relationships strings.Builder
	for _, link := range resources.links {
		relationships.WriteString(`<Relationship Id="`)
		relationships.WriteString(link.relationshipID)
		relationships.WriteString(`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="`)
		_ = xml.EscapeText(&relationships, []byte(link.href))
		relationships.WriteString(`" TargetMode="External"/>`)
	}
	for _, image := range resources.images {
		relationships.WriteString(`<Relationship Id="`)
		relationships.WriteString(image.relationshipID)
		relationships.WriteString(`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/`)
		relationships.WriteString(image.filename)
		relationships.WriteString(`"/>`)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/><Relationship Id="rIdNumbering" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/><Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` + cover + relationships.String() + `</Relationships>`
}

func renderWordImage(relationshipID, description string, width, height, objectID int) string {
	cx := int64(4_572_000)
	cy := int64(6_858_000)
	if width > 0 && height > 0 {
		cy = cx * int64(height) / int64(width)
		if cy > 6_858_000 {
			cy = 6_858_000
			cx = cy * int64(width) / int64(height)
		}
	}
	var escapedDescription strings.Builder
	_ = xml.EscapeText(&escapedDescription, []byte(description))
	return fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Image %d" descr="%s"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="Image %d"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, cx, cy, objectID, objectID, escapedDescription.String(), objectID, objectID, relationshipID, cx, cy)
}

func renderWordCoverImage(
	relationshipID, description string,
	width, height, pageWidth, pageHeight, objectID int,
) string {
	cx := int64(pageWidth) * 635
	cy := int64(pageHeight) * 635
	left, top, right, bottom := wordCoverCrop(width, height, pageWidth, pageHeight)
	var escapedDescription strings.Builder
	_ = xml.EscapeText(&escapedDescription, []byte(description))
	return fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="1" w:lineRule="exact"/><w:ind w:left="0" w:right="0" w:firstLine="0"/><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="0" w:right="0" w:bottom="0" w:left="0" w:header="0" w:footer="0" w:gutter="0"/></w:sectPr></w:pPr><w:r><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="0" locked="1" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:wrapNone/><wp:docPr id="%d" name="Cover" descr="%s"/><wp:cNvGraphicFramePr/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="Cover"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:srcRect l="%d" t="%d" r="%d" b="%d"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r></w:p>`, pageWidth, pageHeight, cx, cy, objectID, escapedDescription.String(), objectID, relationshipID, left, top, right, bottom, cx, cy)
}

func wordCoverCrop(width, height, pageWidth, pageHeight int) (left, top, right, bottom int) {
	if width <= 0 || height <= 0 || pageWidth <= 0 || pageHeight <= 0 {
		return 0, 0, 0, 0
	}
	if int64(width)*int64(pageHeight) > int64(height)*int64(pageWidth) {
		visibleWidth := int64(height) * int64(pageWidth) / int64(pageHeight)
		crop := int((int64(width) - visibleWidth) * 50_000 / int64(width))
		return crop, 0, crop, 0
	}
	visibleHeight := int64(width) * int64(pageHeight) / int64(pageWidth)
	if visibleHeight >= int64(height) {
		return 0, 0, 0, 0
	}
	crop := int((int64(height) - visibleHeight) * 50_000 / int64(height))
	return 0, crop, 0, crop
}

func renderWordStyles(profile domain.ExportProfile) string {
	font := publicationFont(profile.Style.FontFamily)
	size := profile.Style.FontSizePT * 2
	line := int(profile.Style.LineHeight * 240)
	align := "left"
	if profile.Style.TextAlign == "justify" {
		align = "both"
	}
	paragraphSpacing := 0
	firstLine := int(profile.Style.FirstLineIndent * float64(profile.Style.FontSizePT) * 20)
	if profile.Style.ParagraphMode == "spaced" {
		paragraphSpacing = int(profile.Style.ParagraphSpace * float64(profile.Style.FontSizePT) * 20)
		firstLine = 0
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
 <w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/><w:sz w:val="%[2]d"/><w:szCs w:val="%[2]d"/><w:lang w:val="ru-RU"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="%[3]d" w:line="%[4]d" w:lineRule="auto"/><w:ind w:firstLine="%[5]d"/><w:jc w:val="%[6]s"/></w:pPr></w:pPrDefault></w:docDefaults>
 <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
 <w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="2400" w:after="360"/></w:pPr><w:rPr><w:b/><w:sz w:val="48"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/></w:pPr><w:rPr><w:i/><w:sz w:val="28"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="Author"><w:name w:val="Author"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="720"/></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="360" w:after="240"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="300" w:after="160"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="2"/></w:pPr><w:rPr><w:b/><w:sz w:val="24"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="TOCHeading"><w:name w:val="TOC Heading"/><w:basedOn w:val="Heading1"/></w:style>
 <w:style w:type="paragraph" w:styleId="TOC1"><w:name w:val="toc 1"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:firstLine="0"/><w:spacing w:after="120"/></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="720" w:right="360" w:firstLine="0"/><w:spacing w:before="160" w:after="160"/></w:pPr><w:rPr><w:i/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CodeLanguage"><w:name w:val="Code Language"/><w:basedOn w:val="Normal"/><w:next w:val="CodeBlock"/><w:pPr><w:keepNext/><w:jc w:val="left"/><w:ind w:left="180" w:right="180" w:firstLine="0"/><w:spacing w:before="180" w:after="0" w:line="220" w:lineRule="auto"/><w:shd w:fill="E9E8E3"/><w:pBdr><w:top w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/><w:left w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/><w:bottom w:val="single" w:sz="4" w:space="6" w:color="D0D2CD"/><w:right w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/></w:pBdr></w:pPr><w:rPr><w:rFonts w:ascii="IBM Plex Mono" w:hAnsi="IBM Plex Mono"/><w:smallCaps/><w:b/><w:color w:val="545852"/><w:sz w:val="15"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CodeBlock"><w:name w:val="Code Block"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="left"/><w:ind w:left="180" w:right="180" w:firstLine="0"/><w:spacing w:before="0" w:after="180" w:line="205" w:lineRule="auto"/><w:shd w:fill="F6F5F2"/><w:pBdr><w:left w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/><w:bottom w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/><w:right w:val="single" w:sz="5" w:space="6" w:color="B7BBB5"/></w:pBdr></w:pPr><w:rPr><w:rFonts w:ascii="IBM Plex Mono" w:hAnsi="IBM Plex Mono"/><w:color w:val="24292F"/><w:sz w:val="17"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutInfoLabel"><w:name w:val="Callout Info Label"/><w:basedOn w:val="Normal"/><w:next w:val="CalloutInfo"/><w:pPr><w:keepNext/><w:ind w:left="240" w:right="180" w:firstLine="0"/><w:spacing w:before="180" w:after="60"/><w:shd w:fill="F2F7F5"/><w:pBdr><w:top w:val="single" w:sz="4" w:space="5" w:color="C8D5CF"/><w:left w:val="single" w:sz="14" w:space="5" w:color="497568"/><w:right w:val="single" w:sz="4" w:space="5" w:color="C8D5CF"/></w:pBdr></w:pPr><w:rPr><w:b/><w:smallCaps/><w:color w:val="315D51"/><w:sz w:val="16"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutInfo"><w:name w:val="Callout Info"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="240" w:right="180" w:firstLine="0"/><w:spacing w:before="0" w:after="180"/><w:shd w:fill="F2F7F5"/><w:pBdr><w:left w:val="single" w:sz="14" w:space="5" w:color="497568"/><w:bottom w:val="single" w:sz="4" w:space="5" w:color="C8D5CF"/><w:right w:val="single" w:sz="4" w:space="5" w:color="C8D5CF"/></w:pBdr></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutIdeaLabel"><w:name w:val="Callout Idea Label"/><w:basedOn w:val="CalloutInfoLabel"/><w:pPr><w:shd w:fill="FBF8EB"/><w:pBdr><w:top w:val="single" w:sz="4" w:space="5" w:color="D9CDA4"/><w:left w:val="single" w:sz="14" w:space="5" w:color="9A7720"/><w:right w:val="single" w:sz="4" w:space="5" w:color="D9CDA4"/></w:pBdr></w:pPr><w:rPr><w:color w:val="765B13"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutIdea"><w:name w:val="Callout Idea"/><w:basedOn w:val="CalloutInfo"/><w:pPr><w:shd w:fill="FBF8EB"/><w:pBdr><w:left w:val="single" w:sz="14" w:space="5" w:color="9A7720"/><w:bottom w:val="single" w:sz="4" w:space="5" w:color="D9CDA4"/><w:right w:val="single" w:sz="4" w:space="5" w:color="D9CDA4"/></w:pBdr></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutWarningLabel"><w:name w:val="Callout Warning Label"/><w:basedOn w:val="CalloutInfoLabel"/><w:pPr><w:shd w:fill="FDF3F1"/><w:pBdr><w:top w:val="single" w:sz="4" w:space="5" w:color="E4BBB6"/><w:left w:val="single" w:sz="14" w:space="5" w:color="B74C42"/><w:right w:val="single" w:sz="4" w:space="5" w:color="E4BBB6"/></w:pBdr></w:pPr><w:rPr><w:color w:val="913B33"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutWarning"><w:name w:val="Callout Warning"/><w:basedOn w:val="CalloutInfo"/><w:pPr><w:shd w:fill="FDF3F1"/><w:pBdr><w:left w:val="single" w:sz="14" w:space="5" w:color="B74C42"/><w:bottom w:val="single" w:sz="4" w:space="5" w:color="E4BBB6"/><w:right w:val="single" w:sz="4" w:space="5" w:color="E4BBB6"/></w:pBdr></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutTodoLabel"><w:name w:val="Callout Todo Label"/><w:basedOn w:val="CalloutInfoLabel"/><w:pPr><w:shd w:fill="F7F4FC"/><w:pBdr><w:top w:val="single" w:sz="4" w:space="5" w:color="C9C2DF"/><w:left w:val="single" w:sz="14" w:space="5" w:color="735DA8"/><w:right w:val="single" w:sz="4" w:space="5" w:color="C9C2DF"/></w:pBdr></w:pPr><w:rPr><w:color w:val="5B458E"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="CalloutTodo"><w:name w:val="Callout Todo"/><w:basedOn w:val="CalloutInfo"/><w:pPr><w:shd w:fill="F7F4FC"/><w:pBdr><w:left w:val="single" w:sz="14" w:space="5" w:color="735DA8"/><w:bottom w:val="single" w:sz="4" w:space="5" w:color="C9C2DF"/><w:right w:val="single" w:sz="4" w:space="5" w:color="C9C2DF"/></w:pBdr></w:pPr></w:style>
 <w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="Caption"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:ind w:firstLine="0"/><w:spacing w:after="180"/></w:pPr><w:rPr><w:i/><w:color w:val="5B5E59"/><w:sz w:val="18"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="TableHeader"><w:name w:val="Table Header"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:firstLine="0"/><w:spacing w:before="0" w:after="0" w:line="260" w:lineRule="auto"/><w:jc w:val="left"/></w:pPr><w:rPr><w:b/><w:color w:val="2F3833"/><w:sz w:val="18"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="TableBody"><w:name w:val="Table Body"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:firstLine="0"/><w:spacing w:before="0" w:after="0" w:line="260" w:lineRule="auto"/><w:jc w:val="left"/></w:pPr><w:rPr><w:sz w:val="18"/></w:rPr></w:style>
 <w:style w:type="paragraph" w:styleId="SceneBreak"><w:name w:val="Scene Break"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:ind w:firstLine="0"/><w:spacing w:before="240" w:after="240"/></w:pPr></w:style>
 <w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="265D4F"/><w:u w:val="single"/></w:rPr></w:style>
</w:styles>`, font, size, paragraphSpacing, line, firstLine, align)
}

func renderWordSettings() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:updateFields w:val="true"/><w:doNotTrackMoves/><w:doNotTrackFormatting/></w:settings>`
}

func renderWordNumbering() string {
	var ordered, bullets strings.Builder
	for level := 0; level < 9; level++ {
		left := 720 * (level + 1)
		fmt.Fprintf(&ordered, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%%%d."/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="%d"/></w:tabs><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`, level, level+1, left, left)
		bullet := "•"
		if level%3 == 1 {
			bullet = "◦"
		} else if level%3 == 2 {
			bullet = "▪"
		}
		fmt.Fprintf(&bullets, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="%d"/></w:tabs><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`, level, bullet, left, left)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="multilevel"/>` + ordered.String() + `</w:abstractNum><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="multilevel"/>` + bullets.String() + `</w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num><w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num></w:numbering>`
}

func renderWordFooter() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:fldSimple w:instr="PAGE"><w:r><w:t>1</w:t></w:r></w:fldSimple></w:p></w:ftr>`
}

func renderWordCoreProperties(snapshot Snapshot) string {
	var result strings.Builder
	result.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>`)
	_ = xml.EscapeText(&result, []byte(snapshot.Profile.Metadata.Title))
	result.WriteString(`</dc:title><dc:creator>`)
	_ = xml.EscapeText(&result, []byte(snapshot.Profile.Metadata.Author))
	result.WriteString(`</dc:creator><dc:language>`)
	_ = xml.EscapeText(&result, []byte(snapshot.ProjectLanguage))
	result.WriteString(`</dc:language></cp:coreProperties>`)
	return result.String()
}

func wordPageSize(pageSize string) (int, int) {
	switch pageSize {
	case "a4":
		return 11906, 16838
	case "letter":
		return 12240, 15840
	default:
		return 8391, 11906
	}
}
