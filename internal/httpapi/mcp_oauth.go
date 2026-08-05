package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	fositestorage "github.com/ory/fosite/storage"

	"writers.studio/internal/store"
)

var supportedMCPScopes = []string{
	"projects:list", "projects:read", "projects:stage", "projects:apply", "projects:actions",
}

type oauthClientRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

type oauthClientMetadataDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func (s *Server) mountMCPOAuthRoutes(router chi.Router) {
	router.Get("/.well-known/oauth-protected-resource", s.handleMCPProtectedResourceMetadata)
	router.Get("/.well-known/oauth-authorization-server", s.handleMCPAuthorizationServerMetadata)
	router.Post("/oauth/register", s.handleMCPOAuthRegister)
	router.Get("/oauth/clients/{clientID}", s.handleMCPOAuthClientMetadata)
	router.Post("/oauth/token", s.handleMCPOAuthToken)
	router.Post("/oauth/revoke", s.handleMCPOAuthRevoke)
	router.With(s.requireUser).Get("/oauth/authorize", s.handleMCPOAuthAuthorize)
	router.With(s.requireUser).Post("/oauth/authorize", s.handleMCPOAuthAuthorize)
}

func (s *Server) handleMCPProtectedResourceMetadata(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(response, http.StatusOK, map[string]any{
		"resource":                 s.config.PublicBaseURL + "/mcp",
		"authorization_servers":    []string{s.config.PublicBaseURL},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         supportedMCPScopes,
	})
}

func (s *Server) handleMCPAuthorizationServerMetadata(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(response, http.StatusOK, map[string]any{
		"issuer":                                s.config.PublicBaseURL,
		"authorization_endpoint":                s.config.PublicBaseURL + "/oauth/authorize",
		"token_endpoint":                        s.config.PublicBaseURL + "/oauth/token",
		"registration_endpoint":                 s.config.PublicBaseURL + "/oauth/register",
		"revocation_endpoint":                   s.config.PublicBaseURL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      supportedMCPScopes,
		"resource_indicators_supported":         true,
		"client_id_metadata_document_supported": true,
	})
}

func (s *Server) handleMCPOAuthRegister(response http.ResponseWriter, request *http.Request) {
	if !s.config.MCPEnabled {
		writeOAuthError(response, http.StatusNotFound, "invalid_request", "remote MCP is disabled")
		return
	}
	var input oauthClientRegistrationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 64<<10))
	if err := decoder.Decode(&input); err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_client_metadata", "invalid client metadata")
		return
	}
	for _, rawURI := range input.RedirectURIs {
		if !validOAuthRedirectURI(rawURI) {
			writeOAuthError(response, http.StatusBadRequest, "invalid_redirect_uri", "redirect URI must use HTTPS or HTTP loopback")
			return
		}
	}
	if !validOAuthClientCapabilities(input.GrantTypes, input.ResponseTypes) {
		writeOAuthError(response, http.StatusBadRequest, "invalid_client_metadata", "only authorization_code, refresh_token and code are supported")
		return
	}
	if strings.TrimSpace(input.ClientName) == "" {
		input.ClientName = "MCP client"
	}
	client, err := s.store.RegisterMCPOAuthClient(
		request.Context(), input.ClientName, input.RedirectURIs, input.TokenEndpointAuthMethod,
	)
	if err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, http.StatusCreated, map[string]any{
		"client_id": client.ClientID, "client_name": client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": client.TokenEndpointAuthMethod,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"}, "scope": input.Scope,
		"client_id_issued_at": client.CreatedAt.Unix(),
	})
}

func (s *Server) handleMCPOAuthClientMetadata(response http.ResponseWriter, request *http.Request) {
	client, err := s.store.GetMCPOAuthClient(request.Context(), chi.URLParam(request, "clientID"))
	if err != nil {
		writeOAuthError(response, http.StatusNotFound, "invalid_client", "unknown client")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"client_id": client.ClientID, "client_name": client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": client.TokenEndpointAuthMethod,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"client_id_issued_at":        client.CreatedAt.Unix(),
	})
}

