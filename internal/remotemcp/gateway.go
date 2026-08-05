package remotemcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"writers.studio/internal/assistant"
	"writers.studio/internal/assistantapp"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/projectmcp"
	"writers.studio/internal/store"
)

const (
	SessionIdleTimeout     = 30 * time.Minute
	MaxSessionsPerGrant    = 5
	MaxProjectCallsPerTurn = 24
)

type Gateway struct {
	store       *store.Store
	backend     *assistantapp.ProjectBackend
	objects     objectstore.Store
	endpoint    string
	publicURL   *url.URL
	logger      *slog.Logger
	server      *mcp.Server
	handler     http.Handler
	mu          sync.Mutex
	sessions    map[string]*sessionState
	stopJanitor chan struct{}
}

type sessionState struct {
	mu            sync.Mutex
	ID            string
	GrantID       string
	GrantVersion  time.Time
	ClientName    string
	ProjectID     string
	ProjectTitle  string
	ProjectStatus string
	TargetNodeID  *string
	Run           *assistant.Run
	Turn          *assistant.Turn
	Execution     *projectmcp.ExecutionContext
	TurnMode      string
	ToolCalls     int
	LastUsed      time.Time
}

func New(
	dataStore *store.Store,
	backend *assistantapp.ProjectBackend,
	objects objectstore.Store,
	publicBaseURL string,
	logger *slog.Logger,
) (*Gateway, error) {
	if dataStore == nil || backend == nil || objects == nil {
		return nil, errors.New("remote MCP requires a store and project backend")
	}
	publicURL, err := url.Parse(strings.TrimRight(publicBaseURL, "/"))
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		return nil, errors.New("remote MCP requires an absolute PUBLIC_BASE_URL")
	}
	gateway := &Gateway{
		store: dataStore, backend: backend, objects: objects, endpoint: publicURL.String() + "/mcp",
		publicURL: publicURL, logger: logger, sessions: make(map[string]*sessionState),
		stopJanitor: make(chan struct{}),
	}
	gateway.server = gateway.buildServer()
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return gateway.server },
		&mcp.StreamableHTTPOptions{
			JSONResponse: true, SessionTimeout: SessionIdleTimeout,
			DisableLocalhostProtection: false,
		},
	)
	verifier := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		grant, err := dataStore.VerifyMCPBearer(ctx, token, gateway.endpoint)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid or expired MCP access token", auth.ErrInvalidToken)
		}
		expiration := time.Now().UTC().Add(time.Hour)
		if grant.TokenExpiresAt != nil {
			expiration = *grant.TokenExpiresAt
		}
		return &auth.TokenInfo{
			Scopes: grant.Scopes, Expiration: expiration, UserID: grant.ID,
			Extra: map[string]any{"grant": grant},
		}, nil
	}
	authenticated := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: publicURL.String() + "/.well-known/oauth-protected-resource",
	})(gateway.observeDelete(streamable))
	gateway.handler = gateway.protectTransport(authenticated)
	go gateway.janitor()
	return gateway, nil
}

func (g *Gateway) Handler() http.Handler { return g.handler }

func (g *Gateway) Close() {
	select {
	case <-g.stopJanitor:
	default:
		close(g.stopJanitor)
	}
}

func (g *Gateway) buildServer() *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "writers-studio", Title: "Writer's Studio", Version: "1.0.0"},
		&mcp.ServerOptions{
			Instructions:       "Open an authorized project, begin a turn, use project tools, then finish or discard the turn. Review tokens enable agent-authorized Apply, Reject and Undo without opening the web UI.",
			InitializedHandler: g.onInitialized,
		},
	)
	for _, tool := range projectmcp.RemoteCatalog() {
		current := tool
		mcp.AddTool[map[string]any, map[string]any](
			server,
			&mcp.Tool{
				Name: current.Name, Title: current.Name, Description: current.Description,
				InputSchema: current.Schema, Annotations: remoteAnnotations(current.Name),
			},
			func(ctx context.Context, request *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
				output, err := g.call(ctx, request, current.Name, input)
				return nil, output, err
			},
		)
	}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "writers-studio://exports/{id}", Name: "writers-studio-export",
		Title: "Writer's Studio export", Description: "Authorized export artifact created by this MCP grant.",
	}, g.readExportResource)
	return server
}

