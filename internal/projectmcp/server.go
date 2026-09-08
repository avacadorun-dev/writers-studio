package projectmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ExecutionContext struct {
	OwnerUserID    string
	ProjectID      string
	RunID          string
	TurnID         string
	TargetNodeID   *string
	TargetCanvasID *string
	Instruction    string
	// Durable is set only for remote MCP turns. The built-in assistant keeps its
	// existing in-process lifecycle while remote staged calls are committed to a
	// recoverable draft before the result is returned to the client.
	Durable       bool
	ToolCallCount int
}

type Backend interface {
	Call(
		context.Context,
		ExecutionContext,
		string,
		map[string]any,
	) (map[string]any, error)
}

type Session struct {
	context       ExecutionContext
	backend       Backend
	client        *mcp.ClientSession
	serverSession *mcp.ServerSession
}

func NewSession(
	ctx context.Context,
	executionContext ExecutionContext,
	backend Backend,
) (*Session, error) {
	if backend == nil || executionContext.OwnerUserID == "" ||
		executionContext.ProjectID == "" || executionContext.RunID == "" ||
		executionContext.TurnID == "" {
		return nil, errors.New("invalid project MCP session configuration")
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "writers-studio-project", Version: "1.0.0",
	}, &mcp.ServerOptions{Instructions: "Project-scoped Writers Studio tools."})
	for _, tool := range PublicCatalog() {
		current := tool
		mcp.AddTool[map[string]any, map[string]any](
			server,
			&mcp.Tool{
				Name: current.Name, Title: current.Name, Description: current.Description,
				InputSchema: current.Schema,
			},
			func(
				callContext context.Context,
				_ *mcp.CallToolRequest,
				arguments map[string]any,
			) (*mcp.CallToolResult, map[string]any, error) {
				output, err := backend.Call(
					callContext,
					executionContext,
					current.Name,
					arguments,
				)
				return nil, output, err
			},
		)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect project MCP server: %w", err)
	}
	client := mcp.NewClient(&mcp.Implementation{
		Name: "writers-studio-orchestrator", Version: "1.0.0",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		return nil, fmt.Errorf("connect project MCP client: %w", err)
	}
	return &Session{
		context: executionContext, backend: backend,
		client: clientSession, serverSession: serverSession,
	}, nil
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var errorsSeen []error
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			errorsSeen = append(errorsSeen, err)
		}
	}
	if s.serverSession != nil {
		if err := s.serverSession.Close(); err != nil {
			errorsSeen = append(errorsSeen, err)
		}
	}
	return errors.Join(errorsSeen...)
}

func (s *Session) Call(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (map[string]any, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("project MCP session is closed")
	}
	var values map[string]any
	if len(arguments) == 0 {
		values = map[string]any{}
	} else if err := json.Unmarshal(arguments, &values); err != nil {
		return nil, fmt.Errorf("decode tool arguments: %w", err)
	}
	result, err := s.client.CallTool(ctx, &mcp.CallToolParams{
		Name: name, Arguments: values,
	})
	if err != nil {
		return nil, fmt.Errorf("call project tool %s: %w", name, err)
	}
	if result.IsError {
		message := "project tool failed"
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
				message = text.Text
				break
			}
		}
		return nil, errors.New(message)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, fmt.Errorf("encode project tool result: %w", err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		return nil, fmt.Errorf("decode project tool result: %w", err)
	}
	return output, nil
}

func (s *Session) ListTools(ctx context.Context) ([]string, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("project MCP session is closed")
	}
	var names []string
	for tool, err := range s.client.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		names = append(names, tool.Name)
	}
	return names, nil
}
