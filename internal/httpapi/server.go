package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"writers.studio/internal/assistantapp"
	"writers.studio/internal/config"
	"writers.studio/internal/domain"
	"writers.studio/internal/llm"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/remotemcp"
	"writers.studio/internal/store"
	"writers.studio/internal/stt"
)

const sessionCookieName = "writers_studio_session"

type Server struct {
	config                config.Config
	store                 *store.Store
	objects               objectstore.Store
	transcriber           stt.FinalTranscriber
	liveSTT               stt.LiveTranscriber
	assistantProviders    *assistantapp.ProviderRegistry
	assistantOrchestrator *assistantapp.Orchestrator
	assistantExecutions   *assistantExecutionManager
	remoteMCP             *remotemcp.Gateway
	mcpMetadataHTTPClient *http.Client
	logger                *slog.Logger
	router                chi.Router
}

type contextKey string

const userContextKey contextKey = "user"

func New(
	cfg config.Config,
	dataStore *store.Store,
	objects objectstore.Store,
	transcriber stt.FinalTranscriber,
	liveTranscriber stt.LiveTranscriber,
	logger *slog.Logger,
) *Server {
	server := &Server{
		config:                cfg,
		store:                 dataStore,
		objects:               objects,
		transcriber:           transcriber,
		liveSTT:               liveTranscriber,
		logger:                logger,
		mcpMetadataHTTPClient: llm.EndpointPolicy{}.HTTPClient(5 * time.Second),
		assistantExecutions:   newAssistantExecutionManager(),
	}
	backend := assistantapp.NewProjectBackend(dataStore)
	providers, providerErr := assistantapp.NewProviderRegistry(cfg, dataStore)
	if providerErr != nil {
		logger.Error("initialize assistant providers", "errorClass", "invalid_llm_credential_key")
	} else {
		server.assistantProviders = providers
		server.assistantOrchestrator = assistantapp.NewOrchestrator(
			dataStore,
			backend,
			providers,
		)
	}
	if cfg.MCPEnabled {
		gateway, gatewayErr := remotemcp.New(dataStore, backend, objects, cfg.PublicBaseURL, logger)
		if gatewayErr != nil {
			logger.Error("initialize remote MCP gateway", "errorClass", "invalid_mcp_configuration")
		} else {
			server.remoteMCP = gateway
		}
	}
	server.router = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(s.securityHeaders)
	router.Use(s.requestLog)

	router.Get("/api/health", s.handleHealth)
	s.mountMCPOAuthRoutes(router)
	if s.remoteMCP != nil {
		router.Handle("/mcp", s.remoteMCP.Handler())
	}
	router.Route("/api/auth", func(router chi.Router) {
		router.Post("/login", s.handleLogin)
		router.Post("/register", s.handleRegister)
		router.Post("/logout", s.handleLogout)
		router.With(s.requireUser).Get("/me", s.handleMe)
	})

	router.Group(func(router chi.Router) {
		router.Use(s.requireUser)

		router.Get("/api/openapi.json", s.handleOpenAPI)
		router.Get("/api/docs", s.handleOpenAPIDocs)
		router.Get("/api/docs/", s.handleOpenAPIDocs)
		router.Get("/api/stt/capabilities", s.handleSTTCapabilities)
		s.mountAssistantRoutes(router)
		s.mountMCPGrantRoutes(router)

		router.Get("/api/projects", s.handleListProjects)
		router.Post("/api/projects", s.handleCreateProject)
		router.Post("/api/projects/import", s.handleImportProjectTransfer)
		router.Post("/api/projects/import/markdown-tree", s.handleImportMarkdownTree)
		router.Get("/api/projects/{projectID}", s.handleGetProject)
		router.Get("/api/projects/{projectID}/outline", s.handleGetProjectOutline)
		router.Patch("/api/projects/{projectID}", s.handleUpdateProject)
		router.Get("/api/projects/{projectID}/assistant-settings", s.handleGetProjectAssistantSettings)
		router.Put("/api/projects/{projectID}/assistant-settings", s.handleUpdateProjectAssistantSettings)
		router.Get("/api/projects/{projectID}/export-profile", s.handleGetExportProfile)
		router.Put("/api/projects/{projectID}/export-profile", s.handleSaveExportProfile)
		router.Get("/api/projects/{projectID}/transfer", s.handleDownloadProjectTransfer)
		router.Patch("/api/projects/{projectID}/status", s.handleProjectStatus)
		router.Delete("/api/projects/{projectID}/purge", s.handlePurgeProject)
		router.Get("/api/projects/{projectID}/canvases", s.handleListCanvases)
		router.Post("/api/projects/{projectID}/canvases", s.handleCreateCanvas)
		router.Get("/api/canvases/{canvasID}", s.handleGetCanvas)
		router.Patch("/api/canvases/{canvasID}", s.handleUpdateCanvas)
		router.Delete("/api/canvases/{canvasID}", s.handleTrashCanvas)
		router.Post("/api/canvases/{canvasID}/duplicate", s.handleDuplicateCanvas)
		router.Post("/api/canvases/{canvasID}/restore", s.handleRestoreCanvas)
		router.Put("/api/canvases/{canvasID}/revisions", s.handleSaveCanvasRevision)

		router.Post("/api/projects/{projectID}/nodes", s.handleCreateNode)
		router.Post("/api/projects/{projectID}/media", s.handleUploadProjectMedia)
		router.Get("/api/projects/{projectID}/media", s.handleListProjectMedia)
		router.Get("/api/media/{mediaID}/content", s.handleProjectMediaContent)
		router.Get("/api/nodes/{nodeID}", s.handleGetNode)
		router.Patch("/api/nodes/{nodeID}", s.handleUpdateNode)
		router.Post("/api/nodes/{nodeID}/duplicate", s.handleDuplicateNode)
		router.Put("/api/nodes/{nodeID}/revisions", s.handleSaveRevision)
		router.Get("/api/nodes/{nodeID}/revisions", s.handleListRevisions)
		router.Post("/api/nodes/{nodeID}/move", s.handleMoveNode)
		router.Delete("/api/nodes/{nodeID}", s.handleTrashNode)
		router.Post("/api/nodes/{nodeID}/restore", s.handleRestoreNode)
		router.Get("/api/nodes/{nodeID}/versions", s.handleListVersions)
		router.Post("/api/nodes/{nodeID}/versions", s.handleCreateVersion)
		router.Post("/api/versions/{versionID}/restore", s.handleRestoreVersion)

		router.Get("/api/projects/{projectID}/search", s.handleSearchProject)
		router.Get("/api/projects/{projectID}/annotations", s.handleListProjectAnnotations)
		router.Get("/api/nodes/{nodeID}/annotations", s.handleListDocumentAnnotations)
		router.Post("/api/nodes/{nodeID}/annotations", s.handleCreateDocumentAnnotation)
		router.Patch("/api/annotations/{annotationID}", s.handleUpdateDocumentAnnotation)
		router.Delete("/api/annotations/{annotationID}", s.handleDeleteDocumentAnnotation)
		router.Post("/api/projects/{projectID}/replace/preview", s.handlePreviewProjectReplace)
		router.Post("/api/projects/{projectID}/replace/apply", s.handleApplyProjectReplace)
		router.Post(
			"/api/projects/{projectID}/replace/{operationID}/undo",
			s.handleUndoProjectReplace,
		)
		router.Get("/api/projects/{projectID}/dictionary", s.handleListDictionary)
		router.Post("/api/projects/{projectID}/dictionary", s.handleCreateDictionaryTerm)
		router.Put("/api/dictionary/{termID}", s.handleUpdateDictionaryTerm)
		router.Delete("/api/dictionary/{termID}", s.handleDeleteDictionaryTerm)

		router.Post("/api/dictations", s.handleCreateDictation)
		router.Get("/api/dictations/{sessionID}", s.handleGetDictation)
		router.Post("/api/dictations/{sessionID}/transition", s.handleDictationTransition)
		router.Put("/api/dictations/{sessionID}/chunks/{sequence}", s.handleUploadChunk)
		router.Post("/api/dictations/{sessionID}/stop", s.handleStopDictation)
		router.Post("/api/dictations/{sessionID}/retry", s.handleRetryDictation)
		router.Post("/api/dictations/{sessionID}/retranscribe", s.handleRetranscribeDictation)
		router.Put("/api/dictations/{sessionID}/draft", s.handleSaveTranscriptDraft)
		router.Post("/api/dictations/{sessionID}/transfer", s.handleTransferTranscript)
		router.Get("/api/dictations/{sessionID}/audio", s.handleDictationAudio)
		router.Get("/api/dictations/{sessionID}/audio/content", s.handleDictationAudioContent)
		router.Get("/api/source-segments/{segmentID}/audio", s.handleSegmentAudioSource)
		router.Get("/api/dictations/{sessionID}/live", s.handleLiveWebSocket)
		router.Get("/api/recovery", s.handleRecovery)
		router.Get("/api/preferences", s.handleGetPreferences)
		router.Put("/api/preferences", s.handleUpdatePreferences)
		router.Get("/api/account/backup", s.handleAccountBackup)
		router.Delete("/api/account", s.handlePurgeAccount)

		router.Post("/api/projects/{projectID}/exports", s.handleCreateExport)
		router.Get("/api/exports/{exportID}", s.handleGetExport)
		router.Get("/api/exports/{exportID}/download", s.handleDownloadExport)
	})

	router.Get("/", s.staticHandler().ServeHTTP)
	router.NotFound(s.staticHandler().ServeHTTP)
	return router
}