func validOAuthClientCapabilities(grantTypes, responseTypes []string) bool {
	for _, value := range grantTypes {
		if value != "authorization_code" && value != "refresh_token" {
			return false
		}
	}
	for _, value := range responseTypes {
		if value != "code" {
			return false
		}
	}
	return true
}

func validOAuthRedirectURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Fragment != "" || parsed.User != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	return parsed.Scheme == "http" && (host == "127.0.0.1" || host == "::1" || host == "localhost")
}

type oauthAuthorizationRequest struct {
	ClientID        string
	RedirectURI     string
	State           string
	Scope           string
	Resource        string
	Challenge       string
	ChallengeMethod string
	ResponseType    string
}

func authorizationRequest(request *http.Request) oauthAuthorizationRequest {
	values := request.URL.Query()
	if request.Method == http.MethodPost {
		_ = request.ParseForm()
		values = request.PostForm
	}
	return oauthAuthorizationRequest{
		ClientID: values.Get("client_id"), RedirectURI: values.Get("redirect_uri"),
		State: values.Get("state"), Scope: values.Get("scope"), Resource: values.Get("resource"),
		Challenge: values.Get("code_challenge"), ResponseType: values.Get("response_type"),
		ChallengeMethod: values.Get("code_challenge_method"),
	}
}

func (s *Server) validateAuthorizationRequest(
	request *http.Request,
	input oauthAuthorizationRequest,
) (store.MCPOAuthClient, []string, error) {
	client, err := s.resolveMCPOAuthClient(request, input.ClientID)
	if err != nil {
		return store.MCPOAuthClient{}, nil, err
	}
	if input.ResponseType != "code" || input.Challenge == "" || input.ChallengeMethod != "S256" ||
		!client.AllowsRedirectURI(input.RedirectURI) ||
		input.Resource != s.config.PublicBaseURL+"/mcp" {
		return store.MCPOAuthClient{}, nil, http.ErrNotSupported
	}
	scopes := strings.Fields(input.Scope)
	if len(scopes) == 0 {
		scopes = []string{"projects:list", "projects:read"}
	}
	for _, scope := range scopes {
		if !slices.Contains(supportedMCPScopes, scope) {
			return store.MCPOAuthClient{}, nil, http.ErrNotSupported
		}
	}
	if err := s.validateFositeAuthorizationRequest(request, input, client); err != nil {
		return store.MCPOAuthClient{}, nil, err
	}
	return client, scopes, nil
}

// validateFositeAuthorizationRequest delegates the OAuth protocol boundary to
// ORY Fosite. Writer's Studio keeps its grant and opaque-token persistence in
// PostgreSQL, while Fosite validates the public client, exact redirect URI,
// response type, requested scopes and mandatory S256 PKCE request.
func (s *Server) validateFositeAuthorizationRequest(
	request *http.Request,
	input oauthAuthorizationRequest,
	client store.MCPOAuthClient,
) error {
	secret := sha256.Sum256([]byte(s.config.MCPOAuthSecret))
	configuration := &fosite.Config{
		GlobalSecret:                   secret[:],
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		ScopeStrategy:                  fosite.ExactScopeStrategy,
		AudienceMatchingStrategy:       fosite.ExactAudienceMatchingStrategy,
		AccessTokenLifespan:            time.Hour,
		RefreshTokenLifespan:           30 * 24 * time.Hour,
		AuthorizeCodeLifespan:          10 * time.Minute,
	}
	memory := fositestorage.NewMemoryStore()
	memory.Clients[client.ClientID] = &fosite.DefaultClient{
		ID: client.ClientID, RedirectURIs: client.RedirectURIs,
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"}, Scopes: supportedMCPScopes,
		Audience: []string{s.config.PublicBaseURL + "/mcp"}, Public: true,
	}
	provider := compose.Compose(
		configuration,
		memory,
		compose.NewOAuth2HMACStrategy(configuration),
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2PKCEFactory,
	)
	values := url.Values{
		"client_id": {input.ClientID}, "redirect_uri": {input.RedirectURI},
		"state": {input.State}, "scope": {input.Scope}, "resource": {input.Resource},
		"response_type": {input.ResponseType}, "code_challenge": {input.Challenge},
		"code_challenge_method": {input.ChallengeMethod},
	}
	validationRequest, err := http.NewRequestWithContext(
		request.Context(), http.MethodGet,
		s.config.PublicBaseURL+"/oauth/authorize?"+values.Encode(), nil,
	)
	if err != nil {
		return err
	}
	authorizeRequest, err := provider.NewAuthorizeRequest(request.Context(), validationRequest)
	if err != nil {
		return err
	}
	for _, scope := range authorizeRequest.GetRequestedScopes() {
		authorizeRequest.GrantScope(scope)
	}
	_, err = provider.NewAuthorizeResponse(
		request.Context(), authorizeRequest, &fosite.DefaultSession{Subject: "mcp-consent"},
	)
	return err
}

