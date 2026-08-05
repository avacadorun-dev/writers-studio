package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
)

const (
	MCPManualTokenPrefix  = "wsmcp_"
	MCPOAuthTokenPrefix   = "wsmcp_at_"
	MCPRefreshTokenPrefix = "wsmcp_rt_"
	MCPReviewTokenPrefix  = "wsmcp_review_"
)

var mcpScopes = map[string]bool{
	"projects:list":    true,
	"projects:read":    true,
	"projects:stage":   true,
	"projects:apply":   true,
	"projects:actions": true,
}

type MCPGrant struct {
	ID             string     `json:"id"`
	OwnerUserID    string     `json:"-"`
	Name           string     `json:"name"`
	ClientName     string     `json:"clientName"`
	AuthKind       string     `json:"authKind"`
	Scopes         []string   `json:"scopes"`
	ProjectIDs     []string   `json:"projectIds"`
	TokenExpiresAt *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt     *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt      *time.Time `json:"revokedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (g MCPGrant) HasScope(scope string) bool {
	for _, candidate := range g.Scopes {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(scope)) == 1 {
			return true
		}
	}
	return false
}

func (g MCPGrant) AllowsProject(projectID string) bool {
	for _, candidate := range g.ProjectIDs {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(projectID)) == 1 {
			return true
		}
	}
	return false
}

type CreateMCPGrantInput struct {
	Name       string
	ClientName string
	ProjectIDs []string
	Scopes     []string
	ExpiresIn  time.Duration
	AuthKind   string
}

func CreateMCPSecret(prefix string) (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	secret := prefix + base64.RawURLEncoding.EncodeToString(bytes)
	return secret, hashMCPSecret(secret), nil
}

func hashMCPSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func normalizeMCPScopes(scopes []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if !mcpScopes[scope] {
			return nil, fmt.Errorf("unsupported MCP scope %q", scope)
		}
		if !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("at least one MCP scope is required")
	}
	sort.Strings(result)
	return result, nil
}

func normalizeMCPProjects(projectIDs []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		projectID = strings.TrimSpace(projectID)
		if _, err := uuid.Parse(projectID); err != nil {
			return nil, errors.New("invalid MCP project id")
		}
		if !seen[projectID] {
			seen[projectID] = true
			result = append(result, projectID)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("at least one MCP project is required")
	}
	sort.Strings(result)
	return result, nil
}

func (s *Store) CreateMCPGrant(
	ctx context.Context,
	ownerID string,
	input CreateMCPGrantInput,
) (MCPGrant, string, error) {
	name := strings.TrimSpace(input.Name)
	clientName := strings.TrimSpace(input.ClientName)
	if name == "" || len([]rune(name)) > 120 || clientName == "" || len([]rune(clientName)) > 160 {
		return MCPGrant{}, "", errors.New("invalid MCP grant name")
	}
	scopes, err := normalizeMCPScopes(input.Scopes)
	if err != nil {
		return MCPGrant{}, "", err
	}
	projectIDs, err := normalizeMCPProjects(input.ProjectIDs)
	if err != nil {
		return MCPGrant{}, "", err
	}
	if input.AuthKind == "" {
		input.AuthKind = "manual"
	}
	if input.AuthKind != "manual" && input.AuthKind != "oauth" {
		return MCPGrant{}, "", errors.New("invalid MCP grant auth kind")
	}
	var secret string
	var tokenHash *string
	var expiresAt *time.Time
	if input.AuthKind == "manual" {
		if input.ExpiresIn == 0 {
			input.ExpiresIn = 90 * 24 * time.Hour
		}
		allowed := map[time.Duration]bool{
			7 * 24 * time.Hour: true, 30 * 24 * time.Hour: true,
			90 * 24 * time.Hour: true, 365 * 24 * time.Hour: true,
		}
		if !allowed[input.ExpiresIn] {
			return MCPGrant{}, "", errors.New("manual MCP token expiry must be 7, 30, 90, or 365 days")
		}
		var hash string
		secret, hash, err = CreateMCPSecret(MCPManualTokenPrefix)
		if err != nil {
			return MCPGrant{}, "", err
		}
		tokenHash = &hash
		value := time.Now().UTC().Add(input.ExpiresIn)
		expiresAt = &value
	}
	encodedScopes, _ := json.Marshal(scopes)
	grant := MCPGrant{
		ID: uuid.NewString(), OwnerUserID: ownerID, Name: name, ClientName: clientName,
		AuthKind: input.AuthKind, Scopes: scopes, ProjectIDs: projectIDs,
		TokenExpiresAt: expiresAt,
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var owned int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM projects
			WHERE owner_user_id=$1 AND status <> 'trash' AND id=ANY($2::uuid[])
		`, ownerID, projectIDs).Scan(&owned); err != nil {
			return err
		}
		if owned != len(projectIDs) {
			return pgx.ErrNoRows
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO mcp_access_grants(
				id, owner_user_id, name, client_name, auth_kind, token_hash,
				token_expires_at, scopes
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING created_at, updated_at
		`, grant.ID, ownerID, name, clientName, input.AuthKind, tokenHash,
			expiresAt, encodedScopes).Scan(&grant.CreatedAt, &grant.UpdatedAt); err != nil {
			return err
		}
		for _, projectID := range projectIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO mcp_grant_projects(grant_id, project_id) VALUES($1,$2)
			`, grant.ID, projectID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MCPGrant{}, "", err
	}
	return grant, secret, nil
}

