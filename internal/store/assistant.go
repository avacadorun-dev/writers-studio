package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
)

type ProviderConnectionRecord struct {
	ID                   string                       `json:"id"`
	OwnerUserID          string                       `json:"-"`
	Name                 string                       `json:"name"`
	BaseURL              string                       `json:"baseUrl"`
	Dialect              assistant.ProviderDialect    `json:"dialect"`
	Model                string                       `json:"model"`
	EncryptedAPIKey      []byte                       `json:"-"`
	APIKeyNonce          []byte                       `json:"-"`
	APIKeyFingerprint    string                       `json:"-"`
	APIKeyLast4          string                       `json:"apiKeyLast4"`
	EncryptedHeaders     []byte                       `json:"-"`
	HeadersNonce         []byte                       `json:"-"`
	RequestTimeoutMS     int                          `json:"requestTimeoutMs"`
	ContextWindow        *int                         `json:"contextWindow,omitempty"`
	AllowPrivateEndpoint bool                         `json:"allowPrivateEndpoint"`
	Capabilities         assistant.CapabilitySnapshot `json:"capabilities"`
	Status               string                       `json:"status"`
	StatusDetail         string                       `json:"statusDetail"`
	IsDefault            bool                         `json:"isDefault"`
	CheckedAt            *time.Time                   `json:"checkedAt,omitempty"`
	CreatedAt            time.Time                    `json:"createdAt"`
	UpdatedAt            time.Time                    `json:"updatedAt"`
}

func (s *Store) CreateProviderConnection(
	ctx context.Context,
	connection ProviderConnectionRecord,
) (ProviderConnectionRecord, error) {
	if connection.ID == "" {
		connection.ID = uuid.NewString()
	}
	capabilities, err := json.Marshal(connection.Capabilities)
	if err != nil {
		return ProviderConnectionRecord{}, fmt.Errorf("encode provider capabilities: %w", err)
	}
	if strings.TrimSpace(connection.Name) == "" || strings.TrimSpace(connection.BaseURL) == "" ||
		strings.TrimSpace(connection.Model) == "" || len(connection.EncryptedAPIKey) == 0 ||
		len(connection.APIKeyNonce) == 0 {
		return ProviderConnectionRecord{}, errors.New("invalid provider connection")
	}
	if connection.RequestTimeoutMS == 0 {
		connection.RequestTimeoutMS = 600_000
	}
	if connection.Status == "" {
		connection.Status = "unchecked"
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if connection.IsDefault {
			if _, err := tx.Exec(ctx, `
				UPDATE assistant_provider_connections
				SET is_default=false, updated_at=now()
				WHERE owner_user_id=$1 AND is_default
			`, connection.OwnerUserID); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `
			INSERT INTO assistant_provider_connections(
				id, owner_user_id, name, base_url, dialect, model,
				encrypted_api_key, api_key_nonce, api_key_fingerprint, api_key_last4,
				encrypted_headers, headers_nonce, request_timeout_ms, context_window,
				allow_private_endpoint, capabilities, status, status_detail, is_default,
				checked_at
			)
			VALUES(
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20
			)
			RETURNING created_at, updated_at
		`,
			connection.ID,
			connection.OwnerUserID,
			strings.TrimSpace(connection.Name),
			connection.BaseURL,
			connection.Dialect,
			connection.Model,
			connection.EncryptedAPIKey,
			connection.APIKeyNonce,
			connection.APIKeyFingerprint,
			connection.APIKeyLast4,
			nullableBytes(connection.EncryptedHeaders),
			nullableBytes(connection.HeadersNonce),
			connection.RequestTimeoutMS,
			connection.ContextWindow,
			connection.AllowPrivateEndpoint,
			capabilities,
			connection.Status,
			connection.StatusDetail,
			connection.IsDefault,
			connection.CheckedAt,
		).Scan(&connection.CreatedAt, &connection.UpdatedAt)
	})
	if err != nil {
		return ProviderConnectionRecord{}, err
	}
	return connection, nil
}

func (s *Store) ListProviderConnections(
	ctx context.Context,
	ownerID string,
) ([]ProviderConnectionRecord, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			id, owner_user_id, name, base_url, dialect, model,
			encrypted_api_key, api_key_nonce, api_key_fingerprint, api_key_last4,
			COALESCE(encrypted_headers, ''::bytea), COALESCE(headers_nonce, ''::bytea),
			request_timeout_ms, context_window, allow_private_endpoint,
			capabilities, status, status_detail, is_default, checked_at, created_at, updated_at
		FROM assistant_provider_connections
		WHERE owner_user_id=$1 AND status <> 'disabled'
		ORDER BY is_default DESC, updated_at DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ProviderConnectionRecord, 0)
	for rows.Next() {
		connection, err := scanProviderConnection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, connection)
	}
	return result, rows.Err()
}

func (s *Store) GetProviderConnection(
	ctx context.Context,
	ownerID string,
	connectionID string,
) (ProviderConnectionRecord, error) {
	return scanProviderConnection(s.Pool.QueryRow(ctx, `
		SELECT
			id, owner_user_id, name, base_url, dialect, model,
			encrypted_api_key, api_key_nonce, api_key_fingerprint, api_key_last4,
			COALESCE(encrypted_headers, ''::bytea), COALESCE(headers_nonce, ''::bytea),
			request_timeout_ms, context_window, allow_private_endpoint,
			capabilities, status, status_detail, is_default, checked_at, created_at, updated_at
		FROM assistant_provider_connections
		WHERE id=$1 AND owner_user_id=$2 AND status <> 'disabled'
	`, connectionID, ownerID))
}

func (s *Store) UpdateProviderProbe(
	ctx context.Context,
	ownerID string,
	connectionID string,
	status string,
	detail string,
	capabilities assistant.CapabilitySnapshot,
) (ProviderConnectionRecord, error) {
	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return ProviderConnectionRecord{}, err
	}
	command, err := s.Pool.Exec(ctx, `
		UPDATE assistant_provider_connections
		SET status=$3, status_detail=$4, capabilities=$5, checked_at=now(), updated_at=now()
		WHERE id=$1 AND owner_user_id=$2 AND status <> 'disabled'
	`, connectionID, ownerID, status, detail, encoded)
	if err != nil {
		return ProviderConnectionRecord{}, err
	}
	if command.RowsAffected() == 0 {
		return ProviderConnectionRecord{}, pgx.ErrNoRows
	}
	return s.GetProviderConnection(ctx, ownerID, connectionID)
}

