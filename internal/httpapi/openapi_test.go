package httpapi

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"writers.studio/internal/config"
)

func TestOpenAPICoversEveryAPIRoute(t *testing.T) {
	server := &Server{
		config: config.Config{StaticDir: t.TempDir()},
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	actual := map[string]bool{}
	err := chi.Walk(server.routes(), func(
		method string,
		route string,
		_ http.Handler,
		_ ...func(http.Handler) http.Handler,
	) error {
		if strings.HasPrefix(route, "/api/") {
			actual[method+" "+strings.TrimSuffix(route, "/")] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	operations := openAPIOperations()
	if len(operations) != 124 {
		t.Fatalf("operation count=%d, want 124", len(operations))
	}
	expected := map[string]bool{}
	operationIDs := map[string]bool{}
	for _, operation := range operations {
		key := operation.Method + " " + operation.Path
		if expected[key] {
			t.Fatalf("duplicate operation %s", key)
		}
		if operationIDs[operation.OperationID] {
			t.Fatalf("duplicate operationId %s", operation.OperationID)
		}
		expected[key] = true
		operationIDs[operation.OperationID] = true
	}
	for key := range actual {
		if !expected[key] {
			t.Errorf("API route is missing from OpenAPI: %s", key)
		}
	}
	for key := range expected {
		if !actual[key] {
			t.Errorf("OpenAPI operation has no API route: %s", key)
		}
	}
}

func TestOpenAPIDocumentHasResolvableSchemaReferences(t *testing.T) {
	document := openAPIDocument()
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi version=%v", document["openapi"])
	}
	components, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatal("components are missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("component schemas are missing")
	}
	walkJSON(document, func(value any) {
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		reference, ok := object["$ref"].(string)
		if !ok || !strings.HasPrefix(reference, "#/components/schemas/") {
			return
		}
		name := strings.TrimPrefix(reference, "#/components/schemas/")
		if _, exists := schemas[name]; !exists {
			t.Errorf("unresolved schema reference %s", reference)
		}
	})

	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAPIHandlersServeJSONAndSPA(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(staticDir, "index.html"),
		[]byte("<!doctype html><title>API docs</title>"),
		fs.FileMode(0o644),
	); err != nil {
		t.Fatal(err)
	}
	server := &Server{config: config.Config{StaticDir: staticDir}}

	t.Run("specification", func(t *testing.T) {
		response := httptest.NewRecorder()
		server.handleOpenAPI(response, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
			t.Fatalf("content type=%q", contentType)
		}
		if !strings.Contains(response.Body.String(), `"openapi":"3.1.0"`) {
			t.Fatal("response is not an OpenAPI 3.1 document")
		}
	})

	t.Run("swagger UI shell", func(t *testing.T) {
		response := httptest.NewRecorder()
		server.handleOpenAPIDocs(response, httptest.NewRequest(http.MethodGet, "/api/docs", nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "API docs") {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Fatalf("content type=%q", contentType)
		}
	})
}

func walkJSON(value any, visit func(any)) {
	visit(value)
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	}
}
