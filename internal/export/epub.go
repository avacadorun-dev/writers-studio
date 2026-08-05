package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"sort"
	"strings"
)

func renderEPUB(snapshot Snapshot, assets map[string][]byte) ([]byte, error) {
	mediaByID := make(map[string]SnapshotMedia, len(snapshot.Media))
	paths := make(map[string]string, len(snapshot.Media))
	for _, item := range snapshot.Media {
		mediaByID[item.ID] = item
		if len(assets[item.ID]) > 0 && strings.HasPrefix(item.MediaType, "image/") {
			paths[item.ID] = "images/" + item.ID + mediaExtension(item.MediaType)
		}
	}
	publication := buildPublication(snapshot, func(assetID string) string { return paths[assetID] })

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	mimetypeHeader := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mimetype, err := archive.CreateHeader(mimetypeHeader)
	if err != nil {
		return nil, err
	}
	if _, err := mimetype.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"OEBPS/styles.css":       publicationCSS(snapshot, assets, "epub"),
		"OEBPS/nav.xhtml":        renderEPUBNav(snapshot, publication),
		"OEBPS/package.opf":      renderEPUBPackage(snapshot, publication, paths, mediaByID, assets),
	}
	if coverPath := epubCoverPath(snapshot, paths); coverPath != "" {
		files["OEBPS/cover.xhtml"] = renderEPUBCover(publication, coverPath)
	}
	if snapshot.Profile.TitlePage {
		files["OEBPS/title.xhtml"] = renderEPUBTitle(publication)
	}
	if snapshot.Profile.ChapterPageBreak {
		for _, section := range publication.Sections {
			files["OEBPS/"+section.ID+".xhtml"] = renderEPUBSection(snapshot, publication, section)
		}
	} else {
		files["OEBPS/content.xhtml"] = renderEPUBContinuous(snapshot, publication)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := files[name]
		entry, createErr := archive.Create(name)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := entry.Write([]byte(content)); writeErr != nil {
			return nil, writeErr
		}
	}
	assetIDs := make([]string, 0, len(paths))
	for assetID := range paths {
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)
	for _, assetID := range assetIDs {
		relativePath := paths[assetID]
		entry, createErr := archive.Create("OEBPS/" + relativePath)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := entry.Write(assets[assetID]); writeErr != nil {
			return nil, writeErr
		}
	}
	for _, filename := range exportFontFilenames(snapshot.Profile.Style.FontFamily) {
		data := assets[fontAssetKey(filename)]
		if len(data) == 0 {
			continue
		}
		entry, createErr := archive.Create("OEBPS/fonts/" + filename)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := entry.Write(data); writeErr != nil {
			return nil, writeErr
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderEPUBPackage(snapshot Snapshot, publication publication, paths map[string]string, media map[string]SnapshotMedia, assets map[string][]byte) string {
	identifier := snapshot.ExportID
	if identifier == "" {
		identifier = snapshot.ProjectID
	}
	var manifest strings.Builder
	var spine strings.Builder
	manifest.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="styles" href="styles.css" media-type="text/css"/>`)
	if epubCoverPath(snapshot, paths) != "" {
		manifest.WriteString(`<item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/>`)
		spine.WriteString(`<itemref idref="cover"/>`)
	}
	if snapshot.Profile.TitlePage {
		manifest.WriteString(`<item id="title" href="title.xhtml" media-type="application/xhtml+xml"/>`)
		spine.WriteString(`<itemref idref="title"/>`)
	}
	if snapshot.Profile.TableOfContents {
		spine.WriteString(`<itemref idref="nav"/>`)
	}
	if snapshot.Profile.ChapterPageBreak {
		for index, section := range publication.Sections {
			fmt.Fprintf(&manifest, `<item id="section-%d" href="%s.xhtml" media-type="application/xhtml+xml"/>`, index+1, section.ID)
			fmt.Fprintf(&spine, `<itemref idref="section-%d"/>`, index+1)
		}
	} else {
		manifest.WriteString(`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`)
		spine.WriteString(`<itemref idref="content"/>`)
	}
	coverID := ""
	if snapshot.Profile.Metadata.CoverAssetID != nil {
		coverID = *snapshot.Profile.Metadata.CoverAssetID
	}
	assetIDs := make([]string, 0, len(paths))
	for assetID := range paths {
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)
	for _, assetID := range assetIDs {
		relativePath := paths[assetID]
		properties := ""
		if assetID == coverID {
			properties = ` properties="cover-image"`
		}
		fmt.Fprintf(&manifest, `<item id="image-%s" href="%s" media-type="%s"%s/>`, html.EscapeString(assetID), html.EscapeString(relativePath), html.EscapeString(media[assetID].MediaType), properties)
	}
	for index, filename := range exportFontFilenames(snapshot.Profile.Style.FontFamily) {
		if len(assets[fontAssetKey(filename)]) > 0 {
			fmt.Fprintf(&manifest, `<item id="font-%d" href="fonts/%s" media-type="font/woff2"/>`, index+1, html.EscapeString(filename))
		}
	}
	extras := ""
	if publication.Author != "" {
		extras += "<dc:creator>" + html.EscapeString(publication.Author) + "</dc:creator>"
	}
	if publication.Annotation != "" {
		extras += "<dc:description>" + html.EscapeString(publication.Annotation) + "</dc:description>"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="publication-id" xml:lang="%s">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="publication-id">urn:uuid:%s</dc:identifier><dc:title>%s</dc:title><dc:language>%s</dc:language>%s<meta property="dcterms:modified">2026-01-01T00:00:00Z</meta></metadata>
<manifest>%s</manifest><spine>%s</spine></package>`, html.EscapeString(publication.Language), html.EscapeString(identifier), html.EscapeString(publication.Title), html.EscapeString(publication.Language), extras, manifest.String(), spine.String())
}

func renderEPUBNav(snapshot Snapshot, publication publication) string {
	var items strings.Builder
	for _, section := range publication.Sections {
		href := section.ID + ".xhtml"
		if !snapshot.Profile.ChapterPageBreak {
			href = "content.xhtml#" + section.ID
		}
		fmt.Fprintf(&items, `<li><a href="%s">%s</a></li>`, href, html.EscapeString(section.Title))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="%s"><head><title>Оглавление</title><link rel="stylesheet" type="text/css" href="styles.css"/></head><body><nav epub:type="toc" id="toc"><h1>Оглавление</h1><ol>%s</ol></nav></body></html>`, html.EscapeString(publication.Language), items.String())
}

func epubCoverPath(snapshot Snapshot, paths map[string]string) string {
	if snapshot.Profile.Metadata.CoverAssetID == nil {
		return ""
	}
	return paths[*snapshot.Profile.Metadata.CoverAssetID]
}

func renderEPUBCover(publication publication, coverPath string) string {
	cover := `<section class="cover-page"><img class="cover" src="` + html.EscapeString(coverPath) + `" alt="Обложка"/></section>`
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s"><head><title>Обложка — %s</title><link rel="stylesheet" type="text/css" href="styles.css"/></head><body>%s</body></html>`, html.EscapeString(publication.Language), html.EscapeString(publication.Title), cover)
}

func renderEPUBTitle(publication publication) string {
	titlePage := `<section class="title-page"><h1>` + html.EscapeString(publication.Title) + `</h1>`
	if publication.Subtitle != "" {
		titlePage += `<p class="subtitle">` + html.EscapeString(publication.Subtitle) + `</p>`
	}
	if publication.Author != "" {
		titlePage += `<p class="author">` + html.EscapeString(publication.Author) + `</p>`
	}
	titlePage += `</section>`
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s"><head><title>%s</title><link rel="stylesheet" type="text/css" href="styles.css"/></head><body>%s</body></html>`, html.EscapeString(publication.Language), html.EscapeString(publication.Title), titlePage)
}

func renderEPUBSection(snapshot Snapshot, publication publication, section publicationSection) string {
	body := renderEPUBSectionBody(snapshot, section)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s"><head><title>%s</title><link rel="stylesheet" type="text/css" href="styles.css"/></head><body><main>%s</main></body></html>`, html.EscapeString(publication.Language), html.EscapeString(section.Title), body)
}

func renderEPUBContinuous(snapshot Snapshot, publication publication) string {
	var body strings.Builder
	for index, section := range publication.Sections {
		if index > 0 {
			body.WriteString(publicationSceneBreakHTML())
		}
		body.WriteString(renderEPUBSectionBody(snapshot, section))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s"><head><title>%s</title><link rel="stylesheet" type="text/css" href="styles.css"/></head><body><main>%s</main></body></html>`, html.EscapeString(publication.Language), html.EscapeString(publication.Title), body.String())
}

func renderEPUBSectionBody(snapshot Snapshot, section publicationSection) string {
	heading := ""
	if snapshot.Profile.ChapterTitles {
		heading = "<h1>" + html.EscapeString(section.Title) + "</h1>"
	}
	return fmt.Sprintf(`<section class="chapter" id="%s">%s%s</section>`, section.ID, heading, section.HTML)
}
