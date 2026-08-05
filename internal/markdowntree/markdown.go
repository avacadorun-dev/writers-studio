package markdowntree

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"writers.studio/internal/editor"
)

type Warning struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Image struct {
	AssetID string
	Source  string
	Alt     string
	Width   int
	Height  int
}

type Resolver interface {
	Document(sourcePath string, target string) (string, bool, bool)
	Image(sourcePath string, target string) (Image, bool, bool)
}

type ParseResult struct {
	Content       json.RawMessage
	PlainText     string
	Frontmatter   string
	ResolvedLinks int
	Warnings      []Warning
}

type markdownConverter struct {
	sourcePath string
	source     []byte
	resolver   Resolver
	warnings   []Warning
	links      int
}

func ParseMarkdown(sourcePath string, source []byte, resolver Resolver) (ParseResult, error) {
	if !utf8.Valid(source) {
		return ParseResult{}, fmt.Errorf("markdown is not valid UTF-8")
	}
	frontmatter, body := SplitFrontmatter(string(source))
	prepared := rewriteWikiLinks(sourcePath, body, resolver)
	markdown := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	document := markdown.Parser().Parse(text.NewReader([]byte(prepared)))
	converter := markdownConverter{sourcePath: sourcePath, source: []byte(prepared), resolver: resolver}
	root := editor.Node{Type: "doc", Attrs: map[string]any{"schemaVersion": editor.CurrentSchemaVersion}}
	for child := document.FirstChild(); child != nil; child = child.NextSibling() {
		root.Content = append(root.Content, converter.block(child)...)
	}
	if len(root.Content) == 0 {
		root.Content = []editor.Node{{Type: "paragraph", Attrs: blockAttrs()}}
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return ParseResult{}, err
	}
	if err := editor.Validate(encoded); err != nil {
		return ParseResult{}, err
	}
	plainText, err := editor.PlainText(encoded)
	if err != nil {
		return ParseResult{}, err
	}
	return ParseResult{
		Content: encoded, PlainText: plainText, Frontmatter: frontmatter,
		ResolvedLinks: converter.links, Warnings: converter.warnings,
	}, nil
}

func SplitFrontmatter(source string) (string, string) {
	source = strings.TrimPrefix(source, "\ufeff")
	if !strings.HasPrefix(source, "---\n") && !strings.HasPrefix(source, "---\r\n") {
		return "", source
	}
	lines := strings.SplitAfter(source, "\n")
	offset := len(lines[0])
	for index := 1; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		offset += len(lines[index])
		if trimmed == "---" || trimmed == "..." {
			return source[:offset], source[offset:]
		}
	}
	return "", source
}

func blockAttrs() map[string]any {
	return map[string]any{"blockId": uuid.NewString()}
}

