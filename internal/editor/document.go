package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

var (
	ErrInvalidDocument = errors.New("invalid editor document")
	allowedNodes       = map[string]bool{
		"doc": true, "paragraph": true, "heading": true, "blockquote": true,
		"bulletList": true, "orderedList": true, "listItem": true,
		"hardBreak": true, "horizontalRule": true, "sceneBreak": true, "text": true,
		"codeBlock": true, "table": true, "tableRow": true, "tableHeader": true,
		"tableCell": true, "imageBlock": true, "mermaidBlock": true, "callout": true,
		"inlineMath": true, "mathBlock": true,
	}
	allowedMarks = map[string]bool{
		"italic": true, "bold": true, "underline": true, "strike": true, "link": true,
		"code": true,
	}
	codeLanguagePattern = regexp.MustCompile(`^[a-zA-Z0-9_+#.-]{1,40}$`)
	uuidPattern         = regexp.MustCompile(`^[a-fA-F0-9-]{36}$`)
)

const CurrentSchemaVersion = 4
const richSchemaVersion = 2
const deepHeadingSchemaVersion = 3
const mathSchemaVersion = 4

type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
	Content []Node         `json:"content,omitempty"`
}

type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// FencedCodeBlock is the lossless payload extracted from a Markdown fenced
// block. Text intentionally is not trimmed or normalized: callers use it to
// migrate legacy paragraphs without changing code whitespace.
type FencedCodeBlock struct {
	Language string
	Text     string
}

const legacyBlockIDPrefix = "__writers_studio_legacy_block_"

// EffectiveTopLevelBlockIDs returns a stable address for every top-level block.
// Older and imported documents may predate persistent blockId metadata. Their
// synthetic IDs are scoped by the caller to an immutable document revision and
// let exact-range tools address those blocks without mutating canonical state on
// read. A staged edit persists the synthetic ID in its proposed snapshot.
func EffectiveTopLevelBlockIDs(blocks []Node) []string {
	result := make([]string, len(blocks))
	used := make(map[string]bool, len(blocks))
	for index, block := range blocks {
		blockID, _ := block.Attrs["blockId"].(string)
		blockID = strings.TrimSpace(blockID)
		if blockID == "" {
			continue
		}
		result[index] = blockID
		used[blockID] = true
	}
	for index := range blocks {
		if result[index] != "" {
			continue
		}
		candidate := fmt.Sprintf("%s%d", legacyBlockIDPrefix, index)
		for suffix := 1; used[candidate]; suffix++ {
			candidate = fmt.Sprintf("%s%d_%d", legacyBlockIDPrefix, index, suffix)
		}
		result[index] = candidate
		used[candidate] = true
	}
	return result
}

func Validate(raw json.RawMessage) error {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	if root.Type != "doc" {
		return fmt.Errorf("%w: root type must be doc", ErrInvalidDocument)
	}
	version := 1
	if schemaVersion, exists := root.Attrs["schemaVersion"]; exists {
		parsedVersion, ok := numericAttr(schemaVersion)
		if !ok || parsedVersion < 1 || parsedVersion > CurrentSchemaVersion {
			return fmt.Errorf("%w: unsupported schema version", ErrInvalidDocument)
		}
		version = parsedVersion
	}
	if version < richSchemaVersion && requiresRichSchema(root) {
		return fmt.Errorf("%w: rich blocks require schema version %d", ErrInvalidDocument, richSchemaVersion)
	}
	if version < deepHeadingSchemaVersion && requiresDeepHeadingSchema(root) {
		return fmt.Errorf("%w: headings 4..6 require schema version %d", ErrInvalidDocument, deepHeadingSchemaVersion)
	}
	if version < mathSchemaVersion && requiresMathSchema(root) {
		return fmt.Errorf("%w: formulas require schema version %d", ErrInvalidDocument, mathSchemaVersion)
	}
	return validateNode(root, true)
}

func requiresMathSchema(node Node) bool {
	if node.Type == "inlineMath" || node.Type == "mathBlock" {
		return true
	}
	for _, child := range node.Content {
		if requiresMathSchema(child) {
			return true
		}
	}
	return false
}

func requiresDeepHeadingSchema(node Node) bool {
	if node.Type == "heading" {
		level, _ := numericAttr(node.Attrs["level"])
		if level > 3 {
			return true
		}
	}
	for _, child := range node.Content {
		if requiresDeepHeadingSchema(child) {
			return true
		}
	}
	return false
}

