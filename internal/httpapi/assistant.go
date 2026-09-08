package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/assistantapp"
	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
	"writers.studio/internal/llm"
	"writers.studio/internal/store"
)

type assistantExecutionManager struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newAssistantExecutionManager() *assistantExecutionManager {
	return &assistantExecutionManager{cancels: make(map[string]context.CancelFunc)}
}

func (m *assistantExecutionManager) start(
	runID string,
	work func(context.Context),
) bool {
	m.mu.Lock()
	if _, exists := m.cancels[runID]; exists {
		m.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[runID] = cancel
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.cancels, runID)
			m.mu.Unlock()
			cancel()
		}()
		work(ctx)
	}()
	return true
}

func (m *assistantExecutionManager) stop(runID string) bool {
	m.mu.Lock()
	cancel, exists := m.cancels[runID]
	m.mu.Unlock()
	if exists {
		cancel()
	}
	return exists
}

func (m *assistantExecutionManager) isRunning(runID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.cancels[runID]
	return exists
}

func (s *Server) mountAssistantRoutes(router chi.Router) {
	router.Get("/api/assistant/capabilities", s.handleAssistantCapabilities)
	router.Get("/api/assistant/providers", s.handleListAssistantProviders)
	router.Post("/api/assistant/providers", s.handleCreateAssistantProvider)
	router.Post("/api/assistant/providers/system/probe", s.handleProbeSystemAssistantProvider)
	router.Post("/api/assistant/providers/{connectionID}/probe", s.handleProbeAssistantProvider)
	router.Post("/api/assistant/providers/{connectionID}/default", s.handleDefaultAssistantProvider)
	router.Delete("/api/assistant/providers/{connectionID}", s.handleDisableAssistantProvider)

	router.Get("/api/projects/{projectID}/structure", s.handleGetProjectStructure)
	router.Put("/api/projects/{projectID}/structure", s.handleSaveProjectStructure)
	router.Get("/api/projects/{projectID}/assistant-guide", s.handleGetAssistantGuide)
	router.Put("/api/projects/{projectID}/assistant-guide", s.handleSaveAssistantGuide)
	router.Post(
		"/api/projects/{projectID}/assistant-guide/revisions/{revision}/restore",
		s.handleRestoreAssistantGuide,
	)
	router.Get("/api/projects/{projectID}/memory", s.handleListProjectMemory)
	router.Post("/api/projects/{projectID}/memory", s.handleCreateProjectMemory)
	router.Get(
		"/api/projects/{projectID}/memory/{memoryID}/revisions",
		s.handleListProjectMemoryRevisions,
	)
	router.Post(
		"/api/projects/{projectID}/memory/{memoryID}/revisions/{revision}/restore",
		s.handleRestoreProjectMemory,
	)
	router.Put("/api/projects/{projectID}/memory/{memoryID}", s.handleUpdateProjectMemory)
	router.Delete("/api/projects/{projectID}/memory/{memoryID}", s.handleDeactivateProjectMemory)

	router.Get("/api/projects/{projectID}/assistant/runs", s.handleListAssistantRuns)
	router.Post("/api/projects/{projectID}/assistant/runs", s.handleCreateAssistantRun)
	router.Get("/api/assistant/runs/{runID}", s.handleGetAssistantRun)
	router.Post("/api/assistant/runs/{runID}/view", s.handleMarkAssistantRunViewed)
	router.Delete("/api/assistant/runs/{runID}", s.handleDeleteAssistantRunHistory)
	router.Get("/api/assistant/runs/{runID}/turns", s.handleListAssistantTurns)
	router.Post("/api/assistant/runs/{runID}/turns", s.handleCreateAssistantTurn)
	router.Put("/api/assistant/runs/{runID}/turns/{turnID}", s.handleConfirmAssistantTurn)
	router.Post("/api/assistant/runs/{runID}/turns/{turnID}/execute", s.handleExecuteAssistantTurn)
	router.Post("/api/assistant/runs/{runID}/stop", s.handleStopAssistantRun)
	router.Get("/api/assistant/runs/{runID}/events", s.handleAssistantEvents)
	router.Get(
		"/api/assistant/runs/{runID}/change-sets/{changeSetID}",
		s.handleGetAssistantChangeSet,
	)
	router.Get("/api/assistant/runs/{runID}/voice", s.handleAssistantVoiceWebSocket)
	router.Post(
		"/api/assistant/runs/{runID}/change-sets/{changeSetID}/apply",
		s.handleApplyAssistantChangeSet,
	)
	router.Post(
		"/api/assistant/runs/{runID}/change-sets/{changeSetID}/reject",
		s.handleRejectAssistantChangeSet,
	)
	router.Post(
		"/api/assistant/runs/{runID}/change-sets/{changeSetID}/stale",
		s.handleMarkAssistantChangeSetStale,
	)
	router.Post(
		"/api/assistant/runs/{runID}/change-sets/{changeSetID}/undo",
		s.handleUndoAssistantChangeSet,
	)
}

