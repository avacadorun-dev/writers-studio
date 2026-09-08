package canvas

import (
	"encoding/json"
	"fmt"
)

const MaxPatchOperations = 500

type PatchOperation struct {
	Op    string          `json:"op"`
	Kind  string          `json:"kind"`
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

func ApplyPatch(raw json.RawMessage, operations []PatchOperation) (json.RawMessage, error) {
	if len(operations) == 0 || len(operations) > MaxPatchOperations {
		return nil, invalid("patch operation count is invalid")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return nil, invalid("content shape is invalid")
	}
	var nodes, edges []json.RawMessage
	if err := json.Unmarshal(envelope["nodes"], &nodes); err != nil {
		return nil, invalid("patch nodes are invalid")
	}
	if err := json.Unmarshal(envelope["edges"], &edges); err != nil {
		return nil, invalid("patch edges are invalid")
	}
	for _, operation := range operations {
		if !validID(operation.ID) {
			return nil, invalid("patch id is invalid")
		}
		var items *[]json.RawMessage
		switch operation.Kind {
		case "node":
			items = &nodes
		case "edge":
			items = &edges
		default:
			return nil, invalid("patch kind is invalid")
		}
		index := rawItemIndex(*items, operation.ID)
		switch operation.Op {
		case "add":
			if index >= 0 || rawItemID(operation.Value) != operation.ID {
				return nil, invalid("patch add target is invalid")
			}
			*items = append(*items, append(json.RawMessage(nil), operation.Value...))
		case "update":
			if index < 0 || rawItemID(operation.Value) != operation.ID {
				return nil, invalid("patch update target is invalid")
			}
			(*items)[index] = append(json.RawMessage(nil), operation.Value...)
		case "delete":
			if index < 0 || len(operation.Value) != 0 && string(operation.Value) != "null" {
				return nil, invalid("patch delete target is invalid")
			}
			*items = append((*items)[:index], (*items)[index+1:]...)
			if operation.Kind == "node" {
				edges = filterEdgesForDeletedNode(edges, operation.ID)
			}
		default:
			return nil, invalid("patch operation is invalid")
		}
	}
	encodedNodes, _ := json.Marshal(nodes)
	encodedEdges, _ := json.Marshal(edges)
	envelope["nodes"] = encodedNodes
	envelope["edges"] = encodedEdges
	result, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode patched canvas: %w", err)
	}
	if _, err := Validate(result); err != nil {
		return nil, err
	}
	return result, nil
}

func rawItemIndex(items []json.RawMessage, id string) int {
	for index, item := range items {
		if rawItemID(item) == id {
			return index
		}
	}
	return -1
}

func rawItemID(raw json.RawMessage) string {
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &identity) != nil {
		return ""
	}
	return identity.ID
}

func filterEdgesForDeletedNode(edges []json.RawMessage, nodeID string) []json.RawMessage {
	result := edges[:0]
	for _, raw := range edges {
		var endpoints struct {
			FromNode string `json:"fromNode"`
			ToNode   string `json:"toNode"`
		}
		if json.Unmarshal(raw, &endpoints) == nil &&
			endpoints.FromNode != nodeID && endpoints.ToNode != nodeID {
			result = append(result, raw)
		}
	}
	return result
}
