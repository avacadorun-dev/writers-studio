package canvas

import (
	"errors"
	"strings"
	"testing"
)

func validTypeSchema() TypeSchema {
	return TypeSchema{
		CardTypes: []CardType{{
			ID: "quest", Name: "Квест", Category: "game-design", Color: "3", Icon: "ScrollText",
			Fields: []FieldDefinition{
				{ID: "summary", Name: "Описание", Type: "markdown", Required: true},
				{ID: "state", Name: "Статус", Type: "single-select", Options: []string{"idea", "ready"}},
			},
			AllowedRelations: []string{"depends_on"}, DocumentKind: "scene",
		}},
		RelationTypes: []RelationType{{
			ID: "depends_on", Name: "Зависит от", Color: "#112233", Directed: true,
			FromTypes: []string{"quest"}, ToTypes: []string{"quest"},
		}},
	}
}

func TestValidateTypeSchemaAcceptsProjectCatalog(t *testing.T) {
	if err := ValidateTypeSchema(validTypeSchema()); err != nil {
		t.Fatalf("ValidateTypeSchema() error = %v", err)
	}
	if !validFieldType("canvas") || validFieldType("formula") {
		t.Fatal("field type validation is inconsistent")
	}
}

func TestValidateTypeSchemaRejectsInvalidCatalogs(t *testing.T) {
	tests := map[string]func(*TypeSchema){
		"invalid id":            func(value *TypeSchema) { value.CardTypes[0].ID = "bad id" },
		"empty name":            func(value *TypeSchema) { value.CardTypes[0].Name = " " },
		"invalid icon":          func(value *TypeSchema) { value.CardTypes[0].Icon = strings.Repeat("x", 81) },
		"invalid color":         func(value *TypeSchema) { value.CardTypes[0].Color = "pink" },
		"invalid category":      func(value *TypeSchema) { value.CardTypes[0].Category = "system" },
		"duplicate card":        func(value *TypeSchema) { value.CardTypes = append(value.CardTypes, value.CardTypes[0]) },
		"invalid document kind": func(value *TypeSchema) { value.CardTypes[0].DocumentKind = "bad kind" },
		"invalid field id":      func(value *TypeSchema) { value.CardTypes[0].Fields[0].ID = "" },
		"duplicate field": func(value *TypeSchema) {
			value.CardTypes[0].Fields = append(value.CardTypes[0].Fields, value.CardTypes[0].Fields[0])
		},
		"unknown field type":       func(value *TypeSchema) { value.CardTypes[0].Fields[0].Type = "money" },
		"duplicate option":         func(value *TypeSchema) { value.CardTypes[0].Fields[1].Options = []string{"a", "a"} },
		"option on number":         func(value *TypeSchema) { value.CardTypes[0].Fields[0].Options = []string{"a"} },
		"empty option":             func(value *TypeSchema) { value.CardTypes[0].Fields[1].Options = []string{" "} },
		"invalid relation":         func(value *TypeSchema) { value.RelationTypes[0].ID = "bad relation" },
		"duplicate relation":       func(value *TypeSchema) { value.RelationTypes = append(value.RelationTypes, value.RelationTypes[0]) },
		"unknown relation card":    func(value *TypeSchema) { value.RelationTypes[0].ToTypes = []string{"missing"} },
		"unknown allowed relation": func(value *TypeSchema) { value.CardTypes[0].AllowedRelations = []string{"missing"} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := validTypeSchema()
			mutate(&value)
			if err := ValidateTypeSchema(value); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("ValidateTypeSchema() error = %v", err)
			}
		})
	}
}

func TestValidateTypeSchemaLimits(t *testing.T) {
	value := validTypeSchema()
	value.CardTypes = make([]CardType, MaxCardTypes+1)
	if err := ValidateTypeSchema(value); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("type limit error = %v", err)
	}
	value = validTypeSchema()
	value.CardTypes[0].Fields = make([]FieldDefinition, MaxFieldsPerType+1)
	if err := ValidateTypeSchema(value); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("field limit error = %v", err)
	}
	value = validTypeSchema()
	value.CardTypes[0].Fields[1].Options = make([]string, MaxFieldOptions+1)
	if err := ValidateTypeSchema(value); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("option limit error = %v", err)
	}
	value = validTypeSchema()
	value.CardTypes[0].Name = strings.Repeat("я", MaxTypeSchemaBytes)
	if err := ValidateTypeSchema(value); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("byte limit error = %v", err)
	}
}
