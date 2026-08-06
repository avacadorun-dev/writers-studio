package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"writers.studio/internal/store"
)

type openAPIOperation struct {
	Method         string
	Path           string
	Tag            string
	Summary        string
	OperationID    string
	RequestSchema  string
	ResponseSchema string
	SuccessStatus  int
	RequestMedia   string
	ResponseMedia  string
	Public         bool
}

var (
	openAPIOnce  sync.Once
	openAPIBytes []byte
	openAPIErr   error
	pathToken    = regexp.MustCompile(`\{([^}]+)\}`)
)

func jsonOperation(
	method, path, tag, summary, operationID, requestSchema, responseSchema string,
	status int,
) openAPIOperation {
	return openAPIOperation{
		Method: method, Path: path, Tag: tag, Summary: summary,
		OperationID: operationID, RequestSchema: requestSchema,
		ResponseSchema: responseSchema, SuccessStatus: status,
		RequestMedia: "application/json", ResponseMedia: "application/json",
	}
}

func openAPIOperations() []openAPIOperation {
	operations := []openAPIOperation{
		jsonOperation(http.MethodGet, "/api/health", "Meta", "Проверить состояние сервиса", "getHealth", "", "HealthResponse", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/auth/login", "Authentication", "Войти", "login", "AuthRequest", "AuthResponse", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/auth/register", "Authentication", "Зарегистрироваться", "register", "AuthRequest", "AuthResponse", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/auth/logout", "Authentication", "Выйти", "logout", "", "", http.StatusNoContent),
		jsonOperation(http.MethodGet, "/api/auth/me", "Authentication", "Получить текущего пользователя", "getCurrentUser", "", "AuthResponse", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/openapi.json", "Meta", "Получить OpenAPI-спецификацию", "getOpenAPISpec", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/docs", Tag: "Meta", Summary: "Открыть Swagger UI", OperationID: "getSwaggerUI", SuccessStatus: http.StatusOK, ResponseMedia: "text/html"},
		jsonOperation(http.MethodGet, "/api/stt/capabilities", "Speech", "Получить возможности распознавания речи", "getSTTCapabilities", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/mcp/grants", "Agents and MCP", "Получить доступы MCP", "listMCPGrants", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/mcp/grants", "Agents and MCP", "Создать ручной доступ MCP", "createMCPGrant", "Object", "Object", http.StatusCreated),
		jsonOperation(http.MethodPost, "/api/mcp/grants/{grantID}/rotate", "Agents and MCP", "Сменить токен MCP", "rotateMCPGrant", "Object", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/mcp/grants/{grantID}", "Agents and MCP", "Отозвать доступ MCP", "revokeMCPGrant", "", "", http.StatusNoContent),

		jsonOperation(http.MethodGet, "/api/assistant/capabilities", "Assistant providers", "Получить возможности ассистента", "getAssistantCapabilities", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/assistant/providers", "Assistant providers", "Получить подключения провайдеров", "listAssistantProviders", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/providers", "Assistant providers", "Добавить провайдера", "createAssistantProvider", "CreateProviderRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPost, "/api/assistant/providers/system/probe", "Assistant providers", "Проверить системного провайдера", "probeSystemAssistantProvider", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/providers/{connectionID}/probe", "Assistant providers", "Проверить подключение провайдера", "probeAssistantProvider", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/providers/{connectionID}/default", "Assistant providers", "Сделать провайдера основным", "setDefaultAssistantProvider", "", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/assistant/providers/{connectionID}", "Assistant providers", "Отключить провайдера", "disableAssistantProvider", "", "Object", http.StatusOK),

		jsonOperation(http.MethodGet, "/api/projects/{projectID}/structure", "Assistant context", "Получить структуру проекта", "getProjectStructure", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/projects/{projectID}/structure", "Assistant context", "Сохранить структуру проекта", "saveProjectStructure", "SaveStructureRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/assistant-guide", "Assistant context", "Получить руководство ассистента", "getAssistantGuide", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/projects/{projectID}/assistant-guide", "Assistant context", "Сохранить руководство ассистента", "saveAssistantGuide", "GuideRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/assistant-guide/revisions/{revision}/restore", "Assistant context", "Восстановить руководство", "restoreAssistantGuide", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/memory", "Assistant context", "Получить память проекта", "listProjectMemory", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/memory", "Assistant context", "Добавить запись памяти", "createProjectMemory", "MemoryRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/memory/{memoryID}/revisions", "Assistant context", "Получить историю записи памяти", "listProjectMemoryRevisions", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/memory/{memoryID}/revisions/{revision}/restore", "Assistant context", "Восстановить запись памяти", "restoreProjectMemory", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/projects/{projectID}/memory/{memoryID}", "Assistant context", "Обновить запись памяти", "updateProjectMemory", "MemoryRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/projects/{projectID}/memory/{memoryID}", "Assistant context", "Деактивировать запись памяти", "deactivateProjectMemory", "", "Object", http.StatusOK),

		jsonOperation(http.MethodGet, "/api/projects/{projectID}/assistant/runs", "Assistant runs", "Получить сессии ассистента", "listAssistantRuns", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/assistant/runs", "Assistant runs", "Создать сессию ассистента", "createAssistantRun", "CreateRunRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/assistant/runs/{runID}", "Assistant runs", "Получить сессию ассистента", "getAssistantRun", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/view", "Assistant runs", "Отметить чат просмотренным", "markAssistantRunViewed", "", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/assistant/runs/{runID}", "Assistant runs", "Удалить историю сессии", "deleteAssistantRun", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/assistant/runs/{runID}/turns", "Assistant runs", "Получить реплики сессии", "listAssistantTurns", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/turns", "Assistant runs", "Добавить реплику", "createAssistantTurn", "TurnRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPut, "/api/assistant/runs/{runID}/turns/{turnID}", "Assistant runs", "Подтвердить реплику", "confirmAssistantTurn", "ConfirmTurnRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/turns/{turnID}/execute", "Assistant runs", "Запустить обработку реплики", "executeAssistantTurn", "", "Object", http.StatusAccepted),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/stop", "Assistant runs", "Остановить сессию", "stopAssistantRun", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/assistant/runs/{runID}/events", Tag: "Assistant runs", Summary: "Получать события сессии (SSE)", OperationID: "getAssistantEvents", SuccessStatus: http.StatusOK, ResponseMedia: "text/event-stream"},
		jsonOperation(http.MethodGet, "/api/assistant/runs/{runID}/change-sets/{changeSetID}", "Assistant runs", "Получить набор изменений", "getAssistantChangeSet", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/assistant/runs/{runID}/voice", Tag: "Assistant runs", Summary: "Открыть голосовой WebSocket", OperationID: "openAssistantVoiceSocket", SuccessStatus: http.StatusSwitchingProtocols},
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/change-sets/{changeSetID}/apply", "Assistant runs", "Применить набор изменений", "applyAssistantChangeSet", "ApplyChangeSetRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/change-sets/{changeSetID}/reject", "Assistant runs", "Отклонить набор изменений", "rejectAssistantChangeSet", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/change-sets/{changeSetID}/stale", "Assistant runs", "Пометить набор изменений устаревшим", "markAssistantChangeSetStale", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/assistant/runs/{runID}/change-sets/{changeSetID}/undo", "Assistant runs", "Отменить применённый набор изменений", "undoAssistantChangeSet", "", "Object", http.StatusOK),

		jsonOperation(http.MethodGet, "/api/projects", "Projects", "Получить проекты", "listProjects", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects", "Projects", "Создать проект", "createProject", "CreateProjectRequest", "Object", http.StatusCreated),
		{Method: http.MethodPost, Path: "/api/projects/import", Tag: "Projects", Summary: "Импортировать проект", OperationID: "importProject", RequestSchema: "Binary", ResponseSchema: "Object", SuccessStatus: http.StatusCreated, RequestMedia: "application/vnd.writers-studio.project+zip", ResponseMedia: "application/json"},
		{Method: http.MethodPost, Path: "/api/projects/import/markdown-tree", Tag: "Projects", Summary: "Импортировать Markdown-дерево", OperationID: "importMarkdownTree", RequestSchema: "Object", ResponseSchema: "Object", SuccessStatus: http.StatusCreated, RequestMedia: "multipart/form-data", ResponseMedia: "application/json"},
		jsonOperation(http.MethodGet, "/api/projects/{projectID}", "Projects", "Получить проект", "getProject", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/outline", "Projects", "Получить облегчённое дерево проекта", "getProjectOutline", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPatch, "/api/projects/{projectID}", "Projects", "Обновить проект", "updateProject", "UpdateProjectRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/assistant-settings", "Projects", "Получить настройки ассистента проекта", "getProjectAssistantSettings", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/projects/{projectID}/assistant-settings", "Projects", "Обновить настройки ассистента проекта", "updateProjectAssistantSettings", "ProjectAssistantSettingsRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/export-profile", "Exports", "Получить профиль оформления", "getExportProfile", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/projects/{projectID}/export-profile", "Exports", "Сохранить профиль оформления", "saveExportProfile", "SaveExportProfileRequest", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/projects/{projectID}/transfer", Tag: "Projects", Summary: "Скачать переносимую копию проекта", OperationID: "downloadProjectTransfer", SuccessStatus: http.StatusOK, ResponseMedia: "application/vnd.writers-studio.project+zip"},
		jsonOperation(http.MethodPatch, "/api/projects/{projectID}/status", "Projects", "Изменить статус проекта", "updateProjectStatus", "ProjectStatusRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/projects/{projectID}/purge", "Projects", "Удалить проект безвозвратно", "purgeProject", "ConfirmationRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/canvases", "Canvases", "Получить доски проекта", "listCanvases", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/canvases", "Canvases", "Создать доску", "createCanvas", "CreateCanvasRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/canvases/{canvasID}", "Canvases", "Получить доску и ресурсы", "getCanvas", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPatch, "/api/canvases/{canvasID}", "Canvases", "Обновить доску", "updateCanvas", "UpdateCanvasRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/canvases/{canvasID}", "Canvases", "Переместить доску в корзину", "trashCanvas", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/canvases/{canvasID}/duplicate", "Canvases", "Дублировать доску", "duplicateCanvas", "", "Object", http.StatusCreated),
		jsonOperation(http.MethodPost, "/api/canvases/{canvasID}/restore", "Canvases", "Восстановить доску", "restoreCanvas", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/canvases/{canvasID}/revisions", "Canvases", "Сохранить ревизию доски", "saveCanvasRevision", "SaveCanvasRevisionRequest", "Object", http.StatusCreated),

		jsonOperation(http.MethodPost, "/api/projects/{projectID}/nodes", "Documents", "Создать узел", "createNode", "CreateNodeRequest", "Object", http.StatusCreated),
		{Method: http.MethodPost, Path: "/api/projects/{projectID}/media", Tag: "Media", Summary: "Загрузить изображение проекта", OperationID: "uploadProjectMedia", RequestSchema: "Binary", ResponseSchema: "Object", SuccessStatus: http.StatusCreated, RequestMedia: "application/octet-stream", ResponseMedia: "application/json"},
		jsonOperation(http.MethodGet, "/api/projects/{projectID}/media", "Media", "Получить изображения проекта", "listProjectMedia", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/media/{mediaID}/content", Tag: "Media", Summary: "Получить изображение проекта", OperationID: "getProjectMediaContent", SuccessStatus: http.StatusOK, ResponseMedia: "image/*"},
		jsonOperation(http.MethodGet, "/api/nodes/{nodeID}", "Documents", "Получить узел", "getNode", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPatch, "/api/nodes/{nodeID}", "Documents", "Обновить узел", "updateNode", "UpdateNodeRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/nodes/{nodeID}/duplicate", "Documents", "Дублировать узел", "duplicateNode", "", "Object", http.StatusCreated),
		jsonOperation(http.MethodPut, "/api/nodes/{nodeID}/revisions", "Documents", "Сохранить ревизию документа", "saveRevision", "SaveRevisionRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/nodes/{nodeID}/revisions", "Documents", "Получить ревизии документа", "listRevisions", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/nodes/{nodeID}/move", "Documents", "Переместить узел", "moveNode", "MoveNodeRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/nodes/{nodeID}", "Documents", "Переместить узел в корзину", "trashNode", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/nodes/{nodeID}/restore", "Documents", "Восстановить узел", "restoreNode", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/nodes/{nodeID}/versions", "Documents", "Получить именованные версии", "listVersions", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/nodes/{nodeID}/versions", "Documents", "Создать именованную версию", "createVersion", "CreateVersionRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPost, "/api/versions/{versionID}/restore", "Documents", "Восстановить именованную версию", "restoreVersion", "", "Object", http.StatusCreated),

		jsonOperation(http.MethodGet, "/api/projects/{projectID}/search", "Search and replace", "Искать по проекту", "searchProject", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/replace/preview", "Search and replace", "Предпросмотр замены", "previewProjectReplace", "ProjectReplaceRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/replace/apply", "Search and replace", "Применить замену", "applyProjectReplace", "ProjectReplaceRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/replace/{operationID}/undo", "Search and replace", "Отменить замену", "undoProjectReplace", "", "Object", http.StatusCreated),

		jsonOperation(http.MethodGet, "/api/projects/{projectID}/annotations", "Annotations", "Получить заметки проекта", "listProjectAnnotations", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/nodes/{nodeID}/annotations", "Annotations", "Получить заметки документа", "listNodeAnnotations", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/nodes/{nodeID}/annotations", "Annotations", "Создать заметку", "createAnnotation", "CreateAnnotationRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPatch, "/api/annotations/{annotationID}", "Annotations", "Обновить заметку", "updateAnnotation", "UpdateAnnotationRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/annotations/{annotationID}", "Annotations", "Удалить заметку", "deleteAnnotation", "", "", http.StatusNoContent),

		jsonOperation(http.MethodGet, "/api/projects/{projectID}/dictionary", "Dictionary", "Получить словарь проекта", "listDictionaryTerms", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/projects/{projectID}/dictionary", "Dictionary", "Добавить термин", "createDictionaryTerm", "DictionaryTermRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodPut, "/api/dictionary/{termID}", "Dictionary", "Обновить термин", "updateDictionaryTerm", "DictionaryTermRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodDelete, "/api/dictionary/{termID}", "Dictionary", "Удалить термин", "deleteDictionaryTerm", "", "Object", http.StatusOK),

		jsonOperation(http.MethodPost, "/api/dictations", "Dictation", "Создать сессию диктовки", "createDictation", "CreateDictationRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/dictations/{sessionID}", "Dictation", "Получить сессию диктовки", "getDictation", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/dictations/{sessionID}/transition", "Dictation", "Изменить состояние диктовки", "transitionDictation", "DictationTransitionRequest", "Object", http.StatusOK),
		{Method: http.MethodPut, Path: "/api/dictations/{sessionID}/chunks/{sequence}", Tag: "Dictation", Summary: "Загрузить аудиофрагмент", OperationID: "uploadDictationChunk", RequestSchema: "Binary", ResponseSchema: "Object", SuccessStatus: http.StatusOK, RequestMedia: "application/octet-stream", ResponseMedia: "application/json"},
		jsonOperation(http.MethodPost, "/api/dictations/{sessionID}/stop", "Dictation", "Остановить диктовку", "stopDictation", "StopDictationRequest", "Object", http.StatusAccepted),
		jsonOperation(http.MethodPost, "/api/dictations/{sessionID}/retry", "Dictation", "Повторить финализацию", "retryDictation", "", "Object", http.StatusAccepted),
		jsonOperation(http.MethodPost, "/api/dictations/{sessionID}/retranscribe", "Dictation", "Перераспознать диктовку", "retranscribeDictation", "", "Object", http.StatusAccepted),
		jsonOperation(http.MethodPut, "/api/dictations/{sessionID}/draft", "Dictation", "Сохранить черновик расшифровки", "saveTranscriptDraft", "SaveTranscriptDraftRequest", "Object", http.StatusOK),
		jsonOperation(http.MethodPost, "/api/dictations/{sessionID}/transfer", "Dictation", "Перенести расшифровку в документ", "transferTranscript", "TransferTranscriptRequest", "Object", http.StatusCreated),
		jsonOperation(http.MethodGet, "/api/dictations/{sessionID}/audio", "Dictation", "Получить ссылку на аудио", "getDictationAudio", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/dictations/{sessionID}/audio/content", Tag: "Dictation", Summary: "Скачать аудио диктовки", OperationID: "downloadDictationAudio", SuccessStatus: http.StatusOK, ResponseMedia: "audio/webm"},
		jsonOperation(http.MethodGet, "/api/source-segments/{segmentID}/audio", "Dictation", "Получить аудио исходного сегмента", "getSourceSegmentAudio", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/dictations/{sessionID}/live", Tag: "Dictation", Summary: "Открыть WebSocket живой диктовки", OperationID: "openDictationSocket", SuccessStatus: http.StatusSwitchingProtocols},

		jsonOperation(http.MethodGet, "/api/recovery", "Preferences and account", "Получить данные восстановления", "getRecovery", "", "Object", http.StatusOK),
		jsonOperation(http.MethodGet, "/api/preferences", "Preferences and account", "Получить настройки", "getPreferences", "", "Object", http.StatusOK),
		jsonOperation(http.MethodPut, "/api/preferences", "Preferences and account", "Обновить настройки", "updatePreferences", "PreferencesRequest", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/account/backup", Tag: "Preferences and account", Summary: "Скачать резервную копию аккаунта", OperationID: "downloadAccountBackup", SuccessStatus: http.StatusOK, ResponseMedia: "application/zip"},
		jsonOperation(http.MethodDelete, "/api/account", "Preferences and account", "Удалить аккаунт", "purgeAccount", "ConfirmationRequest", "Object", http.StatusOK),

		jsonOperation(http.MethodPost, "/api/projects/{projectID}/exports", "Exports", "Создать экспорт", "createExport", "CreateExportRequest", "Object", http.StatusAccepted),
		jsonOperation(http.MethodGet, "/api/exports/{exportID}", "Exports", "Получить состояние экспорта", "getExport", "", "Object", http.StatusOK),
		{Method: http.MethodGet, Path: "/api/exports/{exportID}/download", Tag: "Exports", Summary: "Скачать экспорт", OperationID: "downloadExport", SuccessStatus: http.StatusOK, ResponseMedia: "application/octet-stream"},
	}
	for index := range operations {
		switch operations[index].OperationID {
		case "getHealth", "login", "register", "logout":
			operations[index].Public = true
		}
	}
	return operations
}

func (s *Server) handleOpenAPI(response http.ResponseWriter, _ *http.Request) {
	data, err := openAPIDocumentJSON()
	if err != nil {
		writeError(response, http.StatusInternalServerError, "openapi_unavailable", "Документация временно недоступна")
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "private, no-cache")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}

func (s *Server) handleOpenAPIDocs(response http.ResponseWriter, request *http.Request) {
	cloned := request.Clone(request.Context())
	cloned.URL.Path = "/"
	cloned.Header.Set("Accept", "text/html")
	s.staticHandler().ServeHTTP(response, cloned)
}

func openAPIDocumentJSON() ([]byte, error) {
	openAPIOnce.Do(func() {
		openAPIBytes, openAPIErr = json.Marshal(openAPIDocument())
	})
	return openAPIBytes, openAPIErr
}

func openAPIDocument() map[string]any {
	paths := map[string]any{}
	for _, operation := range openAPIOperations() {
		pathItem, _ := paths[operation.Path].(map[string]any)
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[operation.Path] = pathItem
		}
		pathItem[strings.ToLower(operation.Method)] = buildOpenAPIOperation(operation)
	}
	return map[string]any{
		"openapi":           "3.1.0",
		"jsonSchemaDialect": "https://spec.openapis.org/oas/3.1/dialect/base",
		"info": map[string]any{
			"title":       "Writer's Studio API",
			"version":     "1.0.0",
			"description": "Внутренний HTTP API Writer's Studio. Защищённые методы используют текущую HttpOnly-сессию браузера.",
		},
		"servers":  []any{map[string]any{"url": "/", "description": "Текущий сервер"}},
		"security": []any{map[string]any{"sessionCookie": []any{}}},
		"tags":     openAPITags(),
		"paths":    paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"sessionCookie": map[string]any{
					"type": "apiKey", "in": "cookie", "name": sessionCookieName,
					"description": "HttpOnly-cookie действующей сессии. Браузер отправляет её автоматически.",
				},
			},
			"schemas": openAPISchemas(),
		},
	}
}

func buildOpenAPIOperation(spec openAPIOperation) map[string]any {
	operation := map[string]any{
		"tags": []string{spec.Tag}, "summary": spec.Summary,
		"operationId": spec.OperationID,
	}
	if spec.Public {
		operation["security"] = []any{}
	}
	parameters := openAPIParameters(spec)
	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}
	if spec.RequestSchema != "" {
		media := spec.RequestMedia
		if media == "" {
			media = "application/json"
		}
		operation["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{media: map[string]any{
				"schema": openAPISchemaReference(spec.RequestSchema),
			}},
		}
	}
	responses := map[string]any{}
	status := spec.SuccessStatus
	if status == 0 {
		status = http.StatusOK
	}
	success := map[string]any{"description": http.StatusText(status)}
	if status != http.StatusNoContent && status != http.StatusSwitchingProtocols {
		media := spec.ResponseMedia
		if media == "" {
			media = "application/json"
		}
		schema := openAPISchemaReference(spec.ResponseSchema)
		if spec.ResponseSchema == "" {
			schema = map[string]any{"type": "string"}
			if media != "text/html" && media != "text/event-stream" {
				schema["format"] = "binary"
			}
		}
		success["content"] = map[string]any{media: map[string]any{"schema": schema}}
	}
	responses[strconv.Itoa(status)] = success
	if !spec.Public {
		responses["401"] = responseReference("Unauthorized")
	}
	if status != http.StatusSwitchingProtocols {
		responses["4XX"] = responseReference("Request error")
		responses["5XX"] = responseReference("Server error")
	}
	operation["responses"] = responses
	return operation
}

func openAPISchemaReference(name string) map[string]any {
	if name == "Binary" {
		return map[string]any{"type": "string", "format": "binary"}
	}
	if name == "" {
		name = "Object"
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func responseReference(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{"application/json": map[string]any{
			"schema": openAPISchemaReference("ErrorResponse"),
		}},
	}
}

func openAPIParameters(spec openAPIOperation) []any {
	parameters := make([]any, 0)
	for _, match := range pathToken.FindAllStringSubmatch(spec.Path, -1) {
		name := match[1]
		schema := map[string]any{"type": "string"}
		if name == "revision" || name == "sequence" {
			schema = map[string]any{"type": "integer", "minimum": 0}
		} else {
			schema["format"] = "uuid"
		}
		parameters = append(parameters, map[string]any{
			"name": name, "in": "path", "required": true, "schema": schema,
		})
	}
	switch spec.OperationID {
	case "listProjects":
		parameters = append(parameters, queryParameter("status", false, enumSchema("active", "archived", "trash"), "Фильтр состояния"))
	case "listCanvases":
		parameters = append(parameters, queryParameter("includeTrashed", false, map[string]any{"type": "boolean", "default": false}, "Включить доски из корзины"))
	case "searchProject":
		parameters = append(parameters,
			queryParameter("q", true, stringSchema(), "Строка поиска"),
			queryParameter("scope", false, enumSchema("all", "text", "titles", "annotations"), "Область поиска"),
		)
	case "listProjectMemory":
		parameters = append(parameters, queryParameter("includeInactive", false, map[string]any{"type": "boolean", "default": false}, "Включить неактивные записи"))
	case "getAssistantEvents":
		parameters = append(parameters,
			queryParameter("after", false, stringSchema(), "Идентификатор последнего события"),
			map[string]any{"name": "Last-Event-ID", "in": "header", "required": false, "schema": stringSchema()},
		)
	case "uploadDictationChunk":
		parameters = append(parameters,
			map[string]any{"name": "X-Audio-Duration-Ms", "in": "header", "required": true, "schema": map[string]any{"type": "integer", "minimum": 0}},
			map[string]any{"name": "X-Content-SHA256", "in": "header", "required": true, "schema": map[string]any{"type": "string", "pattern": "^[a-fA-F0-9]{64}$"}},
		)
	case "uploadProjectMedia":
		parameters = append(parameters,
			map[string]any{"name": "X-File-Name", "in": "header", "required": false, "schema": stringSchema()},
		)
	}
	return parameters
}

func queryParameter(name string, required bool, schema map[string]any, description string) map[string]any {
	return map[string]any{
		"name": name, "in": "query", "required": required,
		"description": description, "schema": schema,
	}
}

func openAPITags() []any {
	names := []string{
		"Meta", "Authentication", "Speech", "Projects", "Canvases", "Documents",
		"Search and replace", "Annotations", "Dictionary", "Dictation",
		"Media", "Preferences and account", "Exports", "Assistant providers",
		"Assistant context", "Assistant runs",
	}
	tags := make([]any, 0, len(names))
	for _, name := range names {
		tags = append(tags, map[string]any{"name": name})
	}
	return tags
}

func openAPISchemas() map[string]any {
	schemas := map[string]any{
		"Object": map[string]any{"type": "object", "additionalProperties": true},
		"ErrorResponse": objectSchema([]string{"error"}, map[string]any{
			"error": objectSchema([]string{"code", "message"}, map[string]any{
				"code": stringSchema(), "message": stringSchema(),
			}),
		}),
		"HealthResponse": objectSchema([]string{"status"}, map[string]any{
			"status": map[string]any{"type": "string", "const": "ok"},
		}),
		"AuthRequest": objectSchema([]string{"email", "password"}, map[string]any{
			"email":    map[string]any{"type": "string", "format": "email"},
			"password": map[string]any{"type": "string", "format": "password", "minLength": 8},
		}),
		"AuthResponse": objectSchema([]string{"user"}, map[string]any{
			"user": objectSchema([]string{"id", "email", "displayName", "createdAt"}, map[string]any{
				"id": uuidSchema(), "email": map[string]any{"type": "string", "format": "email"},
				"displayName": stringSchema(), "createdAt": dateTimeSchema(),
			}),
		}),
		"CreateProjectRequest": objectSchema([]string{"title", "kind", "language", "synopsis"}, map[string]any{
			"title": stringSchema(), "kind": stringSchema(), "language": enumSchema("ru", "en"),
			"synopsis": stringSchema(), "wordGoal": map[string]any{"type": "integer", "minimum": 0},
		}),
		"UpdateProjectRequest": objectSchema([]string{"title", "language", "synopsis"}, map[string]any{
			"title": stringSchema(), "language": enumSchema("ru", "en"),
			"synopsis": stringSchema(), "wordGoal": map[string]any{"type": "integer", "minimum": 1, "maximum": 10000000},
		}),
		"ProjectAssistantSettingsRequest": objectSchema(nil, map[string]any{
			"stepLimit": map[string]any{"type": []string{"integer", "null"}, "minimum": 1, "maximum": store.MaxProjectAssistantStepLimit},
		}),
		"ProjectStatusRequest": objectSchema([]string{"status"}, map[string]any{
			"status": enumSchema("active", "archived", "trash"),
		}),
		"ConfirmationRequest": objectSchema([]string{"confirmation"}, map[string]any{
			"confirmation": stringSchema(),
		}),
		"CreateCanvasRequest": objectSchema([]string{"title"}, map[string]any{
			"title": map[string]any{"type": "string", "minLength": 1, "maxLength": 160},
		}),
		"UpdateCanvasRequest": objectSchema(nil, map[string]any{
			"title":    map[string]any{"type": "string", "minLength": 1, "maxLength": 160},
			"position": map[string]any{"type": "number"},
		}),
		"CanvasDocument": map[string]any{
			"type": "object", "description": "JSON Canvas 1.0 с namespaced расширениями Writer's Studio",
			"properties": map[string]any{
				"nodes":            arraySchema(map[string]any{"type": "object", "additionalProperties": true}),
				"edges":            arraySchema(map[string]any{"type": "object", "additionalProperties": true}),
				"x-writers-studio": map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"nodes", "edges"}, "additionalProperties": true,
		},
		"SaveCanvasRevisionRequest": objectSchema([]string{"baseRevision", "content"}, map[string]any{
			"baseRevision": integerSchema(), "content": refSchema("CanvasDocument"), "source": stringSchema(),
		}),
		"CreateNodeRequest": objectSchema([]string{"kind", "title"}, map[string]any{
			"parentId": uuidSchema(), "kind": stringSchema(), "title": stringSchema(),
		}),
		"UpdateNodeRequest": objectSchema(nil, map[string]any{
			"title": stringSchema(), "status": stringSchema(),
		}),
		"SaveRevisionRequest": objectSchema([]string{"baseRevision", "content", "source"}, map[string]any{
			"baseRevision": integerSchema(), "content": editorDocumentSchema(), "source": stringSchema(),
			"operationId": uuidSchema(), "annotationAnchors": arraySchema(map[string]any{"type": "object", "additionalProperties": true}),
		}),
		"MoveNodeRequest": objectSchema(nil, map[string]any{
			"parentId": uuidSchema(), "position": map[string]any{"type": "number"}, "targetIndex": integerSchema(),
		}),
		"CreateVersionRequest": objectSchema([]string{"name", "note"}, map[string]any{
			"name": stringSchema(), "note": stringSchema(),
		}),
		"ProjectReplaceRequest": objectSchema([]string{"search", "replace", "scope"}, map[string]any{
			"search": stringSchema(), "replace": stringSchema(),
			"scope": enumSchema("all", "text", "titles", "annotations"), "previewToken": stringSchema(),
		}),
		"AnnotationAnchor": objectSchema([]string{"blockId", "start", "end", "exactText", "prefix", "suffix"}, map[string]any{
			"blockId": stringSchema(), "start": integerSchema(), "end": integerSchema(),
			"exactText": stringSchema(), "prefix": stringSchema(), "suffix": stringSchema(),
		}),
		"CreateAnnotationRequest": objectSchema([]string{"kind", "anchor", "note", "baseRevision"}, map[string]any{
			"kind": stringSchema(), "dictionaryTermId": uuidSchema(),
			"anchor": refSchema("AnnotationAnchor"), "note": stringSchema(), "baseRevision": integerSchema(),
		}),
		"UpdateAnnotationRequest": objectSchema(nil, map[string]any{
			"note": stringSchema(), "status": stringSchema(), "anchor": refSchema("AnnotationAnchor"),
		}),
		"DictionaryTermRequest": objectSchema([]string{"canonical", "variants", "kind", "language"}, map[string]any{
			"canonical": stringSchema(), "variants": arraySchema(stringSchema()), "kind": stringSchema(),
			"language": enumSchema("ru", "en"), "active": map[string]any{"type": "boolean"},
		}),
		"CreateExportRequest": objectSchema([]string{"format", "nodeIds", "titlePage", "chapterTitles", "includeNotes", "profile"}, map[string]any{
			"format": enumSchema("docx", "epub", "pdf", "markdown", "markdown_tree", "txt"), "nodeIds": arraySchema(uuidSchema()),
			"titlePage": map[string]any{"type": "boolean"}, "chapterTitles": map[string]any{"type": "boolean"},
			"includeNotes": map[string]any{"type": "boolean"}, "profile": map[string]any{"type": "object", "additionalProperties": true},
		}),
		"SaveExportProfileRequest": objectSchema([]string{"baseRevision", "profile"}, map[string]any{
			"baseRevision": integerSchema(), "profile": map[string]any{"type": "object", "additionalProperties": true},
		}),
		"CreateDictationRequest": objectSchema([]string{"projectId", "clientSessionId", "language"}, map[string]any{
			"projectId": uuidSchema(), "destinationNodeId": uuidSchema(), "clientSessionId": uuidSchema(),
			"language": enumSchema("ru", "en"),
		}),
		"DictationTransitionRequest": objectSchema([]string{"status"}, map[string]any{"status": stringSchema()}),
		"StopDictationRequest": objectSchema([]string{"durationMs", "recordedDurationMs", "manifest"}, map[string]any{
			"durationMs": integerSchema(), "recordedDurationMs": integerSchema(),
			"manifest": map[string]any{"type": "object", "additionalProperties": true},
		}),
		"SaveTranscriptDraftRequest": objectSchema([]string{"content", "source"}, map[string]any{
			"content": editorDocumentSchema(), "source": stringSchema(),
		}),
		"TransferTranscriptRequest": objectSchema([]string{"nodeId", "baseRevision", "content"}, map[string]any{
			"nodeId": uuidSchema(), "baseRevision": integerSchema(), "content": editorDocumentSchema(),
		}),
		"PreferencesRequest": objectSchema([]string{"audioRetentionDays"}, map[string]any{
			"audioRetentionDays": map[string]any{"type": "integer", "minimum": 0},
		}),
		"CreateProviderRequest": objectSchema([]string{"name", "baseUrl", "dialect", "model", "apiKey"}, map[string]any{
			"name": stringSchema(), "baseUrl": map[string]any{"type": "string", "format": "uri"},
			"dialect": stringSchema(), "model": stringSchema(), "apiKey": map[string]any{"type": "string", "format": "password"},
			"headers":          map[string]any{"type": "object", "additionalProperties": stringSchema()},
			"requestTimeoutMs": integerSchema(), "contextWindow": integerSchema(),
			"allowPrivateEndpoint": map[string]any{"type": "boolean"}, "isDefault": map[string]any{"type": "boolean"},
		}),
		"SaveStructureRequest": objectSchema([]string{"baseRevision", "structure"}, map[string]any{
			"baseRevision": integerSchema(), "structure": map[string]any{"type": "object", "additionalProperties": true},
		}),
		"GuideRequest": objectSchema([]string{"content"}, map[string]any{
			"content": stringSchema(), "baseRevision": integerSchema(),
		}),
		"SourceReference": objectSchema([]string{"kind", "id"}, map[string]any{
			"kind": stringSchema(), "id": stringSchema(), "label": stringSchema(), "quote": stringSchema(),
			"start": integerSchema(), "end": integerSchema(),
		}),
		"MemoryRequest": objectSchema([]string{"kind", "statement", "scope", "sourceReferences", "tags", "active", "alwaysRelevant"}, map[string]any{
			"kind":      enumSchema("fact", "character", "world_rule", "plot", "style", "constraint", "decision", "other"),
			"statement": stringSchema(), "scope": enumSchema("project", "node"), "scopeNodeId": uuidSchema(),
			"sourceReferences": arraySchema(refSchema("SourceReference")), "tags": arraySchema(stringSchema()),
			"active": map[string]any{"type": "boolean"}, "alwaysRelevant": map[string]any{"type": "boolean"},
			"baseRevision": integerSchema(),
		}),
		"CreateRunRequest": objectSchema([]string{"targetKind", "inputMode", "silenceCheckpointMs"}, map[string]any{
			"title":      stringSchema(),
			"targetKind": enumSchema("project", "node"), "targetNodeId": uuidSchema(),
			"inputMode":            enumSchema("chat", "voice"),
			"silenceCheckpointMs":  map[string]any{"type": "integer", "enum": []int{0, 3000, 5000, 8000}},
			"providerConnectionId": uuidSchema(),
			"writerPreferences":    map[string]any{"type": "object", "additionalProperties": true},
			"initialInstruction":   stringSchema(),
			"execute":              map[string]any{"type": "boolean"},
			"selectionAttachment":  refSchema("SelectionAttachment"),
		}),
		"SelectionAttachment": objectSchema([]string{"nodeId", "baseRevision"}, map[string]any{
			"nodeId": uuidSchema(), "baseRevision": integerSchema(),
			"from": integerSchema(), "to": integerSchema(), "text": stringSchema(),
		}),
		"TurnRequest": objectSchema([]string{"instruction", "inputMode", "requireConfirmation", "execute"}, map[string]any{
			"instruction": stringSchema(), "inputMode": enumSchema("chat", "voice"),
			"requireConfirmation": map[string]any{"type": "boolean"}, "execute": map[string]any{"type": "boolean"},
			"selectionAttachment": refSchema("SelectionAttachment"),
		}),
		"ConfirmTurnRequest": objectSchema([]string{"instruction", "execute"}, map[string]any{
			"instruction": stringSchema(), "execute": map[string]any{"type": "boolean"},
		}),
		"ApplyChangeSetRequest": objectSchema([]string{"selectedGroups", "annotationResolutions"}, map[string]any{
			"selectedGroups":        arraySchema(stringSchema()),
			"annotationResolutions": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "boolean"}},
		}),
	}
	return schemas
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type": "object", "properties": properties, "additionalProperties": false,
	}
	if len(required) > 0 {
		sorted := append([]string(nil), required...)
		sort.Strings(sorted)
		schema["required"] = sorted
	}
	return schema
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }

func uuidSchema() map[string]any {
	return map[string]any{"type": "string", "format": "uuid"}
}

func dateTimeSchema() map[string]any {
	return map[string]any{"type": "string", "format": "date-time"}
}

func integerSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 0}
}

func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func refSchema(name string) map[string]any {
	return map[string]any{"$ref": fmt.Sprintf("#/components/schemas/%s", name)}
}

func editorDocumentSchema() map[string]any {
	return map[string]any{
		"type": "object", "description": "JSON-документ редактора TipTap/ProseMirror",
		"additionalProperties": true,
	}
}
