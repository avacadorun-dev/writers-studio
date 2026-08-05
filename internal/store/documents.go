package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
)

type SaveRevisionInput struct {
	BaseRevision      int
	SchemaVersion     int
	Content           json.RawMessage
	PlainText         string
	Source            string
	OperationID       *string
	AnnotationAnchors []AnnotationAnchorUpdate
}

type RevisionConflictError struct {
	CurrentRevision int
	ConflictNodeID  string
}

var (
	ErrInvalidDocumentMove = errors.New("invalid document move")
	ErrReplacePreviewStale = errors.New("project replace preview is stale")
)

type ProjectReplaceDocument struct {
	NodeID      string `json:"nodeId"`
	Title       string `json:"title"`
	Revision    int    `json:"revision"`
	MatchCount  int    `json:"matchCount"`
	Snippet     string `json:"snippet"`
	Replacement string `json:"replacement"`
}

type ProjectReplacePreview struct {
	Token      string                   `json:"token"`
	Search     string                   `json:"search"`
	Replace    string                   `json:"replace"`
	Scope      string                   `json:"scope"`
	TotalCount int                      `json:"totalCount"`
	Documents  []ProjectReplaceDocument `json:"documents"`
}

type ProjectReplaceResult struct {
	OperationID  string `json:"operationId"`
	Documents    int    `json:"documents"`
	Replacements int    `json:"replacements"`
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("base revision is stale: current=%d", e.CurrentRevision)
}

