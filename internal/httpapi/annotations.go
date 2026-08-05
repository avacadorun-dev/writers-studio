package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"writers.studio/internal/domain"
	"writers.studio/internal/store"
)

type createAnnotationRequest struct {
	Kind             string                  `json:"kind"`
	DictionaryTermID *string                 `json:"dictionaryTermId"`
	Anchor           domain.AnnotationAnchor `json:"anchor"`
	Note             string                  `json:"note"`
	BaseRevision     int                     `json:"baseRevision"`
}

func (s *Server) handleListProjectAnnotations(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListProjectAnnotations(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"annotations": items})
}

func (s *Server) handleListDocumentAnnotations(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListDocumentAnnotations(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "nodeID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"annotations": items})
}

func (s *Server) handleCreateDocumentAnnotation(response http.ResponseWriter, request *http.Request) {
	var input createAnnotationRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Kind = strings.TrimSpace(input.Kind)
	input.Note = strings.TrimSpace(input.Note)
	item, err := s.store.CreateDocumentAnnotation(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "nodeID"),
		store.CreateAnnotationInput{
			Kind: input.Kind, DictionaryTermID: input.DictionaryTermID, Anchor: input.Anchor,
			Note: input.Note, BaseRevision: input.BaseRevision,
		},
	)
	if conflict, ok := store.AsConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{"error": map[string]any{
			"code": "revision_conflict", "message": "Текст изменился — выделите фрагмент снова",
			"currentRevision": conflict.CurrentRevision,
		}})
		return
	}
	if errors.Is(err, store.ErrInvalidAnnotation) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_annotation", "Проверьте выделение и текст заметки")
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"annotation": item})
}

type updateAnnotationRequest struct {
	Note   *string                  `json:"note"`
	Status *string                  `json:"status"`
	Anchor *domain.AnnotationAnchor `json:"anchor"`
}

func (s *Server) handleUpdateDocumentAnnotation(response http.ResponseWriter, request *http.Request) {
	var input updateAnnotationRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	item, err := s.store.UpdateDocumentAnnotation(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "annotationID"),
		store.UpdateAnnotationInput{Note: input.Note, Status: input.Status, Anchor: input.Anchor},
	)
	if errors.Is(err, store.ErrInvalidAnnotation) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_annotation", "Проверьте изменения заметки")
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"annotation": item})
}

func (s *Server) handleDeleteDocumentAnnotation(response http.ResponseWriter, request *http.Request) {
	if err := s.store.DeleteDocumentAnnotation(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "annotationID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
