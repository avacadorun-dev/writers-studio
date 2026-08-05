//go:build integration

package remotemcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"writers.studio/internal/assistantapp"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/projectmcp"
	"writers.studio/internal/store"
)

func TestRemoteMCPAgentOnlyApplyAndUndo(t *testing.T) {
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
	user, _, err := dataStore.Authenticate(
		ctx,
		"remote-mcp-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	emptyGrants, err := dataStore.ListMCPGrants(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if emptyGrants == nil || len(emptyGrants) != 0 {
		t.Fatalf("empty MCP grant list must serialize as an empty array: %#v", emptyGrants)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=$1", user.ID)
	})
	workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
		Title: "Remote MCP integration", Kind: "novel", Language: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	node := workspace.Nodes[0]
	originalContent := append([]byte(nil), node.Content...)
	originalRevision := node.CurrentRevision
	grant, secret, err := dataStore.CreateMCPGrant(ctx, user.ID, store.CreateMCPGrantInput{
		Name: "Integration grant", ClientName: "Official Go MCP client",
		ProjectIDs: []string{workspace.Project.ID},
		Scopes: []string{
			"projects:list", "projects:read", "projects:stage", "projects:apply", "projects:actions",
		},
		ExpiresIn: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	publicURL := "http://" + listener.Addr().String()
	gateway, err := New(
		dataStore,
		assistantapp.NewProjectBackend(dataStore),
		objectstore.NewMemory(),
		publicURL,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gateway.Close)
	httpServer := &http.Server{Handler: gateway.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	})

	client := mcp.NewClient(&mcp.Implementation{
		Name: "writers-studio-integration", Title: "Integration agent", Version: "1.0.0",
	}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: publicURL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{
			token: secret, base: http.DefaultTransport,
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if session.ID() == "" {
		t.Fatal("stateful MCP session did not receive a session ID")
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != len(projectmcp.RemoteCatalog()) {
		t.Fatalf("remote catalog has %d tools, want %d", len(listed.Tools), len(projectmcp.RemoteCatalog()))
	}

	callRemoteTool(t, ctx, session, "workspace.open_project", map[string]any{
		"projectId": workspace.Project.ID, "targetNodeId": node.ID,
	})
	callRemoteTool(t, ctx, session, "assistant.begin_turn", map[string]any{
		"mode": "change", "summary": "Добавить один абзац через удалённый MCP",
	})
	callRemoteTool(t, ctx, session, "document.append", map[string]any{
		"nodeId": node.ID, "baseRevision": originalRevision,
		"content": map[string]any{
			"blocks": []any{map[string]any{
				"type": "paragraph", "text": "Изменение от внешнего агента.",
			}},
		},
	})
	finished := callRemoteTool(t, ctx, session, "assistant.finish_turn", map[string]any{
		"title": "Удалённая правка", "summary": "Добавлен проверочный абзац.",
	})
	changeSetID := mapString(finished, "changeSetId")
	if changeSetID == "" {
		t.Fatalf("finish_turn returned no changeSetId: %#v", finished)
	}
	staged, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if staged.CurrentRevision != originalRevision || !jsonEquivalent(staged.Content, originalContent) {
		t.Fatal("staged tool changed canonical content before Apply")
	}

	review := callRemoteTool(t, ctx, session, "change_set.review", map[string]any{
		"changeSetId": changeSetID, "action": "apply",
	})
	callRemoteTool(t, ctx, session, "change_set.apply", map[string]any{
		"changeSetId": changeSetID, "reviewToken": mapString(review, "reviewToken"),
		"selectedGroups": []any{}, "annotationResolutions": []any{},
	})
	applied, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.CurrentRevision != originalRevision+1 || bytes.Equal(applied.Content, originalContent) {
		t.Fatalf("Apply did not create the expected revision: %#v", applied)
	}

	undoReview := callRemoteTool(t, ctx, session, "change_set.review", map[string]any{
		"changeSetId": changeSetID, "action": "undo",
	})
	callRemoteTool(t, ctx, session, "change_set.undo", map[string]any{
		"changeSetId": changeSetID, "reviewToken": mapString(undoReview, "reviewToken"),
	})
	undone, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.CurrentRevision != originalRevision+2 || !jsonEquivalent(undone.Content, originalContent) {
		t.Fatalf("Undo did not restore byte-equivalent document JSON: %s != %s", undone.Content, originalContent)
	}

	if _, err := dataStore.VerifyMCPBearer(ctx, secret, publicURL+"/mcp"); err != nil {
		t.Fatalf("manual token unexpectedly invalid before revocation: %v", err)
	}
	if err := dataStore.RevokeMCPGrant(ctx, user.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(ctx, nil)
	if err == nil && result != nil {
		t.Fatal("active MCP session remained authorized after grant revocation")
	}
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}

func callRemoteTool(
	t *testing.T,
	ctx context.Context,
	session *mcp.ClientSession,
	name string,
	arguments map[string]any,
) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s transport error: %v", name, err)
	}
	if result.IsError {
		message := fmt.Sprint(result.Content)
		if len(result.Content) > 0 {
			if content, ok := result.Content[0].(*mcp.TextContent); ok {
				message = content.Text
			}
		}
		t.Fatalf("%s failed: %s", name, message)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("%s structured result: %v", name, err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("%s decode result: %v", name, err)
	}
	return output
}

func mapString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func jsonEquivalent(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}
