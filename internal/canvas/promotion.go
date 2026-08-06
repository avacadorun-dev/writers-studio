package canvas

import (
	"encoding/json"
	"fmt"
	"strings"
)

type PromotionResult struct {
	Content    json.RawMessage
	SourceText string
}

func PromoteNode(raw json.RawMessage, nodeID, canvasID, documentID string) (PromotionResult, error) {
	if !validID(nodeID) || !validID(canvasID) || !validID(documentID) {
		return PromotionResult{}, invalid("promotion ids are invalid")
	}
	var value document
	if err := json.Unmarshal(raw, &value); err != nil {
		return PromotionResult{}, invalid("content shape is invalid")
	}
	index := -1
	for nodeIndex := range value.Nodes {
		if value.Nodes[nodeIndex].ID == nodeID {
			index = nodeIndex
			break
		}
	}
	if index < 0 {
		return PromotionResult{}, invalid("promotion node does not exist")
	}
	item := &value.Nodes[index]
	var source string
	switch item.Type {
	case "text":
		if item.Text != nil {
			source = *item.Text
		}
	case "link":
		if item.URL != nil {
			source = *item.URL
		}
	case "group":
		if item.Label != nil {
			source = *item.Label
		}
	default:
		return PromotionResult{}, invalid("only an idea, link or labelled group can be promoted")
	}
	if strings.TrimSpace(source) == "" {
		return PromotionResult{}, invalid("promotion source is empty")
	}

	extension := map[string]any{}
	if len(item.Extension) > 0 {
		if err := json.Unmarshal(item.Extension, &extension); err != nil {
			return PromotionResult{}, invalid("promotion node extension is invalid")
		}
	}
	extension["resource"] = map[string]string{"kind": "document", "id": documentID}
	extension["promotedFrom"] = map[string]string{"canvasId": canvasID, "canvasNodeId": nodeID}
	if card, ok := extension["card"].(map[string]any); ok {
		card["promotedDocumentId"] = documentID
		extension["card"] = card
	}
	encodedExtension, err := json.Marshal(extension)
	if err != nil {
		return PromotionResult{}, fmt.Errorf("encode promotion extension: %w", err)
	}
	file := "writers-studio://documents/" + documentID
	item.Type = "file"
	item.File = &file
	item.Text = nil
	item.URL = nil
	item.Label = nil
	item.Extension = encodedExtension
	encoded, err := json.Marshal(value)
	if err != nil {
		return PromotionResult{}, fmt.Errorf("encode promoted canvas: %w", err)
	}
	if _, err := Validate(encoded); err != nil {
		return PromotionResult{}, err
	}
	return PromotionResult{Content: encoded, SourceText: source}, nil
}
