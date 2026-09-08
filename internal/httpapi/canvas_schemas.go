package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/store"
)

type saveCanvasTypeSchemaRequest struct {
	BaseRevision int                           `json:"baseRevision"`
	CardTypes    []canvasdocument.CardType     `json:"cardTypes"`
	Relations    []canvasdocument.RelationType `json:"relationTypes"`
	Source       string                        `json:"source"`
}

func (s *Server) handleGetCanvasTypeSchema(response http.ResponseWriter, request *http.Request) {
	schema, err := s.store.GetCanvasTypeSchema(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"schema": schema})
}

func (s *Server) handleSaveCanvasTypeSchema(response http.ResponseWriter, request *http.Request) {
	var input saveCanvasTypeSchemaRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.BaseRevision < 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_schema", "Проверьте базовую ревизию")
		return
	}
	value := canvasdocument.TypeSchema{
		Revision: input.BaseRevision, CardTypes: input.CardTypes, RelationTypes: input.Relations,
	}
	if err := canvasdocument.ValidateTypeSchema(value); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_schema", err.Error())
		return
	}
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = "manual"
	}
	if len(input.Source) > 80 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_schema", "Проверьте источник изменения")
		return
	}
	result, err := s.store.SaveCanvasTypeSchema(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
		input.BaseRevision, value, input.Source,
	)
	if conflict, ok := store.AsCanvasSchemaConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"code": "canvas_schema_revision_conflict", "message": "Каталог типов уже изменился",
				"currentRevision": conflict.CurrentRevision,
			},
		})
		return
	}
	if errors.Is(err, canvasdocument.ErrInvalidDocument) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_schema", err.Error())
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"schema": result})
}