func (s *Store) ListMCPGrants(ctx context.Context, ownerID string) ([]MCPGrant, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT g.id, g.owner_user_id, g.name, g.client_name, g.auth_kind, g.scopes,
			g.token_expires_at, g.last_used_at, g.revoked_at, g.created_at, g.updated_at,
			COALESCE(array_agg(gp.project_id::text ORDER BY gp.project_id::text)
				FILTER (WHERE gp.project_id IS NOT NULL), ARRAY[]::text[])
		FROM mcp_access_grants g
		LEFT JOIN mcp_grant_projects gp ON gp.grant_id=g.id
		WHERE g.owner_user_id=$1
		GROUP BY g.id
		ORDER BY g.created_at DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]MCPGrant, 0)
	for rows.Next() {
		grant, err := scanMCPGrant(rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *Store) GetMCPGrant(ctx context.Context, ownerID, grantID string) (MCPGrant, error) {
	return scanMCPGrant(s.Pool.QueryRow(ctx, `
		SELECT g.id, g.owner_user_id, g.name, g.client_name, g.auth_kind, g.scopes,
			g.token_expires_at, g.last_used_at, g.revoked_at, g.created_at, g.updated_at,
			COALESCE(array_agg(gp.project_id::text ORDER BY gp.project_id::text)
				FILTER (WHERE gp.project_id IS NOT NULL), ARRAY[]::text[])
		FROM mcp_access_grants g
		LEFT JOIN mcp_grant_projects gp ON gp.grant_id=g.id
		WHERE g.id=$1 AND ($2='' OR g.owner_user_id::text=$2)
		GROUP BY g.id
	`, grantID, ownerID))
}

func scanMCPGrant(row rowScanner) (MCPGrant, error) {
	var grant MCPGrant
	var scopes []byte
	if err := row.Scan(
		&grant.ID, &grant.OwnerUserID, &grant.Name, &grant.ClientName, &grant.AuthKind,
		&scopes, &grant.TokenExpiresAt, &grant.LastUsedAt, &grant.RevokedAt,
		&grant.CreatedAt, &grant.UpdatedAt, &grant.ProjectIDs,
	); err != nil {
		return MCPGrant{}, err
	}
	if err := json.Unmarshal(scopes, &grant.Scopes); err != nil {
		return MCPGrant{}, err
	}
	return grant, nil
}

func (s *Store) RotateMCPGrant(
	ctx context.Context,
	ownerID, grantID string,
	expiresIn time.Duration,
) (MCPGrant, string, error) {
	if expiresIn == 0 {
		expiresIn = 90 * 24 * time.Hour
	}
	allowed := map[time.Duration]bool{
		7 * 24 * time.Hour: true, 30 * 24 * time.Hour: true,
		90 * 24 * time.Hour: true, 365 * 24 * time.Hour: true,
	}
	if !allowed[expiresIn] {
		return MCPGrant{}, "", errors.New("invalid MCP token expiry")
	}
	secret, hash, err := CreateMCPSecret(MCPManualTokenPrefix)
	if err != nil {
		return MCPGrant{}, "", err
	}
	expiresAt := time.Now().UTC().Add(expiresIn)
	command, err := s.Pool.Exec(ctx, `
		UPDATE mcp_access_grants
		SET token_hash=$3, token_expires_at=$4,
			revoked_at=NULL, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2 AND auth_kind='manual'
	`, grantID, ownerID, hash, expiresAt)
	if err != nil {
		return MCPGrant{}, "", err
	}
	if command.RowsAffected() == 0 {
		return MCPGrant{}, "", pgx.ErrNoRows
	}
	grant, err := s.GetMCPGrant(ctx, ownerID, grantID)
	return grant, secret, err
}

func (s *Store) RevokeMCPGrant(ctx context.Context, ownerID, grantID string) error {
	command, err := s.Pool.Exec(ctx, `
		UPDATE mcp_access_grants
		SET revoked_at=COALESCE(revoked_at, now()), token_hash=NULL, updated_at=now()
		WHERE id=$1 AND owner_user_id=$2 AND revoked_at IS NULL
	`, grantID, ownerID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE mcp_oauth_tokens SET revoked_at=COALESCE(revoked_at, now())
		WHERE grant_id=$1 AND revoked_at IS NULL
	`, grantID)
	return err
}

func (s *Store) VerifyMCPBearer(ctx context.Context, secret, audience string) (MCPGrant, error) {
	if strings.TrimSpace(secret) == "" {
		return MCPGrant{}, pgx.ErrNoRows
	}
	hash := hashMCPSecret(secret)
	var grantID string
	var oauthScopes []byte
	if strings.HasPrefix(secret, MCPManualTokenPrefix) &&
		!strings.HasPrefix(secret, MCPOAuthTokenPrefix) &&
		!strings.HasPrefix(secret, MCPRefreshTokenPrefix) {
		err := s.Pool.QueryRow(ctx, `
			UPDATE mcp_access_grants
			SET last_used_at=now()
			WHERE token_hash=$1 AND auth_kind='manual' AND revoked_at IS NULL
				AND token_expires_at > now()
			RETURNING id
		`, hash).Scan(&grantID)
		if err != nil {
			return MCPGrant{}, err
		}
	} else if strings.HasPrefix(secret, MCPOAuthTokenPrefix) {
		err := s.Pool.QueryRow(ctx, `
			UPDATE mcp_oauth_tokens t
			SET last_used_at=now()
			FROM mcp_access_grants g
			WHERE t.token_hash=$1 AND t.token_kind='access' AND t.audience=$2
				AND t.expires_at > now() AND t.revoked_at IS NULL
				AND g.id=t.grant_id AND g.revoked_at IS NULL
			RETURNING t.grant_id, t.scopes
		`, hash, audience).Scan(&grantID, &oauthScopes)
		if err != nil {
			return MCPGrant{}, err
		}
	} else {
		return MCPGrant{}, pgx.ErrNoRows
	}
	grant, err := s.GetMCPGrant(ctx, "", grantID)
	if err != nil {
		return MCPGrant{}, err
	}
	if len(oauthScopes) > 0 {
		if err := json.Unmarshal(oauthScopes, &grant.Scopes); err != nil {
			return MCPGrant{}, err
		}
	}
	return grant, nil
}

func (s *Store) ListMCPGrantProjects(
	ctx context.Context,
	grant MCPGrant,
	status string,
) ([]domain.Project, error) {
	if !grant.HasScope("projects:list") {
		return nil, errors.New("MCP grant lacks projects:list")
	}
	if status == "trash" {
		return []domain.Project{}, nil
	}
	if status != "active" && status != "archived" && status != "" {
		return nil, errors.New("invalid project status")
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id::text, p.title, p.kind, p.language, p.synopsis, p.word_goal, p.status,
			p.structure_revision, p.structure_schema, p.trashed_at, p.created_at, p.updated_at
		FROM projects p
		JOIN mcp_grant_projects gp ON gp.project_id=p.id AND gp.grant_id=$1
		WHERE p.owner_user_id=$2 AND p.status <> 'trash'
			AND ($3='' OR p.status=$3)
		ORDER BY p.updated_at DESC
	`, grant.ID, grant.OwnerUserID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []domain.Project
	for rows.Next() {
		var project domain.Project
		var structure []byte
		if err := rows.Scan(
			&project.ID, &project.Title, &project.Kind, &project.Language, &project.Synopsis,
			&project.WordGoal, &project.Status, &project.StructureRevision, &structure,
			&project.TrashedAt, &project.CreatedAt, &project.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(structure, &project.Structure); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) OpenMCPProject(
	ctx context.Context,
	grant MCPGrant,
	projectID string,
	targetNodeID *string,
) (ProjectWorkspace, error) {
	if !grant.HasScope("projects:read") || !grant.AllowsProject(projectID) {
		return ProjectWorkspace{}, pgx.ErrNoRows
	}
	workspace, err := s.GetProject(ctx, grant.OwnerUserID, projectID)
	if err != nil || workspace.Project.Status == "trash" {
		return ProjectWorkspace{}, pgx.ErrNoRows
	}
	if targetNodeID != nil {
		found := false
		for _, node := range workspace.Nodes {
			if node.ID == *targetNodeID && node.TrashedAt == nil {
				found = true
				break
			}
		}
		if !found {
			return ProjectWorkspace{}, pgx.ErrNoRows
		}
	}
	return workspace, nil
}

func (s *Store) SaveAssistantTurnDraft(
	ctx context.Context,
	runID, turnID string,
	operations []assistant.ChangeOperation,
	toolCallCount int,
) error {
	encoded, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO assistant_turn_drafts(turn_id, run_id, operations, tool_call_count)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(turn_id) DO UPDATE
		SET operations=excluded.operations, tool_call_count=excluded.tool_call_count, updated_at=now()
	`, turnID, runID, encoded, toolCallCount)
	return err
}

func (s *Store) LoadAssistantTurnDraft(
	ctx context.Context,
	turnID string,
) ([]assistant.ChangeOperation, int, error) {
	var encoded []byte
	var count int
	if err := s.Pool.QueryRow(ctx, `
		SELECT operations, tool_call_count FROM assistant_turn_drafts WHERE turn_id=$1
	`, turnID).Scan(&encoded, &count); err != nil {
		return nil, 0, err
	}
	var operations []assistant.ChangeOperation
	if err := json.Unmarshal(encoded, &operations); err != nil {
		return nil, 0, err
	}
	return operations, count, nil
}

func (s *Store) DeleteAssistantTurnDraft(ctx context.Context, turnID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM assistant_turn_drafts WHERE turn_id=$1`, turnID)
	return err
}

// RecoverRemoteMCPDrafts publishes every acknowledged-but-unfinished remote
// draft as a partial change set before generic interrupted-run recovery runs.
// It is restart-safe: a pre-existing change set wins and its leftover draft is
// simply removed on the next pass.
func (s *Store) RecoverRemoteMCPDrafts(ctx context.Context) (int, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT d.turn_id, d.run_id, r.owner_user_id, r.project_id, d.operations
		FROM assistant_turn_drafts d
		JOIN assistant_runs r ON r.id=d.run_id AND r.origin='remote_mcp'
		WHERE jsonb_array_length(d.operations) > 0
		ORDER BY d.updated_at
	`)
	if err != nil {
		return 0, err
	}
	type draft struct {
		turnID, runID, ownerID, projectID string
		operations                        []assistant.ChangeOperation
	}
	var drafts []draft
	for rows.Next() {
		var value draft
		var encoded []byte
		if err := rows.Scan(&value.turnID, &value.runID, &value.ownerID, &value.projectID, &encoded); err != nil {
			rows.Close()
			return 0, err
		}
		if err := json.Unmarshal(encoded, &value.operations); err != nil {
			rows.Close()
			return 0, err
		}
		drafts = append(drafts, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	recovered := 0
	for _, draft := range drafts {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM assistant_change_sets WHERE turn_id=$1)
		`, draft.turnID).Scan(&exists); err != nil {
			return recovered, err
		}
		if exists {
			if err := s.DeleteAssistantTurnDraft(ctx, draft.turnID); err != nil {
				return recovered, err
			}
			continue
		}
		fingerprint, err := s.ProjectFingerprint(ctx, draft.ownerID, draft.projectID)
		if err != nil {
			return recovered, err
		}
		risk := assistant.RiskLow
		for _, operation := range draft.operations {
			if operation.RiskLevel == assistant.RiskHigh {
				risk = assistant.RiskHigh
				break
			}
			if operation.RiskLevel == assistant.RiskMedium {
				risk = assistant.RiskMedium
			}
		}
		_, err = s.StageAssistantChangeSetFromDraft(ctx, assistant.ChangeSet{
			RunID: draft.runID, TurnID: draft.turnID,
			Title:     "Частичные изменения внешнего агента",
			Summary:   "MCP-сессия завершилась до finish_turn. Все подтверждённые сервером staged-операции сохранены; канонический проект не изменён.",
			RiskLevel: risk, BaseFingerprint: fingerprint, Operations: draft.operations,
		}, draft.turnID)
		if err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

type MCPReviewGrant struct {
	RunID         string
	ProjectID     string
	ExpectedState assistant.ChangeSetState
	TokenKind     string
	ActionKind    string
	Payload       json.RawMessage
}

func digestMCPChangeSet(changeSet assistant.ChangeSet) (string, error) {
	encoded, err := json.Marshal(changeSet)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Store) IssueMCPReviewToken(
	ctx context.Context,
	grant MCPGrant,
	changeSetID string,
	expectedState assistant.ChangeSetState,
) (assistant.ChangeSet, string, error) {
	if !grant.HasScope("projects:apply") {
		return assistant.ChangeSet{}, "", errors.New("MCP grant lacks projects:apply")
	}
	var runID string
	if err := s.Pool.QueryRow(ctx, `
		SELECT c.run_id
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		WHERE c.id=$1 AND c.state=$2 AND r.owner_user_id=$3 AND r.mcp_grant_id=$4
	`, changeSetID, expectedState, grant.OwnerUserID, grant.ID).Scan(&runID); err != nil {
		return assistant.ChangeSet{}, "", err
	}
	changeSet, err := s.GetAssistantChangeSet(ctx, runID, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, "", err
	}
	digest, err := digestMCPChangeSet(changeSet)
	if err != nil {
		return assistant.ChangeSet{}, "", err
	}
	secret, hash, err := CreateMCPSecret(MCPReviewTokenPrefix)
	if err != nil {
		return assistant.ChangeSet{}, "", err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO mcp_review_tokens(
			token_hash, grant_id, change_set_id, token_kind, expected_state,
			content_digest, expires_at
		) VALUES($1,$2,$3,'review',$4,$5,now()+interval '10 minutes')
	`, hash, grant.ID, changeSet.ID, expectedState, digest)
	if err != nil {
		return assistant.ChangeSet{}, "", err
	}
	return changeSet, secret, nil
}

func (s *Store) ConsumeMCPReviewToken(
	ctx context.Context,
	grant MCPGrant,
	changeSetID, secret string,
	expectedState assistant.ChangeSetState,
) (MCPReviewGrant, error) {
	var result MCPReviewGrant
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		result, err = consumeMCPReviewTokenTx(
			ctx, tx, grant, changeSetID, secret, expectedState,
		)
		return err
	})
	return result, err
}

func consumeMCPReviewTokenTx(
	ctx context.Context,
	tx pgx.Tx,
	grant MCPGrant,
	changeSetID, secret string,
	expectedState assistant.ChangeSetState,
) (MCPReviewGrant, error) {
	hash := hashMCPSecret(secret)
	var tokenDigest string
	var result MCPReviewGrant
	if err := tx.QueryRow(ctx, `
			SELECT c.run_id, r.project_id, t.expected_state, t.content_digest
			FROM mcp_review_tokens t
			JOIN assistant_change_sets c ON c.id=t.change_set_id
			JOIN assistant_runs r ON r.id=c.run_id
			WHERE t.token_hash=$1 AND t.grant_id=$2 AND t.change_set_id=$3
				AND t.token_kind='review' AND t.expected_state=$4
				AND t.used_at IS NULL AND t.expires_at > now()
				AND c.state=$4 AND r.mcp_grant_id=$2
			FOR UPDATE OF t, c
	`, hash, grant.ID, changeSetID, expectedState).Scan(
		&result.RunID, &result.ProjectID, &result.ExpectedState, &tokenDigest,
	); err != nil {
		return MCPReviewGrant{}, err
	}
	changeSet, err := getAssistantChangeSetTx(ctx, tx, result.RunID, changeSetID)
	if err != nil {
		return MCPReviewGrant{}, err
	}
	digest, err := digestMCPChangeSet(changeSet)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(tokenDigest)) != 1 {
		return MCPReviewGrant{}, assistant.ErrStaleChangeSet
	}
	command, err := tx.Exec(ctx, `
			UPDATE mcp_review_tokens SET used_at=now()
			WHERE token_hash=$1 AND used_at IS NULL
	`, hash)
	if err != nil {
		return MCPReviewGrant{}, err
	}
	if command.RowsAffected() != 1 {
		return MCPReviewGrant{}, pgx.ErrNoRows
	}
	return result, nil
}

func (s *Store) ApplyMCPChangeSet(
	ctx context.Context,
	grant MCPGrant,
	changeSetID, reviewToken string,
	selectedGroups []string,
	annotationResolutions map[string]bool,
) (assistant.ChangeSet, error) {
	if !grant.HasScope("projects:apply") {
		return assistant.ChangeSet{}, errors.New("MCP grant lacks projects:apply")
	}
	changeSet, err := s.GetMCPChangeSet(ctx, grant, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	guard := func(ctx context.Context, tx pgx.Tx) error {
		if err := ensureMCPChangeSetProjectWritableTx(ctx, tx, grant, changeSetID); err != nil {
			return err
		}
		_, err := consumeMCPReviewTokenTx(
			ctx, tx, grant, changeSetID, reviewToken, assistant.ChangeStaged,
		)
		return err
	}
	return s.applyAssistantChangeSet(
		ctx, grant.OwnerUserID, changeSet.RunID, changeSetID,
		selectedGroups, annotationResolutions, guard,
	)
}

func (s *Store) RejectMCPChangeSet(
	ctx context.Context,
	grant MCPGrant,
	changeSetID, reviewToken string,
) (assistant.ChangeSet, error) {
	if !grant.HasScope("projects:apply") {
		return assistant.ChangeSet{}, errors.New("MCP grant lacks projects:apply")
	}
	changeSet, err := s.GetMCPChangeSet(ctx, grant, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	guard := func(ctx context.Context, tx pgx.Tx) error {
		_, err := consumeMCPReviewTokenTx(
			ctx, tx, grant, changeSetID, reviewToken, assistant.ChangeStaged,
		)
		return err
	}
	return s.rejectAssistantChangeSet(
		ctx, grant.OwnerUserID, changeSet.RunID, changeSetID, guard,
	)
}

func (s *Store) UndoMCPChangeSet(
	ctx context.Context,
	grant MCPGrant,
	changeSetID, reviewToken string,
) (assistant.ChangeSet, error) {
	if !grant.HasScope("projects:apply") {
		return assistant.ChangeSet{}, errors.New("MCP grant lacks projects:apply")
	}
	changeSet, err := s.GetMCPChangeSet(ctx, grant, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	guard := func(ctx context.Context, tx pgx.Tx) error {
		if err := ensureMCPChangeSetProjectWritableTx(ctx, tx, grant, changeSetID); err != nil {
			return err
		}
		_, err := consumeMCPReviewTokenTx(
			ctx, tx, grant, changeSetID, reviewToken, assistant.ChangeApplied,
		)
		return err
	}
	return s.undoAssistantChangeSet(
		ctx, grant.OwnerUserID, changeSet.RunID, changeSetID, guard,
	)
}

func (s *Store) IssueMCPActionToken(
	ctx context.Context,
	grant MCPGrant,
	actionKind string,
	payload map[string]any,
) (string, error) {
	if !grant.HasScope("projects:actions") || strings.TrimSpace(actionKind) == "" {
		return "", errors.New("MCP grant lacks projects:actions")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	secret, hash, err := CreateMCPSecret("wsmcp_action_")
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO mcp_review_tokens(
			token_hash, grant_id, token_kind, action_kind, content_digest, payload, expires_at
		) VALUES($1,$2,'action',$3,$4,$5,now()+interval '10 minutes')
	`, hash, grant.ID, actionKind, hex.EncodeToString(digest[:]), encoded)
	return secret, err
}

// ConsumeMCPActionToken is replay-safe for its ten-minute lifetime. The first
// caller receives first=true and stores the action result; retries receive the
// same payload/result without starting a second job.
func (s *Store) ConsumeMCPActionToken(
	ctx context.Context,
	grant MCPGrant,
	actionKind, secret string,
) (map[string]any, bool, error) {
	var encoded []byte
	var usedAt *time.Time
	var expectedDigest string
	payload := make(map[string]any)
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT payload, used_at, content_digest
			FROM mcp_review_tokens
			WHERE token_hash=$1 AND grant_id=$2 AND token_kind='action'
				AND action_kind=$3 AND expires_at > now()
			FOR UPDATE
		`, hashMCPSecret(secret), grant.ID, actionKind).Scan(
			&encoded, &usedAt, &expectedDigest,
		); err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return err
		}
		basePayload := make(map[string]any, len(payload))
		for key, value := range payload {
			if key != "result" {
				basePayload[key] = value
			}
		}
		canonical, err := json.Marshal(basePayload)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(canonical)
		if subtle.ConstantTimeCompare(
			[]byte(hex.EncodeToString(digest[:])), []byte(expectedDigest),
		) != 1 {
			return errors.New("MCP action token payload changed")
		}
		if usedAt != nil {
			return nil
		}
		_, err = tx.Exec(ctx, `
			UPDATE mcp_review_tokens SET used_at=now() WHERE token_hash=$1
		`, hashMCPSecret(secret))
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return payload, usedAt == nil, nil
}

func (s *Store) SaveMCPActionResult(
	ctx context.Context,
	grant MCPGrant,
	secret string,
	result map[string]any,
) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	command, err := s.Pool.Exec(ctx, `
		UPDATE mcp_review_tokens
		SET payload=jsonb_set(payload, '{result}', $3::jsonb, true)
		WHERE token_hash=$1 AND grant_id=$2 AND token_kind='action' AND used_at IS NOT NULL
	`, hashMCPSecret(secret), grant.ID, encoded)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func getAssistantChangeSetTx(
	ctx context.Context,
	tx pgx.Tx,
	runID, changeSetID string,
) (assistant.ChangeSet, error) {
	// The existing hydrator uses the pool. Reading inside this transaction would
	// deadlock a one-connection integration pool, so reproduce the digest from
	// rows locked by this transaction.
	var changeSet assistant.ChangeSet
	if err := tx.QueryRow(ctx, `
		SELECT id, run_id, turn_id, state, title, summary, risk_level, base_fingerprint,
			operation_id, applied_at, rejected_at, undone_at, created_at, updated_at
		FROM assistant_change_sets WHERE id=$1 AND run_id=$2
	`, changeSetID, runID).Scan(
		&changeSet.ID, &changeSet.RunID, &changeSet.TurnID, &changeSet.State,
		&changeSet.Title, &changeSet.Summary, &changeSet.RiskLevel,
		&changeSet.BaseFingerprint, &changeSet.OperationID, &changeSet.AppliedAt,
		&changeSet.RejectedAt, &changeSet.UndoneAt, &changeSet.CreatedAt,
		&changeSet.UpdatedAt,
	); err != nil {
		return assistant.ChangeSet{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, position, group_key, selected, dependencies, tool_name,
			target_kind, target_id, base_revision, arguments, before_snapshot,
			proposed_snapshot, trace, risk_level, applied
		FROM assistant_change_operations
		WHERE change_set_id=$1 ORDER BY position
	`, changeSetID)
	if err != nil {
		return assistant.ChangeSet{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var operation assistant.ChangeOperation
		var dependencies []byte
		if err := rows.Scan(
			&operation.ID, &operation.Position, &operation.GroupKey, &operation.Selected,
			&dependencies, &operation.ToolName, &operation.TargetKind, &operation.TargetID,
			&operation.BaseRevision, &operation.Arguments, &operation.BeforeSnapshot,
			&operation.ProposedSnapshot, &operation.Trace, &operation.RiskLevel,
			&operation.Applied,
		); err != nil {
			return assistant.ChangeSet{}, err
		}
		if err := json.Unmarshal(dependencies, &operation.Dependencies); err != nil {
			return assistant.ChangeSet{}, err
		}
		if operation.Dependencies == nil {
			operation.Dependencies = []string{}
		}
		changeSet.Operations = append(changeSet.Operations, operation)
	}
	return changeSet, rows.Err()
}

func (s *Store) ListMCPChangeSets(
	ctx context.Context,
	grant MCPGrant,
	projectID string,
	limit int,
) ([]assistant.ChangeSet, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.run_id
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		WHERE r.mcp_grant_id=$1 AND r.owner_user_id=$2
			AND ($3='' OR r.project_id::text=$3)
		ORDER BY c.created_at DESC LIMIT $4
	`, grant.ID, grant.OwnerUserID, projectID, limit)
	if err != nil {
		return nil, err
	}
	type pair struct{ changeSetID, runID string }
	var pairs []pair
	for rows.Next() {
		var value pair
		if err := rows.Scan(&value.changeSetID, &value.runID); err != nil {
			rows.Close()
			return nil, err
		}
		pairs = append(pairs, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	result := make([]assistant.ChangeSet, 0, len(pairs))
	for _, value := range pairs {
		changeSet, err := s.GetAssistantChangeSet(ctx, value.runID, value.changeSetID)
		if err != nil {
			return nil, err
		}
		result = append(result, changeSet)
	}
	return result, nil
}

func (s *Store) GetMCPChangeSet(
	ctx context.Context,
	grant MCPGrant,
	changeSetID string,
) (assistant.ChangeSet, error) {
	var runID string
	if err := s.Pool.QueryRow(ctx, `
		SELECT c.run_id
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		WHERE c.id=$1 AND r.owner_user_id=$2 AND r.mcp_grant_id=$3
	`, changeSetID, grant.OwnerUserID, grant.ID).Scan(&runID); err != nil {
		return assistant.ChangeSet{}, err
	}
	return s.GetAssistantChangeSet(ctx, runID, changeSetID)
}

func (s *Store) EnsureMCPChangeSetProjectWritable(
	ctx context.Context,
	grant MCPGrant,
	changeSetID string,
) error {
	var writable bool
	err := s.Pool.QueryRow(ctx, `
		SELECT p.status='active' AND p.trashed_at IS NULL
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		JOIN projects p ON p.id=r.project_id
		JOIN mcp_grant_projects gp ON gp.project_id=p.id AND gp.grant_id=$3
		WHERE c.id=$1 AND r.owner_user_id=$2 AND r.mcp_grant_id=$3
	`, changeSetID, grant.OwnerUserID, grant.ID).Scan(&writable)
	if err != nil {
		return err
	}
	if !writable {
		return errors.New("archived projects are read-only over MCP")
	}
	return nil
}

func ensureMCPChangeSetProjectWritableTx(
	ctx context.Context,
	tx pgx.Tx,
	grant MCPGrant,
	changeSetID string,
) error {
	var writable bool
	err := tx.QueryRow(ctx, `
		SELECT p.status='active' AND p.trashed_at IS NULL
		FROM assistant_change_sets c
		JOIN assistant_runs r ON r.id=c.run_id
		JOIN projects p ON p.id=r.project_id
		JOIN mcp_grant_projects gp ON gp.project_id=p.id AND gp.grant_id=$3
		WHERE c.id=$1 AND r.owner_user_id=$2 AND r.mcp_grant_id=$3
		FOR UPDATE OF p
	`, changeSetID, grant.OwnerUserID, grant.ID).Scan(&writable)
	if err != nil {
		return err
	}
	if !writable {
		return errors.New("archived projects are read-only over MCP")
	}
	return nil
}

func (s *Store) RecordMCPAudit(
	ctx context.Context,
	grantID, clientName, sessionID, toolName, status string,
	duration time.Duration,
	affectedIDs []string,
	operationID, changeSetID *string,
) {
	encoded, _ := json.Marshal(affectedIDs)
	var sessionHash *string
	if sessionID != "" {
		value := hashMCPSecret(sessionID)
		sessionHash = &value
	}
	_, _ = s.Pool.Exec(ctx, `
		INSERT INTO mcp_audit_events(
			id, grant_id, client_name, session_id_hash, tool_name, status,
			duration_ms, affected_ids, operation_id, change_set_id
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, uuid.NewString(), grantID, clientName, sessionHash, toolName, status,
		duration.Milliseconds(), encoded, operationID, changeSetID)
}
