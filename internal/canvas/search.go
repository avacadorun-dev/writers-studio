package canvas

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SearchableNode struct {
	NodeID   string
	Title    string
	Text     string
	Resource *ResourceRef
}

func SearchableNodes(raw json.RawMessage) ([]SearchableNode, error) {
	var value document
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%w: content shape is invalid", ErrInvalidDocument)
	}
	result := make([]SearchableNode, 0, len(value.Nodes))
	for _, item := range value.Nodes {
		parts := make([]string, 0, 6)
		title := "Элемент Canvas"
		switch item.Type {
		case "text":
			if item.Text != nil {
				parts = append(parts, *item.Text)
				title = firstSearchLine(*item.Text, title)
			}
		case "file":
			if item.File != nil {
				parts = append(parts, *item.File)
				title = firstSearchLine(*item.File, title)
			}
		case "link":
			if item.URL != nil {
				parts = append(parts, *item.URL)
				title = *item.URL
			}
		case "group":
			if item.Label != nil {
				parts = append(parts, *item.Label)
				title = firstSearchLine(*item.Label, title)
			}
		}
		var resource *ResourceRef
		if len(item.Extension) > 0 {
			var extension map[string]any
			if err := json.Unmarshal(item.Extension, &extension); err != nil {
				return nil, fmt.Errorf("%w: node extension is invalid", ErrInvalidDocument)
			}
			for _, key := range []string{"card", "preview", "element"} {
				if extensionValue, exists := extension[key]; exists {
					if encoded, err := json.Marshal(extensionValue); err == nil {
						parts = append(parts, string(encoded))
					}
				}
			}
			if rawResource, exists := extension["resource"]; exists {
				encoded, _ := json.Marshal(rawResource)
				var parsed ResourceRef
				if json.Unmarshal(encoded, &parsed) == nil && parsed.ID != "" {
					resource = &parsed
				}
			}
			if preview, ok := extension["preview"].(map[string]any); ok {
				if previewTitle, ok := preview["title"].(string); ok && strings.TrimSpace(previewTitle) != "" {
					title = previewTitle
				}
			}
		}
		result = append(result, SearchableNode{
			NodeID: item.ID, Title: title, Text: strings.Join(parts, "\n"), Resource: resource,
		})
	}
	return result, nil
}

func firstSearchLine(value, fallback string) string {
	line := strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	line = strings.TrimSpace(strings.TrimLeft(line, "#"))
	if line == "" {
		return fallback
	}
	return line
}