func (s *Store) DisableProviderConnection(
	ctx context.Context,
	ownerID string,
	connectionID string,
) error {
	command, err := s.Pool.Exec(ctx, `
		UPDATE assistant_provider_connections
		SET status='disabled', is_default=false, encrypted_api_key=''::bytea,
			api_key_nonce=''::bytea, encrypted_headers=NULL, headers_nonce=NULL, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2 AND status <> 'disabled'
	`, connectionID, ownerID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) SetDefaultProviderConnection(
	ctx context.Context,
	ownerID string,
	connectionID string,
) (ProviderConnectionRecord, error) {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var usable bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM assistant_provider_connections
				WHERE id=$1 AND owner_user_id=$2
					AND status IN ('compatible', 'limited')
			)
		`, connectionID, ownerID).Scan(&usable); err != nil {
			return err
		}
		if !usable {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_provider_connections
			SET is_default=false, updated_at=now()
			WHERE owner_user_id=$1 AND is_default
		`, ownerID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE assistant_provider_connections
			SET is_default=true, updated_at=now()
			WHERE id=$1 AND owner_user_id=$2
		`, connectionID, ownerID)
		return err
	})
	if err != nil {
		return ProviderConnectionRecord{}, err
	}
	return s.GetProviderConnection(ctx, ownerID, connectionID)
}

func scanProviderConnection(row rowScanner) (ProviderConnectionRecord, error) {
	var connection ProviderConnectionRecord
	var capabilities []byte
	err := row.Scan(
		&connection.ID,
		&connection.OwnerUserID,
		&connection.Name,
		&connection.BaseURL,
		&connection.Dialect,
		&connection.Model,
		&connection.EncryptedAPIKey,
		&connection.APIKeyNonce,
		&connection.APIKeyFingerprint,
		&connection.APIKeyLast4,
		&connection.EncryptedHeaders,
		&connection.HeadersNonce,
		&connection.RequestTimeoutMS,
		&connection.ContextWindow,
		&connection.AllowPrivateEndpoint,
		&capabilities,
		&connection.Status,
		&connection.StatusDetail,
		&connection.IsDefault,
		&connection.CheckedAt,
		&connection.CreatedAt,
		&connection.UpdatedAt,
	)
	if err != nil {
		return ProviderConnectionRecord{}, err
	}
	if err := json.Unmarshal(capabilities, &connection.Capabilities); err != nil {
		return ProviderConnectionRecord{}, fmt.Errorf("decode provider capabilities: %w", err)
	}
	return connection, nil
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func (s *Store) GetAssistantGuide(
	ctx context.Context,
	ownerID string,
	projectID string,
	includeHistory bool,
) (assistant.Guide, error) {
	var guide assistant.Guide
	err := s.Pool.QueryRow(ctx, `
		SELECT g.project_id, g.current_revision, r.content, r.source, g.created_at, g.updated_at
		FROM assistant_guides g
		JOIN projects p ON p.id=g.project_id AND p.owner_user_id=$1
		JOIN assistant_guide_revisions r
			ON r.project_id=g.project_id AND r.revision=g.current_revision
		WHERE g.project_id=$2 AND g.active
	`, ownerID, projectID).Scan(
		&guide.ProjectID,
		&guide.Revision,
		&guide.Content,
		&guide.Source,
		&guide.CreatedAt,
		&guide.UpdatedAt,
	)
	if err != nil {
		return assistant.Guide{}, err
	}
	if includeHistory {
		guide.History, err = s.ListAssistantGuideRevisions(ctx, ownerID, projectID)
		if err != nil {
			return assistant.Guide{}, err
		}
	}
	return guide, nil
}

func (s *Store) ListAssistantGuideRevisions(
	ctx context.Context,
	ownerID string,
	projectID string,
) ([]assistant.GuideRevision, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.revision, r.content, r.source, r.restored_from_revision,
			r.assistant_run_id, r.created_at
		FROM assistant_guide_revisions r
		JOIN projects p ON p.id=r.project_id AND p.owner_user_id=$1
		WHERE r.project_id=$2
		ORDER BY r.revision DESC
	`, ownerID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []assistant.GuideRevision
	for rows.Next() {
		var revision assistant.GuideRevision
		if err := rows.Scan(
			&revision.ID,
			&revision.Revision,
			&revision.Content,
			&revision.Source,
			&revision.RestoredFromRevision,
			&revision.AssistantRunID,
			&revision.CreatedAt,
		); err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) SaveAssistantGuide(
	ctx context.Context,
	ownerID string,
	projectID string,
	content string,
	source string,
	restoredFromRevision *int,
	assistantRunID *string,
	baseRevision *int,
) (assistant.Guide, error) {
	if err := assistant.ValidateGuide(content); err != nil {
		return assistant.Guide{}, err
	}
	if source != "manual" && source != "assistant" && source != "restore" {
		return assistant.Guide{}, errors.New("invalid assistant guide source")
	}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM projects
				WHERE id=$1 AND owner_user_id=$2 AND status <> 'trash'
			)
		`, projectID, ownerID).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO assistant_guides(project_id, current_revision, updated_by_user_id)
			VALUES($1, 0, $2)
			ON CONFLICT(project_id) DO NOTHING
		`, projectID, ownerID); err != nil {
			return err
		}
		var revision int
		command := `
			UPDATE assistant_guides
			SET current_revision=current_revision+1, updated_by_user_id=$2,
				active=true, updated_at=now()
			WHERE project_id=$1
		`
		arguments := []any{projectID, ownerID}
		if baseRevision != nil {
			command += " AND current_revision=$3"
			arguments = append(arguments, *baseRevision)
		}
		command += " RETURNING current_revision"
		if err := tx.QueryRow(ctx, command, arguments...).Scan(&revision); err != nil {
			if errors.Is(err, pgx.ErrNoRows) && baseRevision != nil {
				return assistant.ErrStaleChangeSet
			}
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO assistant_guide_revisions(
				id, project_id, revision, content, source, restored_from_revision,
				assistant_run_id, created_by_user_id
			)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		`, uuid.NewString(), projectID, revision, content, source, restoredFromRevision,
			assistantRunID, ownerID)
		return err
	})
	if err != nil {
		return assistant.Guide{}, err
	}
	return s.GetAssistantGuide(ctx, ownerID, projectID, true)
}

func (s *Store) RestoreAssistantGuide(
	ctx context.Context,
	ownerID string,
	projectID string,
	revision int,
) (assistant.Guide, error) {
	var content string
	err := s.Pool.QueryRow(ctx, `
		SELECT r.content
		FROM assistant_guide_revisions r
		JOIN projects p ON p.id=r.project_id AND p.owner_user_id=$1
		WHERE r.project_id=$2 AND r.revision=$3
	`, ownerID, projectID, revision).Scan(&content)
	if err != nil {
		return assistant.Guide{}, err
	}
	current, err := s.GetAssistantGuide(ctx, ownerID, projectID, false)
	if err != nil {
		return assistant.Guide{}, err
	}
	return s.SaveAssistantGuide(
		ctx,
		ownerID,
		projectID,
		content,
		"restore",
		&revision,
		nil,
		&current.Revision,
	)
}

func (s *Store) ListProjectMemory(
	ctx context.Context,
	ownerID string,
	projectID string,
	includeInactive bool,
) ([]assistant.MemoryRecord, error) {
	query := `
		SELECT
			m.id, m.project_id, m.kind, m.statement, m.scope, m.scope_node_id,
			m.source_references, m.tags, m.active, m.always_relevant, m.revision,
			m.source, m.operation_id, m.assistant_run_id, m.created_at, m.updated_at
		FROM project_memory_records m
		JOIN projects p ON p.id=m.project_id AND p.owner_user_id=$1
		WHERE m.project_id=$2
	`
	if !includeInactive {
		query += " AND m.active"
	}
	query += " ORDER BY m.always_relevant DESC, m.updated_at DESC, m.id"
	rows, err := s.Pool.Query(ctx, query, ownerID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]assistant.MemoryRecord, 0)
	for rows.Next() {
		record, err := scanMemoryRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) GetProjectMemory(
	ctx context.Context,
	ownerID string,
	memoryID string,
) (assistant.MemoryRecord, error) {
	return scanMemoryRecord(s.Pool.QueryRow(ctx, `
		SELECT
			m.id, m.project_id, m.kind, m.statement, m.scope, m.scope_node_id,
			m.source_references, m.tags, m.active, m.always_relevant, m.revision,
			m.source, m.operation_id, m.assistant_run_id, m.created_at, m.updated_at
		FROM project_memory_records m
		JOIN projects p ON p.id=m.project_id AND p.owner_user_id=$1
		WHERE m.id=$2
	`, ownerID, memoryID))
}

func (s *Store) ListProjectMemoryRevisions(
	ctx context.Context,
	ownerID string,
	projectID string,
	memoryID string,
) ([]assistant.MemoryRevision, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.memory_id, r.revision, r.snapshot, r.source,
			r.operation_id, r.assistant_run_id, r.created_at
		FROM project_memory_revisions r
		JOIN project_memory_records m ON m.id=r.memory_id AND m.project_id=$2
		JOIN projects p ON p.id=m.project_id AND p.owner_user_id=$1
		WHERE r.memory_id=$3
		ORDER BY r.revision DESC
	`, ownerID, projectID, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := make([]assistant.MemoryRevision, 0)
	for rows.Next() {
		var revision assistant.MemoryRevision
		var snapshot []byte
		if err := rows.Scan(
			&revision.ID,
			&revision.MemoryID,
			&revision.Revision,
			&snapshot,
			&revision.Source,
			&revision.OperationID,
			&revision.AssistantRunID,
			&revision.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(snapshot, &revision.Snapshot); err != nil {
			return nil, err
		}
		normalizeMemoryRecordCollections(&revision.Snapshot)
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) RestoreProjectMemory(
	ctx context.Context,
	ownerID string,
	projectID string,
	memoryID string,
	revision int,
) (assistant.MemoryRecord, error) {
	var snapshot []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT r.snapshot
		FROM project_memory_revisions r
		JOIN project_memory_records m ON m.id=r.memory_id AND m.project_id=$2
		JOIN projects p ON p.id=m.project_id AND p.owner_user_id=$1
		WHERE r.memory_id=$3 AND r.revision=$4
	`, ownerID, projectID, memoryID, revision).Scan(&snapshot)
	if err != nil {
		return assistant.MemoryRecord{}, err
	}
	target := assistant.MemoryRecord{}
	if err := json.Unmarshal(snapshot, &target); err != nil {
		return assistant.MemoryRecord{}, err
	}
	normalizeMemoryRecordCollections(&target)
	current, err := s.GetProjectMemory(ctx, ownerID, memoryID)
	if err != nil || current.ProjectID != projectID {
		if err == nil {
			err = pgx.ErrNoRows
		}
		return assistant.MemoryRecord{}, err
	}
	target.ID = memoryID
	target.ProjectID = projectID
	target.Source = "manual"
	target.OperationID = nil
	target.AssistantRunID = nil
	baseRevision := current.Revision
	return s.SaveProjectMemory(ctx, ownerID, target, &baseRevision)
}

func (s *Store) SaveProjectMemory(
	ctx context.Context,
	ownerID string,
	record assistant.MemoryRecord,
	baseRevision *int,
) (assistant.MemoryRecord, error) {
	normalizeMemoryRecordCollections(&record)
	if err := record.Validate(); err != nil {
		return assistant.MemoryRecord{}, err
	}
	if record.Source != "manual" && record.Source != "assistant" {
		return assistant.MemoryRecord{}, errors.New("invalid memory source")
	}
	sourceReferences, err := json.Marshal(record.SourceReferences)
	if err != nil {
		return assistant.MemoryRecord{}, err
	}
	tags, err := json.Marshal(record.Tags)
	if err != nil {
		return assistant.MemoryRecord{}, err
	}
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM projects WHERE id=$1 AND owner_user_id=$2 AND status <> 'trash'
			)
		`, record.ProjectID, ownerID).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return pgx.ErrNoRows
		}
		if record.ScopeNodeID != nil {
			var nodeOwned bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM document_nodes
					WHERE id=$1 AND project_id=$2 AND trashed_at IS NULL
				)
			`, *record.ScopeNodeID, record.ProjectID).Scan(&nodeOwned); err != nil {
				return err
			}
			if !nodeOwned {
				return pgx.ErrNoRows
			}
		}
		var revision int
		if baseRevision == nil {
			revision = 1
			_, err := tx.Exec(ctx, `
				INSERT INTO project_memory_records(
					id, project_id, kind, statement, scope, scope_node_id,
					source_references, tags, active, always_relevant, revision,
					source, operation_id, assistant_run_id, created_by_user_id
				)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			`, record.ID, record.ProjectID, record.Kind, strings.TrimSpace(record.Statement),
				record.Scope, record.ScopeNodeID, sourceReferences, tags, record.Active,
				record.AlwaysRelevant, revision, record.Source, record.OperationID,
				record.AssistantRunID, ownerID)
			if err != nil {
				return err
			}
		} else {
			revision = *baseRevision + 1
			command, err := tx.Exec(ctx, `
				UPDATE project_memory_records
				SET kind=$3, statement=$4, scope=$5, scope_node_id=$6,
					source_references=$7, tags=$8, active=$9, always_relevant=$10,
					revision=$11, source=$12, operation_id=$13, assistant_run_id=$14,
					updated_at=now()
				WHERE id=$1 AND project_id=$2 AND revision=$15
			`, record.ID, record.ProjectID, record.Kind, strings.TrimSpace(record.Statement),
				record.Scope, record.ScopeNodeID, sourceReferences, tags, record.Active,
				record.AlwaysRelevant, revision, record.Source, record.OperationID,
				record.AssistantRunID, *baseRevision)
			if err != nil {
				return err
			}
			if command.RowsAffected() == 0 {
				return assistant.ErrStaleChangeSet
			}
		}
		record.Revision = revision
		snapshot, err := json.Marshal(record)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO project_memory_revisions(
				id, memory_id, revision, snapshot, source, operation_id, assistant_run_id
			)
			VALUES($1,$2,$3,$4,$5,$6,$7)
		`, uuid.NewString(), record.ID, revision, snapshot, record.Source,
			record.OperationID, record.AssistantRunID)
		return err
	})
	if err != nil {
		return assistant.MemoryRecord{}, err
	}
	return s.GetProjectMemory(ctx, ownerID, record.ID)
}