func requiresRichSchema(node Node) bool {
	switch node.Type {
	case "codeBlock", "table", "tableRow", "tableHeader", "tableCell", "imageBlock",
		"mermaidBlock", "callout":
		return true
	}
	for _, mark := range node.Marks {
		if mark.Type == "code" {
			return true
		}
	}
	for _, child := range node.Content {
		if requiresRichSchema(child) {
			return true
		}
	}
	return false
}

func SchemaVersion(raw json.RawMessage) (int, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return 0, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	value, exists := root.Attrs["schemaVersion"]
	if !exists {
		return 1, nil
	}
	version, ok := numericAttr(value)
	if !ok || version < 1 || version > CurrentSchemaVersion {
		return 0, fmt.Errorf("%w: unsupported schema version", ErrInvalidDocument)
	}
	return version, nil
}

// NormalizeSchemaVersion upgrades only documents that use rich blocks, deep
// headings or formulas. Plain schema-v1 documents are returned unchanged so opening or
// editing an existing project does not perform a bulk format conversion.
func NormalizeSchemaVersion(raw json.RawMessage) (json.RawMessage, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	if root.Type != "doc" {
		return nil, fmt.Errorf("%w: root type must be doc", ErrInvalidDocument)
	}
	version, err := SchemaVersion(raw)
	if err != nil {
		return nil, err
	}
	targetVersion := version
	if requiresMathSchema(root) && targetVersion < mathSchemaVersion {
		targetVersion = mathSchemaVersion
	}
	if requiresDeepHeadingSchema(root) && targetVersion < deepHeadingSchemaVersion {
		targetVersion = deepHeadingSchemaVersion
	}
	if requiresRichSchema(root) && targetVersion < richSchemaVersion {
		targetVersion = richSchemaVersion
	}
	if targetVersion == version {
		return raw, Validate(raw)
	}
	if root.Attrs == nil {
		root.Attrs = map[string]any{}
	}
	root.Attrs["schemaVersion"] = targetVersion
	result, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode normalized document", ErrInvalidDocument)
	}
	return result, Validate(result)
}

// ParseFencedCodeBlock recognizes a complete Markdown fenced block. Exactly
// one line ending that separates the code from the closing fence is removed;
// every line ending, tab, space and empty line inside the code is preserved.
func ParseFencedCodeBlock(value string) (FencedCodeBlock, bool) {
	firstLineEnd := strings.IndexByte(value, '\n')
	if firstLineEnd < 0 {
		return FencedCodeBlock{}, false
	}
	openingLine := strings.TrimSuffix(value[:firstLineEnd], "\r")
	if openingLine == "" || (openingLine[0] != '`' && openingLine[0] != '~') {
		return FencedCodeBlock{}, false
	}
	fenceCharacter := openingLine[0]
	fenceLength := 0
	for fenceLength < len(openingLine) && openingLine[fenceLength] == fenceCharacter {
		fenceLength++
	}
	if fenceLength < 3 {
		return FencedCodeBlock{}, false
	}
	info := strings.TrimSpace(openingLine[fenceLength:])
	language := ""
	if info != "" {
		language = strings.Fields(info)[0]
		if !codeLanguagePattern.MatchString(language) {
			return FencedCodeBlock{}, false
		}
	}

	remainder := value[firstLineEnd+1:]
	// A final line ending belongs to the Markdown container, not the fence.
	remainder = strings.TrimSuffix(remainder, "\n")
	remainder = strings.TrimSuffix(remainder, "\r")
	closingStart := strings.LastIndexByte(remainder, '\n')
	bodyEnd := 0
	closingLine := remainder
	if closingStart >= 0 {
		bodyEnd = closingStart
		if bodyEnd > 0 && remainder[bodyEnd-1] == '\r' {
			bodyEnd--
		}
		closingLine = remainder[closingStart+1:]
	}
	closingLine = strings.TrimSuffix(closingLine, "\r")
	trimmedClosing := strings.TrimLeft(closingLine, " \t")
	if len(closingLine)-len(trimmedClosing) > 3 {
		return FencedCodeBlock{}, false
	}
	closingFenceLength := 0
	for closingFenceLength < len(trimmedClosing) &&
		trimmedClosing[closingFenceLength] == fenceCharacter {
		closingFenceLength++
	}
	if closingFenceLength < fenceLength ||
		strings.TrimSpace(trimmedClosing[closingFenceLength:]) != "" {
		return FencedCodeBlock{}, false
	}
	body := ""
	if closingStart >= 0 {
		body = remainder[:bodyEnd]
	}
	return FencedCodeBlock{Language: language, Text: body}, true
}