func remoteAnnotations(name string) *mcp.ToolAnnotations {
	readOnly := false
	destructive := false
	idempotent := false
	switch name {
	case "workspace.list_projects", "workspace.get_context", "change_set.list",
		"change_set.get", "change_set.review", "export.get":
		readOnly = true
	case "change_set.apply", "change_set.reject", "change_set.undo":
		destructive = true
	case "transcript.retranscribe", "export.create":
		idempotent = true
	case "assistant.begin_turn", "assistant.finish_turn", "assistant.discard_turn":
		readOnly = false
	case "workspace.open_project", "workspace.close_project":
		idempotent = true
	default:
		policy := assistant.ClassifyTool(name)
		readOnly = policy.Authority == assistant.AuthorityRead ||
			policy.Authority == assistant.AuthorityNativeOnly
	}
	closedWorld := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint: readOnly, DestructiveHint: &destructive,
		IdempotentHint: idempotent, OpenWorldHint: &closedWorld,
	}
}

func (g *Gateway) onInitialized(_ context.Context, request *mcp.InitializedRequest) {
	if request == nil || request.Session == nil {
		return
	}
	grant, err := grantFromExtra(request.Extra)
	if err != nil {
		return
	}
	clientName := grant.ClientName
	if params := request.Session.InitializeParams(); params != nil && params.ClientInfo != nil {
		if value := strings.TrimSpace(params.ClientInfo.Title); value != "" {
			clientName = value
		} else if value := strings.TrimSpace(params.ClientInfo.Name); value != "" {
			clientName = value
		}
	}
	_, _ = g.session(request.Session.ID(), grant, clientName)
}

