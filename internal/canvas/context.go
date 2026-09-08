package canvas

import "encoding/json"

// ValidateSelection verifies that a Canvas selection references only nodes in
// the supplied immutable snapshot. The order is preserved by the caller.
func ValidateSelection(raw json.RawMessage, ids []string) error {
	if len(ids) > 500 {
		return invalid("canvas selection is too large")
	}
	var value document
	if err := json.Unmarshal(raw, &value); err != nil {
		return invalid("content shape is invalid")
	}
	known := make(map[string]bool, len(value.Nodes))
	for _, node := range value.Nodes {
		known[node.ID] = true
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !validID(id) || !known[id] || seen[id] {
			return invalid("canvas selection is invalid")
		}
		seen[id] = true
	}
	return nil
}
