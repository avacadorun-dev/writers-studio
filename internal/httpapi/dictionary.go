package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type dictionaryTermRequest struct {
	Canonical string   `json:"canonical"`
	Variants  []string `json:"variants"`
	Kind      string   `json:"kind"`
	Language  string   `json:"language"`
	Active    *bool    `json:"active"`
}

func (s *Server) handleListDictionary(response http.ResponseWriter, request *http.Request) {
	terms, err := s.store.ListDictionaryTerms(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"terms": terms})
}

func (s *Server) handleCreateDictionaryTerm(response http.ResponseWriter, request *http.Request) {
	var input dictionaryTermRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	normalizeTermRequest(&input)
	if message := validateTerm(input); message != "" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_term", message)
		return
	}
	term, err := s.store.CreateDictionaryTerm(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.Canonical,
		input.Kind,
		input.Language,
		input.Variants,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"term":   term,
		"notice": "Словарь повлияет на будущие расшифровки. Уже написанный текст не изменится.",
	})
}

func (s *Server) handleUpdateDictionaryTerm(response http.ResponseWriter, request *http.Request) {
	var input dictionaryTermRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	normalizeTermRequest(&input)
	if message := validateTerm(input); message != "" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_term", message)
		return
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	term, err := s.store.UpdateDictionaryTerm(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "termID"),
		input.Canonical,
		input.Kind,
		input.Language,
		input.Variants,
		active,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"term": term})
}

func (s *Server) handleDeleteDictionaryTerm(response http.ResponseWriter, request *http.Request) {
	if err := s.store.DeleteDictionaryTerm(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "termID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"operationId": newOperationID(),
		"undoable":    true,
	})
}

func normalizeTermRequest(input *dictionaryTermRequest) {
	input.Canonical = strings.TrimSpace(input.Canonical)
	seen := make(map[string]bool)
	variants := make([]string, 0, len(input.Variants))
	for _, variant := range input.Variants {
		variant = strings.TrimSpace(variant)
		key := strings.ToLower(variant)
		if variant != "" && !seen[key] {
			seen[key] = true
			variants = append(variants, variant)
		}
	}
	input.Variants = variants
}

func validateTerm(input dictionaryTermRequest) string {
	if input.Canonical == "" || len([]rune(input.Canonical)) > 160 {
		return "Укажите термин не длиннее 160 символов"
	}
	if !map[string]bool{
		"character": true, "place": true, "term": true, "other": true,
	}[input.Kind] {
		return "Выберите тип термина"
	}
	if input.Language != "ru" && input.Language != "en" {
		return "Выберите язык"
	}
	return ""
}
