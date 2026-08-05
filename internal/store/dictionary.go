package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/domain"
)

func (s *Store) ListDictionaryTerms(
	ctx context.Context,
	ownerID, projectID string,
) ([]domain.DictionaryTerm, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id::text, t.project_id::text, t.canonical, t.variants, t.kind, t.language,
			t.pronunciation, t.active, t.revision, t.source, t.operation_id::text,
			t.assistant_run_id::text, t.created_at, t.updated_at
		FROM dictionary_terms t
		JOIN projects p ON p.id=t.project_id
		WHERE t.project_id=$1 AND p.owner_user_id=$2
		ORDER BY t.active DESC, lower(t.canonical)
	`, projectID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list dictionary terms: %w", err)
	}
	defer rows.Close()

	terms := make([]domain.DictionaryTerm, 0)
	for rows.Next() {
		term, err := scanDictionaryTerm(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, rows.Err()
}

func (s *Store) CreateDictionaryTerm(
	ctx context.Context,
	ownerID, projectID, canonical, kind, language string,
	variants []string,
) (domain.DictionaryTerm, error) {
	var term domain.DictionaryTerm
	var variantsJSON []byte
	variantsJSON, err := json.Marshal(variants)
	if err != nil {
		return term, err
	}

	err = s.Pool.QueryRow(ctx, `
		INSERT INTO dictionary_terms(id, project_id, canonical, variants, kind, language)
		SELECT $1, p.id, $4, $5, $6, $7
		FROM projects p
		WHERE p.id=$2 AND p.owner_user_id=$3
		RETURNING id::text, project_id::text, canonical, variants, kind, language,
			pronunciation, active, revision, source, operation_id::text,
			assistant_run_id::text, created_at, updated_at
	`, uuid.NewString(), projectID, ownerID, canonical, variantsJSON, kind, language).Scan(
		&term.ID,
		&term.ProjectID,
		&term.Canonical,
		&variantsJSON,
		&term.Kind,
		&term.Language,
		&term.Pronunciation,
		&term.Active,
		&term.Revision,
		&term.Source,
		&term.OperationID,
		&term.AssistantRunID,
		&term.CreatedAt,
		&term.UpdatedAt,
	)
	if err != nil {
		return domain.DictionaryTerm{}, err
	}
	if err := json.Unmarshal(variantsJSON, &term.Variants); err != nil {
		return domain.DictionaryTerm{}, err
	}
	return term, nil
}

func (s *Store) UpdateDictionaryTerm(
	ctx context.Context,
	ownerID, termID string,
	canonical, kind, language string,
	variants []string,
	active bool,
) (domain.DictionaryTerm, error) {
	var term domain.DictionaryTerm
	variantsJSON, err := json.Marshal(variants)
	if err != nil {
		return term, err
	}

	err = s.Pool.QueryRow(ctx, `
		UPDATE dictionary_terms t
		SET canonical=$3, variants=$4, kind=$5, language=$6, active=$7,
			revision=revision+1, source='manual', updated_at=now()
		FROM projects p
		WHERE t.id=$1 AND p.id=t.project_id AND p.owner_user_id=$2
		RETURNING t.id::text, t.project_id::text, t.canonical, t.variants, t.kind, t.language,
			t.pronunciation, t.active, t.revision, t.source, t.operation_id::text,
			t.assistant_run_id::text, t.created_at, t.updated_at
	`, termID, ownerID, canonical, variantsJSON, kind, language, active).Scan(
		&term.ID,
		&term.ProjectID,
		&term.Canonical,
		&variantsJSON,
		&term.Kind,
		&term.Language,
		&term.Pronunciation,
		&term.Active,
		&term.Revision,
		&term.Source,
		&term.OperationID,
		&term.AssistantRunID,
		&term.CreatedAt,
		&term.UpdatedAt,
	)
	if err != nil {
		return domain.DictionaryTerm{}, err
	}
	if err := json.Unmarshal(variantsJSON, &term.Variants); err != nil {
		return domain.DictionaryTerm{}, err
	}
	return term, nil
}

func (s *Store) DeleteDictionaryTerm(ctx context.Context, ownerID, termID string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var projectID string
		if err := tx.QueryRow(ctx, `
			SELECT t.project_id::text
			FROM dictionary_terms t JOIN projects p ON p.id=t.project_id
			WHERE t.id=$1 AND p.owner_user_id=$2
			FOR UPDATE OF t
		`, termID, ownerID).Scan(&projectID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE document_annotations
			SET status='needs_attention', updated_at=now()
			WHERE dictionary_term_id=$1 AND project_id=$2
		`, termID, projectID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM dictionary_terms WHERE id=$1`, termID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDictionaryTerm(scanner rowScanner) (domain.DictionaryTerm, error) {
	var term domain.DictionaryTerm
	var variants []byte
	if err := scanner.Scan(
		&term.ID,
		&term.ProjectID,
		&term.Canonical,
		&variants,
		&term.Kind,
		&term.Language,
		&term.Pronunciation,
		&term.Active,
		&term.Revision,
		&term.Source,
		&term.OperationID,
		&term.AssistantRunID,
		&term.CreatedAt,
		&term.UpdatedAt,
	); err != nil {
		return term, err
	}
	if err := json.Unmarshal(variants, &term.Variants); err != nil {
		return term, err
	}
	return term, nil
}