func (s *Store) ListDocumentNodes(
	ctx context.Context,
	ownerID string,
	projectID string,
	includeTrashed bool,
) ([]domain.DocumentNode, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT n.id::text, n.project_id::text, n.parent_id::text, n.kind, n.title, n.status,
			n.position::float8, n.current_revision,
			COALESCE(r.content, 'null'::jsonb), COALESCE(r.plain_text, ''), n.source_frontmatter,
			n.trashed_at, n.created_at, n.updated_at
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		LEFT JOIN document_revisions r
			ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.project_id=$1 AND p.owner_user_id=$2 AND ($3 OR n.trashed_at IS NULL)
		ORDER BY n.position, n.created_at
	`, projectID, ownerID, includeTrashed)
	if err != nil {
		return nil, fmt.Errorf("list document nodes: %w", err)
	}
	defer rows.Close()

	nodes := make([]domain.DocumentNode, 0)
	for rows.Next() {
		var node domain.DocumentNode
		if err := rows.Scan(
			&node.ID,
			&node.ProjectID,
			&node.ParentID,
			&node.Kind,
			&node.Title,
			&node.Status,
			&node.Position,
			&node.CurrentRevision,
			&node.Content,
			&node.PlainText,
			&node.SourceFrontmatter,
			&node.TrashedAt,
			&node.CreatedAt,
			&node.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document node: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// ListDocumentNodeSummaries returns the project outline without revision bodies.
// A large Markdown vault should not deserialize every document merely to render navigation.
func (s *Store) ListDocumentNodeSummaries(
	ctx context.Context,
	ownerID string,
	projectID string,
	includeTrashed bool,
) ([]domain.DocumentNode, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT n.id::text, n.project_id::text, n.parent_id::text, n.kind, n.title, n.status,
			n.position::float8, n.current_revision, n.trashed_at, n.created_at, n.updated_at
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		WHERE n.project_id=$1 AND p.owner_user_id=$2 AND ($3 OR n.trashed_at IS NULL)
		ORDER BY n.position, n.created_at
	`, projectID, ownerID, includeTrashed)
	if err != nil {
		return nil, fmt.Errorf("list document node summaries: %w", err)
	}
	defer rows.Close()

	nodes := make([]domain.DocumentNode, 0)
	for rows.Next() {
		var node domain.DocumentNode
		if err := rows.Scan(
			&node.ID,
			&node.ProjectID,
			&node.ParentID,
			&node.Kind,
			&node.Title,
			&node.Status,
			&node.Position,
			&node.CurrentRevision,
			&node.TrashedAt,
			&node.CreatedAt,
			&node.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document node summary: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) GetDocumentNode(ctx context.Context, ownerID, nodeID string) (domain.DocumentNode, error) {
	var node domain.DocumentNode
	err := s.Pool.QueryRow(ctx, `
		SELECT n.id::text, n.project_id::text, n.parent_id::text, n.kind, n.title, n.status,
			n.position::float8, n.current_revision,
			COALESCE(r.content, 'null'::jsonb), COALESCE(r.plain_text, ''), n.source_frontmatter,
			n.trashed_at, n.created_at, n.updated_at
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		LEFT JOIN document_revisions r
			ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.id=$1 AND p.owner_user_id=$2
	`, nodeID, ownerID).Scan(
		&node.ID,
		&node.ProjectID,
		&node.ParentID,
		&node.Kind,
		&node.Title,
		&node.Status,
		&node.Position,
		&node.CurrentRevision,
		&node.Content,
		&node.PlainText,
		&node.SourceFrontmatter,
		&node.TrashedAt,
		&node.CreatedAt,
		&node.UpdatedAt,
	)
	return node, err
}

func (s *Store) CreateDocumentNode(
	ctx context.Context,
	ownerID string,
	projectID string,
	parentID *string,
	kind string,
	title string,
) (domain.DocumentNode, error) {
	nodeID := uuid.NewString()
	content := emptyDocument()
	var node domain.DocumentNode
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var structureJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT structure_schema
			FROM projects
			WHERE id=$1 AND owner_user_id=$2 AND status <> 'trash'
			FOR UPDATE
		`, projectID, ownerID).Scan(&structureJSON); err != nil {
			return err
		}
		var structure domain.ProjectStructure
		if err := json.Unmarshal(structureJSON, &structure); err != nil {
			return err
		}
		var parentKind *string
		if parentID != nil {
			var value string
			if err := tx.QueryRow(ctx, `
				SELECT kind
				FROM document_nodes
				WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
				FOR UPDATE
			`, *parentID, projectID).Scan(&value); err != nil {
				return err
			}
			parentKind = &value
		}
		if err := validateNodePlacement(structure, kind, parentKind); err != nil {
			return err
		}
		contentBearing := false
		for _, nodeType := range structure.NodeTypes {
			if nodeType.Key == kind {
				contentBearing = nodeType.ContentBearing
				break
			}
		}
		var position float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(n.position), 0)::float8 + 1000
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			WHERE n.project_id=$1 AND p.owner_user_id=$2 AND n.parent_id IS NOT DISTINCT FROM $3
		`, projectID, ownerID, parentID).Scan(&position); err != nil {
			return err
		}

		hasContent := contentBearing
		currentRevision := 0
		if hasContent {
			currentRevision = 1
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO document_nodes(
				id, project_id, parent_id, kind, title, position, current_revision
			)
			SELECT $1, p.id, $3, $4, $5, $6, $7
			FROM projects p
			WHERE p.id=$2 AND p.owner_user_id=$8
			RETURNING id::text, project_id::text, parent_id::text, kind, title, status,
				position::float8, current_revision, trashed_at, created_at, updated_at
		`, nodeID, projectID, parentID, kind, title, position, currentRevision, ownerID).Scan(
			&node.ID,
			&node.ProjectID,
			&node.ParentID,
			&node.Kind,
			&node.Title,
			&node.Status,
			&node.Position,
			&node.CurrentRevision,
			&node.TrashedAt,
			&node.CreatedAt,
			&node.UpdatedAt,
		)
		if err != nil {
			return err
		}
		if !hasContent {
			return nil
		}
		node.Content = content
		_, err = tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text, source
			)
			VALUES($1, $2, 1, 0, 1, $3, '', 'node_create')
		`, uuid.NewString(), nodeID, content)
		return err
	})
	return node, err
}