func (g *Gateway) call(
	ctx context.Context,
	request *mcp.CallToolRequest,
	name string,
	input map[string]any,
) (map[string]any, error) {
	started := time.Now()
	grant, err := grantFromExtra(request.Extra)
	if err != nil {
		return nil, err
	}
	clientName := grant.ClientName
	if params := request.Session.InitializeParams(); params != nil && params.ClientInfo != nil {
		if value := strings.TrimSpace(params.ClientInfo.Title); value != "" {
			clientName = value
		} else if value := strings.TrimSpace(params.ClientInfo.Name); value != "" {
			clientName = value
		}
	}
	state, err := g.session(request.Session.ID(), grant, clientName)
	if err != nil {
		return nil, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.LastUsed = time.Now().UTC()
	status := "ok"
	affectedIDs := make([]string, 0, 2)
	if state.ProjectID != "" {
		affectedIDs = append(affectedIDs, state.ProjectID)
	}
	if state.TargetNodeID != nil {
		affectedIDs = append(affectedIDs, *state.TargetNodeID)
	}
	var operationID *string
	var changeSetID *string
	defer func() {
		g.store.RecordMCPAudit(
			context.WithoutCancel(ctx), grant.ID, state.ClientName, state.ID,
			name, status, time.Since(started), affectedIDs, operationID, changeSetID,
		)
	}()
	output, err := g.callLocked(ctx, state, grant, name, input)
	if err != nil {
		status = "error"
		return nil, err
	}
	if value, ok := output["changeSetId"].(string); ok && value != "" {
		changeSetID = &value
	}
	if value, ok := output["operationId"].(string); ok && value != "" {
		operationID = &value
	}
	if changeSet, ok := output["changeSet"].(assistant.ChangeSet); ok && changeSet.OperationID != nil {
		operationID = changeSet.OperationID
	}
	if changeSet, ok := output["changeSet"].(*assistant.ChangeSet); ok &&
		changeSet != nil && changeSet.OperationID != nil {
		operationID = changeSet.OperationID
	}
	return output, nil
}

func grantFromExtra(extra *mcp.RequestExtra) (store.MCPGrant, error) {
	if extra == nil || extra.TokenInfo == nil || extra.TokenInfo.Extra == nil {
		return store.MCPGrant{}, errors.New("authenticated MCP grant is missing")
	}
	grant, ok := extra.TokenInfo.Extra["grant"].(store.MCPGrant)
	if !ok || grant.ID == "" || grant.RevokedAt != nil {
		return store.MCPGrant{}, errors.New("authenticated MCP grant is invalid")
	}
	return grant, nil
}

func (g *Gateway) session(id string, grant store.MCPGrant, clientName string) (*sessionState, error) {
	if id == "" {
		return nil, errors.New("stateful MCP session id is missing")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if state := g.sessions[id]; state != nil {
		if state.GrantID != grant.ID {
			return nil, errors.New("MCP session belongs to another grant")
		}
		if !state.GrantVersion.Equal(grant.UpdatedAt) {
			return nil, errors.New("MCP grant credentials changed; reconnect the session")
		}
		return state, nil
	}
	active := 0
	for _, state := range g.sessions {
		if state.GrantID == grant.ID && state.GrantVersion.Equal(grant.UpdatedAt) {
			active++
		}
	}
	if active >= MaxSessionsPerGrant {
		return nil, errors.New("MCP grant already has five active sessions")
	}
	state := &sessionState{
		ID: id, GrantID: grant.ID, GrantVersion: grant.UpdatedAt,
		ClientName: clientName, LastUsed: time.Now().UTC(),
	}
	g.sessions[id] = state
	return state, nil
}

func (g *Gateway) callLocked(
	ctx context.Context,
	state *sessionState,
	grant store.MCPGrant,
	name string,
	input map[string]any,
) (map[string]any, error) {
	switch name {
	case "workspace.list_projects":
		status := strings.TrimSpace(stringValue(input, "status"))
		projects, err := g.store.ListMCPGrantProjects(ctx, grant, status)
		if err != nil {
			return nil, err
		}
		return map[string]any{"projects": projects}, nil
	case "workspace.open_project":
		if state.Turn != nil {
			return nil, errors.New("finish or discard the active turn before switching projects")
		}
		projectID := stringValue(input, "projectId")
		targetNodeID := nullableStringValue(input, "targetNodeId")
		workspace, err := g.store.OpenMCPProject(ctx, grant, projectID, targetNodeID)
		if err != nil {
			return nil, err
		}
		state.ProjectID = workspace.Project.ID
		state.ProjectTitle = workspace.Project.Title
		state.ProjectStatus = workspace.Project.Status
		state.TargetNodeID = targetNodeID
		return map[string]any{
			"opened": true, "project": workspace.Project,
			"targetNodeId": targetNodeID, "readOnly": workspace.Project.Status == "archived",
		}, nil
	case "workspace.get_context":
		return g.contextOutput(state, grant), nil
	case "workspace.close_project":
		if state.Turn != nil {
			return nil, errors.New("finish or discard the active turn before closing the project")
		}
		state.ProjectID, state.ProjectTitle, state.ProjectStatus = "", "", ""
		state.TargetNodeID = nil
		return map[string]any{"closed": true}, nil
	case "assistant.begin_turn":
		return g.beginTurn(ctx, state, grant, input)
	case "assistant.finish_turn":
		return g.finishTurn(ctx, state, input, false)
	case "assistant.discard_turn":
		return g.discardTurn(ctx, state, stringValue(input, "reason"))
	case "change_set.list":
		if !grant.HasScope("projects:read") {
			return nil, errors.New("MCP grant lacks projects:read")
		}
		projectID := stringValue(input, "projectId")
		if projectID != "" && !grant.AllowsProject(projectID) {
			return nil, pgx.ErrNoRows
		}
		changeSets, err := g.store.ListMCPChangeSets(ctx, grant, projectID, intValue(input, "limit"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"changeSets": changeSets}, nil
	case "change_set.get":
		if !grant.HasScope("projects:read") {
			return nil, errors.New("MCP grant lacks projects:read")
		}
		changeSet, err := g.store.GetMCPChangeSet(ctx, grant, stringValue(input, "changeSetId"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"changeSet": changeSet}, nil
	case "change_set.review":
		return g.reviewChangeSet(ctx, grant, input)
	case "change_set.apply":
		return g.applyChangeSet(ctx, grant, input)
	case "change_set.reject":
		return g.rejectChangeSet(ctx, grant, input)
	case "change_set.undo":
		return g.undoChangeSet(ctx, grant, input)
	case "transcript.retranscribe":
		return g.retranscribe(ctx, state, grant, input)
	case "export.create":
		return g.createExport(ctx, state, grant, input)
	case "export.get":
		return g.getExport(ctx, state, grant, stringValue(input, "exportId"))
	}
	if state.ProjectID == "" {
		return nil, errors.New("open a project before calling project tools")
	}
	if state.Turn == nil || state.Execution == nil {
		return nil, errors.New("assistant.begin_turn is required before project tools")
	}
	policy := assistant.ClassifyTool(name)
	if policy.Authority == assistant.AuthorityNeverExposed {
		return nil, errors.New("tool is outside the remote MCP catalog")
	}
	if policy.Authority == assistant.AuthorityRead {
		if !grant.HasScope("projects:read") {
			return nil, errors.New("MCP grant lacks projects:read")
		}
	} else if name == "transcript.prepare_retranscribe" || name == "export.prepare" {
		if state.TurnMode != "change" {
			return nil, errors.New("project actions require a change-mode turn")
		}
		if !grant.HasScope("projects:actions") {
			return nil, errors.New("MCP grant lacks projects:actions")
		}
		if state.ProjectStatus == "archived" {
			return nil, errors.New("archived projects are read-only over MCP")
		}
	} else {
		if state.TurnMode != "change" {
			return nil, errors.New("staged project tools require a change-mode turn")
		}
		if !grant.HasScope("projects:stage") {
			return nil, errors.New("MCP grant lacks projects:stage")
		}
		if state.ProjectStatus == "archived" {
			return nil, errors.New("archived projects are read-only over MCP")
		}
	}
	state.ToolCalls++
	if state.ToolCalls > MaxProjectCallsPerTurn {
		return nil, errors.New("remote MCP turn exceeded 24 project tool calls")
	}
	state.Execution.ToolCallCount = state.ToolCalls
	if name == "tree.archive" || name == "version.restore" {
		return g.backend.StageRemoteHighRisk(ctx, *state.Execution, name, input)
	}
	output, err := g.backend.Call(ctx, *state.Execution, name, input)
	if err != nil {
		return nil, err
	}
	if name == "transcript.prepare_retranscribe" || name == "export.prepare" {
		actionKind := "retranscribe"
		payload := map[string]any{
			"projectId": state.ProjectID, "sessionId": stringValue(input, "sessionId"),
		}
		if name == "export.prepare" {
			actionKind = "export"
			payload = map[string]any{
				"projectId": state.ProjectID, "format": stringValue(input, "format"),
				"nodeIds":       stringSliceValue(input, "nodeIds"),
				"chapterTitles": boolValue(input, "chapterTitles"),
			}
		}
		token, err := g.store.IssueMCPActionToken(ctx, grant, actionKind, payload)
		if err != nil {
			return nil, err
		}
		output["actionToken"] = token
		output["expiresInSeconds"] = 600
	}
	return output, nil
}

func (g *Gateway) retranscribe(
	ctx context.Context,
	state *sessionState,
	grant store.MCPGrant,
	input map[string]any,
) (map[string]any, error) {
	if !grant.HasScope("projects:actions") {
		return nil, errors.New("MCP grant lacks projects:actions")
	}
	if err := consumeRemoteActionCall(state); err != nil {
		return nil, err
	}
	if state.ProjectID == "" || state.ProjectStatus == "archived" {
		return nil, errors.New("open an active project before retranscription")
	}
	secret := stringValue(input, "actionToken")
	payload, first, err := g.store.ConsumeMCPActionToken(ctx, grant, "retranscribe", secret)
	if err != nil {
		return nil, err
	}
	sessionID := stringValue(input, "sessionId")
	if stringValue(payload, "projectId") != state.ProjectID || stringValue(payload, "sessionId") != sessionID {
		return nil, errors.New("action token is bound to another retranscription")
	}
	if result, ok := payload["result"].(map[string]any); ok {
		result["replayed"] = true
		return result, nil
	}
	if !first {
		return nil, errors.New("retranscription action is already in progress")
	}
	requestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(grant.ID+":"+secret)).String()
	if err := g.store.QueueRetranscription(ctx, grant.OwnerUserID, sessionID, requestID); err != nil {
		return nil, err
	}
	result := map[string]any{"status": "queued", "requestId": requestID, "sessionId": sessionID}
	if err := g.store.SaveMCPActionResult(ctx, grant, secret, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Gateway) createExport(
	ctx context.Context,
	state *sessionState,
	grant store.MCPGrant,
	input map[string]any,
) (map[string]any, error) {
	if !grant.HasScope("projects:actions") {
		return nil, errors.New("MCP grant lacks projects:actions")
	}
	if err := consumeRemoteActionCall(state); err != nil {
		return nil, err
	}
	if state.ProjectID == "" || state.ProjectStatus == "archived" {
		return nil, errors.New("open an active project before creating an export")
	}
	secret := stringValue(input, "actionToken")
	payload, first, err := g.store.ConsumeMCPActionToken(ctx, grant, "export", secret)
	if err != nil {
		return nil, err
	}
	if stringValue(payload, "projectId") != state.ProjectID {
		return nil, errors.New("action token is bound to another project")
	}
	if result, ok := payload["result"].(map[string]any); ok {
		exportID := stringValue(result, "exportId")
		return g.getExport(ctx, state, grant, exportID)
	}
	if !first {
		return nil, errors.New("export action is already in progress")
	}
	job, err := g.store.CreateExport(ctx, grant.OwnerUserID, state.ProjectID, store.CreateExportInput{
		Format: stringValue(payload, "format"), NodeIDs: stringSliceValue(payload, "nodeIds"),
		ChapterTitles: boolValue(payload, "chapterTitles"),
	})
	if err != nil {
		return nil, err
	}
	if err := g.store.SaveMCPActionResult(ctx, grant, secret, map[string]any{"exportId": job.ID}); err != nil {
		return nil, err
	}
	return map[string]any{
		"export": job, "exportId": job.ID, "resourceUri": "writers-studio://exports/" + job.ID,
	}, nil
}

func (g *Gateway) getExport(
	ctx context.Context,
	state *sessionState,
	grant store.MCPGrant,
	exportID string,
) (map[string]any, error) {
	if !grant.HasScope("projects:read") {
		return nil, errors.New("MCP grant lacks projects:read")
	}
	if state.ProjectID == "" || !grant.AllowsProject(state.ProjectID) {
		return nil, errors.New("open the export project first")
	}
	job, err := g.store.GetExport(ctx, grant.OwnerUserID, exportID)
	if err != nil || job.ProjectID != state.ProjectID {
		return nil, pgx.ErrNoRows
	}
	output := map[string]any{"export": job, "exportId": job.ID}
	if job.Status == "ready" && job.StorageKey != nil {
		output["resourceUri"] = "writers-studio://exports/" + job.ID
	}
	return output, nil
}

func (g *Gateway) readExportResource(
	ctx context.Context,
	request *mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	grant, err := grantFromExtra(request.Extra)
	if err != nil {
		return nil, err
	}
	if !grant.HasScope("projects:read") {
		return nil, errors.New("MCP grant lacks projects:read")
	}
	parsed, err := url.Parse(request.Params.URI)
	if err != nil || parsed.Scheme != "writers-studio" || parsed.Host != "exports" {
		return nil, mcp.ResourceNotFoundError(request.Params.URI)
	}
	exportID := strings.TrimPrefix(parsed.Path, "/")
	job, err := g.store.GetExport(ctx, grant.OwnerUserID, exportID)
	if err != nil || !grant.AllowsProject(job.ProjectID) || job.Status != "ready" || job.StorageKey == nil {
		return nil, mcp.ResourceNotFoundError(request.Params.URI)
	}
	data, err := g.objects.Get(ctx, *job.StorageKey)
	if err != nil {
		return nil, err
	}
	mimeType := map[string]string{
		"docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"markdown": "text/markdown; charset=utf-8", "txt": "text/plain; charset=utf-8",
	}[job.Format]
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI: request.Params.URI, MIMEType: mimeType, Blob: data,
	}}}, nil
}

func (g *Gateway) beginTurn(
	ctx context.Context,
	state *sessionState,
	grant store.MCPGrant,
	input map[string]any,
) (map[string]any, error) {
	if state.ProjectID == "" {
		return nil, errors.New("open a project before beginning a turn")
	}
	if state.Turn != nil {
		return nil, errors.New("this MCP session already has an active turn")
	}
	mode := stringValue(input, "mode")
	if mode != "answer" && mode != "change" && mode != "clarify" {
		return nil, errors.New("assistant turn mode is invalid")
	}
	if mode == "change" && (!grant.HasScope("projects:stage") || state.ProjectStatus == "archived") {
		return nil, errors.New("this project or grant is read-only")
	}
	summary := strings.TrimSpace(stringValue(input, "summary"))
	if summary == "" {
		return nil, errors.New("assistant turn summary is required")
	}
	targetKind := assistant.TargetProject
	if state.TargetNodeID != nil {
		targetKind = assistant.TargetNode
	}
	now := time.Now().UTC()
	run, turn, err := g.store.CreateAssistantRunWithFirstTurn(ctx, assistant.Run{
		OwnerUserID: grant.OwnerUserID, ProjectID: state.ProjectID, Title: summary,
		TargetKind: targetKind, TargetNodeID: state.TargetNodeID, InputMode: assistant.InputMCP,
		Status: assistant.RunWorking, SilenceCheckpointMS: 0,
		Origin: assistant.RunOriginRemoteMCP,
		Agent:  &assistant.AgentRef{GrantID: grant.ID, ClientName: state.ClientName},
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderMCP, Name: state.ClientName, Model: "remote-agent",
			Dialect: assistant.DialectMCP,
			Capabilities: assistant.CapabilitySnapshot{
				Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
			},
		},
		ReadContext: map[string]any{"origin": "remote_mcp", "turnMode": mode},
	}, assistant.InputMCP, summary, nil)
	if err != nil {
		return nil, err
	}
	turn.State, turn.ConfirmedAt, turn.StartedAt = assistant.TurnWorking, &now, &now
	if err := g.store.UpdateAssistantTurn(ctx, turn); err != nil {
		return nil, err
	}
	execution := &projectmcp.ExecutionContext{
		OwnerUserID: grant.OwnerUserID, ProjectID: state.ProjectID, RunID: run.ID,
		TurnID: turn.ID, TargetNodeID: state.TargetNodeID, Instruction: summary, Durable: true,
	}
	state.Run, state.Turn, state.Execution, state.TurnMode, state.ToolCalls = &run, &turn, execution, mode, 0
	return map[string]any{
		"accepted": true, "mode": mode, "summary": summary,
		"runId": run.ID, "turnId": turn.ID, "durable": true,
	}, nil
}