func (s *Server) handleHealth(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Pool.Ping(ctx); err != nil {
		writeError(response, http.StatusServiceUnavailable, "database_unavailable", "Сервис временно недоступен")
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		response.Header().Set("Permissions-Policy", "microphone=(self)")
		response.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self'; connect-src 'self' ws: wss:; "+
				"img-src 'self' https: data: blob:; media-src 'self' blob:; "+
				"style-src 'self' 'unsafe-inline'; font-src 'self' data:; worker-src 'self' blob:",
		)
		next.ServeHTTP(response, request)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		start := time.Now()
		wrapped := middleware.NewWrapResponseWriter(response, request.ProtoMajor)
		next.ServeHTTP(wrapped, request)
		s.logger.Info(
			"http request",
			"requestId",
			middleware.GetReqID(request.Context()),
			"method",
			request.Method,
			"path",
			request.URL.Path,
			"status",
			wrapped.Status(),
			"durationMs",
			time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "unauthorized", "Войдите, чтобы продолжить")
			return
		}
		user, err := s.store.UserByToken(request.Context(), cookie.Value)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "unauthorized", "Сессия истекла")
			return
		}
		ctx := context.WithValue(request.Context(), userContextKey, user)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func currentUser(ctx context.Context) domain.User {
	user, _ := ctx.Value(userContextKey).(domain.User)
	return user
}