func (s *Store) SaveRevision(
	ctx context.Context,
	ownerID string,
	nodeID string,
	input SaveRevisionInput,
) (domain.DocumentRevision, error) {
	if input.SchemaVersion == 0 {
		version, err := editor.SchemaVersion(input.Content)
		if err != nil {
			return domain.DocumentRevision{}, err
		}
		input.SchemaVersion = version
	}
	var revision domain.DocumentRevision
	var savedConflict *RevisionConflictError
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var current int
		var projectID string
		var parentID *string
		var kind string
		var title string
		var status string
		var position float64
		err := tx.QueryRow(ctx, `
			SELECT n.current_revision, n.project_id::text, n.parent_id::text, n.kind, n.title, n.status, n.position::float8
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			WHERE n.id=$1 AND p.owner_user_id=$2
			FOR UPDATE
		`, nodeID, ownerID).Scan(&current, &projectID, &parentID, &kind, &title, &status, &position)
		if err != nil {
			return err
		}

		if current != input.BaseRevision {
			conflictNodeID := uuid.NewString()
			conflictTitle := title + " — конфликтующая копия"
			var nextPosition float64
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(MAX(position), $1)::float8 + 1
				FROM document_nodes
				WHERE project_id=$2 AND parent_id IS NOT DISTINCT FROM $3
			`, position, projectID, parentID).Scan(&nextPosition); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO document_nodes(
					id, project_id, parent_id, kind, title, status, position, current_revision
				)
				VALUES($1, $2, $3, $4, $5, $6, $7, 1)
			`, conflictNodeID, projectID, parentID, kind, conflictTitle, status, nextPosition)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO document_revisions(
					id, node_id, revision, base_revision, schema_version, content, plain_text, source, operation_id
				)
				VALUES($1, $2, 1, 0, $3, $4, $5, 'conflict_copy', $6)
			`, uuid.NewString(), conflictNodeID, input.SchemaVersion, input.Content, input.PlainText, input.OperationID)
			if err != nil {
				return err
			}
			if err := copyAnnotationsTx(
				ctx, tx, nodeID, conflictNodeID, projectID, 1, input.Content,
				input.AnnotationAnchors, false,
			); err != nil {
				return err
			}
			savedConflict = &RevisionConflictError{
				CurrentRevision: current,
				ConflictNodeID:  conflictNodeID,
			}
			return nil
		}

		nextRevision := current + 1
		revision.ID = uuid.NewString()
		revision.NodeID = nodeID
		revision.Revision = nextRevision
		revision.BaseRevision = current
		revision.SchemaVersion = input.SchemaVersion
		revision.Content = input.Content
		revision.PlainText = input.PlainText
		revision.Source = input.Source
		revision.OperationID = input.OperationID
		revision.CreatedAt = time.Now()

		_, err = tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text, source, operation_id, created_at
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, revision.ID, nodeID, nextRevision, current, input.SchemaVersion, input.Content, input.PlainText, input.Source, input.OperationID, revision.CreatedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE document_nodes
			SET current_revision=$2, updated_at=now()
			WHERE id=$1
		`, nodeID, nextRevision)
		if err != nil {
			return err
		}
		if input.AnnotationAnchors != nil {
			return applyAnnotationAnchorsTx(
				ctx, tx, nodeID, nextRevision, input.Content, input.AnnotationAnchors,
			)
		}
		// Legacy clients do not send anchor mappings. Keep their save compatible,
		// but never pretend the old offsets were verified against the new text.
		_, err = tx.Exec(ctx, `
			UPDATE document_annotations
			SET anchor_status='detached', revision=revision+1, updated_at=now()
			WHERE node_id=$1
		`, nodeID)
		return err
	})
	if err != nil {
		return revision, err
	}
	if savedConflict != nil {
		return revision, savedConflict
	}
	return revision, nil
}

func (s *Store) ListRevisions(ctx context.Context, ownerID, nodeID string) ([]domain.DocumentRevision, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.node_id::text, r.revision, r.base_revision, r.schema_version,
			r.content, r.plain_text, r.source, r.operation_id::text, r.created_at
		FROM document_revisions r
		JOIN document_nodes n ON n.id=r.node_id
		JOIN projects p ON p.id=n.project_id
		WHERE r.node_id=$1 AND p.owner_user_id=$2
		ORDER BY r.revision DESC
	`, nodeID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	revisions := make([]domain.DocumentRevision, 0)
	for rows.Next() {
		var revision domain.DocumentRevision
		if err := rows.Scan(
			&revision.ID,
			&revision.NodeID,
			&revision.Revision,
			&revision.BaseRevision,
			&revision.SchemaVersion,
			&revision.Content,
			&revision.PlainText,
			&revision.Source,
			&revision.OperationID,
			&revision.CreatedAt,
		); err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) CreateNamedVersion(
	ctx context.Context,
	ownerID, nodeID, name, note string,
) (domain.NamedVersion, error) {
	var version domain.NamedVersion
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO named_versions(id, node_id, revision, name, note)
		SELECT $1, n.id, n.current_revision, $4, $5
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		WHERE n.id=$2 AND p.owner_user_id=$3
		RETURNING id::text, node_id::text, revision, name, note, created_at
	`, uuid.NewString(), nodeID, ownerID, name, note).Scan(
		&version.ID,
		&version.NodeID,
		&version.Revision,
		&version.Name,
		&version.Note,
		&version.CreatedAt,
	)
	return version, err
}

