package projectmcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"writers.studio/internal/assistant"
)

type fakeBackend struct {
	calls []string
	fail  string
}

func (b *fakeBackend) Call(
	_ context.Context,
	executionContext ExecutionContext,
	name string,
	arguments map[string]any,
) (map[string]any, error) {
	if executionContext.ProjectID != "project" || executionContext.OwnerUserID != "owner" {
		return nil, errors.New("scope was not pinned")
	}
	b.calls = append(b.calls, name)
	if name == b.fail {
		return nil, errors.New("safe tool error")
	}
	return map[string]any{"tool": name, "arguments": arguments}, nil
}

func TestMCPServerRegistersAndCallsCompleteCatalog(t *testing.T) {
	t.Parallel()
	backend := &fakeBackend{}
	session, err := NewSession(context.Background(), ExecutionContext{
		OwnerUserID: "owner", ProjectID: "project", RunID: "run", TurnID: "turn",
	}, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	names, err := session.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(PublicCatalog()) || !slices.Contains(names, "document.append") ||
		!slices.Contains(names, "project_memory.add") {
		t.Fatalf("incomplete MCP catalog: %d tools", len(names))
	}
	result, err := session.Call(
		context.Background(),
		"project.get_overview",
		json.RawMessage(`{}`),
	)
	if err != nil || result["tool"] != "project.get_overview" ||
		len(backend.calls) != 1 {
		t.Fatalf("unexpected tool call: %+v, %v", result, err)
	}
}

func TestMCPServerValidatesStrictArgumentsAndErrors(t *testing.T) {
	t.Parallel()
	backend := &fakeBackend{fail: "project.get_overview"}
	session, err := NewSession(context.Background(), ExecutionContext{
		OwnerUserID: "owner", ProjectID: "project", RunID: "run", TurnID: "turn",
	}, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Call(
		context.Background(),
		"tree.get_node",
		json.RawMessage(`{"unknown":"value"}`),
	); err == nil {
		t.Fatal("additional property passed strict MCP validation")
	}
	if _, err := session.Call(
		context.Background(),
		"tree.get_node",
		json.RawMessage(`{}`),
	); err == nil {
		t.Fatal("missing required property passed strict MCP validation")
	}
	if _, err := session.Call(
		context.Background(),
		"project.get_overview",
		json.RawMessage(`{}`),
	); err == nil || !strings.Contains(err.Error(), "safe tool error") {
		t.Fatalf("tool error not surfaced to model: %v", err)
	}
	if _, err := session.Call(
		context.Background(),
		"missing.tool",
		json.RawMessage(`{}`),
	); err == nil {
		t.Fatal("unknown tool was accepted")
	}
	if _, err := session.Call(
		context.Background(),
		"project.get_overview",
		json.RawMessage(`bad`),
	); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

func TestCatalogMatchesPolicyAndStrictSchema(t *testing.T) {
	t.Parallel()
	catalog := PublicCatalog()
	if len(catalog) != 69 {
		t.Fatalf("public Assistant MCP baseline changed: got %d tools, want 69", len(catalog))
	}
	allowed := assistant.AllowedToolNames()
	if len(catalog) != len(allowed) {
		t.Fatalf("catalog size %d differs from policy %d", len(catalog), len(allowed))
	}
	seen := map[string]bool{}
	for _, tool := range catalog {
		if seen[tool.Name] {
			t.Fatalf("duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Schema["additionalProperties"] != false {
			t.Fatalf("tool %q is not strict", tool.Name)
		}
		properties := tool.Schema["properties"].(map[string]any)
		required := tool.Schema["required"].([]string)
		if len(properties) != len(required) {
			t.Fatalf("tool %q has optional non-nullable properties", tool.Name)
		}
		if ToolPolicy(tool.Name).Authority == assistant.AuthorityNeverExposed {
			t.Fatalf("tool %q has no assistant policy", tool.Name)
		}
		assertStrictOpenAIObjectSchemas(t, tool.Name, tool.Schema, "$")
	}
	for _, name := range allowed {
		if !seen[name] {
			t.Fatalf("policy tool %q missing from MCP catalog", name)
		}
	}
	definitions := Definitions(catalog[:2])
	if len(definitions) != 2 || definitions[0].Name != catalog[0].Name {
		t.Fatal("LLM tool definitions drifted from MCP catalog")
	}
}

func TestRemoteCatalogIsStrictSupersetOfInternalCatalog(t *testing.T) {
	t.Parallel()
	internal := PublicCatalog()
	remote := RemoteCatalog()
	if len(internal) != 69 {
		t.Fatalf("internal tool baseline changed: got %d", len(internal))
	}
	if len(remote) <= len(internal) {
		t.Fatalf("remote catalog is not a strict superset: internal=%d remote=%d", len(internal), len(remote))
	}
	remoteByName := make(map[string]ToolSpec, len(remote))
	for _, tool := range remote {
		if _, exists := remoteByName[tool.Name]; exists {
			t.Fatalf("duplicate remote tool %q", tool.Name)
		}
		remoteByName[tool.Name] = tool
		assertStrictOpenAIObjectSchemas(t, tool.Name, tool.Schema, "$")
	}
	for _, tool := range internal {
		remoteTool, exists := remoteByName[tool.Name]
		if !exists {
			t.Fatalf("internal tool %q is missing remotely", tool.Name)
		}
		internalProperties, _ := tool.Schema["properties"].(map[string]any)
		remoteProperties, _ := remoteTool.Schema["properties"].(map[string]any)
		if tool.Description != remoteTool.Description || len(internalProperties) != len(remoteProperties) {
			t.Fatalf("internal tool %q changed in remote catalog", tool.Name)
		}
	}
	for _, required := range []string{
		"workspace.list_projects", "workspace.open_project", "workspace.get_context",
		"workspace.close_project", "assistant.finish_turn", "assistant.discard_turn",
		"change_set.list", "change_set.get", "change_set.review", "change_set.apply",
		"change_set.reject", "change_set.undo", "tree.archive", "version.restore",
		"transcript.retranscribe", "export.create", "export.get",
	} {
		if _, exists := remoteByName[required]; !exists {
			t.Fatalf("remote lifecycle tool %q is missing", required)
		}
	}
}

func assertStrictOpenAIObjectSchemas(
	t *testing.T,
	toolName string,
	value any,
	path string,
) {
	t.Helper()
	schema, ok := value.(map[string]any)
	if !ok {
		return
	}
	if schema["type"] == "object" {
		if schema["additionalProperties"] != false {
			t.Fatalf("tool %q has a non-strict object at %s", toolName, path)
		}
		properties, propertiesOK := schema["properties"].(map[string]any)
		required, requiredOK := schema["required"].([]string)
		if !propertiesOK || !requiredOK || len(properties) != len(required) {
			t.Fatalf("tool %q has optional object properties at %s", toolName, path)
		}
		for name := range properties {
			if !slices.Contains(required, name) {
				t.Fatalf("tool %q does not require %s.%s", toolName, path, name)
			}
		}
	}
	for name, nested := range schema {
		switch typed := nested.(type) {
		case map[string]any:
			assertStrictOpenAIObjectSchemas(t, toolName, typed, path+"."+name)
		case []map[string]any:
			for index, item := range typed {
				assertStrictOpenAIObjectSchemas(
					t,
					toolName,
					item,
					path+"."+name+"["+strconv.Itoa(index)+"]",
				)
			}
		case []any:
			for index, item := range typed {
				assertStrictOpenAIObjectSchemas(
					t,
					toolName,
					item,
					path+"."+name+"["+strconv.Itoa(index)+"]",
				)
			}
		}
	}
}

func TestCompleteCatalogCoversEveryAssistantCapabilityWithoutLexicalRouting(t *testing.T) {
	t.Parallel()
	expected := []string{
		"assistant.get_recent_change_set",
		"project.get_structure",
		"tree.create_node",
		"tree.create_nodes_batch",
		"document.get",
		"document.find_exact",
		"document.find_exact_batch",
		"document.replace_range",
		"annotation.list",
		"annotation.get",
		"annotation.propose_fixes",
		"document.prepare_replace_all",
		"document.prepare_transfer",
		"dictionary.add",
		"dictionary.add_batch",
		"project_memory.add",
		"project_memory.add_batch",
		"assistant_guide.propose_update",
		"version.prepare_restore",
		"transcript.prepare_transfer",
		"export.prepare",
	}
	catalog := PublicCatalog()
	names := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		names = append(names, tool.Name)
	}
	for _, name := range expected {
		if !slices.Contains(names, name) {
			t.Fatalf("complete assistant catalog is missing %q", name)
		}
	}
	if len(Definitions(catalog)) != len(catalog) {
		t.Fatal("the model tool surface drifted from the complete MCP catalog")
	}
}

func TestSessionRejectsInvalidConfigurationAndClosedUse(t *testing.T) {
	t.Parallel()
	if _, err := NewSession(context.Background(), ExecutionContext{}, nil); err == nil {
		t.Fatal("invalid session configuration accepted")
	}
	var session *Session
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Call(context.Background(), "x", nil); err == nil {
		t.Fatal("closed session call succeeded")
	}
	if _, err := session.ListTools(context.Background()); err == nil {
		t.Fatal("closed session list succeeded")
	}
}