func (g *Gateway) finishTurn(
	ctx context.Context,
	state *sessionState,
	input map[string]any,
	partial bool,
) (map[string]any, error) {
	if state.Turn == nil || state.Run == nil || state.Execution == nil {
		return nil, errors.New("there is no active remote MCP turn")
	}
	title := strings.TrimSpace(stringValue(input, "title"))
	summary := strings.TrimSpace(stringValue(input, "summary"))
	if title == "" {
		title = "Изменения внешнего агента"
	}
	if partial {
		title = "Частичные изменения внешнего агента"
	}
	changeSet, err := g.backend.FinalizeChangeSet(ctx, *state.Execution, title, summary)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	turn := *state.Turn
	turn.CompletedAt = &now
	turn.AssistantText = summary
	runStatus := assistant.RunCompleted
	if changeSet != nil {
		turn.State = assistant.TurnAwaitingReview
		runStatus = assistant.RunAwaitingReview
	} else {
		turn.State = assistant.TurnCompleted
	}
	if err := g.store.UpdateAssistantTurn(ctx, turn); err != nil {
		return nil, err
	}
	if err := g.store.UpdateAssistantRunStatus(ctx, state.Run.OwnerUserID, state.Run.ID, runStatus, nil); err != nil {
		return nil, err
	}
	runID, turnID := state.Run.ID, state.Turn.ID
	state.Run, state.Turn, state.Execution, state.TurnMode, state.ToolCalls = nil, nil, nil, "", 0
	output := map[string]any{
		"finished": true, "partial": partial, "runId": runID, "turnId": turnID,
		"changeSet": changeSet,
	}
	if changeSet != nil {
		output["changeSetId"] = changeSet.ID
	}
	return output, nil
}