func (s *Server) resolveMCPOAuthClient(
	request *http.Request,
	clientID string,
) (store.MCPOAuthClient, error) {
	parsed, err := url.Parse(clientID)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Fragment != "" || parsed.Path == "" || parsed.Path == "/" {
		return s.store.GetMCPOAuthClient(request.Context(), clientID)
	}
	metadataRequest, err := http.NewRequestWithContext(request.Context(), http.MethodGet, clientID, nil)
	if err != nil {
		return store.MCPOAuthClient{}, err
	}
	metadataRequest.Header.Set("Accept", "application/json")
	metadataResponse, err := s.mcpMetadataHTTPClient.Do(metadataRequest)
	if err != nil {
		return store.MCPOAuthClient{}, err
	}
	defer metadataResponse.Body.Close()
	if metadataResponse.StatusCode != http.StatusOK {
		return store.MCPOAuthClient{}, http.ErrNotSupported
	}
	encoded, err := io.ReadAll(io.LimitReader(metadataResponse.Body, (64<<10)+1))
	if err != nil || len(encoded) > 64<<10 {
		return store.MCPOAuthClient{}, http.ErrNotSupported
	}
	var metadata oauthClientMetadataDocument
	if err := json.Unmarshal(encoded, &metadata); err != nil || metadata.ClientID != clientID ||
		strings.TrimSpace(metadata.ClientName) == "" || len(metadata.RedirectURIs) == 0 ||
		(metadata.TokenEndpointAuthMethod != "" && metadata.TokenEndpointAuthMethod != "none") ||
		!validOAuthClientCapabilities(metadata.GrantTypes, metadata.ResponseTypes) {
		return store.MCPOAuthClient{}, http.ErrNotSupported
	}
	for _, redirectURI := range metadata.RedirectURIs {
		if !validOAuthRedirectURI(redirectURI) {
			return store.MCPOAuthClient{}, http.ErrNotSupported
		}
	}
	return s.store.SaveMCPOAuthMetadataClient(request.Context(), store.MCPOAuthClient{
		ClientID: metadata.ClientID, ClientName: metadata.ClientName,
		RedirectURIs:            metadata.RedirectURIs,
		TokenEndpointAuthMethod: "none",
	})
}

