package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
)

type CreateProjectInput struct {
	Title    string
	Kind     string
	Language string
	Synopsis string
	WordGoal *int
}

type UpdateProjectInput struct {
	Title    string
	Language string
	Synopsis string
	WordGoal *int
}

const MaxProjectAssistantStepLimit = 1_000_000

type ProjectAssistantSettings struct {
	StepLimit *int `json:"stepLimit,omitempty"`
}

func ValidProjectAssistantStepLimit(limit *int) bool {
	return limit == nil || (*limit >= 1 && *limit <= MaxProjectAssistantStepLimit)
}

type ProjectWorkspace struct {
	Project domain.Project        `json:"project"`
	Nodes   []domain.DocumentNode `json:"nodes"`
}

type ProjectPurgePlan struct {
	Title      string
	ObjectKeys []string
}

func (s *Store) CreateProject(
	ctx context.Context,
	ownerID string,
	input CreateProjectInput,
) (ProjectWorkspace, error) {
	projectID := uuid.NewString()
	chapterID := uuid.NewString()
	revisionID := uuid.NewString()
	content := emptyDocument()
	structure := DefaultProjectStructure(input.Kind, input.Language)
	if err := ValidateProjectStructure(structure); err != nil {
		return ProjectWorkspace{}, err
	}
	structureJSON, err := json.Marshal(structure)
	if err != nil {
		return ProjectWorkspace{}, err
	}
	initialType := initialProjectNodeType(structure)
	initialTitle := initialProjectNodeTitle(initialType, input.Language)

	var project domain.Project
	var chapter domain.DocumentNode
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var returnedStructure []byte
		err := tx.QueryRow(ctx, `
			INSERT INTO projects(
				id, owner_user_id, title, kind, language, synopsis, word_goal, structure_schema
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id::text, title, kind, language, synopsis, word_goal, status,
				structure_revision, structure_schema, trashed_at, created_at, updated_at
		`, projectID, ownerID, input.Title, input.Kind, input.Language, input.Synopsis,
			input.WordGoal, structureJSON).Scan(
			&project.ID,
			&project.Title,
			&project.Kind,
			&project.Language,
			&project.Synopsis,
			&project.WordGoal,
			&project.Status,
			&project.StructureRevision,
			&returnedStructure,
			&project.TrashedAt,
			&project.CreatedAt,
			&project.UpdatedAt,
		)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(returnedStructure, &project.Structure); err != nil {
			return err
		}
		project.Structure = normalizeProjectStructure(project.Structure)

		err = tx.QueryRow(ctx, `
			INSERT INTO document_nodes(
				id, project_id, kind, title, status, position, current_revision
			)
			VALUES($1, $2, $3, $4, 'draft', 1000, 1)
			RETURNING id::text, project_id::text, parent_id::text, kind, title, status,
				position::float8, current_revision, trashed_at, created_at, updated_at
		`, chapterID, projectID, initialType.Key, initialTitle).Scan(
			&chapter.ID,
			&chapter.ProjectID,
			&chapter.ParentID,
			&chapter.Kind,
			&chapter.Title,
			&chapter.Status,
			&chapter.Position,
			&chapter.CurrentRevision,
			&chapter.TrashedAt,
			&chapter.CreatedAt,
			&chapter.UpdatedAt,
		)
		if err != nil {
			return err
		}
		chapter.Content = content

		_, err = tx.Exec(ctx, `
			INSERT INTO document_revisions(
				id, node_id, revision, base_revision, schema_version, content, plain_text, source
			)
			VALUES($1, $2, 1, 0, 1, $3, '', 'project_create')
		`, revisionID, chapterID, content)
		return err
	})
	if err != nil {
		return ProjectWorkspace{}, fmt.Errorf("create project transaction: %w", err)
	}

	return ProjectWorkspace{Project: project, Nodes: []domain.DocumentNode{chapter}}, nil
}

