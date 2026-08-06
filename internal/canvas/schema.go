package canvas

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxTypeSchemaBytes = 512 << 10
	MaxCardTypes       = 100
	MaxRelationTypes   = 100
	MaxFieldsPerType   = 50
	MaxFieldOptions    = 100
)

var schemaIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$`)

type TypeSchema struct {
	Revision      int            `json:"revision"`
	CardTypes     []CardType     `json:"cardTypes"`
	RelationTypes []RelationType `json:"relationTypes"`
}

type CardType struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Category         string            `json:"category"`
	Color            string            `json:"color"`
	Icon             string            `json:"icon"`
	Fields           []FieldDefinition `json:"fields"`
	AllowedRelations []string          `json:"allowedRelations,omitempty"`
	DocumentKind     string            `json:"documentKind,omitempty"`
}

type FieldDefinition struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
}

type RelationType struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Color     string   `json:"color"`
	Directed  bool     `json:"directed"`
	FromTypes []string `json:"fromTypes,omitempty"`
	ToTypes   []string `json:"toTypes,omitempty"`
}

func ValidateTypeSchema(value TypeSchema) error {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > MaxTypeSchemaBytes {
		return invalidSchema("schema is too large")
	}
	if len(value.CardTypes) > MaxCardTypes || len(value.RelationTypes) > MaxRelationTypes {
		return invalidSchema("schema contains too many types")
	}
	cardIDs := make(map[string]struct{}, len(value.CardTypes))
	for _, card := range value.CardTypes {
		if !schemaIDPattern.MatchString(card.ID) || !validSchemaName(card.Name, 120) ||
			!validSchemaName(card.Icon, 80) || !validSchemaColor(card.Color) {
			return invalidSchema("card type is invalid")
		}
		switch card.Category {
		case "general", "narrative", "game-design", "custom":
		default:
			return invalidSchema("card category is invalid")
		}
		if _, exists := cardIDs[card.ID]; exists {
			return invalidSchema("card type ids must be unique")
		}
		cardIDs[card.ID] = struct{}{}
		if len(card.Fields) > MaxFieldsPerType || !validOptionalSchemaID(card.DocumentKind) {
			return invalidSchema("card type fields are invalid")
		}
		fieldIDs := make(map[string]struct{}, len(card.Fields))
		for _, field := range card.Fields {
			if !schemaIDPattern.MatchString(field.ID) || !validSchemaName(field.Name, 120) ||
				!validFieldType(field.Type) || len(field.Options) > MaxFieldOptions {
				return invalidSchema("field definition is invalid")
			}
			if _, exists := fieldIDs[field.ID]; exists {
				return invalidSchema("field ids must be unique within a card type")
			}
			fieldIDs[field.ID] = struct{}{}
			options := map[string]struct{}{}
			for _, option := range field.Options {
				if !validSchemaName(option, 120) {
					return invalidSchema("field option is invalid")
				}
				if _, exists := options[option]; exists {
					return invalidSchema("field options must be unique")
				}
				options[option] = struct{}{}
			}
			if len(field.Options) > 0 && field.Type != "single-select" && field.Type != "multi-select" {
				return invalidSchema("only select fields may define options")
			}
		}
	}

	relationIDs := make(map[string]struct{}, len(value.RelationTypes))
	for _, relation := range value.RelationTypes {
		if !schemaIDPattern.MatchString(relation.ID) || !validSchemaName(relation.Name, 120) ||
			!validSchemaColor(relation.Color) {
			return invalidSchema("relation type is invalid")
		}
		if _, exists := relationIDs[relation.ID]; exists {
			return invalidSchema("relation type ids must be unique")
		}
		relationIDs[relation.ID] = struct{}{}
		for _, cardID := range append(append([]string{}, relation.FromTypes...), relation.ToTypes...) {
			if _, exists := cardIDs[cardID]; !exists {
				return invalidSchema("relation references an unknown card type")
			}
		}
	}
	for _, card := range value.CardTypes {
		for _, relationID := range card.AllowedRelations {
			if _, exists := relationIDs[relationID]; !exists {
				return invalidSchema("card references an unknown relation type")
			}
		}
	}
	return nil
}

func validFieldType(value string) bool {
	switch value {
	case "short-text", "markdown", "number", "checkbox", "single-select", "multi-select",
		"date", "duration", "document", "media", "canvas":
		return true
	default:
		return false
	}
}

func validSchemaName(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len([]rune(trimmed)) <= limit
}

func validSchemaColor(value string) bool {
	return colorPattern.MatchString(value)
}

func validOptionalSchemaID(value string) bool {
	return value == "" || schemaIDPattern.MatchString(value)
}

func invalidSchema(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidDocument, message)
}