func (s *Store) ListNamedVersions(ctx context.Context, ownerID, nodeID string) ([]domain.NamedVersion, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT v.id::text, v.node_id::text, v.revision, v.name, v.note, v.created_at
		FROM named_versions v
		JOIN document_nodes n ON n.id=v.node_id
		JOIN projects p ON p.id=n.project_id
		WHERE v.node_id=$1 AND p.owner_user_id=$2
		ORDER BY v.created_at DESC
	`, nodeID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]domain.NamedVersion, 0)
	for rows.Next() {
		var version domain.NamedVersion
		if err := rows.Scan(
			&version.ID,
			&version.NodeID,
			&version.Revision,
			&version.Name,
			&version.Note,
			&version.CreatedAt,
		); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) GetNamedVersion(
	ctx context.Context,
	ownerID, versionID string,
) (domain.NamedVersion, error) {
	var version domain.NamedVersion
	err := s.Pool.QueryRow(ctx, `
		SELECT v.id::text, v.node_id::text, v.revision, v.name, v.note, v.created_at
		FROM named_versions v
		JOIN document_nodes n ON n.id=v.node_id
		JOIN projects p ON p.id=n.project_id
		WHERE v.id=$1 AND p.owner_user_id=$2
	`, versionID, ownerID).Scan(
		&version.ID,
		&version.NodeID,
		&version.Revision,
		&version.Name,
		&version.Note,
		&version.CreatedAt,
	)
	return version, err
}

func (s *Store) GetNamedVersionDocument(
	ctx context.Context,
	ownerID, versionID string,
) (domain.NamedVersion, domain.DocumentNode, json.RawMessage, error) {
	var version domain.NamedVersion
	var node domain.DocumentNode
	var versionContent json.RawMessage
	err := s.Pool.QueryRow(ctx, `
		SELECT v.id::text, v.node_id::text, v.revision, v.name, v.note, v.created_at,
			n.id::text, n.project_id::text, n.parent_id::text, n.kind, n.title, n.status,
			n.position::float8, n.current_revision, n.trashed_at, n.created_at, n.updated_at,
			cr.content, cr.plain_text, vr.content
		FROM named_versions v
		JOIN document_nodes n ON n.id=v.node_id
		JOIN projects p ON p.id=n.project_id
		JOIN document_revisions vr ON vr.node_id=v.node_id AND vr.revision=v.revision
		JOIN document_revisions cr ON cr.node_id=n.id AND cr.revision=n.current_revision
		WHERE v.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
	`, versionID, ownerID).Scan(
		&version.ID, &version.NodeID, &version.Revision, &version.Name, &version.Note,
		&version.CreatedAt, &node.ID, &node.ProjectID, &node.ParentID, &node.Kind,
		&node.Title, &node.Status, &node.Position, &node.CurrentRevision, &node.TrashedAt,
		&node.CreatedAt, &node.UpdatedAt, &node.Content, &node.PlainText, &versionContent,
	)
	return version, node, versionContent, err
}

func (s *Store) RestoreVersion(
	ctx context.Context,
	ownerID, versionID string,
) (domain.DocumentRevision, error) {
	var nodeID string
	var currentRevision int
	var content json.RawMessage
	var plainText string
	err := s.Pool.QueryRow(ctx, `
		SELECT n.id::text, n.current_revision, r.content, r.plain_text
		FROM named_versions v
		JOIN document_nodes n ON n.id=v.node_id
		JOIN projects p ON p.id=n.project_id
		JOIN document_revisions r ON r.node_id=v.node_id AND r.revision=v.revision
		WHERE v.id=$1 AND p.owner_user_id=$2
	`, versionID, ownerID).Scan(&nodeID, &currentRevision, &content, &plainText)
	if err != nil {
		return domain.DocumentRevision{}, err
	}
	return s.SaveRevision(ctx, ownerID, nodeID, SaveRevisionInput{
		BaseRevision: currentRevision,
		Content:      content,
		PlainText:    plainText,
		Source:       "restore",
	})
}

func (s *Store) MoveDocumentNode(
	ctx context.Context,
	ownerID, nodeID string,
	parentID *string,
	position float64,
) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID string
		var childKind string
		var structureJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT n.project_id::text, n.kind, p.structure_schema
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
			FOR UPDATE
		`, nodeID, ownerID).Scan(&projectID, &childKind, &structureJSON); err != nil {
			return err
		}
		var structure domain.ProjectStructure
		if err := json.Unmarshal(structureJSON, &structure); err != nil {
			return err
		}

		var parentKind *string
		if parentID != nil {
			if *parentID == nodeID {
				return ErrInvalidDocumentMove
			}

			var parentProjectID string
			var parentKindValue string
			if err := tx.QueryRow(ctx, `
				SELECT n.project_id::text, n.kind
				FROM document_nodes n
				JOIN projects p ON p.id=n.project_id
				WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
				FOR UPDATE
			`, *parentID, ownerID).Scan(&parentProjectID, &parentKindValue); err != nil {
				return err
			}
			if parentProjectID != projectID {
				return ErrInvalidDocumentMove
			}
			parentKind = &parentKindValue

			var createsCycle bool
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE descendants(id) AS (
					SELECT id
					FROM document_nodes
					WHERE id=$1 AND project_id=$2
					UNION
					SELECT child.id
					FROM document_nodes child
					JOIN descendants parent ON child.parent_id=parent.id
					WHERE child.project_id=$2
				)
				SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$3)
			`, nodeID, projectID, *parentID).Scan(&createsCycle); err != nil {
				return err
			}
			if createsCycle {
				return ErrInvalidDocumentMove
			}
		}
		if err := validateNodePlacement(structure, childKind, parentKind); err != nil {
			return ErrInvalidDocumentMove
		}

		tag, err := tx.Exec(ctx, `
			UPDATE document_nodes
			SET parent_id=$2, position=$3, updated_at=now()
			WHERE id=$1
		`, nodeID, parentID, position)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (s *Store) MoveDocumentNodeToIndex(
	ctx context.Context,
	ownerID, nodeID string,
	parentID *string,
	targetIndex int,
) error {
	if targetIndex < 0 {
		return ErrInvalidDocumentMove
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID, childKind string
		var currentParentID *string
		var structureJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT n.project_id::text, n.parent_id::text, n.kind, p.structure_schema
			FROM document_nodes n JOIN projects p ON p.id=n.project_id
			WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
			FOR UPDATE
		`, nodeID, ownerID).Scan(&projectID, &currentParentID, &childKind, &structureJSON); err != nil {
			return err
		}
		var structure domain.ProjectStructure
		if err := json.Unmarshal(structureJSON, &structure); err != nil {
			return err
		}

		var parentKind *string
		if parentID != nil {
			if *parentID == nodeID {
				return ErrInvalidDocumentMove
			}
			var parentProjectID, value string
			if err := tx.QueryRow(ctx, `
				SELECT n.project_id::text, n.kind FROM document_nodes n
				JOIN projects p ON p.id=n.project_id
				WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL FOR UPDATE
			`, *parentID, ownerID).Scan(&parentProjectID, &value); err != nil {
				return err
			}
			if parentProjectID != projectID {
				return ErrInvalidDocumentMove
			}
			parentKind = &value
			var createsCycle bool
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE descendants(id) AS (
					SELECT id FROM document_nodes WHERE id=$1 AND project_id=$2
					UNION SELECT child.id FROM document_nodes child
					JOIN descendants parent ON child.parent_id=parent.id WHERE child.project_id=$2
				) SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$3)
			`, nodeID, projectID, *parentID).Scan(&createsCycle); err != nil {
				return err
			}
			if createsCycle {
				return ErrInvalidDocumentMove
			}
		}
		if err := validateNodePlacement(structure, childKind, parentKind); err != nil {
			return ErrInvalidDocumentMove
		}

		loadSiblings := func(parent *string, excludeMoved bool) ([]string, error) {
			query := `SELECT id::text FROM document_nodes
				WHERE project_id=$1 AND parent_id IS NOT DISTINCT FROM $2 AND trashed_at IS NULL`
			args := []any{projectID, parent}
			if excludeMoved {
				query += ` AND id<>$3`
				args = append(args, nodeID)
			}
			query += ` ORDER BY position, created_at FOR UPDATE`
			rows, err := tx.Query(ctx, query, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var ids []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return nil, err
				}
				ids = append(ids, id)
			}
			return ids, rows.Err()
		}
		parentKey := func(value *string) string {
			if value == nil {
				return ""
			}
			return *value
		}
		groupParents := map[string]*string{
			parentKey(currentParentID): currentParentID,
			parentKey(parentID):        parentID,
		}
		groupKeys := make([]string, 0, len(groupParents))
		for key := range groupParents {
			groupKeys = append(groupKeys, key)
		}
		sort.Strings(groupKeys)
		groups := make(map[string][]string, len(groupKeys))
		for _, key := range groupKeys {
			ids, err := loadSiblings(groupParents[key], true)
			if err != nil {
				return err
			}
			groups[key] = ids
		}
		target := groups[parentKey(parentID)]
		if targetIndex > len(target) {
			return ErrInvalidDocumentMove
		}
		target = append(target, "")
		copy(target[targetIndex+1:], target[targetIndex:])
		target[targetIndex] = nodeID

		if !sameOptionalString(currentParentID, parentID) {
			old := groups[parentKey(currentParentID)]
			for index, id := range old {
				if _, err := tx.Exec(ctx, `UPDATE document_nodes SET position=$2, updated_at=now() WHERE id=$1`, id, (index+1)*1000); err != nil {
					return err
				}
			}
		}
		for index, id := range target {
			if _, err := tx.Exec(ctx, `
				UPDATE document_nodes SET parent_id=$2, position=$3, updated_at=now() WHERE id=$1
			`, id, parentID, (index+1)*1000); err != nil {
				return err
			}
		}
		return nil
	})
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Store) TrashDocumentNode(ctx context.Context, ownerID, nodeID string) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE document_nodes n
		SET trashed_at=now(), updated_at=now()
		FROM projects p
		WHERE n.id=$1 AND p.id=n.project_id AND p.owner_user_id=$2
	`, nodeID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) RestoreDocumentNode(ctx context.Context, ownerID, nodeID string) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE document_nodes n
		SET trashed_at=NULL, updated_at=now()
		FROM projects p
		WHERE n.id=$1 AND p.id=n.project_id AND p.owner_user_id=$2
	`, nodeID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateDocumentNode(
	ctx context.Context,
	ownerID, nodeID string,
	title, status *string,
) (domain.DocumentNode, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE document_nodes n
		SET title=COALESCE($3, n.title), status=COALESCE($4, n.status), updated_at=now()
		FROM projects p
		WHERE n.id=$1 AND p.id=n.project_id AND p.owner_user_id=$2
	`, nodeID, ownerID, title, status)
	if err != nil {
		return domain.DocumentNode{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.DocumentNode{}, pgx.ErrNoRows
	}
	return s.GetDocumentNode(ctx, ownerID, nodeID)
}