func (c *markdownConverter) block(node ast.Node) []editor.Node {
	switch current := node.(type) {
	case *ast.Paragraph:
		if image, ok := c.onlyImage(current); ok {
			return []editor.Node{image}
		}
		return []editor.Node{{Type: "paragraph", Attrs: blockAttrs(), Content: c.inlineChildren(current, nil)}}
	case *ast.TextBlock:
		return []editor.Node{{Type: "paragraph", Attrs: blockAttrs(), Content: c.inlineChildren(current, nil)}}
	case *ast.Heading:
		return []editor.Node{{
			Type:    "heading",
			Attrs:   map[string]any{"level": current.Level, "blockId": uuid.NewString()},
			Content: c.inlineChildren(current, nil),
		}}
	case *ast.ThematicBreak:
		return []editor.Node{{Type: "horizontalRule"}}
	case *ast.CodeBlock:
		return []editor.Node{{
			Type: "codeBlock", Attrs: map[string]any{"language": ""},
			Content: textContent(strings.TrimSuffix(string(current.Lines().Value(c.source)), "\n")),
		}}
	case *ast.FencedCodeBlock:
		language := string(current.Language(c.source))
		code := strings.TrimSuffix(string(current.Lines().Value(c.source)), "\n")
		if strings.EqualFold(language, "mermaid") {
			return []editor.Node{{Type: "mermaidBlock", Attrs: map[string]any{"code": code, "caption": ""}}}
		}
		return []editor.Node{{Type: "codeBlock", Attrs: map[string]any{"language": language}, Content: textContent(code)}}
	case *ast.Blockquote:
		children := c.blockChildren(current)
		if callout, ok := asCallout(children); ok {
			return []editor.Node{callout}
		}
		return []editor.Node{{Type: "blockquote", Attrs: blockAttrs(), Content: children}}
	case *ast.List:
		kind := "bulletList"
		attrs := map[string]any(nil)
		if current.IsOrdered() {
			kind = "orderedList"
			start := current.Start
			if start < 1 {
				start = 1
			}
			attrs = map[string]any{"start": start}
		}
		return []editor.Node{{Type: kind, Attrs: attrs, Content: c.blockChildren(current)}}
	case *ast.ListItem:
		return []editor.Node{{Type: "listItem", Content: c.blockChildren(current)}}
	case *extast.Table:
		return []editor.Node{c.table(current)}
	case *ast.HTMLBlock:
		c.warn("unsupported_html", "HTML-блок пропущен")
		return nil
	default:
		children := c.blockChildren(current)
		if len(children) > 0 {
			return children
		}
	}
	return nil
}

func (c *markdownConverter) blockChildren(parent ast.Node) []editor.Node {
	result := make([]editor.Node, 0)
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		result = append(result, c.block(child)...)
	}
	return result
}

func (c *markdownConverter) inlineChildren(parent ast.Node, marks []editor.Mark) []editor.Node {
	result := make([]editor.Node, 0)
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		result = append(result, c.inline(child, marks)...)
	}
	return result
}

func (c *markdownConverter) inline(node ast.Node, marks []editor.Mark) []editor.Node {
	switch current := node.(type) {
	case *ast.Text:
		result := textWithMarks(string(current.Value(c.source)), marks)
		if current.HardLineBreak() {
			result = append(result, editor.Node{Type: "hardBreak"})
		} else if current.SoftLineBreak() {
			result = append(result, textWithMarks(" ", marks)...)
		}
		return result
	case *ast.String:
		return textWithMarks(string(current.Value), marks)
	case *ast.Emphasis:
		kind := "italic"
		if current.Level >= 2 {
			kind = "bold"
		}
		return c.inlineChildren(current, appendMark(marks, editor.Mark{Type: kind}))
	case *extast.Strikethrough:
		return c.inlineChildren(current, appendMark(marks, editor.Mark{Type: "strike"}))
	case *ast.CodeSpan:
		return c.inlineChildren(current, appendMark(marks, editor.Mark{Type: "code"}))
	case *ast.Link:
		href := string(current.Destination)
		if resolved, ok, ambiguous := c.resolveDocument(href); ok {
			href = resolved
			c.links++
		} else if ambiguous {
			c.warn("ambiguous_link", "Неоднозначная ссылка: "+href)
			return c.inlineChildren(current, marks)
		} else if !safeExternalLink(href) && !strings.HasPrefix(href, "/projects/") {
			c.warn("missing_link", "Ссылка не найдена: "+href)
			return c.inlineChildren(current, marks)
		}
		return c.inlineChildren(current, appendMark(marks, editor.Mark{
			Type: "link", Attrs: map[string]any{"href": href},
		}))
	case *ast.Image:
		alt := inlinePlainText(current, c.source)
		if _, ok, ambiguous := c.resolveImage(string(current.Destination), alt); !ok {
			if ambiguous {
				c.warn("ambiguous_image", "Неоднозначное изображение: "+string(current.Destination))
			} else {
				c.warn("missing_image", "Изображение не найдено: "+string(current.Destination))
			}
		}
		return textWithMarks("[Изображение: "+alt+"]", marks)
	case *ast.RawHTML:
		c.warn("unsupported_html", "Inline HTML пропущен")
		return nil
	case *extast.TaskCheckBox:
		if current.IsChecked {
			return textWithMarks("[x] ", marks)
		}
		return textWithMarks("[ ] ", marks)
	default:
		return c.inlineChildren(current, marks)
	}
}