func (s *Server) handleMCPOAuthAuthorize(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	if !s.config.MCPEnabled {
		writeOAuthError(response, http.StatusNotFound, "invalid_request", "remote MCP is disabled")
		return
	}
	input := authorizationRequest(request)
	client, scopes, err := s.validateAuthorizationRequest(request, input)
	if err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_request", "invalid authorization request")
		return
	}
	user := currentUser(request.Context())
	if request.Method == http.MethodGet {
		projects, err := s.store.ListProjects(request.Context(), user.ID, "active")
		if err != nil {
			handleStoreError(response, err)
			return
		}
		archived, err := s.store.ListProjects(request.Context(), user.ID, "archived")
		if err == nil {
			projects = append(projects, archived...)
		}
		ticket := s.signOAuthConsent(user.ID, input)
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = mcpConsentTemplate.Execute(response, map[string]any{
			"Client": client, "Projects": projects, "Scopes": scopes,
			"Request": input, "Ticket": ticket,
		})
		return
	}
	if request.PostForm.Get("decision") != "approve" {
		redirectOAuth(response, request, input.RedirectURI, map[string]string{
			"error": "access_denied", "state": input.State,
		})
		return
	}
	if subtle.ConstantTimeCompare(
		[]byte(request.PostForm.Get("consent_ticket")),
		[]byte(s.signOAuthConsent(user.ID, input)),
	) != 1 {
		writeOAuthError(response, http.StatusBadRequest, "invalid_request", "consent request expired or changed")
		return
	}
	projectIDs := request.PostForm["project_id"]
	grant, _, err := s.store.CreateMCPGrant(request.Context(), user.ID, store.CreateMCPGrantInput{
		Name: "OAuth · " + client.ClientName, ClientName: client.ClientName,
		ProjectIDs: projectIDs, Scopes: scopes, AuthKind: "oauth",
	})
	if err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_scope", err.Error())
		return
	}
	code, err := s.store.CreateMCPOAuthCode(request.Context(), store.MCPOAuthCode{
		ClientID: client.ClientID, GrantID: grant.ID, RedirectURI: input.RedirectURI,
		CodeChallenge: input.Challenge, Resource: input.Resource, Scopes: scopes,
	})
	if err != nil {
		writeOAuthError(response, http.StatusInternalServerError, "server_error", "could not create authorization code")
		return
	}
	redirectOAuth(response, request, input.RedirectURI, map[string]string{
		"code": code, "state": input.State,
	})
}

func (s *Server) signOAuthConsent(userID string, input oauthAuthorizationRequest) string {
	mac := hmac.New(sha256.New, []byte(s.config.MCPOAuthSecret))
	_, _ = mac.Write([]byte(strings.Join([]string{
		userID, input.ClientID, input.RedirectURI, input.State, input.Scope,
		input.Resource, input.Challenge, input.ResponseType,
		input.ChallengeMethod,
	}, "\x00")))
	return hex.EncodeToString(mac.Sum(nil))
}

func redirectOAuth(response http.ResponseWriter, request *http.Request, redirectURI string, values map[string]string) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_request", "invalid redirect URI")
		return
	}
	query := target.Query()
	for key, value := range values {
		if value != "" {
			query.Set(key, value)
		}
	}
	target.RawQuery = query.Encode()
	http.Redirect(response, request, target.String(), http.StatusFound)
}