func (s *Store) ListProjects(ctx context.Context, ownerID, status string) ([]domain.Project, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, title, kind, language, synopsis, word_goal, status,
			structure_revision, structure_schema, trashed_at, created_at, updated_at
		FROM projects
		WHERE owner_user_id=$1 AND status=$2
		ORDER BY updated_at DESC, created_at DESC
	`, ownerID, status)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := make([]domain.Project, 0)
	for rows.Next() {
		var project domain.Project
		var structure []byte
		if err := rows.Scan(
			&project.ID,
			&project.Title,
			&project.Kind,
			&project.Language,
			&project.Synopsis,
			&project.WordGoal,
			&project.Status,
			&project.StructureRevision,
			&structure,
			&project.TrashedAt,
			&project.CreatedAt,
			&project.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		if err := json.Unmarshal(structure, &project.Structure); err != nil {
			return nil, fmt.Errorf("decode project structure: %w", err)
		}
		project.Structure = normalizeProjectStructure(project.Structure)
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, ownerID, projectID string) (ProjectWorkspace, error) {
	project, err := s.getProjectRecord(ctx, ownerID, projectID)
	if err != nil {
		return ProjectWorkspace{}, err
	}

	nodes, err := s.ListDocumentNodes(ctx, ownerID, projectID, true)
	if err != nil {
		return ProjectWorkspace{}, err
	}
	return ProjectWorkspace{Project: project, Nodes: nodes}, nil
}

func (s *Store) GetProjectOutline(ctx context.Context, ownerID, projectID string) (ProjectWorkspace, error) {
	project, err := s.getProjectRecord(ctx, ownerID, projectID)
	if err != nil {
		return ProjectWorkspace{}, err
	}
	nodes, err := s.ListDocumentNodeSummaries(ctx, ownerID, projectID, true)
	if err != nil {
		return ProjectWorkspace{}, err
	}
	return ProjectWorkspace{Project: project, Nodes: nodes}, nil
}

func (s *Store) getProjectRecord(ctx context.Context, ownerID, projectID string) (domain.Project, error) {
	var project domain.Project
	var structure []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, title, kind, language, synopsis, word_goal, status,
			structure_revision, structure_schema, trashed_at, created_at, updated_at
		FROM projects
		WHERE id=$1 AND owner_user_id=$2
	`, projectID, ownerID).Scan(
		&project.ID,
		&project.Title,
		&project.Kind,
		&project.Language,
		&project.Synopsis,
		&project.WordGoal,
		&project.Status,
		&project.StructureRevision,
		&structure,
		&project.TrashedAt,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return domain.Project{}, err
	}
	if err := json.Unmarshal(structure, &project.Structure); err != nil {
		return domain.Project{}, err
	}
	project.Structure = normalizeProjectStructure(project.Structure)
	return project, nil
}

func (s *Store) UpdateProject(
	ctx context.Context,
	ownerID string,
	projectID string,
	input UpdateProjectInput,
) (domain.Project, error) {
	var project domain.Project
	var structure []byte
	err := s.Pool.QueryRow(ctx, `
		UPDATE projects
		SET title=$3, language=$4, synopsis=$5, word_goal=$6, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
		RETURNING id::text, title, kind, language, synopsis, word_goal, status,
			structure_revision, structure_schema, trashed_at, created_at, updated_at
	`, projectID, ownerID, input.Title, input.Language, input.Synopsis, input.WordGoal).Scan(
		&project.ID,
		&project.Title,
		&project.Kind,
		&project.Language,
		&project.Synopsis,
		&project.WordGoal,
		&project.Status,
		&project.StructureRevision,
		&structure,
		&project.TrashedAt,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return domain.Project{}, err
	}
	if err := json.Unmarshal(structure, &project.Structure); err != nil {
		return domain.Project{}, err
	}
	project.Structure = normalizeProjectStructure(project.Structure)
	return project, nil
}

func (s *Store) GetProjectAssistantSettings(
	ctx context.Context,
	ownerID string,
	projectID string,
) (ProjectAssistantSettings, error) {
	var settings ProjectAssistantSettings
	if err := s.Pool.QueryRow(ctx, `
		SELECT assistant_step_limit
		FROM projects
		WHERE id=$1 AND owner_user_id=$2
	`, projectID, ownerID).Scan(&settings.StepLimit); err != nil {
		return ProjectAssistantSettings{}, err
	}
	return settings, nil
}

func (s *Store) UpdateProjectAssistantSettings(
	ctx context.Context,
	ownerID string,
	projectID string,
	settings ProjectAssistantSettings,
) (ProjectAssistantSettings, error) {
	if !ValidProjectAssistantStepLimit(settings.StepLimit) {
		return ProjectAssistantSettings{}, errors.New("invalid project assistant step limit")
	}
	var updated ProjectAssistantSettings
	if err := s.Pool.QueryRow(ctx, `
		UPDATE projects
		SET assistant_step_limit=$3, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
		RETURNING assistant_step_limit
	`, projectID, ownerID, settings.StepLimit).Scan(&updated.StepLimit); err != nil {
		return ProjectAssistantSettings{}, err
	}
	return updated, nil
}