func (s *Server) handleAssistantCapabilities(
	response http.ResponseWriter,
	request *http.Request,
) {
	system, report, systemConfigured := assistant.ProviderRef{}, llm.ProbeReport{}, false
	if s.assistantProviders != nil {
		system, report, systemConfigured = s.assistantProviders.SystemProviderReport(
			request.Context(),
			true,
		)
	}
	systemAvailable := systemConfigured && report.Capabilities.CanRunAssistant()
	voiceAvailable := false
	switch s.config.LiveSTTAdapter {
	case "deepgram":
		voiceAvailable = s.liveSTT != nil
	case "openrouter", "fake":
		voiceAvailable = s.transcriber.Capabilities(request.Context()).Available
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"voiceAvailable":             voiceAvailable,
		"chatAvailable":              s.assistantOrchestrator != nil,
		"systemProvider":             system,
		"systemProviderAvailable":    systemAvailable,
		"systemProviderConfigured":   systemConfigured,
		"systemProviderStatus":       report.Status,
		"systemProviderStatusDetail": report.Detail,
		"customProvidersEnabled": s.assistantProviders != nil &&
			s.assistantProviders.ConnectionsEnabled(),
		"changeReviewRequired": true,
		"partialApply":         true,
		"groupedUndo":          true,
	})
}

func (s *Server) handleListAssistantProviders(
	response http.ResponseWriter,
	request *http.Request,
) {
	user := currentUser(request.Context())
	connections, err := s.store.ListProviderConnections(request.Context(), user.ID)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	var system *assistant.ProviderRef
	var systemStatus string
	var systemStatusDetail string
	if s.assistantProviders != nil {
		if value, report, ok := s.assistantProviders.SystemProviderReport(
			request.Context(),
			true,
		); ok {
			system = &value
			systemStatus = report.Status
			systemStatusDetail = report.Detail
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"system": system, "connections": connections,
		"systemStatus": systemStatus, "systemStatusDetail": systemStatusDetail,
		"connectionsEnabled": s.assistantProviders != nil &&
			s.assistantProviders.ConnectionsEnabled(),
	})
}

func (s *Server) handleProbeSystemAssistantProvider(
	response http.ResponseWriter,
	request *http.Request,
) {
	if s.assistantProviders == nil {
		writeError(response, http.StatusServiceUnavailable, "provider_unavailable", "Системный провайдер не настроен")
		return
	}
	reference, report, configured := s.assistantProviders.RefreshSystemProviderReport(
		request.Context(),
	)
	if !configured {
		writeError(response, http.StatusServiceUnavailable, "provider_unavailable", "Системный провайдер не настроен")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"system": reference, "status": report.Status, "statusDetail": report.Detail,
	})
}

type createProviderRequest struct {
	Name    string                    `json:"name"`
	BaseURL string                    `json:"baseUrl"`
	Dialect assistant.ProviderDialect `json:"dialect"`
	Model   string                    `json:"model"`
	APIKey  string                    `json:"apiKey"`
	Headers map[string]string         `json:"headers"`
	// Kept for backwards-compatible request decoding. The server deliberately
	// ignores per-connection values and applies the global LLM timeout.
	RequestTimeoutMS     *int `json:"requestTimeoutMs,omitempty"`
	ContextWindow        *int `json:"contextWindow"`
	AllowPrivateEndpoint bool `json:"allowPrivateEndpoint"`
	IsDefault            bool `json:"isDefault"`
}

func (s *Server) handleCreateAssistantProvider(
	response http.ResponseWriter,
	request *http.Request,
) {
	if s.assistantProviders == nil || !s.assistantProviders.ConnectionsEnabled() {
		writeError(
			response,
			http.StatusServiceUnavailable,
			"provider_connections_disabled",
			"Подключение своих моделей пока недоступно: оператору нужно настроить ключ шифрования",
		)
		return
	}
	var input createProviderRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Model = strings.TrimSpace(input.Model)
	baseURL, err := assistantapp.NormalizeProviderBaseURL(strings.TrimSpace(input.BaseURL))
	if err != nil || input.Name == "" || input.Model == "" ||
		(input.Dialect != assistant.DialectAuto &&
			input.Dialect != assistant.DialectResponses &&
			input.Dialect != assistant.DialectChatCompletions) {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_provider",
			"Проверьте адрес, модель и диалект",
		)
		return
	}
	headers, err := safeProviderHeaders(input.Headers)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_headers", err.Error())
		return
	}
	connectionID := uuid.NewString()
	secret, err := s.assistantProviders.EncryptCredential(connectionID, input.APIKey)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_credential", "API-ключ не указан")
		return
	}
	encryptedHeaders, headerNonce, err := s.assistantProviders.EncryptHeaders(
		connectionID,
		headers,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	record := store.ProviderConnectionRecord{
		ID: connectionID, OwnerUserID: currentUser(request.Context()).ID,
		Name: input.Name, BaseURL: baseURL, Dialect: input.Dialect, Model: input.Model,
		EncryptedAPIKey: secret.Ciphertext, APIKeyNonce: secret.Nonce,
		APIKeyFingerprint: secret.Fingerprint, APIKeyLast4: secret.Last4,
		EncryptedHeaders: encryptedHeaders, HeadersNonce: headerNonce,
		RequestTimeoutMS: s.assistantProviders.RequestTimeoutMS(), ContextWindow: input.ContextWindow,
		AllowPrivateEndpoint: input.AllowPrivateEndpoint, IsDefault: input.IsDefault,
		Status: "checking",
	}
	report, dialect, err := s.assistantProviders.ProbeConnection(
		request.Context(),
		record,
		input.APIKey,
		headers,
	)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "provider_unreachable", "Не удалось безопасно подключиться к провайдеру")
		return
	}
	record.Dialect = dialect
	record.Status = report.Status
	record.StatusDetail = report.Detail
	record.Capabilities = report.Capabilities
	now := time.Now()
	record.CheckedAt = &now
	created, err := s.store.CreateProviderConnection(request.Context(), record)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"connection": created})
}