func scanMemoryRecord(row rowScanner) (assistant.MemoryRecord, error) {
	var record assistant.MemoryRecord
	var references []byte
	var tags []byte
	err := row.Scan(
		&record.ID,
		&record.ProjectID,
		&record.Kind,
		&record.Statement,
		&record.Scope,
		&record.ScopeNodeID,
		&references,
		&tags,
		&record.Active,
		&record.AlwaysRelevant,
		&record.Revision,
		&record.Source,
		&record.OperationID,
		&record.AssistantRunID,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return assistant.MemoryRecord{}, err
	}
	if err := json.Unmarshal(references, &record.SourceReferences); err != nil {
		return assistant.MemoryRecord{}, err
	}
	if err := json.Unmarshal(tags, &record.Tags); err != nil {
		return assistant.MemoryRecord{}, err
	}
	normalizeMemoryRecordCollections(&record)
	return record, nil
}

func normalizeMemoryRecordCollections(record *assistant.MemoryRecord) {
	if record.SourceReferences == nil {
		record.SourceReferences = make([]assistant.SourceReference, 0)
	}
	if record.Tags == nil {
		record.Tags = make([]string, 0)
	}
}

func (s *Store) CreateAssistantRun(ctx context.Context, run assistant.Run) (assistant.Run, error) {
	return insertAssistantRun(ctx, s.Pool, run)
}

type assistantQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertAssistantRun(
	ctx context.Context,
	query assistantQueryRower,
	run assistant.Run,
) (assistant.Run, error) {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status == "" {
		run.Status = assistant.RunActive
	}
	if run.Origin == "" {
		run.Origin = assistant.RunOriginInternal
	}
	run.Title = normalizedAssistantRunTitle(run.Title)
	if err := run.Validate(); err != nil {
		return assistant.Run{}, err
	}
	capabilities, err := json.Marshal(run.Provider.Capabilities)
	if err != nil {
		return assistant.Run{}, err
	}
	readContext, err := json.Marshal(run.ReadContext)
	if err != nil {
		return assistant.Run{}, err
	}
	var providerConnectionID *string
	if run.Provider.ConnectionID != nil {
		providerConnectionID = run.Provider.ConnectionID
	}
	var mcpGrantID *string
	var mcpClientName string
	if run.Agent != nil {
		mcpGrantID = &run.Agent.GrantID
		mcpClientName = strings.TrimSpace(run.Agent.ClientName)
	}
	err = query.QueryRow(ctx, `
		INSERT INTO assistant_runs(
			id, owner_user_id, project_id, title, target_kind, target_node_id, input_mode,
			status, silence_checkpoint_ms, provider_kind, provider_connection_id,
			provider_name, model, dialect, capability_snapshot, guide_revision, read_context,
			origin, mcp_grant_id, mcp_client_name
		)
		SELECT
			$1, p.owner_user_id, p.id, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18, $19, $20
		FROM projects p
		WHERE p.id=$2 AND p.owner_user_id=$3 AND p.status <> 'trash'
			AND (
				$5 = 'project'
				OR EXISTS(
					SELECT 1 FROM document_nodes n
					WHERE n.id=$6 AND n.project_id=p.id AND n.trashed_at IS NULL
				)
			)
		RETURNING started_at, updated_at, last_viewed_at
	`,
		run.ID,
		run.ProjectID,
		run.OwnerUserID,
		run.Title,
		run.TargetKind,
		run.TargetNodeID,
		run.InputMode,
		run.Status,
		run.SilenceCheckpointMS,
		run.Provider.Kind,
		providerConnectionID,
		run.Provider.Name,
		run.Provider.Model,
		run.Provider.Dialect,
		capabilities,
		run.GuideRevision,
		readContext,
		run.Origin,
		mcpGrantID,
		mcpClientName,
	).Scan(&run.StartedAt, &run.UpdatedAt, &run.LastViewedAt)
	if err != nil {
		return assistant.Run{}, err
	}
	return run, nil
}

func normalizedAssistantRunTitle(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "Новый чат"
	}
	runes := []rune(value)
	if len(runes) > 80 {
		return strings.TrimSpace(string(runes[:79])) + "…"
	}
	return value
}

func (s *Store) CreateAssistantRunWithFirstTurn(
	ctx context.Context,
	run assistant.Run,
	mode assistant.InputMode,
	instruction string,
	selection *assistant.SelectionAttachment,
) (assistant.Run, assistant.Turn, error) {
	var createdRun assistant.Run
	var createdTurn assistant.Turn
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		createdRun, err = insertAssistantRun(ctx, tx, run)
		if err != nil {
			return err
		}
		createdTurn, err = insertAssistantTurn(
			ctx, tx, createdRun.OwnerUserID, createdRun.ID, mode, instruction,
			assistant.TurnQueued, selection,
		)
		return err
	})
	if err != nil {
		return assistant.Run{}, assistant.Turn{}, err
	}
	createdRun.LatestTurn = &createdTurn
	return createdRun, createdTurn, nil
}

func (s *Store) GetAssistantRun(
	ctx context.Context,
	ownerID string,
	runID string,
) (assistant.Run, error) {
	var run assistant.Run
	var capabilities []byte
	var readContext []byte
	var providerConnectionID *string
	var mcpGrantID *string
	var mcpClientName string
	err := s.Pool.QueryRow(ctx, `
		SELECT
			r.id, r.owner_user_id, r.project_id, r.title, r.target_kind, r.target_node_id,
			r.input_mode, r.status, r.silence_checkpoint_ms, r.provider_kind,
			r.provider_connection_id, r.provider_name, r.model, r.dialect,
			r.capability_snapshot, r.guide_revision, r.read_context, r.error_code,
			r.started_at, r.updated_at, r.last_viewed_at, r.origin,
			r.mcp_grant_id, r.mcp_client_name,
			(
				r.updated_at > r.last_viewed_at
				AND (r.status IN ('awaiting_review', 'completed', 'failed')
					OR COALESCE(r.error_code='stale_change_set', false))
			),
			r.ended_at
		FROM assistant_runs r
		WHERE r.id=$1 AND r.owner_user_id=$2
	`, runID, ownerID).Scan(
		&run.ID,
		&run.OwnerUserID,
		&run.ProjectID,
		&run.Title,
		&run.TargetKind,
		&run.TargetNodeID,
		&run.InputMode,
		&run.Status,
		&run.SilenceCheckpointMS,
		&run.Provider.Kind,
		&providerConnectionID,
		&run.Provider.Name,
		&run.Provider.Model,
		&run.Provider.Dialect,
		&capabilities,
		&run.GuideRevision,
		&readContext,
		&run.ErrorCode,
		&run.StartedAt,
		&run.UpdatedAt,
		&run.LastViewedAt,
		&run.Origin,
		&mcpGrantID,
		&mcpClientName,
		&run.Unread,
		&run.EndedAt,
	)
	if err != nil {
		return assistant.Run{}, err
	}
	run.Provider.ConnectionID = providerConnectionID
	if mcpGrantID != nil {
		run.Agent = &assistant.AgentRef{GrantID: *mcpGrantID, ClientName: mcpClientName}
	}
	if err := json.Unmarshal(capabilities, &run.Provider.Capabilities); err != nil {
		return assistant.Run{}, err
	}
	if err := json.Unmarshal(readContext, &run.ReadContext); err != nil {
		return assistant.Run{}, err
	}
	run.Capabilities = assistant.AssistantCapability{
		VoiceAvailable:       true,
		ChatAvailable:        true,
		ProviderAvailable:    run.Provider.Capabilities.CanRunAssistant(),
		GuideAvailable:       true,
		MemoryAvailable:      true,
		ChangeReviewRequired: true,
	}
	turn, err := s.getLatestAssistantTurn(ctx, run.ID)
	if err == nil {
		run.LatestTurn = &turn
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return assistant.Run{}, err
	}
	changeSet, err := s.getPendingAssistantChangeSet(ctx, run.ID)
	if err == nil {
		run.PendingChangeSet = &changeSet
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return assistant.Run{}, err
	}
	latestChangeSet, err := s.GetLatestAssistantChangeSet(ctx, ownerID, run.ID)
	if err == nil {
		run.LatestChangeSet = &latestChangeSet
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return assistant.Run{}, err
	}
	return run, nil
}