func (s *Store) DuplicateDocumentNode(
	ctx context.Context,
	ownerID, nodeID string,
) (domain.DocumentNode, error) {
	var duplicate domain.DocumentNode
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var parentID *string
		var projectID, kind, title, status string
		var position float64
		var currentRevision int
		var content json.RawMessage
		var plainText string
		if err := tx.QueryRow(ctx, `
			SELECT n.project_id::text, n.parent_id::text, n.kind, n.title, n.status,
				n.position::float8, n.current_revision,
				COALESCE(r.content, 'null'::jsonb), COALESCE(r.plain_text, '')
			FROM document_nodes n
			JOIN projects p ON p.id=n.project_id
			LEFT JOIN document_revisions r
				ON r.node_id=n.id AND r.revision=n.current_revision
			WHERE n.id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
		`, nodeID, ownerID).Scan(
			&projectID,
			&parentID,
			&kind,
			&title,
			&status,
			&position,
			&currentRevision,
			&content,
			&plainText,
		); err != nil {
			return err
		}

		newID := uuid.NewString()
		newRevision := 0
		if currentRevision > 0 {
			newRevision = 1
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO document_nodes(
				id, project_id, parent_id, kind, title, status, position, current_revision
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id::text, project_id::text, parent_id::text, kind, title, status,
				position::float8, current_revision, trashed_at, created_at, updated_at
		`, newID, projectID, parentID, kind, title+" — копия", status, position+1,
			newRevision).Scan(
			&duplicate.ID,
			&duplicate.ProjectID,
			&duplicate.ParentID,
			&duplicate.Kind,
			&duplicate.Title,
			&duplicate.Status,
			&duplicate.Position,
			&duplicate.CurrentRevision,
			&duplicate.TrashedAt,
			&duplicate.CreatedAt,
			&duplicate.UpdatedAt,
		); err != nil {
			return err
		}
		if newRevision == 0 {
			return nil
		}
		duplicate.Content = content
		duplicate.PlainText = plainText
		schemaVersion, err := editor.SchemaVersion(content)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version,
				content, plain_text, source
			)
			VALUES($1, $2, 1, 0, $3, $4, $5, 'duplicate')
		`, uuid.NewString(), newID, schemaVersion, content, plainText)
		if err != nil {
			return err
		}
		return copyAnnotationsTx(ctx, tx, nodeID, newID, projectID, 1, content, nil, true)
	})
	return duplicate, err
}