func safeProviderHeaders(input map[string]string) (map[string]string, error) {
	if len(input) > 20 {
		return nil, errors.New("Можно добавить не более 20 заголовков")
	}
	blocked := map[string]bool{
		"authorization": true, "cookie": true, "host": true, "connection": true,
		"content-length": true, "transfer-encoding": true, "proxy-authorization": true,
	}
	result := make(map[string]string, len(input))
	for name, value := range input {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" || len(name) > 100 || len(value) > 2_000 ||
			blocked[strings.ToLower(name)] || strings.ContainsAny(name, "\r\n:") ||
			strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("Недопустимый дополнительный заголовок")
		}
		result[name] = value
	}
	return result, nil
}

func (s *Server) handleProbeAssistantProvider(
	response http.ResponseWriter,
	request *http.Request,
) {
	if s.assistantProviders == nil {
		handleAssistantError(response, errors.New("assistant provider registry is unavailable"))
		return
	}
	user := currentUser(request.Context())
	connection, err := s.store.GetProviderConnection(
		request.Context(),
		user.ID,
		chi.URLParam(request, "connectionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	report, err := s.assistantProviders.ProbeStoredConnection(
		request.Context(),
		connection,
	)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "provider_unreachable", "Не удалось безопасно подключиться к провайдеру")
		return
	}
	updated, err := s.store.UpdateProviderProbe(
		request.Context(),
		user.ID,
		connection.ID,
		report.Status,
		report.Detail,
		report.Capabilities,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"connection": updated})
}

func (s *Server) handleDefaultAssistantProvider(
	response http.ResponseWriter,
	request *http.Request,
) {
	connection, err := s.store.SetDefaultProviderConnection(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "connectionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"connection": connection})
}

func (s *Server) handleDisableAssistantProvider(
	response http.ResponseWriter,
	request *http.Request,
) {
	err := s.store.DisableProviderConnection(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "connectionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]bool{"disabled": true})
}

type saveStructureRequest struct {
	BaseRevision int                     `json:"baseRevision"`
	Structure    domain.ProjectStructure `json:"structure"`
}

func (s *Server) handleGetProjectStructure(
	response http.ResponseWriter,
	request *http.Request,
) {
	structure, revision, err := s.store.GetProjectStructure(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"structure": structure, "revision": revision,
	})
}

func (s *Server) handleSaveProjectStructure(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input saveStructureRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	structure, revision, err := s.store.SaveProjectStructure(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.BaseRevision,
		input.Structure,
	)
	if err != nil {
		if errors.Is(err, store.ErrInvalidProjectStructure) {
			writeError(response, http.StatusUnprocessableEntity, "invalid_structure", "Структура проекта некорректна")
			return
		}
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"structure": structure, "revision": revision,
	})
}

type guideRequest struct {
	Content      string `json:"content"`
	BaseRevision *int   `json:"baseRevision"`
}

func (s *Server) handleGetAssistantGuide(
	response http.ResponseWriter,
	request *http.Request,
) {
	guide, err := s.store.GetAssistantGuide(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		true,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(response, http.StatusOK, map[string]any{"exists": false})
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"exists": true, "guide": guide})
}

func (s *Server) handleSaveAssistantGuide(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input guideRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	current, currentErr := s.store.GetAssistantGuide(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		false,
	)
	expectedRevision := 0
	switch {
	case currentErr == nil && (input.BaseRevision == nil || *input.BaseRevision != current.Revision):
		handleAssistantError(response, assistant.ErrStaleChangeSet)
		return
	case currentErr == nil:
		expectedRevision = current.Revision
	case errors.Is(currentErr, pgx.ErrNoRows) && input.BaseRevision != nil &&
		*input.BaseRevision != 0:
		handleAssistantError(response, assistant.ErrStaleChangeSet)
		return
	case currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows):
		handleStoreError(response, currentErr)
		return
	}
	guide, err := s.store.SaveAssistantGuide(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.Content,
		"manual",
		nil,
		nil,
		&expectedRevision,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"guide": guide})
}