func (g *Gateway) discardTurn(ctx context.Context, state *sessionState, reason string) (map[string]any, error) {
	if state.Turn == nil || state.Run == nil {
		return nil, errors.New("there is no active remote MCP turn")
	}
	if err := g.backend.DiscardPendingPersistent(ctx, state.Turn.ID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	turn := *state.Turn
	turn.State, turn.CompletedAt, turn.AssistantText = assistant.TurnCancelled, &now, strings.TrimSpace(reason)
	if err := g.store.UpdateAssistantTurn(ctx, turn); err != nil {
		return nil, err
	}
	code := "remote_mcp_discarded"
	if err := g.store.UpdateAssistantRunStatus(
		ctx, state.Run.OwnerUserID, state.Run.ID, assistant.RunCancelled, &code,
	); err != nil {
		return nil, err
	}
	runID, turnID := state.Run.ID, state.Turn.ID
	state.Run, state.Turn, state.Execution, state.TurnMode, state.ToolCalls = nil, nil, nil, "", 0
	return map[string]any{"discarded": true, "runId": runID, "turnId": turnID}, nil
}

func consumeRemoteActionCall(state *sessionState) error {
	if state.Turn == nil || state.Execution == nil {
		return errors.New("assistant.begin_turn is required before project actions")
	}
	if state.TurnMode != "change" {
		return errors.New("project actions require a change-mode turn")
	}
	state.ToolCalls++
	if state.ToolCalls > MaxProjectCallsPerTurn {
		return errors.New("remote MCP turn exceeded 24 project tool calls")
	}
	state.Execution.ToolCallCount = state.ToolCalls
	return nil
}

func (g *Gateway) reviewChangeSet(ctx context.Context, grant store.MCPGrant, input map[string]any) (map[string]any, error) {
	if !grant.HasScope("projects:apply") {
		return nil, errors.New("MCP grant lacks projects:apply")
	}
	action := stringValue(input, "action")
	state := assistant.ChangeStaged
	if action == "undo" {
		state = assistant.ChangeApplied
	} else if action != "apply" && action != "reject" {
		return nil, errors.New("invalid change-set review action")
	}
	if action != "reject" {
		if err := g.store.EnsureMCPChangeSetProjectWritable(
			ctx, grant, stringValue(input, "changeSetId"),
		); err != nil {
			return nil, err
		}
	}
	changeSet, token, err := g.store.IssueMCPReviewToken(
		ctx, grant, stringValue(input, "changeSetId"), state,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"changeSet": changeSet, "reviewToken": token,
		"expiresInSeconds": 600, "action": action,
	}, nil
}

func (g *Gateway) applyChangeSet(ctx context.Context, grant store.MCPGrant, input map[string]any) (map[string]any, error) {
	if !grant.HasScope("projects:apply") {
		return nil, errors.New("MCP grant lacks projects:apply")
	}
	changeSetID := stringValue(input, "changeSetId")
	selectedGroups := stringSliceValue(input, "selectedGroups")
	resolutions := make(map[string]bool)
	if values, ok := input["annotationResolutions"].([]any); ok {
		for _, value := range values {
			entry, _ := value.(map[string]any)
			annotationID := stringValue(entry, "annotationId")
			resolved, valid := entry["resolved"].(bool)
			if annotationID != "" && valid {
				resolutions[annotationID] = resolved
			}
		}
	}
	changeSet, err := g.store.ApplyMCPChangeSet(
		ctx, grant, changeSetID, stringValue(input, "reviewToken"),
		selectedGroups, resolutions,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{"applied": true, "changeSetId": changeSet.ID, "changeSet": changeSet}, nil
}

func (g *Gateway) rejectChangeSet(ctx context.Context, grant store.MCPGrant, input map[string]any) (map[string]any, error) {
	if !grant.HasScope("projects:apply") {
		return nil, errors.New("MCP grant lacks projects:apply")
	}
	changeSetID := stringValue(input, "changeSetId")
	changeSet, err := g.store.RejectMCPChangeSet(
		ctx, grant, changeSetID, stringValue(input, "reviewToken"),
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rejected": true, "changeSetId": changeSet.ID, "changeSet": changeSet}, nil
}

func (g *Gateway) undoChangeSet(ctx context.Context, grant store.MCPGrant, input map[string]any) (map[string]any, error) {
	if !grant.HasScope("projects:apply") {
		return nil, errors.New("MCP grant lacks projects:apply")
	}
	changeSetID := stringValue(input, "changeSetId")
	changeSet, err := g.store.UndoMCPChangeSet(
		ctx, grant, changeSetID, stringValue(input, "reviewToken"),
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{"undone": true, "changeSetId": changeSet.ID, "changeSet": changeSet}, nil
}

func (g *Gateway) contextOutput(state *sessionState, grant store.MCPGrant) map[string]any {
	var runID, turnID string
	if state.Run != nil {
		runID = state.Run.ID
	}
	if state.Turn != nil {
		turnID = state.Turn.ID
	}
	return map[string]any{
		"grantId": grant.ID, "clientName": state.ClientName, "scopes": grant.Scopes,
		"projectId": state.ProjectID, "projectTitle": state.ProjectTitle,
		"projectStatus": state.ProjectStatus, "targetNodeId": state.TargetNodeID,
		"runId": runID, "turnId": turnID, "toolCalls": state.ToolCalls,
		"toolCallLimit": MaxProjectCallsPerTurn,
	}
}

func (g *Gateway) protectTransport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !strings.EqualFold(request.Host, g.publicURL.Host) {
			http.Error(response, "invalid MCP Host", http.StatusForbidden)
			return
		}
		if origin := request.Header.Get("Origin"); origin != "" &&
			!strings.EqualFold(strings.TrimRight(origin, "/"), g.publicURL.Scheme+"://"+g.publicURL.Host) {
			http.Error(response, "invalid MCP Origin", http.StatusForbidden)
			return
		}
		_ = http.NewResponseController(response).SetWriteDeadline(time.Time{})
		next.ServeHTTP(response, request)
	})
}

func (g *Gateway) observeDelete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			grantID := ""
			if tokenInfo := auth.TokenInfoFromContext(request.Context()); tokenInfo != nil {
				grantID = tokenInfo.UserID
			}
			g.closeSession(request.Context(), request.Header.Get("Mcp-Session-Id"), grantID, true)
		}
		next.ServeHTTP(response, request)
	})
}