// NormalizeFencedCodeBlocks migrates legacy top-level paragraphs that consist
// entirely of a Markdown fenced block into native codeBlock nodes.
func NormalizeFencedCodeBlocks(raw json.RawMessage) (json.RawMessage, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	if root.Type != "doc" {
		return nil, fmt.Errorf("%w: root type must be doc", ErrInvalidDocument)
	}
	changed := false
	for index := range root.Content {
		block := &root.Content[index]
		if block.Type != "paragraph" {
			continue
		}
		fenced, ok := ParseFencedCodeBlock(inlineText(*block))
		if !ok {
			continue
		}
		attrs := map[string]any{}
		if blockID, exists := block.Attrs["blockId"]; exists {
			attrs["blockId"] = blockID
		}
		if fenced.Language != "" {
			attrs["language"] = fenced.Language
		}
		*block = Node{Type: "codeBlock", Attrs: attrs}
		if fenced.Text != "" {
			block.Content = []Node{{Type: "text", Text: fenced.Text}}
		}
		changed = true
	}
	if !changed {
		return raw, Validate(raw)
	}
	if root.Attrs == nil {
		root.Attrs = map[string]any{}
	}
	version := 1
	if current, ok := numericAttr(root.Attrs["schemaVersion"]); ok {
		version = current
	}
	if version < richSchemaVersion {
		root.Attrs["schemaVersion"] = richSchemaVersion
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode fenced code migration", ErrInvalidDocument)
	}
	return result, Validate(result)
}

// ParseInlineCodeNodes converts Markdown-style backtick spans into canonical
// code marks. Delimiters may contain more than one backtick, which lets inline
// code itself contain shorter backtick runs. Text inside the span is never
// trimmed or otherwise normalized.
func ParseInlineCodeNodes(value string, marks []Mark) ([]Node, bool) {
	if value == "" || hasMark(marks, "code") {
		return []Node{{Type: "text", Text: value, Marks: marks}}, false
	}
	result := make([]Node, 0, 3)
	cursor := 0
	search := 0
	changed := false
	for search < len(value) {
		opening := strings.IndexByte(value[search:], '`')
		if opening < 0 {
			break
		}
		opening += search
		openingLength := backtickRunLength(value, opening)
		closing := matchingBacktickRun(value, opening+openingLength, openingLength)
		if closing < 0 {
			search = opening + openingLength
			continue
		}
		if opening > cursor {
			result = append(result, Node{
				Type: "text", Text: value[cursor:opening], Marks: cloneMarks(marks),
			})
		}
		codeMarks := cloneMarks(marks)
		codeMarks = append(codeMarks, Mark{Type: "code"})
		result = append(result, Node{
			Type: "text", Text: value[opening+openingLength : closing], Marks: codeMarks,
		})
		cursor = closing + openingLength
		search = cursor
		changed = true
	}
	if !changed {
		return []Node{{Type: "text", Text: value, Marks: marks}}, false
	}
	if cursor < len(value) {
		result = append(result, Node{
			Type: "text", Text: value[cursor:], Marks: cloneMarks(marks),
		})
	}
	return result, true
}

// NormalizeInlineCodeMarks migrates literal Markdown backtick spans throughout
// prose containers. Native code blocks and text that already has a code mark
// are left byte-for-byte unchanged.
func NormalizeInlineCodeMarks(raw json.RawMessage) (json.RawMessage, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	if root.Type != "doc" {
		return nil, fmt.Errorf("%w: root type must be doc", ErrInvalidDocument)
	}
	if !normalizeInlineCodeChildren(&root) {
		return NormalizeSchemaVersion(raw)
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%w: encode inline code migration", ErrInvalidDocument)
	}
	return NormalizeSchemaVersion(result)
}

func normalizeInlineCodeChildren(node *Node) bool {
	if node.Type == "codeBlock" {
		return false
	}
	changed := false
	children := make([]Node, 0, len(node.Content))
	for _, child := range node.Content {
		if child.Type == "text" {
			parts, parsed := ParseInlineCodeNodes(child.Text, child.Marks)
			if parsed {
				for index := range parts {
					parts[index].Attrs = child.Attrs
				}
				children = append(children, parts...)
				changed = true
				continue
			}
		} else if normalizeInlineCodeChildren(&child) {
			changed = true
		}
		if len(children) > 0 && children[len(children)-1].Type == "text" &&
			child.Type == "text" &&
			reflect.DeepEqual(children[len(children)-1].Marks, child.Marks) &&
			reflect.DeepEqual(children[len(children)-1].Attrs, child.Attrs) {
			children[len(children)-1].Text += child.Text
			changed = true
			continue
		}
		children = append(children, child)
	}
	if changed {
		node.Content = children
	}
	return changed
}