func (s *Server) handleRestoreAssistantGuide(
	response http.ResponseWriter,
	request *http.Request,
) {
	revision, err := strconv.Atoi(chi.URLParam(request, "revision"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_revision", "Некорректная версия")
		return
	}
	guide, err := s.store.RestoreAssistantGuide(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		revision,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"guide": guide})
}

func (s *Server) handleListProjectMemory(
	response http.ResponseWriter,
	request *http.Request,
) {
	records, err := s.store.ListProjectMemory(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		request.URL.Query().Get("includeInactive") == "true",
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"records": records})
}

func (s *Server) handleListProjectMemoryRevisions(
	response http.ResponseWriter,
	request *http.Request,
) {
	revisions, err := s.store.ListProjectMemoryRevisions(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		chi.URLParam(request, "memoryID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"revisions": revisions})
}

func (s *Server) handleRestoreProjectMemory(
	response http.ResponseWriter,
	request *http.Request,
) {
	revision, err := strconv.Atoi(chi.URLParam(request, "revision"))
	if err != nil || revision < 1 {
		writeError(response, http.StatusBadRequest, "invalid_revision", "Некорректная версия записи")
		return
	}
	record, err := s.store.RestoreProjectMemory(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		chi.URLParam(request, "memoryID"),
		revision,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"record": record})
}

type memoryRequest struct {
	Kind             assistant.MemoryKind        `json:"kind"`
	Statement        string                      `json:"statement"`
	Scope            string                      `json:"scope"`
	ScopeNodeID      *string                     `json:"scopeNodeId"`
	SourceReferences []assistant.SourceReference `json:"sourceReferences"`
	Tags             []string                    `json:"tags"`
	Active           bool                        `json:"active"`
	AlwaysRelevant   bool                        `json:"alwaysRelevant"`
	BaseRevision     *int                        `json:"baseRevision"`
}

func memoryFromRequest(projectID string, memoryID string, input memoryRequest) assistant.MemoryRecord {
	return assistant.MemoryRecord{
		ID: memoryID, ProjectID: projectID, Kind: input.Kind,
		Statement: strings.TrimSpace(input.Statement), Scope: input.Scope,
		ScopeNodeID: input.ScopeNodeID, SourceReferences: input.SourceReferences,
		Tags: input.Tags, Active: input.Active, AlwaysRelevant: input.AlwaysRelevant,
		Source: "manual",
	}
}

func (s *Server) handleCreateProjectMemory(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input memoryRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	record, err := s.store.SaveProjectMemory(
		request.Context(),
		currentUser(request.Context()).ID,
		memoryFromRequest(chi.URLParam(request, "projectID"), "", input),
		nil,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"record": record})
}

func (s *Server) handleUpdateProjectMemory(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input memoryRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.BaseRevision == nil {
		writeError(response, http.StatusUnprocessableEntity, "base_revision_required", "Укажите версию записи")
		return
	}
	record, err := s.store.SaveProjectMemory(
		request.Context(),
		currentUser(request.Context()).ID,
		memoryFromRequest(
			chi.URLParam(request, "projectID"),
			chi.URLParam(request, "memoryID"),
			input,
		),
		input.BaseRevision,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"record": record})
}

func (s *Server) handleDeactivateProjectMemory(
	response http.ResponseWriter,
	request *http.Request,
) {
	user := currentUser(request.Context())
	record, err := s.store.GetProjectMemory(
		request.Context(),
		user.ID,
		chi.URLParam(request, "memoryID"),
	)
	if err != nil || record.ProjectID != chi.URLParam(request, "projectID") {
		if err == nil {
			err = pgx.ErrNoRows
		}
		handleStoreError(response, err)
		return
	}
	base := record.Revision
	record.Active = false
	record.Source = "manual"
	record, err = s.store.SaveProjectMemory(request.Context(), user.ID, record, &base)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"record": record})
}

type createRunRequest struct {
	Title                string                               `json:"title"`
	TargetKind           assistant.TargetKind                 `json:"targetKind"`
	TargetNodeID         *string                              `json:"targetNodeId"`
	TargetCanvasID       *string                              `json:"targetCanvasId"`
	CanvasSelectionIDs   []string                             `json:"canvasSelectionIds"`
	CanvasViewport       map[string]float64                   `json:"canvasViewport"`
	InputMode            assistant.InputMode                  `json:"inputMode"`
	SilenceCheckpointMS  int                                  `json:"silenceCheckpointMs"`
	ProviderConnectionID *string                              `json:"providerConnectionId"`
	WriterPreferences    *assistant.WriterPreferencesSnapshot `json:"writerPreferences"`
	InitialInstruction   string                               `json:"initialInstruction"`
	Execute              bool                                 `json:"execute"`
	SelectionAttachment  *assistant.SelectionAttachment       `json:"selectionAttachment"`
}

