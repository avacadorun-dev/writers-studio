package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MCPOAuthClient struct {
	ClientID                string    `json:"client_id"`
	ClientName              string    `json:"client_name"`
	RedirectURIs            []string  `json:"redirect_uris"`
	TokenEndpointAuthMethod string    `json:"token_endpoint_auth_method"`
	CreatedAt               time.Time `json:"client_id_issued_at"`
}

func (s *Store) RegisterMCPOAuthClient(
	ctx context.Context,
	clientName string,
	redirectURIs []string,
	authMethod string,
) (MCPOAuthClient, error) {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" || len(redirectURIs) == 0 {
		return MCPOAuthClient{}, errors.New("invalid OAuth client metadata")
	}
	if authMethod == "" {
		authMethod = "none"
	}
	if authMethod != "none" {
		return MCPOAuthClient{}, errors.New("only public PKCE clients are supported in v1")
	}
	encoded, err := json.Marshal(redirectURIs)
	if err != nil {
		return MCPOAuthClient{}, err
	}
	client := MCPOAuthClient{
		ClientID: "wsmcp_client_" + uuid.NewString(), ClientName: clientName,
		RedirectURIs: redirectURIs, TokenEndpointAuthMethod: authMethod,
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO mcp_oauth_clients(
			client_id, client_name, redirect_uris, token_endpoint_auth_method
		) VALUES($1,$2,$3,$4)
		RETURNING created_at
	`, client.ClientID, clientName, encoded, authMethod).Scan(&client.CreatedAt)
	return client, err
}

// SaveMCPOAuthMetadataClient stores a validated Client ID Metadata Document
// client so authorization codes and tokens can use the same durable foreign
// keys as dynamically registered clients. The caller is responsible for
// fetching the HTTPS document and validating that client_id exactly matches
// its URL.
func (s *Store) SaveMCPOAuthMetadataClient(
	ctx context.Context,
	client MCPOAuthClient,
) (MCPOAuthClient, error) {
	client.ClientID = strings.TrimSpace(client.ClientID)
	client.ClientName = strings.TrimSpace(client.ClientName)
	if client.ClientID == "" || client.ClientName == "" || len(client.RedirectURIs) == 0 {
		return MCPOAuthClient{}, errors.New("invalid OAuth client metadata document")
	}
	if client.TokenEndpointAuthMethod == "" {
		client.TokenEndpointAuthMethod = "none"
	}
	if client.TokenEndpointAuthMethod != "none" {
		return MCPOAuthClient{}, errors.New("only public PKCE clients are supported in v1")
	}
	encoded, err := json.Marshal(client.RedirectURIs)
	if err != nil {
		return MCPOAuthClient{}, err
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO mcp_oauth_clients(
			client_id, client_name, redirect_uris, token_endpoint_auth_method
		) VALUES($1,$2,$3,$4)
		ON CONFLICT(client_id) DO UPDATE SET
			client_name=EXCLUDED.client_name,
			redirect_uris=EXCLUDED.redirect_uris,
			token_endpoint_auth_method=EXCLUDED.token_endpoint_auth_method,
			updated_at=now()
		RETURNING created_at
	`, client.ClientID, client.ClientName, encoded, client.TokenEndpointAuthMethod).Scan(&client.CreatedAt)
	return client, err
}

func (s *Store) GetMCPOAuthClient(ctx context.Context, clientID string) (MCPOAuthClient, error) {
	var client MCPOAuthClient
	var redirects []byte
	if err := s.Pool.QueryRow(ctx, `
		SELECT client_id, client_name, redirect_uris, token_endpoint_auth_method, created_at
		FROM mcp_oauth_clients WHERE client_id=$1
	`, clientID).Scan(
		&client.ClientID, &client.ClientName, &redirects,
		&client.TokenEndpointAuthMethod, &client.CreatedAt,
	); err != nil {
		return MCPOAuthClient{}, err
	}
	if err := json.Unmarshal(redirects, &client.RedirectURIs); err != nil {
		return MCPOAuthClient{}, err
	}
	return client, nil
}

