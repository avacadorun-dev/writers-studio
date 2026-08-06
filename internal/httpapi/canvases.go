package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/store"
)

type createCanvasRequest struct {
	Title string `json:"title"`
}

func (s *Server) handleImportCanvas(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, canvasdocument.MaxSnapshotBytes)
	content, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(response, http.StatusRequestEntityTooLarge, "canvas_file_too_large", "Файл доски больше 12 МБ")
			return
		}
		writeError(response, http.StatusBadRequest, "canvas_file_unreadable", "Не удалось прочитать файл доски")
		return
	}
	references, err := canvasdocument.Validate(content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_file", err.Error())
		return
	}
	title := strings.TrimSpace(request.URL.Query().Get("title"))
	if title == "" {
		title = strings.TrimSpace(request.Header.Get("X-Canvas-Title"))
	}
	if title == "" {
		title = "Импортированная доска"
	}
	if len([]rune(title)) > 160 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas", "Проверьте название доски")
		return
	}
	canvas, err := s.store.ImportCanvas(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
		store.ImportCanvasInput{
			Title: title, SchemaVersion: references.SchemaVersion,
			Content: content, References: references,
		},
	)
	if errors.Is(err, canvasdocument.ErrInvalidDocument) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_file", err.Error())
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"canvas": canvas})
}

func (s *Server) handleExportCanvas(response http.ResponseWriter, request *http.Request) {
	format := request.URL.Query().Get("format")
	if format != "" && format != "canvas" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_export", "Поддерживается только формат canvas")
		return
	}
	workspace, err := s.store.GetCanvasWorkspace(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set(
		"Content-Disposition", fmt.Sprintf(`attachment; filename="canvas-%s.canvas"`, workspace.Canvas.ID),
	)
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(workspace.Revision.Content)
}

func (s *Server) handleListCanvases(response http.ResponseWriter, request *http.Request) {
	includeTrashed := false
	if value := request.URL.Query().Get("includeTrashed"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_query", "Проверьте параметры списка досок")
			return
		}
		includeTrashed = parsed
	}
	canvases, err := s.store.ListCanvases(
		request.Context(), currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"), includeTrashed,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"canvases": canvases})
}

func (s *Server) handleCreateCanvas(response http.ResponseWriter, request *http.Request) {
	var input createCanvasRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len([]rune(input.Title)) > 160 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas", "Проверьте название доски")
		return
	}
	canvas, err := s.store.CreateCanvas(
		request.Context(), currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"), input.Title,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"canvas": canvas})
}

func (s *Server) handleGetCanvas(response http.ResponseWriter, request *http.Request) {
	workspace, err := s.store.GetCanvasWorkspace(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, workspace)
}

type updateCanvasRequest struct {
	Title    *string  `json:"title"`
	Position *float64 `json:"position"`
}

func (s *Server) handleUpdateCanvas(response http.ResponseWriter, request *http.Request) {
	var input updateCanvasRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.Title == nil && input.Position == nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas", "Нет изменений")
		return
	}
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" || len([]rune(trimmed)) > 160 {
			writeError(response, http.StatusUnprocessableEntity, "invalid_canvas", "Проверьте название доски")
			return
		}
		input.Title = &trimmed
	}
	if input.Position != nil && (math.IsNaN(*input.Position) || math.IsInf(*input.Position, 0) ||
		*input.Position < -1e12 || *input.Position > 1e12) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas", "Проверьте положение доски")
		return
	}
	canvas, err := s.store.UpdateCanvas(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
		store.UpdateCanvasInput{Title: input.Title, Position: input.Position},
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"canvas": canvas})
}

func (s *Server) handleDuplicateCanvas(response http.ResponseWriter, request *http.Request) {
	canvas, err := s.store.DuplicateCanvas(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"canvas": canvas})
}

func (s *Server) handleTrashCanvas(response http.ResponseWriter, request *http.Request) {
	canvas, err := s.store.TrashCanvas(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"canvas": canvas})
}

func (s *Server) handleRestoreCanvas(response http.ResponseWriter, request *http.Request) {
	canvas, err := s.store.RestoreCanvas(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"canvas": canvas})
}

type saveCanvasRevisionRequest struct {
	BaseRevision int             `json:"baseRevision"`
	Content      json.RawMessage `json:"content"`
	Source       string          `json:"source"`
}

func (s *Server) handleSaveCanvasRevision(response http.ResponseWriter, request *http.Request) {
	var input saveCanvasRevisionRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	references, err := canvasdocument.Validate(input.Content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_document", err.Error())
		return
	}
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = "manual"
	}
	if len(input.Source) > 80 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_document", "Проверьте источник изменения")
		return
	}
	revision, err := s.store.SaveCanvasRevision(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
		store.SaveCanvasRevisionInput{
			BaseRevision: input.BaseRevision, SchemaVersion: references.SchemaVersion,
			Content: input.Content, Source: input.Source, References: references,
		},
	)
	if conflict, ok := store.AsCanvasConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"code": "canvas_revision_conflict", "message": "Мы сохранили оба варианта доски",
				"currentRevision":  conflict.CurrentRevision,
				"conflictCanvasId": conflict.ConflictCanvasID,
			},
		})
		return
	}
	if errors.Is(err, canvasdocument.ErrInvalidDocument) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_document", err.Error())
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"revision": revision})
}