func (s *Server) handleCreateAssistantRun(
	response http.ResponseWriter,
	request *http.Request,
) {
	if s.assistantProviders == nil || s.assistantOrchestrator == nil {
		writeError(response, http.StatusServiceUnavailable, "assistant_unavailable", "Ассистент пока не настроен")
		return
	}
	var input createRunRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	user := currentUser(request.Context())
	var provider assistant.ProviderRef
	if input.ProviderConnectionID != nil {
		connection, err := s.store.GetProviderConnection(
			request.Context(),
			user.ID,
			*input.ProviderConnectionID,
		)
		if err != nil {
			handleStoreError(response, err)
			return
		}
		provider = assistant.ProviderRef{
			Kind: assistant.ProviderConnection, ConnectionID: &connection.ID,
			Name: connection.Name, Model: connection.Model, Dialect: connection.Dialect,
			Capabilities: connection.Capabilities,
		}
	} else {
		var ok bool
		provider, ok = s.assistantProviders.SystemProviderRef(request.Context(), true)
		if !ok {
			writeError(response, http.StatusServiceUnavailable, "provider_unavailable", "LLM-провайдер не настроен")
			return
		}
	}
	if !provider.Capabilities.CanRunAssistant() {
		writeError(response, http.StatusUnprocessableEntity, "provider_incompatible", "Провайдер не прошёл проверку инструментов")
		return
	}
	var guideRevision *int
	guide, err := s.store.GetAssistantGuide(
		request.Context(),
		user.ID,
		chi.URLParam(request, "projectID"),
		false,
	)
	if err == nil {
		guideRevision = &guide.Revision
	} else if !errors.Is(err, pgx.ErrNoRows) {
		handleStoreError(response, err)
		return
	}
	readContext := map[string]any{"createdFrom": "assistant_picker"}
	if input.TargetKind == assistant.TargetCanvas {
		if input.TargetCanvasID == nil {
			writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_context", "Не выбрана доска")
			return
		}
		workspace, canvasErr := s.store.GetCanvasWorkspace(
			request.Context(), user.ID, *input.TargetCanvasID,
		)
		if canvasErr != nil || workspace.Canvas.ProjectID != chi.URLParam(request, "projectID") ||
			workspace.Canvas.TrashedAt != nil {
			handleStoreError(response, pgx.ErrNoRows)
			return
		}
		if canvasErr := canvasdocument.ValidateSelection(
			workspace.Revision.Content, input.CanvasSelectionIDs,
		); canvasErr != nil {
			writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_context", "Выбор на доске устарел")
			return
		}
		readContext["canvasBaseRevision"] = workspace.Revision.Revision
	}
	if len(input.CanvasSelectionIDs) > 0 {
		if len(input.CanvasSelectionIDs) > 500 {
			writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_context", "Слишком много выбранных элементов")
			return
		}
		readContext["canvasSelectionIds"] = input.CanvasSelectionIDs
	}
	if len(input.CanvasViewport) > 0 {
		readContext["canvasViewport"] = input.CanvasViewport
	}
	if input.WriterPreferences != nil {
		readContext["writerPreferences"] = input.WriterPreferences.Normalized()
	}
	title := assistantThreadTitle(input.Title, input.InitialInstruction, input.InputMode)
	runInput := assistant.Run{
		OwnerUserID: user.ID, ProjectID: chi.URLParam(request, "projectID"),
		Title:      title,
		TargetKind: input.TargetKind, TargetNodeID: input.TargetNodeID,
		TargetCanvasID: input.TargetCanvasID,
		InputMode:      input.InputMode, SilenceCheckpointMS: input.SilenceCheckpointMS,
		Provider: provider, GuideRevision: guideRevision,
		ReadContext: readContext,
	}
	var run assistant.Run
	var firstTurn *assistant.Turn
	if strings.TrimSpace(input.InitialInstruction) != "" {
		if err := s.validateAssistantSelectionForTarget(
			request.Context(), user.ID, runInput.ProjectID, runInput.TargetNodeID,
			input.SelectionAttachment,
		); err != nil {
			writeError(response, http.StatusUnprocessableEntity, "invalid_selection", "Выделение больше не соответствует рукописи. Выделите фрагмент снова.")
			return
		}
		createdRun, turn, createErr := s.store.CreateAssistantRunWithFirstTurn(
			request.Context(), runInput, input.InputMode, input.InitialInstruction,
			input.SelectionAttachment,
		)
		run, err = createdRun, createErr
		if err == nil {
			firstTurn = &turn
		}
	} else {
		run, err = s.store.CreateAssistantRun(request.Context(), runInput)
	}
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	if firstTurn != nil && input.Execute {
		if !s.startAssistantExecution(user.ID, run.ID, firstTurn.ID) {
			_ = s.store.DeleteAssistantRunHistory(request.Context(), user.ID, run.ID)
			writeError(response, http.StatusConflict, "run_busy", "Не удалось запустить новое поручение")
			return
		}
	}
	writeJSON(response, http.StatusCreated, map[string]any{"run": run, "turn": firstTurn})
}