func (s *Store) MarkAssistantRunViewed(
	ctx context.Context,
	ownerID string,
	runID string,
) (assistant.Run, error) {
	command, err := s.Pool.Exec(ctx, `
		UPDATE assistant_runs
		SET last_viewed_at=now()
		WHERE id=$1 AND owner_user_id=$2
	`, runID, ownerID)
	if err != nil {
		return assistant.Run{}, err
	}
	if command.RowsAffected() == 0 {
		return assistant.Run{}, pgx.ErrNoRows
	}
	return s.GetAssistantRun(ctx, ownerID, runID)
}

func (s *Store) ListAssistantRuns(
	ctx context.Context,
	ownerID string,
	projectID string,
	limit int,
) ([]assistant.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id
		FROM assistant_runs
		WHERE owner_user_id=$1 AND project_id=$2
		ORDER BY updated_at DESC
		LIMIT $3
	`, ownerID, projectID, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	// Release the query connection before hydrating each run. Keeping this
	// cursor open while GetAssistantRun performs nested queries can exhaust a
	// small connection pool when more than one list refresh happens at once.
	rows.Close()
	runs := make([]assistant.Run, 0, len(ids))
	for _, id := range ids {
		run, err := s.GetAssistantRun(ctx, ownerID, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *Store) UpdateAssistantRunStatus(
	ctx context.Context,
	ownerID string,
	runID string,
	status assistant.RunStatus,
	errorCode *string,
) error {
	terminal := status == assistant.RunCompleted || status == assistant.RunCancelled ||
		status == assistant.RunFailed
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `
			UPDATE assistant_runs
			SET status=$3, error_code=$4, updated_at=now(),
				ended_at=CASE WHEN $5 THEN now() ELSE NULL END
			WHERE id=$1 AND owner_user_id=$2
		`, runID, ownerID, status, errorCode, terminal)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		if status != assistant.RunCancelled && status != assistant.RunFailed {
			return nil
		}
		turnState := assistant.TurnFailed
		fallbackCode := "failed"
		if status == assistant.RunCancelled {
			turnState = assistant.TurnCancelled
			fallbackCode = "cancelled"
		}
		code := errorCode
		if code == nil {
			code = &fallbackCode
		}
		_, err = tx.Exec(ctx, `
			UPDATE assistant_turns
			SET state=$3, error_code=$4, completed_at=COALESCE(completed_at, now())
			WHERE run_id=$1 AND state='working'
				AND EXISTS (
					SELECT 1 FROM assistant_runs r
					WHERE r.id=$1 AND r.owner_user_id=$2
				)
		`, runID, ownerID, turnState, code)
		return err
	})
}

// RecoverInterruptedAssistantRuns closes executions whose in-memory orchestrator
// disappeared with the previous process. Canonical project state is unaffected;
// the author can retry the preserved instruction in the same run.
func (s *Store) RecoverInterruptedAssistantRuns(ctx context.Context) (int, error) {
	recovered := 0
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT r.id, t.id, r.status, r.error_code
			FROM assistant_runs r
			JOIN assistant_turns t ON t.run_id=r.id
			WHERE t.state='working'
			FOR UPDATE OF r, t
		`)
		if err != nil {
			return err
		}
		type interruptedExecution struct {
			runID     string
			turnID    string
			runStatus assistant.RunStatus
			runError  *string
		}
		var executions []interruptedExecution
		for rows.Next() {
			var execution interruptedExecution
			if err := rows.Scan(
				&execution.runID, &execution.turnID,
				&execution.runStatus, &execution.runError,
			); err != nil {
				rows.Close()
				return err
			}
			executions = append(executions, execution)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		for _, execution := range executions {
			errorCode := "execution_interrupted"
			turnState := assistant.TurnFailed
			if execution.runStatus == assistant.RunCancelled {
				errorCode = "cancelled"
				turnState = assistant.TurnCancelled
			} else if execution.runStatus != assistant.RunWorking &&
				execution.runError != nil && strings.TrimSpace(*execution.runError) != "" {
				errorCode = *execution.runError
			}
			if _, err := tx.Exec(ctx, `
				UPDATE assistant_turns
				SET state=$2, error_code=$3, completed_at=now()
				WHERE id=$1 AND state='working'
			`, execution.turnID, turnState, errorCode); err != nil {
				return err
			}
			if execution.runStatus == assistant.RunWorking {
				errorCode = "execution_interrupted"
				if _, err := tx.Exec(ctx, `
					UPDATE assistant_runs
					SET status='active', error_code=$2, updated_at=now(), ended_at=NULL
					WHERE id=$1 AND status='working'
				`, execution.runID, errorCode); err != nil {
					return err
				}
			}
			eventData, _ := json.Marshal(map[string]any{
				"errorCode": errorCode, "recoverable": execution.runStatus == assistant.RunWorking,
				"recovered": true,
			})
			var sequence int64
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(max(sequence), 0) + 1
				FROM assistant_events
				WHERE run_id=$1
			`, execution.runID).Scan(&sequence); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO assistant_events(id, run_id, turn_id, sequence, event_type, data)
				VALUES($1,$2,$3,$4,$5,$6)
			`, uuid.NewString(), execution.runID, execution.turnID, sequence,
				assistant.EventRunError, eventData); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}

func (s *Store) CreateAssistantTurn(
	ctx context.Context,
	ownerID string,
	runID string,
	mode assistant.InputMode,
	instruction string,
	state assistant.TurnState,
) (assistant.Turn, error) {
	return s.CreateAssistantTurnWithSelection(
		ctx, ownerID, runID, mode, instruction, state, nil,
	)
}

func (s *Store) CreateAssistantTurnWithSelection(
	ctx context.Context,
	ownerID string,
	runID string,
	mode assistant.InputMode,
	instruction string,
	state assistant.TurnState,
	selection *assistant.SelectionAttachment,
) (assistant.Turn, error) {
	return insertAssistantTurn(ctx, s.Pool, ownerID, runID, mode, instruction, state, selection)
}