func (c MCPOAuthClient) AllowsRedirectURI(candidate string) bool {
	for _, allowed := range c.RedirectURIs {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

type MCPOAuthCode struct {
	ClientID      string
	GrantID       string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scopes        []string
}

func (s *Store) CreateMCPOAuthCode(
	ctx context.Context,
	input MCPOAuthCode,
) (string, error) {
	secret, hash, err := CreateMCPSecret("wsmcp_code_")
	if err != nil {
		return "", err
	}
	encodedScopes, _ := json.Marshal(input.Scopes)
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO mcp_oauth_codes(
			code_hash, client_id, grant_id, redirect_uri, code_challenge,
			resource, scopes, expires_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '10 minutes')
	`, hash, input.ClientID, input.GrantID, input.RedirectURI, input.CodeChallenge,
		input.Resource, encodedScopes)
	if err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) ConsumeMCPOAuthCode(
	ctx context.Context,
	secret, clientID, redirectURI, resource string,
) (MCPOAuthCode, error) {
	var code MCPOAuthCode
	var scopes []byte
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT client_id, grant_id, redirect_uri, code_challenge, resource, scopes
			FROM mcp_oauth_codes
			WHERE code_hash=$1 AND client_id=$2 AND redirect_uri=$3 AND resource=$4
				AND used_at IS NULL AND expires_at > now()
			FOR UPDATE
		`, hashMCPSecret(secret), clientID, redirectURI, resource).Scan(
			&code.ClientID, &code.GrantID, &code.RedirectURI, &code.CodeChallenge,
			&code.Resource, &scopes,
		); err != nil {
			return err
		}
		command, err := tx.Exec(ctx, `
			UPDATE mcp_oauth_codes SET used_at=now()
			WHERE code_hash=$1 AND used_at IS NULL
		`, hashMCPSecret(secret))
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return MCPOAuthCode{}, err
	}
	if err := json.Unmarshal(scopes, &code.Scopes); err != nil {
		return MCPOAuthCode{}, err
	}
	return code, nil
}

func (s *Store) GetMCPOAuthCode(
	ctx context.Context,
	secret, clientID, redirectURI, resource string,
) (MCPOAuthCode, error) {
	var code MCPOAuthCode
	var scopes []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT client_id, grant_id, redirect_uri, code_challenge, resource, scopes
		FROM mcp_oauth_codes
		WHERE code_hash=$1 AND client_id=$2 AND redirect_uri=$3 AND resource=$4
			AND used_at IS NULL AND expires_at > now()
	`, hashMCPSecret(secret), clientID, redirectURI, resource).Scan(
		&code.ClientID, &code.GrantID, &code.RedirectURI, &code.CodeChallenge,
		&code.Resource, &scopes,
	)
	if err != nil {
		return MCPOAuthCode{}, err
	}
	if err := json.Unmarshal(scopes, &code.Scopes); err != nil {
		return MCPOAuthCode{}, err
	}
	return code, nil
}

type MCPOAuthTokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Scopes       []string
}

func (s *Store) IssueMCPOAuthTokenPair(
	ctx context.Context,
	grantID, clientID, audience string,
	scopes []string,
) (MCPOAuthTokenPair, error) {
	access, accessHash, err := CreateMCPSecret(MCPOAuthTokenPrefix)
	if err != nil {
		return MCPOAuthTokenPair{}, err
	}
	refresh, refreshHash, err := CreateMCPSecret(MCPRefreshTokenPrefix)
	if err != nil {
		return MCPOAuthTokenPair{}, err
	}
	encodedScopes, _ := json.Marshal(scopes)
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO mcp_oauth_tokens(
				token_hash, grant_id, client_id, token_kind, scopes, audience, expires_at
			) VALUES($1,$2,$3,'access',$4,$5,now()+interval '1 hour')
		`, accessHash, grantID, clientID, encodedScopes, audience); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO mcp_oauth_tokens(
				token_hash, grant_id, client_id, token_kind, scopes, audience, expires_at
			) VALUES($1,$2,$3,'refresh',$4,$5,now()+interval '30 days')
		`, refreshHash, grantID, clientID, encodedScopes, audience)
		return err
	})
	if err != nil {
		return MCPOAuthTokenPair{}, err
	}
	return MCPOAuthTokenPair{
		AccessToken: access, RefreshToken: refresh, ExpiresIn: 3600, Scopes: scopes,
	}, nil
}

func (s *Store) RotateMCPOAuthRefreshToken(
	ctx context.Context,
	secret, clientID, audience string,
) (MCPOAuthTokenPair, error) {
	var grantID string
	var scopesJSON []byte
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT t.grant_id, t.scopes
			FROM mcp_oauth_tokens t
			JOIN mcp_access_grants g ON g.id=t.grant_id
			WHERE t.token_hash=$1 AND t.client_id=$2 AND t.audience=$3
				AND t.token_kind='refresh' AND t.revoked_at IS NULL AND t.expires_at > now()
				AND g.revoked_at IS NULL
			FOR UPDATE OF t
		`, hashMCPSecret(secret), clientID, audience).Scan(&grantID, &scopesJSON); err != nil {
			return err
		}
		command, err := tx.Exec(ctx, `
			UPDATE mcp_oauth_tokens SET revoked_at=now()
			WHERE token_hash=$1 AND revoked_at IS NULL
		`, hashMCPSecret(secret))
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return MCPOAuthTokenPair{}, err
	}
	var scopes []string
	if err := json.Unmarshal(scopesJSON, &scopes); err != nil {
		return MCPOAuthTokenPair{}, err
	}
	return s.IssueMCPOAuthTokenPair(ctx, grantID, clientID, audience, scopes)
}

func (s *Store) RevokeMCPOAuthToken(ctx context.Context, secret string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE mcp_oauth_tokens SET revoked_at=COALESCE(revoked_at, now())
		WHERE token_hash=$1
	`, hashMCPSecret(secret))
	return err
}