func assistantThreadTitle(explicit, instruction string, mode assistant.InputMode) string {
	value := strings.TrimSpace(explicit)
	if value == "" {
		value = strings.TrimSpace(instruction)
	}
	if value == "" && mode == assistant.InputVoice {
		value = "Голосовое поручение"
	}
	if value == "" {
		value = "Новый чат"
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 80 {
		return strings.TrimSpace(string(runes[:79])) + "…"
	}
	return value
}

func (s *Server) handleListAssistantRuns(
	response http.ResponseWriter,
	request *http.Request,
) {
	runs, err := s.store.ListAssistantRuns(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		30,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetAssistantRun(
	response http.ResponseWriter,
	request *http.Request,
) {
	run, err := s.store.GetAssistantRun(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"run": run})
}

func (s *Server) handleMarkAssistantRunViewed(
	response http.ResponseWriter,
	request *http.Request,
) {
	run, err := s.store.MarkAssistantRunViewed(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"run": run})
}

func (s *Server) handleDeleteAssistantRunHistory(
	response http.ResponseWriter,
	request *http.Request,
) {
	runID := chi.URLParam(request, "runID")
	if s.assistantExecutions != nil && s.assistantExecutions.isRunning(runID) {
		// A running request must be stopped explicitly first so deletion cannot
		// race a provider/tool write.
		handleAssistantError(response, assistant.ErrRunBusy)
		return
	}
	if err := s.store.DeleteAssistantRunHistory(
		request.Context(),
		currentUser(request.Context()).ID,
		runID,
	); err != nil {
		handleAssistantError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleListAssistantTurns(
	response http.ResponseWriter,
	request *http.Request,
) {
	turns, err := s.store.ListAssistantTurns(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"turns": turns})
}

type turnRequest struct {
	Instruction         string                         `json:"instruction"`
	InputMode           assistant.InputMode            `json:"inputMode"`
	RequireConfirmation bool                           `json:"requireConfirmation"`
	Execute             bool                           `json:"execute"`
	SelectionAttachment *assistant.SelectionAttachment `json:"selectionAttachment"`
}

func (s *Server) handleCreateAssistantTurn(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input turnRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	state := assistant.TurnQueued
	if input.RequireConfirmation {
		state = assistant.TurnAwaitingConfirmation
	}
	user := currentUser(request.Context())
	runID := chi.URLParam(request, "runID")
	if err := s.validateAssistantSelection(
		request.Context(), user.ID, runID, input.SelectionAttachment,
	); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_selection", "Выделение больше не соответствует рукописи. Выделите фрагмент снова.")
		return
	}
	turn, err := s.store.CreateAssistantTurnWithSelection(
		request.Context(),
		user.ID,
		runID,
		input.InputMode,
		input.Instruction,
		state,
		input.SelectionAttachment,
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	if input.Execute && state == assistant.TurnQueued {
		if !s.startAssistantExecution(user.ID, runID, turn.ID) {
			writeError(response, http.StatusConflict, "run_busy", "Ассистент уже выполняет поручение")
			return
		}
	}
	writeJSON(response, http.StatusCreated, map[string]any{"turn": turn})
}

func (s *Server) validateAssistantSelection(
	ctx context.Context,
	ownerID string,
	runID string,
	selection *assistant.SelectionAttachment,
) error {
	if selection == nil {
		return nil
	}
	if err := selection.Validate(); err != nil {
		return err
	}
	run, err := s.store.GetAssistantRun(ctx, ownerID, runID)
	if err != nil {
		return err
	}
	return s.validateAssistantSelectionForTarget(
		ctx, ownerID, run.ProjectID, run.TargetNodeID, selection,
	)
}

func (s *Server) validateAssistantSelectionForTarget(
	ctx context.Context,
	ownerID string,
	projectID string,
	targetNodeID *string,
	selection *assistant.SelectionAttachment,
) error {
	if selection == nil {
		return nil
	}
	if err := selection.Validate(); err != nil {
		return err
	}
	if targetNodeID != nil && *targetNodeID != selection.NodeID {
		return assistant.ErrInvalidTurn
	}
	node, err := s.store.GetDocumentNode(ctx, ownerID, selection.NodeID)
	if err != nil || node.ProjectID != projectID {
		return assistant.ErrInvalidTurn
	}
	content := node.Content
	if node.CurrentRevision != selection.BaseRevision {
		revisions, listErr := s.store.ListRevisions(ctx, ownerID, node.ID)
		if listErr != nil {
			return listErr
		}
		found := false
		for _, revision := range revisions {
			if revision.Revision == selection.BaseRevision {
				content = revision.Content
				found = true
				break
			}
		}
		if !found {
			return assistant.ErrInvalidTurn
		}
	}
	return assistant.ValidateSelectionAttachment(content, *selection)
}

func (s *Server) handleConfirmAssistantTurn(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input struct {
		Instruction string `json:"instruction"`
		Execute     bool   `json:"execute"`
	}
	if !decodeJSON(response, request, &input) {
		return
	}
	user := currentUser(request.Context())
	runID := chi.URLParam(request, "runID")
	turns, err := s.store.ListAssistantTurns(request.Context(), user.ID, runID)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	var selected *assistant.Turn
	for index := range turns {
		if turns[index].ID == chi.URLParam(request, "turnID") {
			selected = &turns[index]
			break
		}
	}
	if selected == nil {
		handleStoreError(response, pgx.ErrNoRows)
		return
	}
	if selected.State != assistant.TurnAwaitingConfirmation &&
		selected.State != assistant.TurnQueued {
		writeError(response, http.StatusConflict, "turn_not_editable", "Поручение уже выполняется")
		return
	}
	selected.Instruction = strings.TrimSpace(input.Instruction)
	selected.Transcript = selected.Instruction
	selected.State = assistant.TurnQueued
	if err := selected.ValidateInstruction(); err != nil {
		handleAssistantError(response, err)
		return
	}
	if err := s.store.UpdateAssistantTurn(request.Context(), *selected); err != nil {
		handleStoreError(response, err)
		return
	}
	if input.Execute && !s.startAssistantExecution(user.ID, runID, selected.ID) {
		writeError(response, http.StatusConflict, "run_busy", "Ассистент уже выполняет поручение")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"turn": selected})
}

func (s *Server) handleExecuteAssistantTurn(
	response http.ResponseWriter,
	request *http.Request,
) {
	if s.assistantOrchestrator == nil {
		writeError(response, http.StatusServiceUnavailable, "assistant_unavailable", "Ассистент пока не настроен")
		return
	}
	if !s.startAssistantExecution(
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
		chi.URLParam(request, "turnID"),
	) {
		writeError(response, http.StatusConflict, "run_busy", "Ассистент уже выполняет поручение")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]bool{"started": true})
}