func backtickRunLength(value string, start int) int {
	end := start
	for end < len(value) && value[end] == '`' {
		end++
	}
	return end - start
}

func matchingBacktickRun(value string, start, expectedLength int) int {
	for start < len(value) {
		candidate := strings.IndexByte(value[start:], '`')
		if candidate < 0 {
			return -1
		}
		candidate += start
		length := backtickRunLength(value, candidate)
		if length == expectedLength {
			return candidate
		}
		start = candidate + length
	}
	return -1
}

func hasMark(marks []Mark, markType string) bool {
	for _, mark := range marks {
		if mark.Type == markType {
			return true
		}
	}
	return false
}

func cloneMarks(marks []Mark) []Mark {
	if len(marks) == 0 {
		return nil
	}
	return append([]Mark(nil), marks...)
}

func validateNode(node Node, root bool) error {
	if !allowedNodes[node.Type] {
		return fmt.Errorf("%w: unknown node %q", ErrInvalidDocument, node.Type)
	}
	if node.Type == "heading" {
		level, ok := numericAttr(node.Attrs["level"])
		if !ok || level < 1 || level > 6 {
			return fmt.Errorf("%w: heading level must be 1..6", ErrInvalidDocument)
		}
	}
	if node.Type == "paragraph" || node.Type == "heading" || node.Type == "blockquote" {
		if blockID, ok := node.Attrs["blockId"]; ok {
			if _, ok := blockID.(string); !ok {
				return fmt.Errorf("%w: blockId must be a string", ErrInvalidDocument)
			}
		}
	}
	if blockID, ok := node.Attrs["blockId"]; ok {
		value, valid := blockID.(string)
		if !valid || len(value) > 160 {
			return fmt.Errorf("%w: blockId must be a short string", ErrInvalidDocument)
		}
	}
	if node.Type == "codeBlock" {
		language, parsed := optionalStringAttr(node.Attrs, "language")
		if attributeHasNonStringValue(node.Attrs, "language", parsed) ||
			(language != "" && !codeLanguagePattern.MatchString(language)) {
			return fmt.Errorf("%w: invalid code language", ErrInvalidDocument)
		}
	}
	if node.Type == "callout" {
		kind, _ := optionalStringAttr(node.Attrs, "kind")
		if kind != "info" && kind != "idea" && kind != "warning" && kind != "todo" {
			return fmt.Errorf("%w: invalid callout kind", ErrInvalidDocument)
		}
	}
	if node.Type == "imageBlock" {
		if err := validateImageBlock(node); err != nil {
			return err
		}
	}
	if node.Type == "mermaidBlock" {
		code, codeOK := optionalStringAttr(node.Attrs, "code")
		caption, captionOK := optionalStringAttr(node.Attrs, "caption")
		if !codeOK || attributeHasNonStringValue(node.Attrs, "caption", captionOK) ||
			strings.TrimSpace(code) == "" || len(code) > 100_000 || len(caption) > 1_000 {
			return fmt.Errorf("%w: invalid Mermaid block", ErrInvalidDocument)
		}
	}
	if node.Type == "inlineMath" || node.Type == "mathBlock" {
		latex, latexOK := optionalStringAttr(node.Attrs, "latex")
		description, descriptionOK := optionalStringAttr(node.Attrs, "description")
		if !latexOK || attributeHasNonStringValue(node.Attrs, "description", descriptionOK) ||
			len(latex) > 10_000 || len(description) > 1_000 || len(node.Content) > 0 {
			return fmt.Errorf("%w: invalid formula", ErrInvalidDocument)
		}
	}
	if node.Type == "tableCell" || node.Type == "tableHeader" {
		for _, attribute := range []string{"colspan", "rowspan"} {
			if value, exists := node.Attrs[attribute]; exists {
				number, ok := numericAttr(value)
				if !ok || number < 1 || number > 100 {
					return fmt.Errorf("%w: invalid table cell span", ErrInvalidDocument)
				}
			}
		}
	}
	if node.Type == "text" && node.Text == "" {
		return fmt.Errorf("%w: empty text node", ErrInvalidDocument)
	}
	if root && node.Text != "" {
		return fmt.Errorf("%w: doc cannot contain text directly", ErrInvalidDocument)
	}
	for _, mark := range node.Marks {
		if !allowedMarks[mark.Type] {
			return fmt.Errorf("%w: unknown mark %q", ErrInvalidDocument, mark.Type)
		}
		if mark.Type == "link" {
			href, _ := mark.Attrs["href"].(string)
			if !safeLink(href) {
				return fmt.Errorf("%w: unsafe link", ErrInvalidDocument)
			}
		}
	}
	for _, child := range node.Content {
		if err := validateNode(child, false); err != nil {
			return err
		}
	}
	return nil
}

