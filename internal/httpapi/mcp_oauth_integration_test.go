//go:build integration

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"writers.studio/internal/config"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/store"
)

func TestMCPOAuthPKCEDiscoveryDCRRefreshAndRevoke(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dataStore, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, loginToken, err := dataStore.Authenticate(
		ctx,
		"mcp-oauth-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
		Title: "OAuth project", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	publicURL := "http://" + listener.Addr().String()
	server := New(
		config.Config{
			AppEnv: "test", StaticDir: ".", PublicBaseURL: publicURL,
			LLMAdapter: "disabled", SessionTTL: time.Hour,
			MCPEnabled: true, MCPOAuthSecret: strings.Repeat("o", 64),
		},
		dataStore,
		objectstore.NewMemory(),
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() {
		if server.remoteMCP != nil {
			server.remoteMCP.Close()
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	})

	metadata := getJSONMap(t, ctx, http.DefaultClient, publicURL+"/.well-known/oauth-authorization-server")
	if metadata["client_id_metadata_document_supported"] != true ||
		metadata["registration_endpoint"] != publicURL+"/oauth/register" {
		t.Fatalf("authorization metadata is incomplete: %#v", metadata)
	}
	resourceMetadata := getJSONMap(t, ctx, http.DefaultClient, publicURL+"/.well-known/oauth-protected-resource")
	if resourceMetadata["resource"] != publicURL+"/mcp" {
		t.Fatalf("protected resource metadata has the wrong audience: %#v", resourceMetadata)
	}

	redirectURI := "http://127.0.0.1:39091/oauth/callback"
	var metadataClientID string
	metadataServer := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, map[string]any{
			"client_id": metadataClientID, "client_name": "CIMD integration agent",
			"redirect_uris":              []string{redirectURI},
			"token_endpoint_auth_method": "none",
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
		})
	}))
	defer metadataServer.Close()
	metadataClientID = metadataServer.URL + "/client-metadata.json"
	server.mcpMetadataHTTPClient = metadataServer.Client()
	cimdVerifier := strings.Repeat("c", 64)
	cimdDigest := sha256.Sum256([]byte(cimdVerifier))
	cimdAuthorization := url.Values{
		"client_id": {metadataClientID}, "redirect_uri": {redirectURI},
		"response_type": {"code"}, "state": {uuid.NewString() + uuid.NewString()},
		"scope": {"projects:list projects:read"}, "resource": {publicURL + "/mcp"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(cimdDigest[:])},
		"code_challenge_method": {"S256"},
	}
	cimdRequest, _ := http.NewRequestWithContext(
		ctx, http.MethodGet, publicURL+"/oauth/authorize?"+cimdAuthorization.Encode(), nil,
	)
	cimdRequest.Header.Set("Cookie", sessionCookieName+"="+loginToken)
	cimdResponse, err := http.DefaultClient.Do(cimdRequest)
	if err != nil {
		t.Fatal(err)
	}
	cimdResponse.Body.Close()
	if cimdResponse.StatusCode != http.StatusOK {
		t.Fatalf("Client ID Metadata Document authorization failed: %d", cimdResponse.StatusCode)
	}
	if persistedClient, err := dataStore.GetMCPOAuthClient(ctx, metadataClientID); err != nil ||
		persistedClient.ClientName != "CIMD integration agent" {
		t.Fatalf("validated CIMD client was not persisted: %#v %v", persistedClient, err)
	}

	registrationBody := strings.NewReader(`{
		"client_name":"Integration OAuth agent",
		"redirect_uris":["` + redirectURI + `"],
		"token_endpoint_auth_method":"none",
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"]
	}`)
	registrationRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, publicURL+"/oauth/register", registrationBody)
	registrationRequest.Header.Set("Content-Type", "application/json")
	registrationResponse, err := http.DefaultClient.Do(registrationRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer registrationResponse.Body.Close()
	if registrationResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(registrationResponse.Body)
		t.Fatalf("DCR failed: %d %s", registrationResponse.StatusCode, body)
	}
	var registration map[string]any
	if err := json.NewDecoder(registrationResponse.Body).Decode(&registration); err != nil {
		t.Fatal(err)
	}
	clientID, _ := registration["client_id"].(string)
	if clientID == "" {
		t.Fatalf("DCR returned no client id: %#v", registration)
	}

	verifier := strings.Repeat("v", 64)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	state := uuid.NewString() + uuid.NewString()
	authorization := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI},
		"response_type": {"code"}, "state": {state},
		"scope":    {"projects:list projects:read projects:stage projects:apply projects:actions"},
		"resource": {publicURL + "/mcp"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"},
	}
	consentRequest, _ := http.NewRequestWithContext(
		ctx, http.MethodGet, publicURL+"/oauth/authorize?"+authorization.Encode(), nil,
	)
	consentRequest.Header.Set("Cookie", sessionCookieName+"="+loginToken)
	consentResponse, err := http.DefaultClient.Do(consentRequest)
	if err != nil {
		t.Fatal(err)
	}
	consentHTML, _ := io.ReadAll(consentResponse.Body)
	consentResponse.Body.Close()
	if consentResponse.StatusCode != http.StatusOK {
		t.Fatalf("authorize GET failed: %d %s", consentResponse.StatusCode, consentHTML)
	}
	match := regexp.MustCompile(`name="consent_ticket" value="([^"]+)"`).FindSubmatch(consentHTML)
	if len(match) != 2 {
		t.Fatal("OAuth consent did not contain a signed consent ticket")
	}
	authorization.Set("consent_ticket", string(match[1]))
	authorization.Set("decision", "approve")
	authorization.Add("project_id", workspace.Project.ID)
	redirectClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	approveRequest, _ := http.NewRequestWithContext(
		ctx, http.MethodPost, publicURL+"/oauth/authorize", strings.NewReader(authorization.Encode()),
	)
	approveRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	approveRequest.Header.Set("Cookie", sessionCookieName+"="+loginToken)
	approveResponse, err := redirectClient.Do(approveRequest)
	if err != nil {
		t.Fatal(err)
	}
	approveResponse.Body.Close()
	if approveResponse.StatusCode != http.StatusFound {
		t.Fatalf("authorize approval failed: %d", approveResponse.StatusCode)
	}
	callback, err := url.Parse(approveResponse.Header.Get("Location"))
	if err != nil || callback.Query().Get("state") != state || callback.Query().Get("code") == "" {
		t.Fatalf("invalid OAuth callback: %s", approveResponse.Header.Get("Location"))
	}

	invalidTokenRequest, _ := http.NewRequestWithContext(
		ctx, http.MethodPost, publicURL+"/oauth/token",
		strings.NewReader(url.Values{
			"grant_type": {"authorization_code"}, "client_id": {clientID},
			"redirect_uri": {redirectURI}, "resource": {publicURL + "/mcp"},
			"code": {callback.Query().Get("code")}, "code_verifier": {strings.Repeat("x", 64)},
		}.Encode()),
	)
	invalidTokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalidTokenResponse, err := http.DefaultClient.Do(invalidTokenRequest)
	if err != nil {
		t.Fatal(err)
	}
	invalidTokenResponse.Body.Close()
	if invalidTokenResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong PKCE verifier returned %d", invalidTokenResponse.StatusCode)
	}

	tokens := postOAuthForm(t, ctx, publicURL+"/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID},
		"redirect_uri": {redirectURI}, "resource": {publicURL + "/mcp"},
		"code": {callback.Query().Get("code")}, "code_verifier": {verifier},
	})
	accessToken, _ := tokens["access_token"].(string)
	refreshToken, _ := tokens["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" || tokens["expires_in"] != float64(3600) {
		t.Fatalf("token response is incomplete: %#v", tokens)
	}
	verified, err := dataStore.VerifyMCPBearer(ctx, accessToken, publicURL+"/mcp")
	if err != nil || !verified.AllowsProject(workspace.Project.ID) {
		t.Fatalf("OAuth access token did not preserve grant scope: %#v %v", verified, err)
	}
	rotated := postOAuthForm(t, ctx, publicURL+"/oauth/token", url.Values{
		"grant_type": {"refresh_token"}, "client_id": {clientID},
		"resource": {publicURL + "/mcp"}, "refresh_token": {refreshToken},
	})
	rotatedAccess, _ := rotated["access_token"].(string)
	if rotatedAccess == "" || rotatedAccess == accessToken {
		t.Fatalf("refresh rotation did not issue a new token: %#v", rotated)
	}
	revokeRequest, _ := http.NewRequestWithContext(
		ctx, http.MethodPost, publicURL+"/oauth/revoke",
		strings.NewReader(url.Values{"token": {rotatedAccess}}.Encode()),
	)
	revokeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeResponse, err := http.DefaultClient.Do(revokeRequest)
	if err != nil {
		t.Fatal(err)
	}
	revokeResponse.Body.Close()
	if revokeResponse.StatusCode != http.StatusOK {
		t.Fatalf("token revocation failed: %d", revokeResponse.StatusCode)
	}
	if _, err := dataStore.VerifyMCPBearer(ctx, rotatedAccess, publicURL+"/mcp"); err == nil {
		t.Fatal("revoked OAuth token remained valid")
	}
}

func getJSONMap(t *testing.T, ctx context.Context, client *http.Client, endpoint string) map[string]any {
	t.Helper()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d", endpoint, response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func postOAuthForm(t *testing.T, ctx context.Context, endpoint string, form url.Values) map[string]any {
	t.Helper()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s returned %d: %s", endpoint, response.StatusCode, body)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