func (s *Server) startAssistantExecution(ownerID, runID, turnID string) bool {
	if s.assistantExecutions == nil || s.assistantOrchestrator == nil {
		return false
	}
	return s.assistantExecutions.start(runID, func(ctx context.Context) {
		_, err := s.assistantOrchestrator.ExecuteTurn(ctx, ownerID, runID, turnID)
		if err != nil {
			errorCode := "assistant_execution_failed"
			if failedRun, loadErr := s.store.GetAssistantRun(
				context.WithoutCancel(ctx),
				ownerID,
				runID,
			); loadErr == nil && failedRun.LatestTurn != nil &&
				failedRun.LatestTurn.ErrorCode != nil {
				errorCode = *failedRun.LatestTurn.ErrorCode
			}
			s.logger.Warn(
				"assistant turn failed",
				"runId",
				runID,
				"turnId",
				turnID,
				"errorClass",
				errorCode,
			)
		}
	})
}

func (s *Server) handleStopAssistantRun(
	response http.ResponseWriter,
	request *http.Request,
) {
	user := currentUser(request.Context())
	runID := chi.URLParam(request, "runID")
	if _, err := s.store.GetAssistantRun(request.Context(), user.ID, runID); err != nil {
		handleStoreError(response, err)
		return
	}
	stopped := s.assistantExecutions != nil && s.assistantExecutions.stop(runID)
	code := "cancelled"
	_ = s.store.UpdateAssistantRunStatus(
		request.Context(),
		user.ID,
		runID,
		assistant.RunCancelled,
		&code,
	)
	writeJSON(response, http.StatusOK, map[string]bool{"stopped": stopped})
}