func (s *Store) SearchProject(
	ctx context.Context,
	ownerID, projectID, query, scope string,
) ([]domain.SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []domain.SearchResult{}, nil
	}
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := s.Pool.Query(ctx, `
		SELECT n.id::text, n.kind, n.title, r.plain_text, r.revision
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.project_id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
		  AND r.plain_text ILIKE $3
		  AND ($4='all' OR ($4='manuscript' AND n.kind IN ('chapter','scene'))
		       OR ($4='drafts' AND n.kind='draft') OR ($4='notes' AND n.kind='note'))
		ORDER BY n.position
		LIMIT 100
	`, projectID, ownerID, pattern, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	results := make([]domain.SearchResult, 0)
	for rows.Next() {
		var nodeID, kind, title, text string
		var revision int
		if err := rows.Scan(&nodeID, &kind, &title, &text, &revision); err != nil {
			return nil, err
		}
		lowerText := strings.ToLower(text)
		count := strings.Count(lowerText, lowerQuery)
		snippet := searchSnippet(text, lowerQuery)
		results = append(results, domain.SearchResult{
			NodeID:      nodeID,
			NodeKind:    kind,
			Breadcrumb:  title,
			Snippet:     snippet,
			MatchCount:  count,
			Revision:    revision,
			SourceScope: "document",
		})
	}
	return results, rows.Err()
}

