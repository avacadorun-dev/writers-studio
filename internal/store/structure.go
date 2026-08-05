package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
)

var (
	ErrUnknownNodeType         = errors.New("node type is not allowed by project structure")
	ErrInvalidParentType       = errors.New("parent type does not allow this child")
	ErrInvalidProjectStructure = errors.New("invalid project structure schema")
)

func DefaultProjectStructure(kind string, language string) domain.ProjectStructure {
	localize := func(ru string, en string) string {
		if language == "en" {
			return en
		}
		return ru
	}
	structure := domain.ProjectStructure{
		SchemaVersion: 1,
		Template:      "manuscript",
		RootLabel:     localize("Рукопись", "Manuscript"),
		NodeTypes: []domain.ProjectNodeType{
			{
				Key: "part", Label: localize("Часть", "Part"),
				PluralLabel: localize("Части", "Parts"), Icon: "folder",
				AllowedChildren: []string{"chapter", "scene", "draft", "note"}, CanBeRoot: true,
			},
			{
				Key: "chapter", Label: localize("Глава", "Chapter"),
				PluralLabel: localize("Главы", "Chapters"), Icon: "book-open",
				ContentBearing: true, AllowedChildren: []string{"scene", "draft", "note"},
				CanBeRoot: true,
			},
			{
				Key: "scene", Label: localize("Сцена", "Scene"),
				PluralLabel: localize("Сцены", "Scenes"), Icon: "file-text",
				ContentBearing: true, AllowedChildren: []string{"note"}, CanBeRoot: true,
			},
			{
				Key: "draft", Label: localize("Черновик", "Draft"),
				PluralLabel: localize("Черновики", "Drafts"), Icon: "notebook-pen",
				ContentBearing: true, CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		},
	}
	switch kind {
	case "story":
		structure.Template = "short_story"
		structure.RootLabel = localize("Рассказ", "Short story")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "scene", Label: localize("Сцена", "Scene"),
				PluralLabel: localize("Сцены", "Scenes"), Icon: "file-text",
				ContentBearing: true, AllowedChildren: []string{"beat", "note"},
				CanBeRoot: true,
			},
			{
				Key: "beat", Label: localize("Бит", "Beat"),
				PluralLabel: localize("Биты", "Beats"), Icon: "list-tree",
				ContentBearing: true, AllowedChildren: []string{"note"}, CanBeRoot: true,
			},
			{
				Key: "draft", Label: localize("Черновик", "Draft"),
				PluralLabel: localize("Черновики", "Drafts"), Icon: "notebook-pen",
				ContentBearing: true, CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	case "screenplay":
		structure.Template = "screenplay"
		structure.RootLabel = localize("Сценарий", "Screenplay")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "act", Label: localize("Акт", "Act"),
				PluralLabel: localize("Акты", "Acts"), Icon: "folder",
				AllowedChildren: []string{"sequence", "scene", "note"}, CanBeRoot: true,
			},
			{
				Key: "sequence", Label: localize("Секвенция", "Sequence"),
				PluralLabel: localize("Секвенции", "Sequences"), Icon: "list-tree",
				AllowedChildren: []string{"scene", "note"}, CanBeRoot: true,
			},
			{
				Key: "scene", Label: localize("Сцена", "Scene"),
				PluralLabel: localize("Сцены", "Scenes"), Icon: "clapperboard",
				ContentBearing: true, AllowedChildren: []string{"beat", "note"},
				CanBeRoot: true,
			},
			{
				Key: "beat", Label: localize("Бит", "Beat"),
				PluralLabel: localize("Биты", "Beats"), Icon: "list-tree",
				ContentBearing: true, AllowedChildren: []string{"note"}, CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	case "game_design":
		structure.Template = "game_design"
		structure.RootLabel = localize("Гейм-дизайн документ", "Game Design Document")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "section", Label: localize("Раздел", "Section"),
				PluralLabel: localize("Разделы", "Sections"), Icon: "folder",
				ContentBearing:  true,
				AllowedChildren: []string{"section", "system", "feature", "asset", "note"},
				CanBeRoot:       true,
			},
			{
				Key: "system", Label: localize("Система", "System"),
				PluralLabel: localize("Системы", "Systems"), Icon: "workflow",
				ContentBearing: true, AllowedChildren: []string{"feature", "asset", "note"},
				CanBeRoot: true,
			},
			{
				Key: "feature", Label: localize("Механика", "Feature"),
				PluralLabel: localize("Механики", "Features"), Icon: "gamepad-2",
				ContentBearing: true, AllowedChildren: []string{"asset", "note"}, CanBeRoot: true,
			},
			{
				Key: "asset", Label: localize("Ассет", "Asset"),
				PluralLabel: localize("Ассеты", "Assets"), Icon: "boxes",
				ContentBearing: true, AllowedChildren: []string{"note"}, CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	case "nonfiction":
		structure.Template = "nonfiction"
		structure.RootLabel = localize("Нон-фикшн", "Non-fiction")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "part", Label: localize("Часть", "Part"),
				PluralLabel: localize("Части", "Parts"), Icon: "folder",
				AllowedChildren: []string{"chapter", "section", "research", "note"},
				CanBeRoot:       true,
			},
			{
				Key: "chapter", Label: localize("Глава", "Chapter"),
				PluralLabel: localize("Главы", "Chapters"), Icon: "book-open",
				ContentBearing:  true,
				AllowedChildren: []string{"section", "research", "note"},
				CanBeRoot:       true,
			},
			{
				Key: "section", Label: localize("Раздел", "Section"),
				PluralLabel: localize("Разделы", "Sections"), Icon: "list-tree",
				ContentBearing:  true,
				AllowedChildren: []string{"section", "research", "note"},
				CanBeRoot:       true,
			},
			{
				Key: "research", Label: localize("Исследование", "Research"),
				PluralLabel: localize("Исследования", "Research"), Icon: "search",
				ContentBearing: true, AllowedChildren: []string{"note"}, CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	case "free":
		structure.Template = "free"
		structure.RootLabel = localize("Материалы", "Materials")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "folder", Label: localize("Папка", "Folder"),
				PluralLabel: localize("Папки", "Folders"), Icon: "folder",
				AllowedChildren: []string{"folder", "document", "note"}, CanBeRoot: true,
			},
			{
				Key: "document", Label: localize("Документ", "Document"),
				PluralLabel: localize("Документы", "Documents"), Icon: "file-text",
				ContentBearing: true, AllowedChildren: []string{"document", "note"},
				CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	case "custom":
		structure.Template = "custom"
		structure.RootLabel = localize("Своя структура", "Custom structure")
		structure.NodeTypes = []domain.ProjectNodeType{
			{
				Key: "group", Label: localize("Группа", "Group"),
				PluralLabel: localize("Группы", "Groups"), Icon: "folder",
				AllowedChildren: []string{"group", "document", "note"}, CanBeRoot: true,
			},
			{
				Key: "document", Label: localize("Документ", "Document"),
				PluralLabel: localize("Документы", "Documents"), Icon: "file-text",
				ContentBearing: true, AllowedChildren: []string{"document", "note"},
				CanBeRoot: true,
			},
			{
				Key: "note", Label: localize("Заметка", "Note"),
				PluralLabel: localize("Заметки", "Notes"), Icon: "sticky-note",
				ContentBearing: true, CanBeRoot: true,
			},
		}
	}
	return normalizeProjectStructure(structure)
}

func normalizeProjectStructure(structure domain.ProjectStructure) domain.ProjectStructure {
	if structure.NodeTypes == nil {
		structure.NodeTypes = make([]domain.ProjectNodeType, 0)
	}
	for index := range structure.NodeTypes {
		if structure.NodeTypes[index].AllowedChildren == nil {
			// Project schemas written before dynamic nesting used JSON null for
			// leaf types. Keep the API contract iterable for every client.
			structure.NodeTypes[index].AllowedChildren = make([]string, 0)
		}
	}
	return structure
}

func ValidateProjectStructure(structure domain.ProjectStructure) error {
	if structure.SchemaVersion < 1 || strings.TrimSpace(structure.Template) == "" ||
		strings.TrimSpace(structure.RootLabel) == "" || len(structure.NodeTypes) == 0 {
		return ErrInvalidProjectStructure
	}
	types := map[string]domain.ProjectNodeType{}
	hasRoot := false
	hasContent := false
	for _, nodeType := range structure.NodeTypes {
		if strings.TrimSpace(nodeType.Key) == "" || strings.TrimSpace(nodeType.Label) == "" ||
			types[nodeType.Key].Key != "" {
			return ErrInvalidProjectStructure
		}
		types[nodeType.Key] = nodeType
		hasRoot = hasRoot || nodeType.CanBeRoot
		hasContent = hasContent || nodeType.ContentBearing
	}
	if !hasRoot || !hasContent {
		return ErrInvalidProjectStructure
	}
	for _, nodeType := range structure.NodeTypes {
		for _, child := range nodeType.AllowedChildren {
			if types[child].Key == "" {
				return ErrInvalidProjectStructure
			}
		}
	}
	return nil
}

func (s *Store) GetProjectStructure(
	ctx context.Context,
	ownerID string,
	projectID string,
) (domain.ProjectStructure, int, error) {
	var encoded []byte
	var revision int
	err := s.Pool.QueryRow(ctx, `
		SELECT structure_schema, structure_revision
		FROM projects
		WHERE id=$1 AND owner_user_id=$2
	`, projectID, ownerID).Scan(&encoded, &revision)
	if err != nil {
		return domain.ProjectStructure{}, 0, err
	}
	var structure domain.ProjectStructure
	if err := json.Unmarshal(encoded, &structure); err != nil {
		return domain.ProjectStructure{}, 0, fmt.Errorf("decode project structure: %w", err)
	}
	structure = normalizeProjectStructure(structure)
	if err := ValidateProjectStructure(structure); err != nil {
		return domain.ProjectStructure{}, 0, err
	}
	return structure, revision, nil
}

func (s *Store) SaveProjectStructure(
	ctx context.Context,
	ownerID string,
	projectID string,
	baseRevision int,
	structure domain.ProjectStructure,
) (domain.ProjectStructure, int, error) {
	structure = normalizeProjectStructure(structure)
	if err := ValidateProjectStructure(structure); err != nil {
		return domain.ProjectStructure{}, 0, err
	}
	encoded, err := json.Marshal(structure)
	if err != nil {
		return domain.ProjectStructure{}, 0, err
	}
	var revision int
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var currentRevision int
		if err := tx.QueryRow(ctx, `
			SELECT structure_revision
			FROM projects
			WHERE id=$1 AND owner_user_id=$2
			FOR UPDATE
		`, projectID, ownerID).Scan(&currentRevision); err != nil {
			return err
		}
		if currentRevision != baseRevision {
			return assistantStructureStaleError()
		}
		type placement struct {
			id       string
			parentID *string
			kind     string
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, parent_id::text, kind
			FROM document_nodes
			WHERE project_id=$1 AND trashed_at IS NULL
		`, projectID)
		if err != nil {
			return err
		}
		var placements []placement
		kindsByID := map[string]string{}
		for rows.Next() {
			var item placement
			if err := rows.Scan(&item.id, &item.parentID, &item.kind); err != nil {
				rows.Close()
				return err
			}
			placements = append(placements, item)
			kindsByID[item.id] = item.kind
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, item := range placements {
			var parentKind *string
			if item.parentID != nil {
				value, exists := kindsByID[*item.parentID]
				if !exists {
					return assistantStructureStaleError()
				}
				parentKind = &value
			}
			if err := validateNodePlacement(structure, item.kind, parentKind); err != nil {
				return assistantStructureStaleError()
			}
		}
		return tx.QueryRow(ctx, `
			UPDATE projects
			SET structure_schema=$3, structure_revision=structure_revision+1, updated_at=now()
			WHERE id=$1 AND owner_user_id=$2
			RETURNING structure_revision
		`, projectID, ownerID, encoded).Scan(&revision)
	})
	if err != nil {
		return domain.ProjectStructure{}, 0, err
	}
	return structure, revision, err
}

func validateNodePlacement(
	structure domain.ProjectStructure,
	childKind string,
	parentKind *string,
) error {
	types := make(map[string]domain.ProjectNodeType, len(structure.NodeTypes))
	for _, nodeType := range structure.NodeTypes {
		types[nodeType.Key] = nodeType
	}
	child, exists := types[childKind]
	if !exists {
		return ErrUnknownNodeType
	}
	if parentKind == nil {
		if !child.CanBeRoot {
			return ErrInvalidParentType
		}
		return nil
	}
	parent, exists := types[*parentKind]
	if !exists {
		return ErrUnknownNodeType
	}
	for _, allowed := range parent.AllowedChildren {
		if allowed == childKind {
			return nil
		}
	}
	return ErrInvalidParentType
}

func (s *Store) ValidateNodePlacement(
	ctx context.Context,
	ownerID string,
	projectID string,
	childKind string,
	parentID *string,
) error {
	structure, _, err := s.GetProjectStructure(ctx, ownerID, projectID)
	if err != nil {
		return err
	}
	var parentKind *string
	if parentID != nil {
		node, err := s.GetDocumentNode(ctx, ownerID, *parentID)
		if err != nil {
			return err
		}
		if node.ProjectID != projectID || node.TrashedAt != nil {
			return pgx.ErrNoRows
		}
		parentKind = &node.Kind
	}
	return validateNodePlacement(structure, childKind, parentKind)
}

func assistantStructureStaleError() error {
	return errors.New("project structure revision is stale or would orphan existing node types")
}