func (s *Server) handleAssistantEvents(
	response http.ResponseWriter,
	request *http.Request,
) {
	user := currentUser(request.Context())
	runID := chi.URLParam(request, "runID")
	if _, err := s.store.GetAssistantRun(request.Context(), user.ID, runID); err != nil {
		handleStoreError(response, err)
		return
	}
	after := assistantEventCursor(
		request.URL.Query().Get("after"), request.Header.Get("Last-Event-ID"),
	)
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeError(response, http.StatusInternalServerError, "stream_unavailable", "Поток событий недоступен")
		return
	}
	poll := time.NewTicker(400 * time.Millisecond)
	defer poll.Stop()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		events, err := s.store.ListAssistantEvents(
			request.Context(),
			user.ID,
			runID,
			after,
			100,
		)
		if err != nil {
			return
		}
		for _, event := range events {
			after = event.Sequence
			encoded, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(
				response,
				"id: %d\nevent: %s\ndata: %s\n\n",
				event.Sequence,
				event.Type,
				encoded,
			)
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-request.Context().Done():
			return
		case <-poll.C:
		case <-keepalive.C:
			_, _ = response.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

func assistantEventCursor(queryValue, lastEventID string) int64 {
	result := int64(0)
	for _, raw := range []string{queryValue, lastEventID} {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && value > result {
			result = value
		}
	}
	return result
}

type applyChangeSetRequest struct {
	SelectedGroups        []string        `json:"selectedGroups"`
	AnnotationResolutions map[string]bool `json:"annotationResolutions"`
}

func (s *Server) handleGetAssistantChangeSet(
	response http.ResponseWriter,
	request *http.Request,
) {
	user := currentUser(request.Context())
	runID := chi.URLParam(request, "runID")
	if _, err := s.store.GetAssistantRun(request.Context(), user.ID, runID); err != nil {
		handleStoreError(response, err)
		return
	}
	changeSet, err := s.store.GetAssistantChangeSet(
		request.Context(),
		runID,
		chi.URLParam(request, "changeSetID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"changeSet": changeSet})
}

func (s *Server) handleApplyAssistantChangeSet(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input applyChangeSetRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	changeSet, err := s.store.ApplyAssistantChangeSet(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
		chi.URLParam(request, "changeSetID"),
		input.SelectedGroups,
		input.AnnotationResolutions,
	)
	if err != nil {
		if changeSet.ID != "" && changeSet.State == assistant.ChangeStale {
			s.appendAssistantHTTPEvent(
				request.Context(), changeSet, assistant.EventChangeSetStale,
			)
		}
		handleAssistantError(response, err)
		return
	}
	s.appendAssistantHTTPEvent(
		request.Context(),
		changeSet,
		assistant.EventChangeSetApplied,
	)
	writeJSON(response, http.StatusOK, map[string]any{"changeSet": changeSet})
}

func (s *Server) handleRejectAssistantChangeSet(
	response http.ResponseWriter,
	request *http.Request,
) {
	changeSet, err := s.store.RejectAssistantChangeSet(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
		chi.URLParam(request, "changeSetID"),
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	s.appendAssistantHTTPEvent(
		request.Context(),
		changeSet,
		assistant.EventChangeSetRejected,
	)
	writeJSON(response, http.StatusOK, map[string]any{"changeSet": changeSet})
}

func (s *Server) handleMarkAssistantChangeSetStale(
	response http.ResponseWriter,
	request *http.Request,
) {
	changeSet, err := s.store.MarkAssistantChangeSetStale(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
		chi.URLParam(request, "changeSetID"),
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	s.appendAssistantHTTPEvent(request.Context(), changeSet, assistant.EventChangeSetStale)
	writeJSON(response, http.StatusOK, map[string]any{"changeSet": changeSet})
}

func (s *Server) handleUndoAssistantChangeSet(
	response http.ResponseWriter,
	request *http.Request,
) {
	changeSet, err := s.store.UndoAssistantChangeSet(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "runID"),
		chi.URLParam(request, "changeSetID"),
	)
	if err != nil {
		handleAssistantError(response, err)
		return
	}
	s.appendAssistantHTTPEvent(
		request.Context(),
		changeSet,
		assistant.EventChangeSetUndone,
	)
	writeJSON(response, http.StatusOK, map[string]any{"changeSet": changeSet})
}

func (s *Server) appendAssistantHTTPEvent(
	ctx context.Context,
	changeSet assistant.ChangeSet,
	eventType assistant.EventType,
) {
	data, _ := assistant.NewEventData(map[string]any{
		"changeSetId": changeSet.ID, "state": changeSet.State,
		"operationId": changeSet.OperationID,
	})
	turnID := changeSet.TurnID
	_, _ = s.store.AppendAssistantEvent(ctx, assistant.Event{
		RunID: changeSet.RunID, TurnID: &turnID, Type: eventType, Data: data,
	})
}

func handleAssistantError(response http.ResponseWriter, err error) {
	switch {
	case store.IsNotFound(err):
		writeError(response, http.StatusNotFound, "not_found", "Ресурс не найден")
	case errors.Is(err, assistant.ErrStaleChangeSet):
		writeError(response, http.StatusConflict, "stale_change_set", "Проект изменился; поручение нужно пересчитать")
	case errors.Is(err, assistant.ErrRunBusy):
		writeError(response, http.StatusConflict, "run_busy", "Сначала остановите текущее поручение")
	case errors.Is(err, assistant.ErrInvalidRun),
		errors.Is(err, assistant.ErrInvalidTurn),
		errors.Is(err, assistant.ErrInvalidMemory),
		errors.Is(err, assistant.ErrInvalidChangeSet):
		writeError(response, http.StatusUnprocessableEntity, "invalid_assistant_request", "Проверьте параметры поручения")
	default:
		writeError(response, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие")
	}
}