func PlainText(raw json.RawMessage) (string, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	var builder strings.Builder
	appendText(&builder, root)
	return strings.TrimSpace(builder.String()), nil
}

// TopLevelBlockText returns the textual representation used by stable editor
// ranges for a top-level block. Offsets are Unicode code points, not bytes or
// UTF-16 code units, so anchors remain portable between Go and the browser.
func TopLevelBlockText(raw json.RawMessage, blockID string) (string, bool, error) {
	if err := Validate(raw); err != nil {
		return "", false, err
	}
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", false, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	ids := EffectiveTopLevelBlockIDs(root.Content)
	for index, id := range ids {
		if id != blockID {
			continue
		}
		var builder strings.Builder
		appendInlineText(&builder, root.Content[index])
		return builder.String(), true, nil
	}
	return "", false, nil
}

func ValidateAnchor(raw json.RawMessage, anchor domain.AnnotationAnchor) error {
	text, found, err := TopLevelBlockText(raw, anchor.BlockID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: annotation block not found", ErrInvalidDocument)
	}
	runes := []rune(text)
	if anchor.Start < 0 || anchor.End <= anchor.Start || anchor.End > len(runes) {
		return fmt.Errorf("%w: annotation range is outside its block", ErrInvalidDocument)
	}
	if string(runes[anchor.Start:anchor.End]) != anchor.ExactText {
		return fmt.Errorf("%w: annotation text does not match its range", ErrInvalidDocument)
	}
	return nil
}

// AnchorAtRange builds the portable anchor representation used by the browser.
// Offsets and the 32-character context windows are Unicode code points.
func AnchorAtRange(
	raw json.RawMessage,
	blockID string,
	start int,
	end int,
) (domain.AnnotationAnchor, error) {
	text, found, err := TopLevelBlockText(raw, blockID)
	if err != nil {
		return domain.AnnotationAnchor{}, err
	}
	if !found {
		return domain.AnnotationAnchor{}, fmt.Errorf("%w: annotation block not found", ErrInvalidDocument)
	}
	runes := []rune(text)
	if start < 0 || end <= start || end > len(runes) {
		return domain.AnnotationAnchor{}, fmt.Errorf("%w: annotation range is outside its block", ErrInvalidDocument)
	}
	prefixStart := max(0, start-32)
	suffixEnd := min(len(runes), end+32)
	return domain.AnnotationAnchor{
		BlockID:   blockID,
		Start:     start,
		End:       end,
		ExactText: string(runes[start:end]),
		Prefix:    string(runes[prefixStart:start]),
		Suffix:    string(runes[end:suffixEnd]),
	}, nil
}

func appendInlineText(builder *strings.Builder, node Node) {
	switch node.Type {
	case "text":
		builder.WriteString(node.Text)
	case "hardBreak":
		builder.WriteByte('\n')
	case "inlineMath":
		builder.WriteString("[Формула: " + stringAttr(node.Attrs, "latex") + "]")
	}
	for _, child := range node.Content {
		appendInlineText(builder, child)
	}
}

// ReplaceText applies a literal, case-insensitive replacement only inside text
// nodes. Node attributes, marks, block IDs, and source references are preserved.
func ReplaceText(
	raw json.RawMessage,
	search string,
	replacement string,
) (json.RawMessage, int, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil, 0, fmt.Errorf("%w: empty search text", ErrInvalidDocument)
	}
	if err := Validate(raw); err != nil {
		return nil, 0, err
	}
	expression, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(search))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: invalid search text", ErrInvalidDocument)
	}
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, 0, fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	count := replaceTextNode(&root, expression, replacement)
	if count == 0 {
		return raw, 0, nil
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: encode replacement", ErrInvalidDocument)
	}
	return result, count, nil
}

