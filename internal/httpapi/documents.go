package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/editor"
	"writers.studio/internal/store"
)

type createNodeRequest struct {
	ParentID *string `json:"parentId"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
}

func (s *Server) handleCreateNode(response http.ResponseWriter, request *http.Request) {
	var input createNodeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind == "" || len([]rune(input.Kind)) > 80 ||
		input.Title == "" || len([]rune(input.Title)) > 200 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_node", "Проверьте тип и название")
		return
	}
	node, err := s.store.CreateDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.ParentID,
		input.Kind,
		input.Title,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"node": node})
}

func (s *Server) handleGetNode(response http.ResponseWriter, request *http.Request) {
	node, err := s.store.GetDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"node": node})
}

type updateNodeRequest struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
}

func (s *Server) handleUpdateNode(response http.ResponseWriter, request *http.Request) {
	var input updateNodeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.Title == nil && input.Status == nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_node", "Нет изменений")
		return
	}
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		input.Title = &trimmed
		if trimmed == "" || len([]rune(trimmed)) > 200 {
			writeError(response, http.StatusUnprocessableEntity, "invalid_node", "Проверьте название")
			return
		}
	}
	if input.Status != nil && !map[string]bool{
		"idea": true, "draft": true, "editing": true, "ready": true,
	}[*input.Status] {
		writeError(response, http.StatusUnprocessableEntity, "invalid_node", "Проверьте статус")
		return
	}
	node, err := s.store.UpdateDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
		input.Title,
		input.Status,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"node": node})
}

func (s *Server) handleDuplicateNode(response http.ResponseWriter, request *http.Request) {
	node, err := s.store.DuplicateDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"node": node})
}

type saveRevisionRequest struct {
	BaseRevision      int                            `json:"baseRevision"`
	Content           json.RawMessage                `json:"content"`
	Source            string                         `json:"source"`
	OperationID       *string                        `json:"operationId"`
	AnnotationAnchors []store.AnnotationAnchorUpdate `json:"annotationAnchors"`
}

func (s *Server) handleSaveRevision(response http.ResponseWriter, request *http.Request) {
	var input saveRevisionRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if err := editor.Validate(input.Content); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	schemaVersion, err := editor.SchemaVersion(input.Content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	plainText, err := editor.PlainText(input.Content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	if input.Source == "" {
		input.Source = "manual"
	}

	revision, err := s.store.SaveRevision(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
		store.SaveRevisionInput{
			BaseRevision:      input.BaseRevision,
			SchemaVersion:     schemaVersion,
			Content:           input.Content,
			PlainText:         plainText,
			Source:            input.Source,
			OperationID:       input.OperationID,
			AnnotationAnchors: input.AnnotationAnchors,
		},
	)
	if conflict, ok := store.AsConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"code":            "revision_conflict",
				"message":         "Мы сохранили оба варианта",
				"currentRevision": conflict.CurrentRevision,
				"conflictNodeId":  conflict.ConflictNodeID,
			},
		})
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"revision": revision})
}

func (s *Server) handleListRevisions(response http.ResponseWriter, request *http.Request) {
	revisions, err := s.store.ListRevisions(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"revisions": revisions})
}

type moveNodeRequest struct {
	ParentID    *string `json:"parentId"`
	Position    float64 `json:"position"`
	TargetIndex *int    `json:"targetIndex"`
}

func (s *Server) handleMoveNode(response http.ResponseWriter, request *http.Request) {
	var input moveNodeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	var err error
	if input.TargetIndex != nil {
		if *input.TargetIndex < 0 {
			writeError(response, http.StatusUnprocessableEntity, "invalid_move", "Некорректная позиция материала")
			return
		}
		err = s.store.MoveDocumentNodeToIndex(
			request.Context(), currentUser(request.Context()).ID,
			chi.URLParam(request, "nodeID"), input.ParentID, *input.TargetIndex,
		)
	} else {
		err = s.store.MoveDocumentNode(
			request.Context(), currentUser(request.Context()).ID,
			chi.URLParam(request, "nodeID"), input.ParentID, input.Position,
		)
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"operationId": newOperationID(),
		"undoable":    true,
	})
}

func (s *Server) handleTrashNode(response http.ResponseWriter, request *http.Request) {
	if err := s.store.TrashDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"operationId": newOperationID(),
		"undoable":    true,
	})
}

func (s *Server) handleRestoreNode(response http.ResponseWriter, request *http.Request) {
	if err := s.store.RestoreDocumentNode(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"restored": true})
}

type createVersionRequest struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

func (s *Server) handleCreateVersion(response http.ResponseWriter, request *http.Request) {
	var input createVersionRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 160 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_version", "Укажите название версии")
		return
	}
	version, err := s.store.CreateNamedVersion(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
		input.Name,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"version": version})
}

func (s *Server) handleListVersions(response http.ResponseWriter, request *http.Request) {
	versions, err := s.store.ListNamedVersions(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	revisions, err := s.store.ListRevisions(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"versions":  versions,
		"revisions": revisions,
	})
}

func (s *Server) handleRestoreVersion(response http.ResponseWriter, request *http.Request) {
	revision, err := s.store.RestoreVersion(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "versionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"revision": revision,
		"undoable": true,
	})
}

func (s *Server) handleSearchProject(response http.ResponseWriter, request *http.Request) {
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	scope := request.URL.Query().Get("scope")
	if scope == "" {
		scope = "all"
	}
	results, err := s.store.SearchProject(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		query,
		scope,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"results": results})
}

type projectReplaceRequest struct {
	Search       string `json:"search"`
	Replace      string `json:"replace"`
	Scope        string `json:"scope"`
	PreviewToken string `json:"previewToken"`
}

func (s *Server) handlePreviewProjectReplace(response http.ResponseWriter, request *http.Request) {
	var input projectReplaceRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if !validReplaceRequest(input) {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_replace",
			"Проверьте текст и область замены",
		)
		return
	}
	preview, err := s.store.PreviewProjectReplace(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.Search,
		input.Replace,
		input.Scope,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"preview": preview})
}

func (s *Server) handleApplyProjectReplace(response http.ResponseWriter, request *http.Request) {
	var input projectReplaceRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if !validReplaceRequest(input) || len(input.PreviewToken) != 64 {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_replace",
			"Сначала обновите предварительный просмотр",
		)
		return
	}
	result, err := s.store.ApplyProjectReplace(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.Search,
		input.Replace,
		input.Scope,
		input.PreviewToken,
	)
	if errors.Is(err, store.ErrReplacePreviewStale) {
		writeError(
			response,
			http.StatusConflict,
			"replace_preview_stale",
			"Документы изменились. Обновите предварительный просмотр",
		)
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"result":   result,
		"undoable": true,
	})
}

func (s *Server) handleUndoProjectReplace(response http.ResponseWriter, request *http.Request) {
	operationID := chi.URLParam(request, "operationID")
	if _, err := uuid.Parse(operationID); err != nil {
		writeError(response, http.StatusNotFound, "operation_not_found", "Операция не найдена")
		return
	}
	result, err := s.store.UndoProjectReplace(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		operationID,
	)
	if errors.Is(err, store.ErrReplacePreviewStale) {
		writeError(
			response,
			http.StatusConflict,
			"replace_undo_stale",
			"После замены появились новые правки. Восстановите редакции через историю",
		)
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"result": result})
}

func validReplaceRequest(input projectReplaceRequest) bool {
	search := strings.TrimSpace(input.Search)
	return search != "" &&
		len([]rune(search)) <= 500 &&
		len([]rune(input.Replace)) <= 5_000 &&
		map[string]bool{
			"all": true, "manuscript": true, "drafts": true, "notes": true,
		}[input.Scope]
}

func isInvalidDocument(err error) bool {
	return errors.Is(err, editor.ErrInvalidDocument)
}