func (s *Server) staticHandler() http.Handler {
	root := http.Dir(s.config.StaticDir)
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writeError(response, http.StatusNotFound, "not_found", "Ресурс не найден")
			return
		}
		cleanPath := filepath.Clean(strings.TrimPrefix(request.URL.Path, "/"))
		if cleanPath == "." {
			cleanPath = "index.html"
		}
		fileInfo, err := fs.Stat(os.DirFS(s.config.StaticDir), cleanPath)
		if err != nil || fileInfo.IsDir() {
			if !acceptsSPAFallback(request, cleanPath) {
				http.NotFound(response, request)
				return
			}
			cleanPath = "index.html"
			cloned := request.Clone(request.Context())
			cloned.URL.Path = "/"
			setStaticCacheHeaders(response, cleanPath)
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			fileServer.ServeHTTP(response, cloned)
			return
		}
		if contentType := mime.TypeByExtension(filepath.Ext(cleanPath)); contentType != "" {
			response.Header().Set("Content-Type", contentType)
		}
		setStaticCacheHeaders(response, cleanPath)
		fileServer.ServeHTTP(response, request)
	})
}

func acceptsSPAFallback(request *http.Request, cleanPath string) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	if filepath.Ext(cleanPath) != "" {
		return false
	}
	return strings.Contains(request.Header.Get("Accept"), "text/html")
}

func setStaticCacheHeaders(response http.ResponseWriter, cleanPath string) {
	if strings.HasPrefix(cleanPath, "assets/") {
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	response.Header().Set("Cache-Control", "no-cache")
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_json", "Проверьте заполненные поля")
		return false
	}
	return true
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func handleStoreError(response http.ResponseWriter, err error) {
	if store.IsNotFound(err) {
		writeError(response, http.StatusNotFound, "not_found", "Ресурс не найден")
		return
	}
	if errors.Is(err, store.ErrInvalidDocumentMove) {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_move",
			"Документ нельзя переместить в выбранный раздел",
		)
		return
	}
	if errors.Is(err, store.ErrInvalidAnnotation) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_annotation", "Проверьте выделение и заметку")
		return
	}
	if errors.Is(err, store.ErrUnknownNodeType) ||
		errors.Is(err, store.ErrInvalidParentType) {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_structure_placement",
			"Этот тип материала нельзя создать в выбранном разделе",
		)
		return
	}
	if errors.Is(err, store.ErrUnauthorized) {
		writeError(response, http.StatusUnauthorized, "unauthorized", "Неверная почта или пароль")
		return
	}
	writeError(response, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие")
}

func newOperationID() string {
	return uuid.NewString()
}