func replaceTextNode(node *Node, expression *regexp.Regexp, replacement string) int {
	count := 0
	if node.Type == "text" {
		matches := expression.FindAllStringIndex(node.Text, -1)
		count += len(matches)
		node.Text = expression.ReplaceAllStringFunc(node.Text, func(string) string {
			return replacement
		})
		return count
	}
	content := make([]Node, 0, len(node.Content))
	for index := range node.Content {
		count += replaceTextNode(&node.Content[index], expression, replacement)
		if node.Content[index].Type == "text" && node.Content[index].Text == "" {
			continue
		}
		content = append(content, node.Content[index])
	}
	node.Content = content
	return count
}

func appendText(builder *strings.Builder, node Node) {
	switch node.Type {
	case "text":
		builder.WriteString(node.Text)
	case "hardBreak":
		builder.WriteByte('\n')
	case "inlineMath":
		builder.WriteString("[Формула: " + stringAttr(node.Attrs, "latex") + "]")
	case "sceneBreak":
		builder.WriteString("\n\n* * *\n\n")
	case "horizontalRule":
		builder.WriteString("\n\n---\n\n")
	case "imageBlock":
		label := firstNonEmpty(stringAttr(node.Attrs, "caption"), stringAttr(node.Attrs, "alt"))
		if label != "" {
			builder.WriteString("[Изображение: " + label + "]")
		} else {
			builder.WriteString("[Изображение]")
		}
	case "mermaidBlock":
		if caption := stringAttr(node.Attrs, "caption"); caption != "" {
			builder.WriteString(caption)
			builder.WriteByte('\n')
		}
		builder.WriteString(stringAttr(node.Attrs, "code"))
	case "mathBlock":
		builder.WriteString("Формула: " + stringAttr(node.Attrs, "latex"))
	}
	for _, child := range node.Content {
		appendText(builder, child)
	}
	switch node.Type {
	case "tableCell", "tableHeader":
		builder.WriteByte('\t')
	case "tableRow":
		builder.WriteByte('\n')
	case "paragraph", "heading", "blockquote", "listItem", "codeBlock", "callout", "table",
		"imageBlock", "mermaidBlock", "mathBlock":
		builder.WriteString("\n\n")
	}
}

func FromTranscript(segments []domain.TranscriptSegment) (json.RawMessage, error) {
	content := make([]Node, 0, len(segments))
	dialogueNext := false
	var paragraph *Node
	flushParagraph := func() {
		if paragraph == nil {
			return
		}
		content = append(content, *paragraph)
		paragraph = nil
	}
	for _, segment := range segments {
		if segment.Command != nil && segment.Command.Executed {
			switch segment.Command.Kind {
			case "new_paragraph":
				flushParagraph()
			case "scene_break":
				flushParagraph()
				content = append(content, Node{Type: "sceneBreak"})
			case "dialogue_start":
				flushParagraph()
				dialogueNext = true
			}
			continue
		}
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		if dialogueNext {
			text = "— " + strings.TrimLeft(text, "—–- \t")
			dialogueNext = false
		}
		if paragraph == nil {
			paragraph = &Node{
				Type: "paragraph",
				Attrs: map[string]any{
					"blockId":    uuid.NewString(),
					"sourceRefs": []string{},
				},
			}
		}
		sourceRefs := paragraph.Attrs["sourceRefs"].([]string)
		paragraph.Attrs["sourceRefs"] = append(sourceRefs, segment.ID)
		if len(paragraph.Content) > 0 {
			paragraph.Content = append(paragraph.Content, Node{Type: "text", Text: " "})
		}
		paragraph.Content = append(paragraph.Content, Node{Type: "text", Text: text})
	}
	flushParagraph()
	if len(content) == 0 {
		content = append(content, Node{
			Type: "paragraph",
			Attrs: map[string]any{
				"blockId":    uuid.NewString(),
				"sourceRefs": []string{},
			},
		})
	}
	return json.Marshal(Node{
		Type: "doc", Attrs: map[string]any{"schemaVersion": 1}, Content: content,
	})
}

func Markdown(raw json.RawMessage) (string, error) {
	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("%w: malformed JSON", ErrInvalidDocument)
	}
	var builder strings.Builder
	appendMarkdown(&builder, root)
	return strings.TrimRight(builder.String(), "\r\n"), nil
}

