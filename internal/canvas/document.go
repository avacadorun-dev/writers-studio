package canvas

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

const (
	SchemaVersion = 1
	MaxNodes      = 2_000
	MaxEdges      = 4_000
	MaxTextRunes  = 100_000
)

var (
	ErrInvalidDocument = errors.New("invalid canvas document")
	colorPattern       = regexp.MustCompile(`^(?:[1-6]|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8})$`)
)

type ResourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type References struct {
	DocumentIDs []string
	MediaIDs    []string
}

type document struct {
	Nodes []node `json:"nodes"`
	Edges []edge `json:"edges"`
}

type node struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	X         float64         `json:"x"`
	Y         float64         `json:"y"`
	Width     float64         `json:"width"`
	Height    float64         `json:"height"`
	Color     string          `json:"color"`
	Text      *string         `json:"text"`
	File      *string         `json:"file"`
	Label     *string         `json:"label"`
	Extension json.RawMessage `json:"x-writers-studio"`
}

type edge struct {
	ID       string `json:"id"`
	FromNode string `json:"fromNode"`
	FromSide string `json:"fromSide"`
	FromEnd  string `json:"fromEnd"`
	ToNode   string `json:"toNode"`
	ToSide   string `json:"toSide"`
	ToEnd    string `json:"toEnd"`
	Color    string `json:"color"`
	Label    string `json:"label"`
}

type resourceExtension struct {
	Resource *ResourceRef `json:"resource"`
}

func EmptyDocument() json.RawMessage {
	return json.RawMessage(`{"nodes":[],"edges":[],"x-writers-studio":{"schemaVersion":1}}`)
}

func Validate(raw json.RawMessage) (References, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return References{}, invalid("content is not valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic map[string]any
	if err := decoder.Decode(&generic); err != nil || generic == nil {
		return References{}, invalid("content must be a JSON object")
	}

	var value document
	if err := json.Unmarshal(raw, &value); err != nil {
		return References{}, invalid("content shape is invalid")
	}
	if value.Nodes == nil {
		value.Nodes = []node{}
	}
	if value.Edges == nil {
		value.Edges = []edge{}
	}
	if len(value.Nodes) > MaxNodes || len(value.Edges) > MaxEdges {
		return References{}, invalid("canvas is too large")
	}

	nodeIDs := make(map[string]struct{}, len(value.Nodes))
	documentIDs := map[string]struct{}{}
	mediaIDs := map[string]struct{}{}
	for _, item := range value.Nodes {
		if !validID(item.ID) {
			return References{}, invalid("node id is missing or too long")
		}
		if _, exists := nodeIDs[item.ID]; exists {
			return References{}, invalid("node ids must be unique")
		}
		nodeIDs[item.ID] = struct{}{}
		if !integerInRange(item.X, -10_000_000, 10_000_000) ||
			!integerInRange(item.Y, -10_000_000, 10_000_000) ||
			!integerInRange(item.Width, 40, 10_000) ||
			!integerInRange(item.Height, 40, 10_000) {
			return References{}, invalid("node geometry is invalid")
		}
		if item.Color != "" && !colorPattern.MatchString(item.Color) {
			return References{}, invalid("node color is invalid")
		}
		switch item.Type {
		case "text":
			if item.Text == nil || len([]rune(*item.Text)) > MaxTextRunes {
				return References{}, invalid("text node is invalid")
			}
		case "file":
			if item.File == nil || strings.TrimSpace(*item.File) == "" || len([]rune(*item.File)) > 1_024 {
				return References{}, invalid("file node is invalid")
			}
			var extension resourceExtension
			if len(item.Extension) == 0 || json.Unmarshal(item.Extension, &extension) != nil ||
				extension.Resource == nil || !validID(extension.Resource.ID) {
				return References{}, invalid("file node resource is invalid")
			}
			switch extension.Resource.Kind {
			case "document":
				documentIDs[extension.Resource.ID] = struct{}{}
			case "media":
				mediaIDs[extension.Resource.ID] = struct{}{}
			default:
				return References{}, invalid("file node resource kind is invalid")
			}
		case "group":
			if item.Label != nil && len([]rune(*item.Label)) > 500 {
				return References{}, invalid("group label is too long")
			}
		default:
			return References{}, invalid("unsupported node type")
		}
	}

	edgeIDs := make(map[string]struct{}, len(value.Edges))
	for _, item := range value.Edges {
		if !validID(item.ID) {
			return References{}, invalid("edge id is missing or too long")
		}
		if _, exists := edgeIDs[item.ID]; exists {
			return References{}, invalid("edge ids must be unique")
		}
		edgeIDs[item.ID] = struct{}{}
		if _, exists := nodeIDs[item.FromNode]; !exists {
			return References{}, invalid("edge source does not exist")
		}
		if _, exists := nodeIDs[item.ToNode]; !exists {
			return References{}, invalid("edge target does not exist")
		}
		if !validSide(item.FromSide) || !validSide(item.ToSide) ||
			!validEnd(item.FromEnd, "none") || !validEnd(item.ToEnd, "arrow") ||
			(item.Color != "" && !colorPattern.MatchString(item.Color)) || len([]rune(item.Label)) > 500 {
			return References{}, invalid("edge properties are invalid")
		}
	}

	return References{
		DocumentIDs: sortedKeys(documentIDs),
		MediaIDs:    sortedKeys(mediaIDs),
	}, nil
}

func invalid(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidDocument, message)
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 200
}

func integerInRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value &&
		value >= minimum && value <= maximum
}

func validSide(value string) bool {
	return value == "" || value == "top" || value == "right" || value == "bottom" || value == "left"
}

func validEnd(value, defaultValue string) bool {
	return value == "" || value == defaultValue || value == "none" || value == "arrow"
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	// Resource lookup does not require canonical order; insertion sort keeps this package dependency-light.
	for index := 1; index < len(result); index++ {
		for cursor := index; cursor > 0 && result[cursor] < result[cursor-1]; cursor-- {
			result[cursor], result[cursor-1] = result[cursor-1], result[cursor]
		}
	}
	return result
}
