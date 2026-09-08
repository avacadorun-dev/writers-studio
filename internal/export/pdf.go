package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"os/exec"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

var ErrPDFRendererUnavailable = errors.New("pdf renderer unavailable")

func renderPDF(ctx context.Context, snapshot Snapshot, assets map[string][]byte, configuredPath string) ([]byte, error) {
	chromiumPath := resolveChromiumPath(configuredPath)
	if chromiumPath == "" {
		return nil, ErrPDFRendererUnavailable
	}

	resolveImage := dataImageResolver(snapshot, assets)
	publication := buildPublication(snapshot, resolveImage)
	htmlDocument := renderPrintHTML(snapshot, publication, resolveImage, assets)

	allocatorOptions := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromiumPath),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-component-update", true),
	)
	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer cancelAllocator()
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext)
	defer cancelBrowser()

	var data []byte
	if err := chromedp.Run(
		browserContext,
		network.Enable(),
		blockExternalNetwork(),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(actionContext context.Context) error {
			frameTree, frameErr := page.GetFrameTree().Do(actionContext)
			if frameErr != nil {
				return frameErr
			}
			return page.SetDocumentContent(frameTree.Frame.ID, htmlDocument).Do(actionContext)
		}),
		emulation.SetEmulatedMedia().WithMedia("print"),
		chromedp.Evaluate(
			`document.fonts.ready.then(() => true)`,
			new(bool),
			func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
				return params.WithAwaitPromise(true)
			},
		),
		chromedp.ActionFunc(func(actionContext context.Context) error {
			var printErr error
			data, _, printErr = page.PrintToPDF().
				WithPrintBackground(true).
				WithDisplayHeaderFooter(false).
				WithPreferCSSPageSize(true).
				WithMarginTop(0).
				WithMarginRight(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithGenerateTaggedPDF(true).
				WithGenerateDocumentOutline(true).
				Do(actionContext)
			return printErr
		}),
	); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPDFRendererUnavailable, err)
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return nil, fmt.Errorf("invalid PDF renderer output")
	}
	// Tagged output contains page-tab metadata introduced after PDF 1.4.
	// Chromium currently keeps the older header, so advertise the actual
	// feature level without touching offsets (the replacement has equal size).
	if len(data) >= 8 && string(data[:8]) == "%PDF-1.4" {
		copy(data[:8], []byte("%PDF-1.7"))
	}
	return finalizePDF(data, snapshot)
}

func resolveChromiumPath(configuredPath string) string {
	chromiumPath := strings.TrimSpace(configuredPath)
	if chromiumPath != "" {
		return chromiumPath
	}
	for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	return ""
}

// Network.setBlockedURLs used a wildcard `urls` array before the URLPattern
// variant was introduced. The runtime image deliberately pins an older,
// security-supported Chromium, while the generated CDP client tracks tip-of-
// tree. Calling the stable wire contract directly keeps both versions
// compatible and prevents any external resource fetch during rendering.
func blockExternalNetwork() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return cdp.Execute(ctx, network.CommandSetBlockedURLs, struct {
			URLs []string `json:"urls"`
		}{URLs: []string{"http://*/*", "https://*/*"}}, nil)
	})
}

func finalizePDF(data []byte, snapshot Snapshot) ([]byte, error) {
	properties := map[string]string{
		"Title":   snapshot.Profile.Metadata.Title,
		"Creator": "Writers Studio",
	}
	if snapshot.Profile.Metadata.Author != "" {
		properties["Author"] = snapshot.Profile.Metadata.Author
	}
	if snapshot.Profile.Metadata.Annotation != "" {
		properties["Subject"] = snapshot.Profile.Metadata.Annotation
	}
	var result bytes.Buffer
	if err := api.AddProperties(bytes.NewReader(data), &result, properties, nil); err != nil {
		return nil, fmt.Errorf("write PDF metadata: %w", err)
	}
	configuration := model.NewDefaultConfiguration()
	configuration.ValidationMode = model.ValidationStrict
	if err := api.Validate(bytes.NewReader(result.Bytes()), configuration); err != nil {
		return nil, fmt.Errorf("validate PDF: %w", err)
	}
	return result.Bytes(), nil
}

func renderPrintHTML(snapshot Snapshot, publication publication, resolveImage imageResolver, assets map[string][]byte) string {
	var coverPage strings.Builder
	var body strings.Builder
	cover := ""
	if snapshot.Profile.Metadata.CoverAssetID != nil {
		cover = resolveImage(*snapshot.Profile.Metadata.CoverAssetID)
	}
	if cover != "" {
		coverPage.WriteString(`<section class="cover-page"><img class="cover" src="` + cover + `" alt="Обложка"/></section>`)
	}
	if snapshot.Profile.TitlePage {
		body.WriteString(`<section class="title-page"><h1>` + html.EscapeString(publication.Title) + `</h1><p class="subtitle">` + html.EscapeString(publication.Subtitle) + `</p><p class="author">` + html.EscapeString(publication.Author) + `</p></section>`)
	}
	if snapshot.Profile.TableOfContents {
		body.WriteString(`<nav class="toc"><h1>Оглавление</h1><ol>`)
		for _, section := range publication.Sections {
			body.WriteString(`<li><a href="#` + section.ID + `">` + html.EscapeString(section.Title) + `</a></li>`)
		}
		body.WriteString(`</ol></nav>`)
	}
	for index, section := range publication.Sections {
		if index > 0 && !snapshot.Profile.ChapterPageBreak {
			body.WriteString(publicationSceneBreakHTML())
		}
		body.WriteString(`<section class="chapter" id="` + section.ID + `">`)
		if snapshot.Profile.ChapterTitles {
			body.WriteString("<h1>" + html.EscapeString(section.Title) + "</h1>")
		}
		body.WriteString(section.HTML + "</section>")
	}
	return `<!doctype html><html lang="` + html.EscapeString(publication.Language) + `"><head><meta charset="utf-8"><meta name="author" content="` + html.EscapeString(publication.Author) + `"><title>` + html.EscapeString(publication.Title) + `</title><style>` + publicationCSS(snapshot, assets, "data") + `</style></head><body>` + coverPage.String() + `<main>` + body.String() + `</main></body></html>`
}
