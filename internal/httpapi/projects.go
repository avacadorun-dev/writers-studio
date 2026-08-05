package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
	"writers.studio/internal/store"
)

type exportProfileRequest struct {
	BaseRevision int                  `json:"baseRevision"`
	Profile      domain.ExportProfile `json:"profile"`
}

func (s *Server) handleGetExportProfile(response http.ResponseWriter, request *http.Request) {
	profile, revision, err := s.store.GetExportProfile(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"profile": profile, "revision": revision})
}

func (s *Server) handleSaveExportProfile(response http.ResponseWriter, request *http.Request) {
	var input exportProfileRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	profile, revision, err := s.store.SaveExportProfile(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
		input.BaseRevision, input.Profile,
	)
	if errors.Is(err, store.ErrExportProfileStale) {
		writeError(response, http.StatusConflict, "export_profile_stale", "Профиль экспорта изменён в другой вкладке")
		return
	}
	if errors.Is(err, exportformat.ErrInvalidProfile) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_export_profile", "Проверьте настройки оформления экспорта")
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"profile": profile, "revision": revision})
}

func (s *Server) handleListProjectMedia(response http.ResponseWriter, request *http.Request) {
	media, err := s.store.ListProjectMedia(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"media": media})
}

type createProjectRequest struct {
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Synopsis string `json:"synopsis"`
	WordGoal *int   `json:"wordGoal"`
}

func (s *Server) handleListProjects(response http.ResponseWriter, request *http.Request) {
	status := request.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" && status != "trash" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_status", "Неизвестный статус проекта")
		return
	}
	projects, err := s.store.ListProjects(request.Context(), currentUser(request.Context()).ID, status)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleCreateProject(response http.ResponseWriter, request *http.Request) {
	var input createProjectRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if message := validateProject(input); message != "" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_project", message)
		return
	}

	workspace, err := s.store.CreateProject(
		request.Context(),
		currentUser(request.Context()).ID,
		store.CreateProjectInput{
			Title:    input.Title,
			Kind:     input.Kind,
			Language: input.Language,
			Synopsis: strings.TrimSpace(input.Synopsis),
			WordGoal: input.WordGoal,
		},
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, workspace)
}

func (s *Server) handleGetProject(response http.ResponseWriter, request *http.Request) {
	workspace, err := s.store.GetProject(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, workspace)
}

func (s *Server) handleGetProjectOutline(response http.ResponseWriter, request *http.Request) {
	workspace, err := s.store.GetProjectOutline(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, workspace)
}

type updateProjectRequest struct {
	Title    string `json:"title"`
	Language string `json:"language"`
	Synopsis string `json:"synopsis"`
	WordGoal *int   `json:"wordGoal"`
}

func (s *Server) handleUpdateProject(response http.ResponseWriter, request *http.Request) {
	var input updateProjectRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Synopsis = strings.TrimSpace(input.Synopsis)
	if message := validateProjectUpdate(input); message != "" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_project", message)
		return
	}
	project, err := s.store.UpdateProject(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		store.UpdateProjectInput{
			Title: input.Title, Language: input.Language,
			Synopsis: input.Synopsis, WordGoal: input.WordGoal,
		},
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"project": project})
}

type projectAssistantSettingsRequest struct {
	StepLimit *int `json:"stepLimit"`
}

func (s *Server) handleGetProjectAssistantSettings(
	response http.ResponseWriter,
	request *http.Request,
) {
	settings, err := s.store.GetProjectAssistantSettings(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleUpdateProjectAssistantSettings(
	response http.ResponseWriter,
	request *http.Request,
) {
	var input projectAssistantSettingsRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if !store.ValidProjectAssistantStepLimit(input.StepLimit) {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_assistant_step_limit",
			"Лимит должен быть от 1 до 1 000 000 шагов или не задан",
		)
		return
	}
	settings, err := s.store.UpdateProjectAssistantSettings(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		store.ProjectAssistantSettings{StepLimit: input.StepLimit},
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"settings": settings})
}

type projectStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleProjectStatus(response http.ResponseWriter, request *http.Request) {
	var input projectStatusRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.Status != "active" && input.Status != "archived" && input.Status != "trash" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_status", "Неизвестный статус проекта")
		return
	}
	project, err := s.store.UpdateProjectStatus(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		input.Status,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"project": project})
}

type purgeProjectRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Server) handlePurgeProject(response http.ResponseWriter, request *http.Request) {
	var input purgeProjectRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	projectID := chi.URLParam(request, "projectID")
	plan, err := s.store.GetProjectPurgePlan(
		request.Context(),
		currentUser(request.Context()).ID,
		projectID,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if input.Confirmation != plan.Title {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"confirmation_mismatch",
			"Введите точное название проекта",
		)
		return
	}
	for _, key := range plan.ObjectKeys {
		if err := s.objects.Delete(request.Context(), key); err != nil {
			writeError(
				response,
				http.StatusServiceUnavailable,
				"purge_storage_failed",
				"Удаление не завершено. Проект остаётся в корзине; повторите попытку",
			)
			return
		}
	}
	if err := s.store.PurgeTrashedProject(
		request.Context(),
		currentUser(request.Context()).ID,
		projectID,
		len(plan.ObjectKeys),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"purged": true,
		"notice": "Рабочие данные удалены. Срок хранения инфраструктурных резервных копий определяется оператором развёртывания.",
	})
}

func validateProject(input createProjectRequest) string {
	if len([]rune(input.Title)) < 1 || len([]rune(input.Title)) > 120 {
		return "Название должно содержать от 1 до 120 символов"
	}
	kinds := map[string]bool{
		"novel": true, "story": true, "screenplay": true, "nonfiction": true,
		"game_design": true, "free": true, "custom": true,
	}
	if !kinds[input.Kind] {
		return "Выберите тип проекта"
	}
	if input.Language != "ru" && input.Language != "en" {
		return "Выберите язык диктовки"
	}
	if input.WordGoal != nil && (*input.WordGoal < 1 || *input.WordGoal > 10_000_000) {
		return "Цель должна быть от 1 до 10 000 000 слов"
	}
	return ""
}

func validateProjectUpdate(input updateProjectRequest) string {
	if len([]rune(input.Title)) < 1 || len([]rune(input.Title)) > 120 {
		return "Название должно содержать от 1 до 120 символов"
	}
	if input.Language != "ru" && input.Language != "en" {
		return "Выберите язык диктовки"
	}
	if input.WordGoal != nil && (*input.WordGoal < 1 || *input.WordGoal > 10_000_000) {
		return "Цель должна быть от 1 до 10 000 000 слов"
	}
	return ""
}
