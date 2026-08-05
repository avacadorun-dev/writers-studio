package markdowntree

import (
	"path"
	"regexp"
	"strings"

	"writers.studio/internal/editor"
)

var calloutPattern = regexp.MustCompile("(?i)^\\[!(note|info|tip|abstract|warning|danger|todo)\\][+-]?\\s*")

func rewriteWikiLinks(_ string, source string, _ Resolver) string {
	if !strings.Contains(source, "[[") {
		return source
	}
	var output strings.Builder
	for index := 0; index < len(source); {
		if source[index] == byte(96) {
			count := 1
			for index+count < len(source) && source[index+count] == byte(96) {
				count++
			}
			fence := strings.Repeat(string(byte(96)), count)
			end := strings.Index(source[index+count:], fence)
			if end < 0 {
				output.WriteString(source[index:])
				break
			}
			stop := index + count + end + count
			output.WriteString(source[index:stop])
			index = stop
			continue
		}
		embed := strings.HasPrefix(source[index:], "![[")
		startSize := 2
		if embed {
			startSize = 3
		} else if !strings.HasPrefix(source[index:], "[[") {
			output.WriteByte(source[index])
			index++
			continue
		}
		end := strings.Index(source[index+startSize:], "]]")
		if end < 0 {
			output.WriteByte(source[index])
			index++
			continue
		}
		end += index + startSize
		inside := source[index+startSize : end]
		parts := strings.SplitN(inside, "|", 2)
		target := strings.TrimSpace(parts[0])
		label := target
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			label = strings.TrimSpace(parts[1])
		}
		escapedTarget := strings.ReplaceAll(target, ">", "%3E")
		if embed && SupportedImageExtension(strings.SplitN(target, "#", 2)[0]) {
			output.WriteString("![" + escapeMarkdownLabel(path.Base(target)) + "](<" + escapedTarget + ">)")
		} else {
			output.WriteString("[" + escapeMarkdownLabel(label) + "](<" + escapedTarget + ">)")
		}
		index = end + 2
	}
	return output.String()
}

func escapeMarkdownLabel(value string) string {
	return strings.NewReplacer("\\", "\\\\", "]", "\\]").Replace(value)
}

func asCallout(children []editor.Node) (editor.Node, bool) {
	if len(children) == 0 || children[0].Type != "paragraph" || len(children[0].Content) == 0 ||
		children[0].Content[0].Type != "text" {
		return editor.Node{}, false
	}
	var leading strings.Builder
	for _, child := range children[0].Content {
		if child.Type != "text" {
			break
		}
		leading.WriteString(child.Text)
	}
	match := calloutPattern.FindStringSubmatch(leading.String())
	if match == nil {
		return editor.Node{}, false
	}
	kind := map[string]string{
		"note": "info", "info": "info", "tip": "idea", "abstract": "idea",
		"warning": "warning", "danger": "warning", "todo": "todo",
	}[strings.ToLower(match[1])]
	remaining := len(match[0])
	content := children[0].Content[:0]
	for _, child := range children[0].Content {
		if remaining > 0 && child.Type == "text" {
			if len(child.Text) <= remaining {
				remaining -= len(child.Text)
				continue
			}
			child.Text = child.Text[remaining:]
			remaining = 0
		}
		content = append(content, child)
	}
	children[0].Content = content
	return editor.Node{Type: "callout", Attrs: map[string]any{"kind": kind}, Content: children}, true
}