func (s *Store) UpdateProjectStatus(
	ctx context.Context,
	ownerID string,
	projectID string,
	status string,
) (domain.Project, error) {
	var project domain.Project
	var structure []byte
	var trashedAt any
	var purgeAfter any
	if status == "trash" {
		now := time.Now()
		trashedAt = now
		purgeAfter = now.Add(30 * 24 * time.Hour)
	}

	err := s.Pool.QueryRow(ctx, `
		UPDATE projects
		SET status=$3, trashed_at=$4, purge_after=$5, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2
		RETURNING id::text, title, kind, language, synopsis, word_goal, status,
			structure_revision, structure_schema, trashed_at, created_at, updated_at
	`, projectID, ownerID, status, trashedAt, purgeAfter).Scan(
		&project.ID,
		&project.Title,
		&project.Kind,
		&project.Language,
		&project.Synopsis,
		&project.WordGoal,
		&project.Status,
		&project.StructureRevision,
		&structure,
		&project.TrashedAt,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err == nil {
		err = json.Unmarshal(structure, &project.Structure)
		project.Structure = normalizeProjectStructure(project.Structure)
	}
	return project, err
}

func (s *Store) GetProjectPurgePlan(
	ctx context.Context,
	ownerID, projectID string,
) (ProjectPurgePlan, error) {
	var plan ProjectPurgePlan
	var status string
	if err := s.Pool.QueryRow(ctx, `
		SELECT title, status
		FROM projects
		WHERE id=$1 AND owner_user_id=$2
	`, projectID, ownerID).Scan(&plan.Title, &status); err != nil {
		return ProjectPurgePlan{}, err
	}
	if status != "trash" {
		return ProjectPurgePlan{}, fmt.Errorf("project must be in trash before purge")
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT storage_key
		FROM audio_chunks c
		JOIN dictation_sessions s ON s.id=c.session_id
		WHERE s.project_id=$1
		UNION
		SELECT storage_key
		FROM audio_assets a
		JOIN dictation_sessions s ON s.id=a.session_id
		WHERE s.project_id=$1
		UNION
		SELECT storage_key
		FROM export_jobs
		WHERE project_id=$1 AND storage_key IS NOT NULL
		UNION
		SELECT storage_key
		FROM project_media
		WHERE project_id=$1
	`, projectID)
	if err != nil {
		return ProjectPurgePlan{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return ProjectPurgePlan{}, err
		}
		plan.ObjectKeys = append(plan.ObjectKeys, key)
	}
	return plan, rows.Err()
}

func (s *Store) PurgeTrashedProject(
	ctx context.Context,
	ownerID, projectID string,
	objectCount int,
) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM projects
				WHERE id=$1 AND owner_user_id=$2 AND status='trash'
			)
		`, projectID, ownerID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM background_jobs job
			WHERE job.payload->>'sessionId' IN (
				SELECT session.id::text
				FROM dictation_sessions session
				WHERE session.project_id=$1
			)
			   OR job.payload->>'exportId' IN (
				SELECT export.id::text
				FROM export_jobs export
				WHERE export.project_id=$1
			)
		`, projectID); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]int{"deletedObjectCount": objectCount})
		if _, err := tx.Exec(ctx, `
			INSERT INTO deletion_audit_events(
				id, owner_user_id, subject_type, subject_id, event_type, metadata
			)
			VALUES($1, $2, 'project', $3, 'project_purged', $4)
		`, uuid.NewString(), ownerID, projectID, metadata); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM projects
			WHERE id=$1 AND owner_user_id=$2 AND status='trash'
		`, projectID, ownerID)
		return err
	})
}

func emptyDocument() json.RawMessage {
	blockID := uuid.NewString()
	return json.RawMessage(fmt.Sprintf(
		`{"type":"doc","attrs":{"schemaVersion":1},"content":[{"type":"paragraph","attrs":{"blockId":%q,"sourceRefs":[]}}]}`,
		blockID,
	))
}

func defaultChapterTitle(language string) string {
	if language == "en" {
		return "Chapter 1"
	}
	return "Глава 1"
}

func initialProjectNodeType(structure domain.ProjectStructure) domain.ProjectNodeType {
	for _, nodeType := range structure.NodeTypes {
		if nodeType.CanBeRoot && nodeType.ContentBearing {
			return nodeType
		}
	}
	for _, nodeType := range structure.NodeTypes {
		if nodeType.CanBeRoot {
			return nodeType
		}
	}
	return structure.NodeTypes[0]
}

func initialProjectNodeTitle(nodeType domain.ProjectNodeType, language string) string {
	if language == "en" {
		return nodeType.Label + " 1"
	}
	return nodeType.Label + " 1"
}