func insertAssistantTurn(
	ctx context.Context,
	query assistantQueryRower,
	ownerID string,
	runID string,
	mode assistant.InputMode,
	instruction string,
	state assistant.TurnState,
	selection *assistant.SelectionAttachment,
) (assistant.Turn, error) {
	turn := assistant.Turn{
		ID: uuid.NewString(), RunID: runID, InputMode: mode,
		Instruction: strings.TrimSpace(instruction), Transcript: strings.TrimSpace(instruction),
		State: state, SelectionAttachment: selection,
	}
	if err := turn.ValidateInstruction(); err != nil {
		return assistant.Turn{}, err
	}
	if selection != nil {
		if err := selection.Validate(); err != nil {
			return assistant.Turn{}, err
		}
	}
	selectionJSON, err := json.Marshal(selection)
	if err != nil {
		return assistant.Turn{}, err
	}
	err = query.QueryRow(ctx, `
		INSERT INTO assistant_turns(
			id, run_id, sequence, input_mode, state, transcript, instruction,
			guide_revision, selection_attachment
		)
		SELECT
			$1, r.id,
			COALESCE((SELECT max(t.sequence)+1 FROM assistant_turns t WHERE t.run_id=r.id), 1),
			$4, $5, $6, $7,
			(
				SELECT g.current_revision
				FROM assistant_guides g
				WHERE g.project_id=r.project_id AND g.active
			), $8
		FROM assistant_runs r
		WHERE r.id=$2 AND r.owner_user_id=$3
			AND r.status IN ('active', 'working', 'awaiting_review')
		RETURNING sequence, guide_revision, created_at
	`, turn.ID, runID, ownerID, mode, state, turn.Transcript, turn.Instruction,
		selectionJSON).Scan(
		&turn.Sequence,
		&turn.GuideRevision,
		&turn.CreatedAt,
	)
	if err != nil {
		return assistant.Turn{}, err
	}
	return turn, nil
}

// PinAssistantTurnGuideRevision captures the active guide at the moment the
// author confirms a turn. This keeps retries reproducible while allowing a
// guide edited during a long run to affect the next confirmed turn.
func (s *Store) PinAssistantTurnGuideRevision(
	ctx context.Context,
	ownerID string,
	runID string,
	turnID string,
) (*int, error) {
	var revision *int
	err := s.Pool.QueryRow(ctx, `
		UPDATE assistant_turns t
		SET guide_revision=(
			SELECT g.current_revision
			FROM assistant_guides g
			WHERE g.project_id=r.project_id AND g.active
		)
		FROM assistant_runs r
		WHERE t.id=$1 AND t.run_id=$2 AND r.id=t.run_id AND r.owner_user_id=$3
			AND t.state IN ('queued', 'awaiting_confirmation')
		RETURNING t.guide_revision
	`, turnID, runID, ownerID).Scan(&revision)
	return revision, err
}

func (s *Store) UpdateAssistantTurn(
	ctx context.Context,
	turn assistant.Turn,
) error {
	selectionJSON, err := json.Marshal(turn.SelectionAttachment)
	if err != nil {
		return err
	}
	command, err := s.Pool.Exec(ctx, `
		UPDATE assistant_turns
		SET state=$2, transcript=$3, instruction=$4, assistant_text=$5,
			provider_response_id=$6, error_code=$7,
			confirmed_at=$8, started_at=$9, completed_at=$10,
			selection_attachment=$11
		WHERE id=$1
	`, turn.ID, turn.State, turn.Transcript, turn.Instruction, turn.AssistantText,
		turn.ProviderResponseID, turn.ErrorCode, turn.ConfirmedAt, turn.StartedAt, turn.CompletedAt,
		selectionJSON)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAssistantDraftTurn(
	ctx context.Context,
	ownerID string,
	runID string,
	turnID string,
) error {
	command, err := s.Pool.Exec(ctx, `
		DELETE FROM assistant_turns t
		USING assistant_runs r
		WHERE t.id=$1 AND t.run_id=$2
			AND t.state IN ('collecting', 'awaiting_confirmation')
			AND r.id=t.run_id AND r.owner_user_id=$3
	`, turnID, runID, ownerID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAssistantRunHistory(
	ctx context.Context,
	ownerID string,
	runID string,
) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var status assistant.RunStatus
		if err := tx.QueryRow(ctx, `
			SELECT status
			FROM assistant_runs
			WHERE id=$1 AND owner_user_id=$2
			FOR UPDATE
		`, runID, ownerID).Scan(&status); err != nil {
			return err
		}
		if status == assistant.RunWorking {
			return assistant.ErrRunBusy
		}
		// Applied project data is intentionally not reverted. Nullable provenance
		// links are detached before the private conversation/tool history cascades.
		for _, command := range []string{
			`UPDATE assistant_guide_revisions SET assistant_run_id=NULL WHERE assistant_run_id=$1`,
			`UPDATE project_memory_records SET assistant_run_id=NULL WHERE assistant_run_id=$1`,
			`UPDATE project_memory_revisions SET assistant_run_id=NULL WHERE assistant_run_id=$1`,
		} {
			if _, err := tx.Exec(ctx, command, runID); err != nil {
				return err
			}
		}
		command, err := tx.Exec(ctx, `
			DELETE FROM assistant_runs
			WHERE id=$1 AND owner_user_id=$2
		`, runID, ownerID)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (s *Store) ListAssistantTurns(
	ctx context.Context,
	ownerID string,
	runID string,
) ([]assistant.Turn, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			t.id, t.run_id, t.sequence, t.input_mode, t.state, t.transcript,
			t.instruction, t.assistant_text, t.guide_revision, t.provider_response_id,
			t.error_code, t.created_at, t.confirmed_at, t.started_at, t.completed_at,
			COALESCE(t.selection_attachment, 'null'::jsonb)
		FROM assistant_turns t
		JOIN assistant_runs r ON r.id=t.run_id AND r.owner_user_id=$1
		WHERE t.run_id=$2
		ORDER BY t.sequence
	`, ownerID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := make([]assistant.Turn, 0)
	for rows.Next() {
		var turn assistant.Turn
		var selectionJSON []byte
		if err := rows.Scan(
			&turn.ID,
			&turn.RunID,
			&turn.Sequence,
			&turn.InputMode,
			&turn.State,
			&turn.Transcript,
			&turn.Instruction,
			&turn.AssistantText,
			&turn.GuideRevision,
			&turn.ProviderResponseID,
			&turn.ErrorCode,
			&turn.CreatedAt,
			&turn.ConfirmedAt,
			&turn.StartedAt,
			&turn.CompletedAt,
			&selectionJSON,
		); err != nil {
			return nil, err
		}
		if string(selectionJSON) != "null" {
			var selection assistant.SelectionAttachment
			if err := json.Unmarshal(selectionJSON, &selection); err != nil {
				return nil, err
			}
			turn.SelectionAttachment = &selection
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func (s *Store) AppendAssistantVoiceSegment(
	ctx context.Context,
	ownerID string,
	runID string,
	turnID string,
	providerTurnID string,
	text string,
	startMS int64,
	endMS int64,
	confidence *float64,
) (string, int, error) {
	text = strings.TrimSpace(text)
	if text == "" || startMS < 0 || endMS <= startMS {
		return "", 0, errors.New("invalid assistant voice segment")
	}
	id := uuid.NewString()
	var sequence int
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `
			SELECT true
			FROM assistant_turns t
			JOIN assistant_runs r ON r.id=t.run_id AND r.owner_user_id=$1
			WHERE t.id=$2 AND t.run_id=$3
			FOR UPDATE OF t
		`, ownerID, turnID, runID).Scan(&owned); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(sequence)+1, 1)
			FROM assistant_voice_segments
			WHERE turn_id=$1
		`, turnID).Scan(&sequence); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO assistant_voice_segments(
				id, run_id, turn_id, sequence, provider_turn_id, text,
				start_ms, end_ms, confidence
			)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, id, runID, turnID, sequence, providerTurnID, text, startMS, endMS, confidence)
		return err
	})
	return id, sequence, err
}