func searchSnippet(text, lowerQuery string) string {
	lowerRunes := []rune(strings.ToLower(text))
	textRunes := []rune(text)
	queryRunes := []rune(lowerQuery)
	index := -1
	for position := 0; position+len(queryRunes) <= len(lowerRunes); position++ {
		if string(lowerRunes[position:position+len(queryRunes)]) == lowerQuery {
			index = position
			break
		}
	}
	if index < 0 {
		return strings.TrimSpace(text)
	}
	start := max(0, index-60)
	end := min(len(textRunes), index+len(queryRunes)+100)
	return strings.TrimSpace(string(textRunes[start:end]))
}

type replaceCandidate struct {
	NodeID          string
	Title           string
	CurrentRevision int
	Content         json.RawMessage
	PlainText       string
}

type replaceQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Store) PreviewProjectReplace(
	ctx context.Context,
	ownerID, projectID, search, replacement, scope string,
) (ProjectReplacePreview, error) {
	candidates, err := listReplaceCandidates(ctx, s.Pool, ownerID, projectID, scope, false)
	if err != nil {
		return ProjectReplacePreview{}, err
	}
	return buildReplacePreview(projectID, search, replacement, scope, candidates)
}

func (s *Store) ApplyProjectReplace(
	ctx context.Context,
	ownerID, projectID, search, replacement, scope, previewToken string,
) (ProjectReplaceResult, error) {
	var result ProjectReplaceResult
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		candidates, err := listReplaceCandidates(ctx, tx, ownerID, projectID, scope, true)
		if err != nil {
			return err
		}
		preview, err := buildReplacePreview(projectID, search, replacement, scope, candidates)
		if err != nil {
			return err
		}
		if preview.Token != previewToken {
			return ErrReplacePreviewStale
		}

		result.OperationID = uuid.NewString()
		for _, candidate := range candidates {
			content, count, err := editor.ReplaceText(candidate.Content, search, replacement)
			if err != nil {
				return err
			}
			if count == 0 {
				continue
			}
			plainText, err := editor.PlainText(content)
			if err != nil {
				return err
			}
			nextRevision := candidate.CurrentRevision + 1
			schemaVersion, err := editor.SchemaVersion(content)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_revisions(
					id, node_id, revision, base_revision, schema_version,
					content, plain_text, source, operation_id
				)
				VALUES($1, $2, $3, $4, $5, $6, $7, 'project_replace', $8)
			`, uuid.NewString(), candidate.NodeID, nextRevision, candidate.CurrentRevision,
				schemaVersion, content, plainText, result.OperationID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE document_nodes
				SET current_revision=$2, updated_at=now()
				WHERE id=$1
			`, candidate.NodeID, nextRevision); err != nil {
				return err
			}
			result.Documents++
			result.Replacements += count
		}
		return nil
	})
	return result, err
}

