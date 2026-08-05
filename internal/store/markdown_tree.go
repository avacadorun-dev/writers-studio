package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/editor"
	"writers.studio/internal/markdowntree"
)

type MarkdownTreeImportInput struct {
	Project          markdowntree.PreparedProject
	MediaStorageKeys map[string]string
}

func (s *Store) ImportMarkdownTree(
	ctx context.Context,
	ownerID string,
	input MarkdownTreeImportInput,
) (ProjectTransferImportResult, error) {
	project := input.Project
	if project.ID == "" || project.FirstNodeID == "" || len(project.Nodes) == 0 {
		return ProjectTransferImportResult{}, fmt.Errorf("invalid prepared markdown tree")
	}
	structure, err := json.Marshal(project.Structure)
	if err != nil {
		return ProjectTransferImportResult{}, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO projects(
				id, owner_user_id, title, kind, language, synopsis, status,
				structure_revision, structure_schema
			)
			VALUES($1, $2, $3, 'free', $4, '', 'active', 1, $5)
		`, project.ID, ownerID, project.Title, project.Language, structure); err != nil {
			return err
		}

		nodeRows := make([][]any, 0, len(project.Nodes))
		revisionRows := make([][]any, 0, len(project.Nodes))
		for _, node := range project.Nodes {
			content := node.Content
			currentRevision := 0
			if len(content) > 0 {
				schemaVersion, schemaErr := editor.SchemaVersion(content)
				if schemaErr != nil || editor.Validate(content) != nil {
					return fmt.Errorf("invalid prepared markdown document %s", node.ID)
				}
				currentRevision = 1
				revisionRows = append(revisionRows, []any{
					uuid.NewString(), node.ID, 1, 0, schemaVersion, content, node.PlainText,
					"markdown_tree_import",
				})
			}
			nodeRows = append(nodeRows, []any{
				node.ID, project.ID, node.ParentID, node.Kind, node.Title, "draft",
				node.Position, currentRevision, node.Frontmatter,
			})
		}
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"document_nodes"},
			[]string{
				"id", "project_id", "parent_id", "kind", "title", "status",
				"position", "current_revision", "source_frontmatter",
			},
			pgx.CopyFromRows(nodeRows),
		); err != nil {
			return fmt.Errorf("copy markdown tree nodes: %w", err)
		}
		if len(revisionRows) > 0 {
			if _, err := tx.CopyFrom(
				ctx,
				pgx.Identifier{"document_revisions"},
				[]string{
					"id", "node_id", "revision", "base_revision", "schema_version",
					"content", "plain_text", "source",
				},
				pgx.CopyFromRows(revisionRows),
			); err != nil {
				return fmt.Errorf("copy markdown tree revisions: %w", err)
			}
		}

		if len(project.Media) > 0 {
			mediaRows := make([][]any, 0, len(project.Media))
			for _, media := range project.Media {
				storageKey := input.MediaStorageKeys[media.ID]
				if storageKey == "" {
					return fmt.Errorf("markdown tree media storage key is missing")
				}
				mediaRows = append(mediaRows, []any{
					media.ID, project.ID, ownerID, storageKey, media.MediaType,
					media.OriginalName, media.SHA256, media.ByteSize, media.Width,
					media.Height, media.SourcePath,
				})
			}
			if _, err := tx.CopyFrom(
				ctx,
				pgx.Identifier{"project_media"},
				[]string{
					"id", "project_id", "owner_user_id", "storage_key", "media_type",
					"original_name", "sha256", "byte_size", "width", "height", "source_path",
				},
				pgx.CopyFromRows(mediaRows),
			); err != nil {
				return fmt.Errorf("copy markdown tree media: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return ProjectTransferImportResult{}, fmt.Errorf("import markdown tree: %w", err)
	}
	return ProjectTransferImportResult{
		ProjectID: project.ID, FirstNodeID: project.FirstNodeID, Title: project.Title,
	}, nil
}