func (s *Store) getLatestAssistantTurn(
	ctx context.Context,
	runID string,
) (assistant.Turn, error) {
	var turn assistant.Turn
	var selectionJSON []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT
			id, run_id, sequence, input_mode, state, transcript, instruction,
			assistant_text, guide_revision, provider_response_id, error_code,
			created_at, confirmed_at, started_at, completed_at,
			COALESCE(selection_attachment, 'null'::jsonb)
		FROM assistant_turns
		WHERE run_id=$1
		ORDER BY sequence DESC
		LIMIT 1
	`, runID).Scan(
		&turn.ID,
		&turn.RunID,
		&turn.Sequence,
		&turn.InputMode,
		&turn.State,
		&turn.Transcript,
		&turn.Instruction,
		&turn.AssistantText,
		&turn.GuideRevision,
		&turn.ProviderResponseID,
		&turn.ErrorCode,
		&turn.CreatedAt,
		&turn.ConfirmedAt,
		&turn.StartedAt,
		&turn.CompletedAt,
		&selectionJSON,
	)
	if err == nil && string(selectionJSON) != "null" {
		var selection assistant.SelectionAttachment
		if decodeErr := json.Unmarshal(selectionJSON, &selection); decodeErr != nil {
			return assistant.Turn{}, decodeErr
		}
		turn.SelectionAttachment = &selection
	}
	return turn, err
}

func (s *Store) AppendAssistantEvent(
	ctx context.Context,
	event assistant.Event,
) (assistant.Event, error) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if len(event.Data) == 0 {
		event.Data = json.RawMessage(`{}`)
	}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			SELECT id FROM assistant_runs WHERE id=$1 FOR UPDATE
		`, event.RunID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(sequence)+1, 1)
			FROM assistant_events
			WHERE run_id=$1
		`, event.RunID).Scan(&event.Sequence); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO assistant_events(id, run_id, turn_id, sequence, event_type, data)
			VALUES($1,$2,$3,$4,$5,$6)
			RETURNING created_at
		`, event.ID, event.RunID, event.TurnID, event.Sequence, event.Type, event.Data).
			Scan(&event.CreatedAt)
	})
	return event, err
}

func (s *Store) ListAssistantEvents(
	ctx context.Context,
	ownerID string,
	runID string,
	afterSequence int64,
	limit int,
) ([]assistant.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.run_id, e.turn_id, e.sequence, e.event_type, e.data, e.created_at
		FROM assistant_events e
		JOIN assistant_runs r ON r.id=e.run_id AND r.owner_user_id=$1
		WHERE e.run_id=$2 AND e.sequence>$3
		ORDER BY e.sequence
		LIMIT $4
	`, ownerID, runID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []assistant.Event
	for rows.Next() {
		var event assistant.Event
		if err := rows.Scan(
			&event.ID,
			&event.RunID,
			&event.TurnID,
			&event.Sequence,
			&event.Type,
			&event.Data,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) StartAssistantToolEvent(
	ctx context.Context,
	runID string,
	turnID string,
	callID string,
	toolName string,
	arguments json.RawMessage,
) error {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO assistant_tool_events(
			id, run_id, turn_id, call_id, tool_name, arguments
		)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT(run_id, call_id) DO NOTHING
	`, uuid.NewString(), runID, turnID, callID, toolName, arguments)
	return err
}

func (s *Store) CompleteAssistantToolEvent(
	ctx context.Context,
	runID string,
	callID string,
	result json.RawMessage,
	isError bool,
) error {
	if len(result) == 0 {
		result = json.RawMessage(`{}`)
	}
	command, err := s.Pool.Exec(ctx, `
		UPDATE assistant_tool_events
		SET result=$3, is_error=$4, completed_at=now()
		WHERE run_id=$1 AND call_id=$2
	`, runID, callID, result, isError)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetAssistantGuideRevision(
	ctx context.Context,
	ownerID string,
	projectID string,
	revision int,
) (assistant.GuideRevision, error) {
	var result assistant.GuideRevision
	err := s.Pool.QueryRow(ctx, `
		SELECT r.id, r.revision, r.content, r.source, r.restored_from_revision,
			r.assistant_run_id, r.created_at
		FROM assistant_guide_revisions r
		JOIN projects p ON p.id=r.project_id AND p.owner_user_id=$1
		WHERE r.project_id=$2 AND r.revision=$3
	`, ownerID, projectID, revision).Scan(
		&result.ID,
		&result.Revision,
		&result.Content,
		&result.Source,
		&result.RestoredFromRevision,
		&result.AssistantRunID,
		&result.CreatedAt,
	)
	return result, err
}

func (s *Store) ProjectFingerprint(
	ctx context.Context,
	ownerID string,
	projectID string,
) (string, error) {
	return projectFingerprint(ctx, s.Pool, ownerID, projectID)
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func projectFingerprint(
	ctx context.Context,
	querier queryRower,
	ownerID string,
	projectID string,
) (string, error) {
	var snapshot []byte
	err := querier.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'project', jsonb_build_object(
				'id', p.id, 'title', p.title, 'synopsis', p.synopsis,
				'status', p.status, 'updatedAt', p.updated_at,
				'structureRevision', p.structure_revision
			),
			'nodes', COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'id', n.id, 'parentId', n.parent_id, 'kind', n.kind, 'title', n.title,
					'status', n.status, 'position', n.position,
					'revision', n.current_revision, 'trashedAt', n.trashed_at
				) ORDER BY n.id)
				FROM document_nodes n WHERE n.project_id=p.id
			), '[]'::jsonb),
			'dictionary', COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'id', d.id, 'canonical', d.canonical, 'variants', d.variants,
					'kind', d.kind, 'language', d.language, 'active', d.active,
					'pronunciation', d.pronunciation, 'revision', d.revision
				) ORDER BY d.id)
				FROM dictionary_terms d WHERE d.project_id=p.id
			), '[]'::jsonb),
			'guideRevision', (
				SELECT g.current_revision FROM assistant_guides g WHERE g.project_id=p.id
			),
			'memory', COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'id', m.id, 'revision', m.revision, 'active', m.active,
					'alwaysRelevant', m.always_relevant
				) ORDER BY m.id)
				FROM project_memory_records m WHERE m.project_id=p.id
			), '[]'::jsonb)
		)::text
		FROM projects p
		WHERE p.id=$1 AND p.owner_user_id=$2
	`, projectID, ownerID).Scan(&snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(snapshot)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) getPendingAssistantChangeSet(
	ctx context.Context,
	runID string,
) (assistant.ChangeSet, error) {
	var changeSet assistant.ChangeSet
	err := s.Pool.QueryRow(ctx, `
		SELECT id
		FROM assistant_change_sets
		WHERE run_id=$1 AND state='staged'
		ORDER BY created_at DESC
		LIMIT 1
	`, runID).Scan(&changeSet.ID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSet.ID)
}

func (s *Store) GetAssistantChangeSet(
	ctx context.Context,
	runID string,
	changeSetID string,
) (assistant.ChangeSet, error) {
	changeSet := assistant.ChangeSet{
		Operations: make([]assistant.ChangeOperation, 0),
	}
	err := s.Pool.QueryRow(ctx, `
		SELECT
			id, run_id, turn_id, state, title, summary, risk_level, base_fingerprint,
			operation_id, applied_at, rejected_at, undone_at, created_at, updated_at
		FROM assistant_change_sets
		WHERE id=$1 AND run_id=$2
	`, changeSetID, runID).Scan(
		&changeSet.ID,
		&changeSet.RunID,
		&changeSet.TurnID,
		&changeSet.State,
		&changeSet.Title,
		&changeSet.Summary,
		&changeSet.RiskLevel,
		&changeSet.BaseFingerprint,
		&changeSet.OperationID,
		&changeSet.AppliedAt,
		&changeSet.RejectedAt,
		&changeSet.UndoneAt,
		&changeSet.CreatedAt,
		&changeSet.UpdatedAt,
	)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, position, group_key, selected, dependencies, tool_name,
			target_kind, target_id, base_revision, arguments, before_snapshot,
			proposed_snapshot, trace, risk_level, applied
		FROM assistant_change_operations
		WHERE change_set_id=$1
		ORDER BY position
	`, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var operation assistant.ChangeOperation
		var dependencies []byte
		if err := rows.Scan(
			&operation.ID,
			&operation.Position,
			&operation.GroupKey,
			&operation.Selected,
			&dependencies,
			&operation.ToolName,
			&operation.TargetKind,
			&operation.TargetID,
			&operation.BaseRevision,
			&operation.Arguments,
			&operation.BeforeSnapshot,
			&operation.ProposedSnapshot,
			&operation.Trace,
			&operation.RiskLevel,
			&operation.Applied,
		); err != nil {
			return assistant.ChangeSet{}, err
		}
		if err := json.Unmarshal(dependencies, &operation.Dependencies); err != nil {
			return assistant.ChangeSet{}, err
		}
		if operation.Dependencies == nil {
			// Keep the wire contract stable for operations staged before the
			// dependency graph existed. A JSON null used to crash the review UI.
			operation.Dependencies = make([]string, 0)
		}
		changeSet.Operations = append(changeSet.Operations, operation)
	}
	return changeSet, rows.Err()
}

