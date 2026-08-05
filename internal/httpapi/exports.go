package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"writers.studio/internal/domain"
	exportformat "writers.studio/internal/export"
	"writers.studio/internal/store"
)

type createExportRequest struct {
	Format        string               `json:"format"`
	NodeIDs       []string             `json:"nodeIds"`
	TitlePage     bool                 `json:"titlePage"`
	ChapterTitles bool                 `json:"chapterTitles"`
	IncludeNotes  bool                 `json:"includeNotes"`
	Profile       domain.ExportProfile `json:"profile"`
}

func (s *Server) handleCreateExport(response http.ResponseWriter, request *http.Request) {
	var input createExportRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.Format != "docx" && input.Format != "epub" && input.Format != "pdf" && input.Format != "markdown" && input.Format != "markdown_tree" && input.Format != "txt" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_format", "Выберите формат DOCX, EPUB, PDF, Markdown, Markdown-дерево или TXT")
		return
	}
	job, err := s.store.CreateExport(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
		store.CreateExportInput{
			Format:        input.Format,
			NodeIDs:       input.NodeIDs,
			TitlePage:     input.TitlePage,
			ChapterTitles: input.ChapterTitles,
			IncludeNotes:  input.IncludeNotes,
			Profile:       input.Profile,
		},
	)
	if errors.Is(err, exportformat.ErrInvalidProfile) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_export_profile", "Проверьте настройки оформления экспорта")
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"export": job})
}

func (s *Server) handleGetExport(response http.ResponseWriter, request *http.Request) {
	job, err := s.store.GetExport(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "exportID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if job.Status == "ready" && job.StorageKey != nil {
		url := "/api/exports/" + chi.URLParam(request, "exportID") + "/download"
		job.DownloadURL = &url
	}
	writeJSON(response, http.StatusOK, map[string]any{"export": job})
}

func (s *Server) handleDownloadExport(response http.ResponseWriter, request *http.Request) {
	job, err := s.store.GetExport(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "exportID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if job.Status != "ready" || job.StorageKey == nil {
		writeError(response, http.StatusConflict, "export_not_ready", "Файл ещё не готов")
		return
	}
	data, err := s.objects.Get(request.Context(), *job.StorageKey)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	contentType := map[string]string{
		"docx":          "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"epub":          "application/epub+zip",
		"pdf":           "application/pdf",
		"markdown":      "text/markdown; charset=utf-8",
		"markdown_tree": "application/zip",
		"txt":           "text/plain; charset=utf-8",
	}[job.Format]
	extension := job.Format
	if extension == "markdown" {
		extension = "md"
	} else if extension == "markdown_tree" {
		extension = "zip"
	}
	filename := "publication"
	var snapshot exportformat.Snapshot
	if json.Unmarshal(job.Snapshot, &snapshot) == nil {
		if job.Format == "markdown_tree" {
			filename = exportformat.SafeFilename(snapshot.ProjectTitle) + "-markdown"
		} else {
			filename = exportformat.SafeFilename(snapshot.Profile.Metadata.Title)
		}
		if filename == "publication" {
			filename = exportformat.SafeFilename(snapshot.ProjectTitle)
		}
	}
	response.Header().Set("Content-Type", contentType)
	response.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(
			`attachment; filename="publication.%s"; filename*=UTF-8''%s`,
			extension,
			url.PathEscape(filename+"."+extension),
		),
	)
	response.Header().Set("Cache-Control", "private, no-store")
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}