func (c *markdownConverter) onlyImage(paragraph *ast.Paragraph) (editor.Node, bool) {
	imageNode, ok := paragraph.FirstChild().(*ast.Image)
	if !ok || imageNode.NextSibling() != nil {
		return editor.Node{}, false
	}
	alt := inlinePlainText(imageNode, c.source)
	image, ok, ambiguous := c.resolveImage(string(imageNode.Destination), alt)
	if !ok {
		if ambiguous {
			c.warn("ambiguous_image", "Неоднозначное изображение: "+string(imageNode.Destination))
		} else {
			c.warn("missing_image", "Изображение не найдено: "+string(imageNode.Destination))
		}
		return editor.Node{}, false
	}
	attrs := map[string]any{"src": image.Source, "alt": alt, "caption": ""}
	if image.AssetID != "" {
		attrs["assetId"] = image.AssetID
	}
	if image.Width > 0 {
		attrs["width"] = image.Width
	}
	if image.Height > 0 {
		attrs["height"] = image.Height
	}
	return editor.Node{Type: "imageBlock", Attrs: attrs}, true
}

func (c *markdownConverter) table(table *extast.Table) editor.Node {
	result := editor.Node{Type: "table"}
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		converted := editor.Node{Type: "tableRow"}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			kind := "tableCell"
			if _, header := row.(*extast.TableHeader); header {
				kind = "tableHeader"
			}
			converted.Content = append(converted.Content, editor.Node{
				Type:  kind,
				Attrs: map[string]any{"colspan": 1, "rowspan": 1},
				Content: []editor.Node{{
					Type: "paragraph", Attrs: blockAttrs(), Content: c.inlineChildren(cell, nil),
				}},
			})
		}
		result.Content = append(result.Content, converted)
	}
	return result
}

func (c *markdownConverter) resolveDocument(target string) (string, bool, bool) {
	if safeExternalLink(target) || strings.HasPrefix(target, "/projects/") {
		return target, true, false
	}
	if c.resolver == nil {
		return "", false, false
	}
	return c.resolver.Document(c.sourcePath, target)
}

func (c *markdownConverter) resolveImage(target string, alt string) (Image, bool, bool) {
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
		return Image{Source: target, Alt: alt}, true, false
	}
	if c.resolver == nil {
		return Image{}, false, false
	}
	return c.resolver.Image(c.sourcePath, target)
}

func (c *markdownConverter) warn(code string, message string) {
	c.warnings = append(c.warnings, Warning{Path: c.sourcePath, Code: code, Message: message})
}

func textContent(value string) []editor.Node {
	if value == "" {
		return nil
	}
	return []editor.Node{{Type: "text", Text: value}}
}

func textWithMarks(value string, marks []editor.Mark) []editor.Node {
	if value == "" {
		return nil
	}
	return []editor.Node{{Type: "text", Text: value, Marks: append([]editor.Mark(nil), marks...)}}
}

func appendMark(marks []editor.Mark, mark editor.Mark) []editor.Mark {
	result := append([]editor.Mark(nil), marks...)
	return append(result, mark)
}

func inlinePlainText(parent ast.Node, source []byte) string {
	var builder strings.Builder
	_ = ast.Walk(parent, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch current := node.(type) {
		case *ast.Text:
			builder.Write(current.Value(source))
		case *ast.String:
			builder.Write(current.Value)
		}
		return ast.WalkContinue, nil
	})
	return builder.String()
}

func safeExternalLink(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return ((parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "") ||
		parsed.Scheme == "mailto"
}
