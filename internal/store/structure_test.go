package store

import (
	"encoding/json"
	"errors"
	"testing"

	"writers.studio/internal/domain"
)

func TestDefaultProjectStructures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind        string
		template    string
		initialType string
	}{
		{kind: "novel", template: "manuscript", initialType: "chapter"},
		{kind: "story", template: "short_story", initialType: "scene"},
		{kind: "screenplay", template: "screenplay", initialType: "scene"},
		{kind: "nonfiction", template: "nonfiction", initialType: "chapter"},
		{kind: "game_design", template: "game_design", initialType: "section"},
		{kind: "free", template: "free", initialType: "document"},
		{kind: "custom", template: "custom", initialType: "document"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.kind, func(t *testing.T) {
			t.Parallel()
			structure := DefaultProjectStructure(test.kind, "ru")
			if err := ValidateProjectStructure(structure); err != nil {
				t.Fatal(err)
			}
			if structure.Template != test.template ||
				initialProjectNodeType(structure).Key != test.initialType {
				t.Fatalf("unexpected structure: %+v", structure)
			}
			for _, nodeType := range structure.NodeTypes {
				if nodeType.AllowedChildren == nil {
					t.Fatalf("%s returned nil allowedChildren for %s", test.kind, nodeType.Key)
				}
			}
			encoded, err := json.Marshal(structure)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip domain.ProjectStructure
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatal(err)
			}
			for _, nodeType := range roundTrip.NodeTypes {
				if nodeType.AllowedChildren == nil {
					t.Fatalf("%s encoded null allowedChildren for %s", test.kind, nodeType.Key)
				}
			}
		})
	}
	english := DefaultProjectStructure("novel", "en")
	if english.RootLabel != "Manuscript" ||
		initialProjectNodeTitle(initialProjectNodeType(english), "en") != "Chapter 1" {
		t.Fatalf("English structure was not localized: %+v", english)
	}
}

func TestNormalizeLegacyProjectStructureCollections(t *testing.T) {
	t.Parallel()
	legacy := domain.ProjectStructure{
		SchemaVersion: 1,
		Template:      "legacy",
		RootLabel:     "Legacy",
		NodeTypes: []domain.ProjectNodeType{{
			Key: "document", Label: "Document", ContentBearing: true, CanBeRoot: true,
		}},
	}
	normalized := normalizeProjectStructure(legacy)
	if normalized.NodeTypes == nil || normalized.NodeTypes[0].AllowedChildren == nil {
		t.Fatalf("legacy collections were not normalized: %+v", normalized)
	}
}

func TestValidateProjectStructure(t *testing.T) {
	t.Parallel()
	valid := domain.ProjectStructure{
		SchemaVersion: 1, Template: "custom", RootLabel: "Project",
		NodeTypes: []domain.ProjectNodeType{
			{
				Key: "folder", Label: "Folder", CanBeRoot: true,
				AllowedChildren: []string{"document"},
			},
			{Key: "document", Label: "Document", ContentBearing: true},
		},
	}
	if err := ValidateProjectStructure(valid); err != nil {
		t.Fatal(err)
	}
	invalid := []domain.ProjectStructure{
		{},
		{SchemaVersion: 1, Template: "x", RootLabel: "x"},
		{
			SchemaVersion: 1, Template: "x", RootLabel: "x",
			NodeTypes: []domain.ProjectNodeType{{Key: "", Label: "x"}},
		},
		{
			SchemaVersion: 1, Template: "x", RootLabel: "x",
			NodeTypes: []domain.ProjectNodeType{
				{Key: "same", Label: "x"}, {Key: "same", Label: "x"},
			},
		},
		{
			SchemaVersion: 1, Template: "x", RootLabel: "x",
			NodeTypes: []domain.ProjectNodeType{{
				Key: "folder", Label: "x", AllowedChildren: []string{"missing"},
				CanBeRoot: true, ContentBearing: true,
			}},
		},
		{
			SchemaVersion: 1, Template: "x", RootLabel: "x",
			NodeTypes: []domain.ProjectNodeType{{
				Key: "document", Label: "x", ContentBearing: true,
			}},
		},
		{
			SchemaVersion: 1, Template: "x", RootLabel: "x",
			NodeTypes: []domain.ProjectNodeType{{
				Key: "folder", Label: "x", CanBeRoot: true,
			}},
		},
	}
	for _, structure := range invalid {
		if !errors.Is(ValidateProjectStructure(structure), ErrInvalidProjectStructure) {
			t.Fatalf("invalid structure accepted: %+v", structure)
		}
	}
}

func TestValidateNodePlacement(t *testing.T) {
	t.Parallel()
	structure := domain.ProjectStructure{
		SchemaVersion: 1, Template: "custom", RootLabel: "Project",
		NodeTypes: []domain.ProjectNodeType{
			{
				Key: "folder", Label: "Folder", CanBeRoot: true,
				AllowedChildren: []string{"document"},
			},
			{Key: "document", Label: "Document", ContentBearing: true},
		},
	}
	folder := "folder"
	document := "document"
	if err := validateNodePlacement(structure, "folder", nil); err != nil {
		t.Fatal(err)
	}
	if err := validateNodePlacement(structure, "document", &folder); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(
		validateNodePlacement(structure, "document", nil),
		ErrInvalidParentType,
	) {
		t.Fatal("non-root document was accepted at root")
	}
	if !errors.Is(
		validateNodePlacement(structure, "folder", &document),
		ErrInvalidParentType,
	) {
		t.Fatal("invalid child was accepted")
	}
	if !errors.Is(
		validateNodePlacement(structure, "missing", nil),
		ErrUnknownNodeType,
	) {
		t.Fatal("unknown type was accepted")
	}
	missing := "missing"
	if !errors.Is(
		validateNodePlacement(structure, "document", &missing),
		ErrUnknownNodeType,
	) {
		t.Fatal("unknown parent type was accepted")
	}
}
