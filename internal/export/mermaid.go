package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"

	"writers.studio/internal/editor"
)

func prepareMermaidAssets(
	ctx context.Context,
	snapshot Snapshot,
	assets map[string][]byte,
	configuredChromiumPath string,
) (Snapshot, map[string][]byte, error) {
	if countMermaidBlocks(snapshot) == 0 {
		return snapshot, assets, nil
	}
	chromiumPath := resolveChromiumPath(configuredChromiumPath)
	scriptPath := resolveMermaidScriptPath()
	if chromiumPath == "" || scriptPath == "" {
		return snapshot, assets, errors.New("Mermaid renderer is unavailable")
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return snapshot, assets, fmt.Errorf("read Mermaid renderer: %w", err)
	}
	if len(script) == 0 || len(script) > 8<<20 {
		return snapshot, assets, errors.New("Mermaid renderer has an invalid size")
	}

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
	if err := chromedp.Run(
		browserContext,
		network.Enable(),
		blockExternalNetwork(),
		chromedp.Navigate("about:blank"),
		emulation.SetDeviceMetricsOverride(1400, 1000, 2, false),
	); err != nil {
		return snapshot, assets, err
	}

	preparedAssets := make(map[string][]byte, len(assets)+countMermaidBlocks(snapshot))
	for assetID, data := range assets {
		preparedAssets[assetID] = data
	}
	prepared := snapshot
	prepared.Documents = append([]SnapshotDocument(nil), snapshot.Documents...)
	prepared.Media = append([]SnapshotMedia(nil), snapshot.Media...)
	diagramIndex := 0
	var lastRenderErr error
	for documentIndex := range prepared.Documents {
		var root editor.Node
		if json.Unmarshal(prepared.Documents[documentIndex].Content, &root) != nil {
			continue
		}
		changed := replaceMermaidNodes(&root, func(code, _ string) (string, bool) {
			diagramIndex++
			png, width, height, renderErr := renderMermaidPNG(
				browserContext, script, code, fmt.Sprintf("diagram-%d", diagramIndex),
			)
			if renderErr != nil {
				lastRenderErr = renderErr
				return "", false
			}
			assetID := uuid.NewSHA1(
				uuid.NameSpaceURL,
				[]byte(fmt.Sprintf("writers-studio:export:%s:%d:%s", snapshot.ExportID, diagramIndex, code)),
			).String()
			preparedAssets[assetID] = png
			prepared.Media = append(prepared.Media, SnapshotMedia{
				ID: assetID, MediaType: "image/png", OriginalName: "mermaid.png",
				Width: width, Height: height,
			})
			return assetID, true
		})
		if !changed {
			continue
		}
		encoded, encodeErr := json.Marshal(root)
		if encodeErr == nil {
			prepared.Documents[documentIndex].Content = encoded
		}
	}
	return prepared, preparedAssets, lastRenderErr
}

func countMermaidBlocks(snapshot Snapshot) int {
	count := 0
	var visit func(editor.Node)
	visit = func(node editor.Node) {
		if node.Type == "mermaidBlock" {
			count++
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	for _, document := range snapshot.Documents {
		var root editor.Node
		if json.Unmarshal(document.Content, &root) == nil {
			visit(root)
		}
	}
	return count
}

func replaceMermaidNodes(
	node *editor.Node,
	render func(code, caption string) (string, bool),
) bool {
	changed := false
	for index := range node.Content {
		child := &node.Content[index]
		if child.Type == "mermaidBlock" {
			code, _ := child.Attrs["code"].(string)
			caption, _ := child.Attrs["caption"].(string)
			if assetID, ok := render(code, caption); ok {
				attrs := map[string]any{
					"assetId": assetID, "src": "/api/media/" + assetID + "/content",
					"alt": "Mermaid-схема", "caption": caption,
				}
				if blockID, exists := child.Attrs["blockId"]; exists {
					attrs["blockId"] = blockID
				}
				*child = editor.Node{Type: "imageBlock", Attrs: attrs}
				changed = true
				continue
			}
		}
		changed = replaceMermaidNodes(child, render) || changed
	}
	return changed
}

func renderMermaidPNG(
	ctx context.Context,
	script []byte,
	source string,
	diagramID string,
) ([]byte, int, int, error) {
	renderContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	htmlDocument := `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;padding:0;background:#fff}#diagram{display:inline-block;max-width:1280px;padding:24px;background:#fff;color:#24292f}#diagram svg{display:block;max-width:1232px;height:auto}</style></head><body><div id="diagram"></div></body></html>`
	encodedSource, _ := json.Marshal(source)
	encodedID, _ := json.Marshal(diagramID)
	expression := `(async()=>{try{mermaid.initialize({startOnLoad:false,securityLevel:"strict",theme:"neutral",fontFamily:"Arial, sans-serif",flowchart:{htmlLabels:false}});const rendered=await mermaid.render(` + string(encodedID) + `,` + string(encodedSource) + `);const host=document.getElementById("diagram");host.innerHTML=rendered.svg;const svg=host.querySelector("svg");if(svg){svg.removeAttribute("height");svg.setAttribute("role","img");}return "ok";}catch(error){return "error:"+String(error&&error.message||error);}})()`
	status := ""
	var png []byte
	if err := chromedp.Run(
		renderContext,
		chromedp.ActionFunc(func(actionContext context.Context) error {
			frameTree, frameErr := page.GetFrameTree().Do(actionContext)
			if frameErr != nil {
				return frameErr
			}
			return page.SetDocumentContent(frameTree.Frame.ID, htmlDocument).Do(actionContext)
		}),
		chromedp.ActionFunc(func(actionContext context.Context) error {
			_, exception, evaluateErr := runtime.Evaluate(string(script)).WithReturnByValue(false).Do(actionContext)
			if evaluateErr != nil {
				return evaluateErr
			}
			if exception != nil {
				return fmt.Errorf("load Mermaid renderer: %s", exception.Text)
			}
			return nil
		}),
		chromedp.Evaluate(
			expression,
			&status,
			func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
				return params.WithAwaitPromise(true)
			},
		),
		chromedp.Screenshot("#diagram", &png, chromedp.NodeVisible, chromedp.ByQuery),
	); err != nil {
		return nil, 0, 0, err
	}
	if status != "ok" {
		return nil, 0, 0, fmt.Errorf("render Mermaid diagram: %s", strings.TrimPrefix(status, "error:"))
	}
	return validatedMermaidPNG(png)
}

func validatedMermaidPNG(png []byte) ([]byte, int, int, error) {
	configuration, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		return nil, 0, 0, err
	}
	return png, configuration.Width, configuration.Height, nil
}

func resolveMermaidScriptPath() string {
	if configured := strings.TrimSpace(os.Getenv("MERMAID_JS_PATH")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
		return ""
	}
	for _, candidate := range []string{
		filepath.Join("node_modules", "mermaid", "dist", "mermaid.min.js"),
		filepath.Join("/app", "mermaid.min.js"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