func (s *Store) GetLatestAssistantChangeSet(
	ctx context.Context,
	ownerID string,
	runID string,
) (assistant.ChangeSet, error) {
	var changeSetID string
	if err := s.Pool.QueryRow(ctx, `
		SELECT c.id::text
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		WHERE c.run_id=$1 AND r.owner_user_id=$2
		ORDER BY c.created_at DESC
		LIMIT 1
	`, runID, ownerID).Scan(&changeSetID); err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func (s *Store) StageAssistantChangeSet(
	ctx context.Context,
	changeSet assistant.ChangeSet,
) (assistant.ChangeSet, error) {
	return s.stageAssistantChangeSet(ctx, changeSet, "")
}

// StageAssistantChangeSetFromDraft atomically publishes a durable remote draft
// as a normal Assistant change set and removes the draft. A crash can therefore
// leave either the draft or the change set, never an acknowledged gap between
// the two durable representations.
func (s *Store) StageAssistantChangeSetFromDraft(
	ctx context.Context,
	changeSet assistant.ChangeSet,
	turnID string,
) (assistant.ChangeSet, error) {
	if strings.TrimSpace(turnID) == "" || turnID != changeSet.TurnID {
		return assistant.ChangeSet{}, errors.New("assistant draft turn does not match change set")
	}
	return s.stageAssistantChangeSet(ctx, changeSet, turnID)
}

func (s *Store) stageAssistantChangeSet(
	ctx context.Context,
	changeSet assistant.ChangeSet,
	draftTurnID string,
) (assistant.ChangeSet, error) {
	if changeSet.ID == "" {
		changeSet.ID = uuid.NewString()
	}
	for index := range changeSet.Operations {
		if changeSet.Operations[index].ID == "" {
			changeSet.Operations[index].ID = uuid.NewString()
		}
		if strings.TrimSpace(changeSet.Operations[index].GroupKey) == "" {
			changeSet.Operations[index].GroupKey = "main"
		}
		if !changeSet.Operations[index].Selected {
			// New operations are selected by default. The review endpoint can
			// explicitly persist exclusions before Apply.
			changeSet.Operations[index].Selected = true
		}
		if changeSet.Operations[index].Dependencies == nil {
			changeSet.Operations[index].Dependencies = make([]string, 0)
		}
		changeSet.Operations[index].Position = index
	}
	if err := changeSet.Validate(); err != nil {
		return assistant.ChangeSet{}, err
	}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := stageAssistantChangeSetTx(ctx, tx, changeSet); err != nil {
			return err
		}
		if draftTurnID != "" {
			command, err := tx.Exec(ctx, `
				DELETE FROM assistant_turn_drafts WHERE turn_id=$1 AND run_id=$2
			`, draftTurnID, changeSet.RunID)
			if err != nil {
				return err
			}
			if command.RowsAffected() != 1 {
				return errors.New("durable Assistant turn draft is missing")
			}
			command, err = tx.Exec(ctx, `
				UPDATE assistant_turns
				SET state='awaiting_review', completed_at=COALESCE(completed_at, now())
				WHERE id=$1 AND run_id=$2 AND state IN ('queued','working')
			`, draftTurnID, changeSet.RunID)
			if err != nil {
				return err
			}
			if command.RowsAffected() != 1 {
				return errors.New("durable Assistant turn is not active")
			}
			command, err = tx.Exec(ctx, `
				UPDATE assistant_runs
				SET status='awaiting_review', error_code=NULL, updated_at=now()
				WHERE id=$1
			`, changeSet.RunID)
			if err != nil {
				return err
			}
			if command.RowsAffected() != 1 {
				return errors.New("durable Assistant run is missing")
			}
		}
		return nil
	})
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, changeSet.RunID, changeSet.ID)
}

func stageAssistantChangeSetTx(
	ctx context.Context,
	tx pgx.Tx,
	changeSet assistant.ChangeSet,
) error {
	_, err := tx.Exec(ctx, `
			INSERT INTO assistant_change_sets(
				id, run_id, turn_id, state, title, summary, risk_level, base_fingerprint
			)
			VALUES($1,$2,$3,'staged',$4,$5,$6,$7)
		`, changeSet.ID, changeSet.RunID, changeSet.TurnID, changeSet.Title,
		changeSet.Summary, changeSet.RiskLevel, changeSet.BaseFingerprint)
	if err != nil {
		return err
	}
	for _, operation := range changeSet.Operations {
		dependencies, err := json.Marshal(operation.Dependencies)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
				INSERT INTO assistant_change_operations(
					id, change_set_id, position, group_key, selected, dependencies,
					tool_name, target_kind, target_id, base_revision, arguments,
					before_snapshot, proposed_snapshot, trace, risk_level
				)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		`, operation.ID, changeSet.ID, operation.Position, operation.GroupKey,
			operation.Selected, dependencies, operation.ToolName, operation.TargetKind,
			operation.TargetID, operation.BaseRevision, operation.Arguments,
			nullableJSON(operation.BeforeSnapshot), operation.ProposedSnapshot,
			nonemptyJSON(operation.Trace), operation.RiskLevel); err != nil {
			return err
		}
	}
	return nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nonemptyJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