func appendMarkdown(builder *strings.Builder, node Node) {
	switch node.Type {
	case "text":
		text := node.Text
		for _, mark := range node.Marks {
			switch mark.Type {
			case "bold":
				text = "**" + text + "**"
			case "italic":
				text = "*" + text + "*"
			case "strike":
				text = "~~" + text + "~~"
			case "link":
				if href, ok := mark.Attrs["href"].(string); ok {
					text = "[" + text + "](" + href + ")"
				}
			case "code":
				text = markdownInlineCode(text)
			case "underline":
				text = "<u>" + text + "</u>"
			}
		}
		builder.WriteString(text)
	case "inlineMath":
		builder.WriteString("$" + stringAttr(node.Attrs, "latex") + "$")
	case "heading":
		level, _ := numericAttr(node.Attrs["level"])
		builder.WriteString(strings.Repeat("#", max(1, level)))
		builder.WriteByte(' ')
	case "blockquote":
		var content strings.Builder
		for _, child := range node.Content {
			appendMarkdown(&content, child)
		}
		appendMarkdownQuote(builder, "", content.String())
		return
	case "hardBreak":
		builder.WriteByte('\n')
	case "sceneBreak":
		builder.WriteString("\n\n* * *\n\n")
	case "horizontalRule":
		builder.WriteString("\n\n---\n\n")
	case "codeBlock":
		language := stringAttr(node.Attrs, "language")
		code := inlineText(node)
		fence := markdownCodeFence(code)
		builder.WriteString(fence + language + "\n" + code + "\n" + fence + "\n\n")
		return
	case "imageBlock":
		alt := strings.ReplaceAll(stringAttr(node.Attrs, "alt"), "]", "\\]")
		src := stringAttr(node.Attrs, "src")
		builder.WriteString("![" + alt + "](" + src + ")")
		if caption := stringAttr(node.Attrs, "caption"); caption != "" {
			builder.WriteString("\n\n*" + caption + "*")
		}
		builder.WriteString("\n\n")
		return
	case "mermaidBlock":
		builder.WriteString("```mermaid\n" + stringAttr(node.Attrs, "code") + "\n```\n")
		if caption := stringAttr(node.Attrs, "caption"); caption != "" {
			builder.WriteString("\n*" + caption + "*\n")
		}
		builder.WriteByte('\n')
		return
	case "mathBlock":
		builder.WriteString("$$\n" + stringAttr(node.Attrs, "latex") + "\n$$\n\n")
		return
	case "table":
		appendMarkdownTable(builder, node)
		return
	case "bulletList":
		appendMarkdownList(builder, node, false)
		return
	case "orderedList":
		appendMarkdownList(builder, node, true)
		return
	case "callout":
		var content strings.Builder
		for _, child := range node.Content {
			appendMarkdown(&content, child)
		}
		kind := map[string]string{
			"info": "NOTE", "idea": "TIP", "warning": "WARNING", "todo": "TODO",
		}[stringAttr(node.Attrs, "kind")]
		appendMarkdownQuote(builder, "[!"+kind+"]", content.String())
		return
	}
	for _, child := range node.Content {
		appendMarkdown(builder, child)
	}
	switch node.Type {
	case "paragraph", "heading", "blockquote", "listItem":
		builder.WriteString("\n\n")
	}
}

