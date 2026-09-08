package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
)

type CreateExportInput struct {
	Format        string
	NodeIDs       []string
	TitlePage     bool
	ChapterTitles bool
	IncludeNotes  bool
	Profile       domain.ExportProfile
}

func (s *Store) CreateExport(
	ctx context.Context,
	ownerID, projectID string,
	input CreateExportInput,
) (domain.ExportJob, error) {
	jobID := uuid.NewString()
	var job domain.ExportJob
	err := pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		var projectTitle, projectLanguage, projectSynopsis string
		var projectStructureJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT title, language, synopsis, structure_schema
			FROM projects
			WHERE id=$1 AND owner_user_id=$2 AND status='active'
		`, projectID, ownerID).Scan(&projectTitle, &projectLanguage, &projectSynopsis, &projectStructureJSON); err != nil {
			return err
		}
		legacyProfile := input.Profile.SchemaVersion == 0
		input.Profile = exportformat.NormalizeProfile(input.Profile, projectTitle, projectSynopsis)
		if legacyProfile {
			input.Profile.TitlePage = input.TitlePage
			input.Profile.ChapterTitles = input.ChapterTitles
			input.Profile.IncludeNotes = input.IncludeNotes
		}
		if err := exportformat.ValidateProfile(input.Profile); err != nil {
			return err
		}
		input.TitlePage = input.Profile.TitlePage
		input.ChapterTitles = input.Profile.ChapterTitles
		input.IncludeNotes = input.Profile.IncludeNotes
		if input.Profile.Metadata.CoverAssetID != nil {
			var coverExists bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM project_media
					WHERE id=$1 AND project_id=$2 AND owner_user_id=$3
				)
			`, *input.Profile.Metadata.CoverAssetID, projectID, ownerID).Scan(&coverExists); err != nil {
				return err
			}
			if !coverExists {
				return exportformat.ErrInvalidProfile
			}
		}
		if len(input.NodeIDs) != len(uniqueStrings(input.NodeIDs)) {
			return fmt.Errorf("export contains duplicate documents")
		}
		if input.Format == "markdown_tree" {
			var structure domain.ProjectStructure
			if err := json.Unmarshal(projectStructureJSON, &structure); err != nil {
				return err
			}
			snapshot, err := buildMarkdownTreeSnapshot(
				ctx, tx, jobID, ownerID, projectID, projectTitle, projectLanguage,
				input.Profile, structure,
			)
			if err != nil {
				return err
			}
			return queueExportSnapshot(ctx, tx, jobID, ownerID, projectID, input.Format, snapshot, &job)
		}

		rows, err := tx.Query(ctx, `
			WITH RECURSIVE outline AS (
				SELECT n.*, ARRAY[n.position]::double precision[] AS sort_path
				FROM document_nodes n
				WHERE n.project_id=$1 AND n.parent_id IS NULL AND n.trashed_at IS NULL

				UNION ALL

				SELECT child.*, parent.sort_path || child.position
				FROM document_nodes child
				JOIN outline parent ON child.parent_id=parent.id
				WHERE child.project_id=$1 AND child.trashed_at IS NULL
			)
			SELECT n.id::text, n.kind, n.title, n.current_revision, r.content, r.plain_text
			FROM outline n
			JOIN document_revisions r ON r.node_id=n.id AND r.revision=n.current_revision
			WHERE n.current_revision > 0
			  AND ($2 OR n.kind <> 'note')
			  AND (cardinality($3::uuid[]) = 0 OR n.id = ANY($3::uuid[]))
			ORDER BY n.sort_path, n.created_at
		`, projectID, input.IncludeNotes, input.NodeIDs)
		if err != nil {
			return err
		}
		defer rows.Close()

		snapshot := exportformat.Snapshot{
			SchemaVersion:   2,
			ExportID:        jobID,
			ProjectID:       projectID,
			ProjectTitle:    projectTitle,
			ProjectLanguage: projectLanguage,
			TitlePage:       input.TitlePage,
			ChapterTitles:   input.ChapterTitles,
			IncludeNotes:    input.IncludeNotes,
			Profile:         input.Profile,
			Documents:       make([]exportformat.SnapshotDocument, 0),
		}
		for rows.Next() {
			var document exportformat.SnapshotDocument
			if err := rows.Scan(
				&document.NodeID,
				&document.Kind,
				&document.Title,
				&document.Revision,
				&document.Content,
				&document.PlainText,
			); err != nil {
				return err
			}
			snapshot.Documents = append(snapshot.Documents, document)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(snapshot.Documents) == 0 {
			return fmt.Errorf("export contains no documents")
		}
		if len(input.NodeIDs) > 0 {
			byID := make(map[string]exportformat.SnapshotDocument, len(snapshot.Documents))
			for _, document := range snapshot.Documents {
				byID[document.NodeID] = document
			}
			ordered := make([]exportformat.SnapshotDocument, 0, len(input.NodeIDs))
			for _, nodeID := range input.NodeIDs {
				document, exists := byID[nodeID]
				if !exists {
					return fmt.Errorf("export document is unavailable")
				}
				ordered = append(ordered, document)
			}
			snapshot.Documents = ordered
		}

		mediaIDs := make(map[string]bool)
		for _, document := range snapshot.Documents {
			for _, mediaID := range documentMediaIDs(document.Content) {
				mediaIDs[mediaID] = true
			}
		}
		if input.Profile.Metadata.CoverAssetID != nil {
			mediaIDs[*input.Profile.Metadata.CoverAssetID] = true
		}
		if len(mediaIDs) > 0 {
			ids := make([]string, 0, len(mediaIDs))
			for id := range mediaIDs {
				ids = append(ids, id)
			}
			mediaRows, err := tx.Query(ctx, `
				SELECT id::text, media_type, storage_key, original_name, source_path, sha256, width, height
				FROM project_media
				WHERE project_id=$1 AND owner_user_id=$2 AND id=ANY($3::uuid[])
					AND media_type LIKE 'image/%'
			`, projectID, ownerID, ids)
			if err != nil {
				return err
			}
			for mediaRows.Next() {
				var item exportformat.SnapshotMedia
				if err := mediaRows.Scan(
					&item.ID, &item.MediaType, &item.StorageKey, &item.OriginalName,
					&item.SourcePath, &item.SHA256, &item.Width, &item.Height,
				); err != nil {
					mediaRows.Close()
					return err
				}
				snapshot.Media = append(snapshot.Media, item)
			}
			if err := mediaRows.Err(); err != nil {
				mediaRows.Close()
				return err
			}
			mediaRows.Close()
			if len(snapshot.Media) != len(mediaIDs) {
				return fmt.Errorf("export media is unavailable")
			}
		}
		return queueExportSnapshot(ctx, tx, jobID, ownerID, projectID, input.Format, snapshot, &job)
	})
	return job, err
}

func buildMarkdownTreeSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	jobID string,
	ownerID string,
	projectID string,
	projectTitle string,
	projectLanguage string,
	profile domain.ExportProfile,
	structure domain.ProjectStructure,
) (exportformat.Snapshot, error) {
	contentKinds := make(map[string]bool, len(structure.NodeTypes))
	for _, nodeType := range structure.NodeTypes {
		contentKinds[nodeType.Key] = nodeType.ContentBearing
	}
	snapshot := exportformat.Snapshot{
		SchemaVersion: 3, ExportID: jobID, ProjectID: projectID,
		ProjectTitle: projectTitle, ProjectLanguage: projectLanguage, Profile: profile,
		Documents: make([]exportformat.SnapshotDocument, 0),
		TreeNodes: make([]exportformat.SnapshotTreeNode, 0),
	}
	rows, err := tx.Query(ctx, `
		SELECT n.id::text, n.parent_id::text, n.kind, n.title, n.position::float8,
			n.current_revision, COALESCE(r.content, 'null'::jsonb),
			COALESCE(r.plain_text, ''), n.source_frontmatter
		FROM document_nodes n
		LEFT JOIN document_revisions r
			ON r.node_id=n.id AND r.revision=n.current_revision
		WHERE n.project_id=$1 AND n.trashed_at IS NULL
		ORDER BY n.position, n.created_at
	`, projectID)
	if err != nil {
		return exportformat.Snapshot{}, err
	}
	for rows.Next() {
		var node exportformat.SnapshotTreeNode
		if err := rows.Scan(
			&node.NodeID, &node.ParentID, &node.Kind, &node.Title, &node.Position,
			&node.Revision, &node.Content, &node.PlainText, &node.Frontmatter,
		); err != nil {
			rows.Close()
			return exportformat.Snapshot{}, err
		}
		node.ContentBearing = contentKinds[node.Kind]
		snapshot.TreeNodes = append(snapshot.TreeNodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return exportformat.Snapshot{}, err
	}
	rows.Close()
	if len(snapshot.TreeNodes) == 0 {
		return exportformat.Snapshot{}, fmt.Errorf("export contains no nodes")
	}

	mediaRows, err := tx.Query(ctx, `
		SELECT id::text, media_type, storage_key, original_name, source_path,
			sha256, width, height
		FROM project_media
		WHERE project_id=$1 AND owner_user_id=$2 AND media_type LIKE 'image/%'
		ORDER BY created_at, id
	`, projectID, ownerID)
	if err != nil {
		return exportformat.Snapshot{}, err
	}
	for mediaRows.Next() {
		var item exportformat.SnapshotMedia
		if err := mediaRows.Scan(
			&item.ID, &item.MediaType, &item.StorageKey, &item.OriginalName,
			&item.SourcePath, &item.SHA256, &item.Width, &item.Height,
		); err != nil {
			mediaRows.Close()
			return exportformat.Snapshot{}, err
		}
		snapshot.Media = append(snapshot.Media, item)
	}
	if err := mediaRows.Err(); err != nil {
		mediaRows.Close()
		return exportformat.Snapshot{}, err
	}
	mediaRows.Close()
	return snapshot, nil
}

func queueExportSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	jobID string,
	ownerID string,
	projectID string,
	format string,
	snapshot exportformat.Snapshot,
	job *domain.ExportJob,
) error {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO export_jobs(id, project_id, owner_user_id, format, status, snapshot)
		VALUES($1, $2, $3, $4, 'queued', $5)
		RETURNING id::text, project_id::text, format, status, snapshot, storage_key,
			error_code, created_at, updated_at
	`, jobID, projectID, ownerID, format, snapshotJSON).Scan(
		&job.ID, &job.ProjectID, &job.Format, &job.Status, &job.Snapshot,
		&job.StorageKey, &job.ErrorCode, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"exportId": jobID})
	_, err = tx.Exec(ctx, `
		INSERT INTO background_jobs(id, kind, status, idempotency_key, payload)
		VALUES($1, 'render_export', 'queued', $2, $3)
	`, uuid.NewString(), "export:"+jobID, payload)
	return err
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (s *Store) GetExport(ctx context.Context, ownerID, exportID string) (domain.ExportJob, error) {
	var job domain.ExportJob
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, project_id::text, format, status, snapshot, storage_key,
			error_code, created_at, updated_at
		FROM export_jobs
		WHERE id=$1 AND owner_user_id=$2
	`, exportID, ownerID).Scan(
		&job.ID,
		&job.ProjectID,
		&job.Format,
		&job.Status,
		&job.Snapshot,
		&job.StorageKey,
		&job.ErrorCode,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	return job, err
}

func (s *Store) GetExportForWorker(ctx context.Context, exportID string) (domain.ExportJob, error) {
	var job domain.ExportJob
	err := s.Pool.QueryRow(ctx, `
		UPDATE export_jobs
		SET status='running', updated_at=now()
		WHERE id=$1
		RETURNING id::text, project_id::text, format, status, snapshot, storage_key,
			error_code, created_at, updated_at
	`, exportID).Scan(
		&job.ID,
		&job.ProjectID,
		&job.Format,
		&job.Status,
		&job.Snapshot,
		&job.StorageKey,
		&job.ErrorCode,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	return job, err
}

func (s *Store) CompleteExport(ctx context.Context, exportID, storageKey string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE export_jobs
		SET status='ready', storage_key=$2, error_code=NULL, updated_at=now()
		WHERE id=$1
	`, exportID, storageKey)
	return err
}

func (s *Store) FailExport(ctx context.Context, exportID, errorCode string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE export_jobs
		SET status='failed', error_code=$2, updated_at=now()
		WHERE id=$1
	`, exportID, errorCode)
	return err
}