func (s *Server) handleMCPOAuthToken(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	if !s.config.MCPEnabled {
		writeOAuthError(response, http.StatusNotFound, "invalid_request", "remote MCP is disabled")
		return
	}
	if err := request.ParseForm(); err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_request", "invalid token request")
		return
	}
	clientID := request.Form.Get("client_id")
	if _, err := s.store.GetMCPOAuthClient(request.Context(), clientID); err != nil {
		writeOAuthError(response, http.StatusUnauthorized, "invalid_client", "unknown client")
		return
	}
	resource := request.Form.Get("resource")
	if resource != s.config.PublicBaseURL+"/mcp" {
		writeOAuthError(response, http.StatusBadRequest, "invalid_target", "resource must exactly match the MCP endpoint")
		return
	}
	var pair store.MCPOAuthTokenPair
	var err error
	switch request.Form.Get("grant_type") {
	case "authorization_code":
		pendingCode, lookupErr := s.store.GetMCPOAuthCode(
			request.Context(), request.Form.Get("code"), clientID,
			request.Form.Get("redirect_uri"), resource,
		)
		if lookupErr != nil || !validPKCE(request.Form.Get("code_verifier"), pendingCode.CodeChallenge) {
			writeOAuthError(response, http.StatusBadRequest, "invalid_grant", "authorization code or PKCE verifier is invalid")
			return
		}
		code, consumeErr := s.store.ConsumeMCPOAuthCode(
			request.Context(), request.Form.Get("code"), clientID,
			request.Form.Get("redirect_uri"), resource,
		)
		if consumeErr != nil {
			writeOAuthError(response, http.StatusBadRequest, "invalid_grant", "authorization code or PKCE verifier is invalid")
			return
		}
		pair, err = s.store.IssueMCPOAuthTokenPair(
			request.Context(), code.GrantID, clientID, resource, code.Scopes,
		)
	case "refresh_token":
		pair, err = s.store.RotateMCPOAuthRefreshToken(
			request.Context(), request.Form.Get("refresh_token"), clientID, resource,
		)
	default:
		writeOAuthError(response, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant type")
		return
	}
	if err != nil {
		writeOAuthError(response, http.StatusBadRequest, "invalid_grant", "grant is invalid or expired")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"access_token": pair.AccessToken, "token_type": "Bearer",
		"expires_in": pair.ExpiresIn, "refresh_token": pair.RefreshToken,
		"scope": strings.Join(pair.Scopes, " "),
	})
}

func validPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	digest := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

func (s *Server) handleMCPOAuthRevoke(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	if !s.config.MCPEnabled {
		writeOAuthError(response, http.StatusNotFound, "invalid_request", "remote MCP is disabled")
		return
	}
	_ = request.ParseForm()
	_ = s.store.RevokeMCPOAuthToken(request.Context(), request.Form.Get("token"))
	response.WriteHeader(http.StatusOK)
}

func writeOAuthError(response http.ResponseWriter, status int, code, description string) {
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, status, map[string]string{"error": code, "error_description": description})
}

var mcpConsentTemplate = template.Must(template.New("mcp-consent").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<meta name="referrer" content="no-referrer"><title>Доступ агента · Writer's Studio</title>
<style>body{font:16px system-ui;max-width:720px;margin:48px auto;padding:0 24px;color:#231f20}fieldset{border:1px solid #ddd;border-radius:12px;margin:20px 0;padding:16px}label{display:block;margin:10px 0}.actions{display:flex;gap:12px}button{padding:10px 18px;border-radius:8px;border:1px solid #999;background:#fff}.primary{background:#231f20;color:white}</style></head>
<body><h1>Разрешить доступ агенту?</h1><p><strong>{{.Client.ClientName}}</strong> запрашивает доступ к Writer's Studio.</p>
<form method="post" action="/oauth/authorize">
<input type="hidden" name="client_id" value="{{.Request.ClientID}}"><input type="hidden" name="redirect_uri" value="{{.Request.RedirectURI}}">
<input type="hidden" name="state" value="{{.Request.State}}"><input type="hidden" name="scope" value="{{.Request.Scope}}">
<input type="hidden" name="resource" value="{{.Request.Resource}}"><input type="hidden" name="code_challenge" value="{{.Request.Challenge}}">
<input type="hidden" name="code_challenge_method" value="{{.Request.ChallengeMethod}}">
<input type="hidden" name="response_type" value="{{.Request.ResponseType}}"><input type="hidden" name="consent_ticket" value="{{.Ticket}}">
<fieldset><legend>Проекты</legend>{{range .Projects}}<label><input type="checkbox" name="project_id" value="{{.ID}}"> {{.Title}} · {{.Status}}</label>{{end}}</fieldset>
<fieldset><legend>Права</legend>{{range .Scopes}}<label><input type="checkbox" checked disabled> {{.}}</label>{{end}}</fieldset>
<div class="actions"><button class="primary" name="decision" value="approve">Разрешить</button><button name="decision" value="deny">Отклонить</button></div>
</form></body></html>`))