func markdownCodeFence(code string) string {
	longest := 0
	current := 0
	for _, character := range code {
		if character == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return strings.Repeat("`", max(3, longest+1))
}

func markdownInlineCode(code string) string {
	longest := 0
	current := 0
	for _, character := range code {
		if character == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	fence := strings.Repeat("`", max(1, longest+1))
	return fence + code + fence
}

func appendMarkdownQuote(builder *strings.Builder, header string, value string) {
	if header != "" {
		builder.WriteString("> " + header + "\n")
	}
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		builder.WriteString(">")
		if line != "" {
			builder.WriteString(" " + line)
		}
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
}

func appendMarkdownList(builder *strings.Builder, list Node, ordered bool) {
	start := 1
	if ordered {
		if value, ok := numericAttr(list.Attrs["start"]); ok && value > 0 {
			start = value
		}
	}
	itemIndex := 0
	for _, item := range list.Content {
		if item.Type != "listItem" {
			continue
		}
		var content strings.Builder
		for _, child := range item.Content {
			appendMarkdown(&content, child)
		}
		lines := strings.Split(strings.TrimSpace(content.String()), "\n")
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", start+itemIndex)
		}
		indent := strings.Repeat(" ", len(prefix))
		for index, line := range lines {
			if index == 0 {
				builder.WriteString(prefix)
			} else {
				builder.WriteString(indent)
			}
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
		itemIndex++
	}
	builder.WriteByte('\n')
}

func appendMarkdownTable(builder *strings.Builder, table Node) {
	rows := make([][]string, 0, len(table.Content))
	for _, row := range table.Content {
		if row.Type != "tableRow" {
			continue
		}
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			var content strings.Builder
			for _, child := range cell.Content {
				appendMarkdown(&content, child)
			}
			value := strings.Join(strings.Fields(strings.TrimSpace(content.String())), " ")
			cells = append(cells, strings.ReplaceAll(value, "|", "\\|"))
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return
	}
	columns := 0
	for _, row := range rows {
		columns = max(columns, len(row))
	}
	writeRow := func(row []string) {
		builder.WriteString("| ")
		for column := 0; column < columns; column++ {
			if column < len(row) {
				builder.WriteString(row[column])
			}
			builder.WriteString(" | ")
		}
		builder.WriteByte('\n')
	}
	writeRow(rows[0])
	separator := make([]string, columns)
	for index := range separator {
		separator[index] = "---"
	}
	writeRow(separator)
	for _, row := range rows[1:] {
		writeRow(row)
	}
	builder.WriteByte('\n')
}

func inlineText(node Node) string {
	var builder strings.Builder
	for _, child := range node.Content {
		if child.Type == "text" {
			builder.WriteString(child.Text)
		} else if child.Type == "hardBreak" {
			builder.WriteByte('\n')
		} else {
			builder.WriteString(inlineText(child))
		}
	}
	return builder.String()
}

func validateImageBlock(node Node) error {
	src, srcOK := optionalStringAttr(node.Attrs, "src")
	alt, altOK := optionalStringAttr(node.Attrs, "alt")
	caption, captionOK := optionalStringAttr(node.Attrs, "caption")
	assetID, assetIDOK := optionalStringAttr(node.Attrs, "assetId")
	if !srcOK || attributeHasNonStringValue(node.Attrs, "alt", altOK) ||
		attributeHasNonStringValue(node.Attrs, "caption", captionOK) ||
		attributeHasNonStringValue(node.Attrs, "assetId", assetIDOK) {
		return fmt.Errorf("%w: invalid image block", ErrInvalidDocument)
	}
	if !safeImageSource(src) || len(alt) > 1_000 || len(caption) > 1_000 {
		return fmt.Errorf("%w: invalid image block", ErrInvalidDocument)
	}
	if assetID != "" && !uuidPattern.MatchString(assetID) {
		return fmt.Errorf("%w: invalid image asset", ErrInvalidDocument)
	}
	if strings.HasPrefix(src, "/api/media/") {
		expectedSource := "/api/media/" + assetID + "/content"
		if assetID == "" || src != expectedSource {
			return fmt.Errorf("%w: image source does not match its asset", ErrInvalidDocument)
		}
	} else if assetID != "" {
		return fmt.Errorf("%w: image asset requires a project media source", ErrInvalidDocument)
	}
	for _, attribute := range []string{"width", "height"} {
		if value, exists := node.Attrs[attribute]; exists && value != nil {
			number, ok := numericAttr(value)
			if !ok || number < 1 || number > 20_000 {
				return fmt.Errorf("%w: invalid image dimensions", ErrInvalidDocument)
			}
		}
	}
	return nil
}

func attributeHasNonStringValue(attrs map[string]any, key string, parsed bool) bool {
	value, exists := attrs[key]
	return exists && value != nil && !parsed
}

func safeImageSource(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 2_000 {
		return false
	}
	if strings.HasPrefix(value, "/api/media/") && strings.HasSuffix(value, "/content") {
		id := strings.TrimSuffix(strings.TrimPrefix(value, "/api/media/"), "/content")
		return uuidPattern.MatchString(id)
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func optionalStringAttr(attrs map[string]any, key string) (string, bool) {
	value, exists := attrs[key]
	if !exists || value == nil {
		return "", false
	}
	result, ok := value.(string)
	return result, ok
}

func stringAttr(attrs map[string]any, key string) string {
	value, _ := optionalStringAttr(attrs, key)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safeLink(href string) bool {
	href = strings.TrimSpace(href)
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "mailto:") {
		return true
	}
	parsed, err := url.Parse(href)
	if err != nil || parsed.RawQuery != "" || parsed.User != nil || parsed.Host != "" {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(parts) == 4 && parts[0] == "projects" && uuidPattern.MatchString(parts[1]) &&
		parts[2] == "studio" && uuidPattern.MatchString(parts[3])
}

func numericAttr(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int(number), number == float64(int(number))
}