func (g *Gateway) closeSession(ctx context.Context, sessionID, grantID string, partial bool) {
	g.mu.Lock()
	state := g.sessions[sessionID]
	if state != nil && (grantID == "" || state.GrantID == grantID) {
		delete(g.sessions, sessionID)
	} else {
		state = nil
	}
	g.mu.Unlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.Turn == nil {
		return
	}
	if g.backend.PendingCount(state.Turn.ID) > 0 {
		_, _ = g.finishTurn(context.WithoutCancel(ctx), state, map[string]any{
			"summary": "MCP session ended before the agent explicitly finished the turn.",
		}, partial)
	} else {
		_, _ = g.discardTurn(context.WithoutCancel(ctx), state, "MCP session ended")
	}
}

func (g *Gateway) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().UTC().Add(-SessionIdleTimeout)
			g.mu.Lock()
			var expired []string
			for id, state := range g.sessions {
				state.mu.Lock()
				idle := state.LastUsed.Before(cutoff)
				state.mu.Unlock()
				if idle {
					expired = append(expired, id)
				}
			}
			g.mu.Unlock()
			for _, id := range expired {
				g.closeSession(context.Background(), id, "", true)
			}
		case <-g.stopJanitor:
			return
		}
	}
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func nullableStringValue(values map[string]any, key string) *string {
	value := stringValue(values, key)
	if value == "" {
		return nil
	}
	return &value
}

func intValue(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func stringSliceValue(values map[string]any, key string) []string {
	switch items := values[key].(type) {
	case []string:
		return append([]string(nil), items...)
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				result = append(result, strings.TrimSpace(value))
			}
		}
		return result
	default:
		return nil
	}
}

func boolValue(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}