func (s *Store) UndoProjectReplace(
	ctx context.Context,
	ownerID, projectID, operationID string,
) (ProjectReplaceResult, error) {
	var result ProjectReplaceResult
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT n.id::text, n.current_revision, applied.revision, applied.base_revision,
				previous.content, previous.plain_text
			FROM document_revisions applied
			JOIN document_nodes n ON n.id=applied.node_id
			JOIN projects p ON p.id=n.project_id
			JOIN document_revisions previous
				ON previous.node_id=applied.node_id AND previous.revision=applied.base_revision
			WHERE applied.operation_id=$1 AND n.project_id=$2 AND p.owner_user_id=$3
			ORDER BY n.position
			FOR UPDATE OF n
		`, operationID, projectID, ownerID)
		if err != nil {
			return err
		}
		defer rows.Close()

		type undoCandidate struct {
			nodeID, content, plainText string
			current, applied, base     int
		}
		candidates := make([]undoCandidate, 0)
		for rows.Next() {
			var candidate undoCandidate
			if err := rows.Scan(
				&candidate.nodeID,
				&candidate.current,
				&candidate.applied,
				&candidate.base,
				&candidate.content,
				&candidate.plainText,
			); err != nil {
				return err
			}
			if candidate.current != candidate.applied {
				return ErrReplacePreviewStale
			}
			candidates = append(candidates, candidate)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(candidates) == 0 {
			return pgx.ErrNoRows
		}

		result.OperationID = uuid.NewString()
		for _, candidate := range candidates {
			nextRevision := candidate.current + 1
			schemaVersion, err := editor.SchemaVersion(json.RawMessage(candidate.content))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_revisions(
					id, node_id, revision, base_revision, schema_version,
					content, plain_text, source, operation_id
				)
				VALUES($1, $2, $3, $4, $5, $6, $7, 'project_replace_undo', $8)
			`, uuid.NewString(), candidate.nodeID, nextRevision, candidate.current,
				schemaVersion, candidate.content, candidate.plainText, result.OperationID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE document_nodes SET current_revision=$2, updated_at=now() WHERE id=$1
			`, candidate.nodeID, nextRevision); err != nil {
				return err
			}
			result.Documents++
		}
		return nil
	})
	return result, err
}

func listReplaceCandidates(
	ctx context.Context,
	querier replaceQuerier,
	ownerID, projectID, scope string,
	lock bool,
) ([]replaceCandidate, error) {
	query := `
		SELECT n.id::text, n.title, n.current_revision, r.content, r.plain_text
		FROM document_nodes n
		JOIN projects p ON p.id=n.project_id
		JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.project_id=$1 AND p.owner_user_id=$2 AND n.trashed_at IS NULL
		  AND n.kind IN ('chapter','scene','draft','note')
		  AND ($3='all' OR ($3='manuscript' AND n.kind IN ('chapter','scene'))
		       OR ($3='drafts' AND n.kind='draft') OR ($3='notes' AND n.kind='note'))
		ORDER BY n.position, n.created_at`
	if lock {
		query += " FOR UPDATE OF n"
	}
	rows, err := querier.Query(ctx, query, projectID, ownerID, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]replaceCandidate, 0)
	for rows.Next() {
		var candidate replaceCandidate
		if err := rows.Scan(
			&candidate.NodeID,
			&candidate.Title,
			&candidate.CurrentRevision,
			&candidate.Content,
			&candidate.PlainText,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func buildReplacePreview(
	projectID, search, replacement, scope string,
	candidates []replaceCandidate,
) (ProjectReplacePreview, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return ProjectReplacePreview{}, fmt.Errorf("%w: empty search text", editor.ErrInvalidDocument)
	}
	preview := ProjectReplacePreview{
		Search:    search,
		Replace:   replacement,
		Scope:     scope,
		Documents: make([]ProjectReplaceDocument, 0),
	}
	tokenInput := strings.Builder{}
	_, _ = fmt.Fprintf(&tokenInput, "%s\x00%s\x00%s\x00%s", projectID, search, replacement, scope)
	for _, candidate := range candidates {
		content, count, err := editor.ReplaceText(candidate.Content, search, replacement)
		if err != nil {
			return ProjectReplacePreview{}, err
		}
		if count == 0 {
			continue
		}
		plainText, err := editor.PlainText(content)
		if err != nil {
			return ProjectReplacePreview{}, err
		}
		preview.TotalCount += count
		preview.Documents = append(preview.Documents, ProjectReplaceDocument{
			NodeID:      candidate.NodeID,
			Title:       candidate.Title,
			Revision:    candidate.CurrentRevision,
			MatchCount:  count,
			Snippet:     searchSnippet(candidate.PlainText, strings.ToLower(search)),
			Replacement: searchSnippet(plainText, strings.ToLower(replacement)),
		})
		_, _ = fmt.Fprintf(
			&tokenInput,
			"\x00%s:%d:%d",
			candidate.NodeID,
			candidate.CurrentRevision,
			count,
		)
	}
	sum := sha256.Sum256([]byte(tokenInput.String()))
	preview.Token = fmt.Sprintf("%x", sum)
	return preview, nil
}

func AsConflict(err error) (*RevisionConflictError, bool) {
	var conflict *RevisionConflictError
	ok := errors.As(err, &conflict)
	return conflict, ok
}
