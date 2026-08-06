package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/store"
)

type promoteCanvasNodeRequest struct {
	BaseRevision int     `json:"baseRevision"`
	NodeID       string  `json:"nodeId"`
	Title        string  `json:"title"`
	Kind         string  `json:"kind"`
	ParentID     *string `json:"parentId"`
}

func (s *Server) handlePromoteCanvasNode(response http.ResponseWriter, request *http.Request) {
	var input promoteCanvasNodeRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.NodeID = strings.TrimSpace(input.NodeID)
	input.Title = strings.TrimSpace(input.Title)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.BaseRevision < 1 || input.NodeID == "" || input.Title == "" || input.Kind == "" ||
		len([]rune(input.Title)) > 200 || len(input.Kind) > 80 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_promotion", "Проверьте параметры документа")
		return
	}
	result, err := s.store.PromoteCanvasNode(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "canvasID"),
		store.PromoteCanvasNodeInput{
			BaseRevision: input.BaseRevision, NodeID: input.NodeID, Title: input.Title,
			Kind: input.Kind, ParentID: input.ParentID,
		},
	)
	if conflict, ok := store.AsCanvasPromotionConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"code": "canvas_promotion_stale", "message": "Доска уже изменилась",
				"currentRevision": conflict.CurrentRevision,
			},
		})
		return
	}
	if errors.Is(err, canvasdocument.ErrInvalidDocument) ||
		errors.Is(err, store.ErrInvalidParentType) || errors.Is(err, store.ErrUnknownNodeType) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_canvas_promotion", err.Error())
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, result)
}
