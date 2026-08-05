package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"writers.studio/internal/store"
)

type createMCPGrantRequest struct {
	Name        string   `json:"name"`
	ClientName  string   `json:"clientName"`
	ProjectIDs  []string `json:"projectIds"`
	Scopes      []string `json:"scopes"`
	ExpiresDays int      `json:"expiresDays"`
}

type rotateMCPGrantRequest struct {
	ExpiresDays int `json:"expiresDays"`
}

func (s *Server) mountMCPGrantRoutes(router chi.Router) {
	router.Get("/api/mcp/grants", s.handleListMCPGrants)
	router.Post("/api/mcp/grants", s.handleCreateMCPGrant)
	router.Post("/api/mcp/grants/{grantID}/rotate", s.handleRotateMCPGrant)
	router.Delete("/api/mcp/grants/{grantID}", s.handleRevokeMCPGrant)
}

func (s *Server) handleListMCPGrants(response http.ResponseWriter, request *http.Request) {
	grants, err := s.store.ListMCPGrants(request.Context(), currentUser(request.Context()).ID)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"enabled":  s.config.MCPEnabled,
		"endpoint": s.config.PublicBaseURL + "/mcp",
		"grants":   grants,
	})
}

func (s *Server) handleCreateMCPGrant(response http.ResponseWriter, request *http.Request) {
	if !s.config.MCPEnabled {
		writeError(response, http.StatusNotFound, "mcp_disabled", "Удалённый MCP выключен")
		return
	}
	var input createMCPGrantRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	grant, secret, err := s.store.CreateMCPGrant(
		request.Context(), currentUser(request.Context()).ID,
		store.CreateMCPGrantInput{
			Name: input.Name, ClientName: input.ClientName, ProjectIDs: input.ProjectIDs,
			Scopes: input.Scopes, ExpiresIn: time.Duration(input.ExpiresDays) * 24 * time.Hour,
			AuthKind: "manual",
		},
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Pragma", "no-cache")
	writeJSON(response, http.StatusCreated, map[string]any{
		"grant":    grant,
		"token":    secret,
		"endpoint": s.config.PublicBaseURL + "/mcp",
		"config": map[string]any{
			"url":     s.config.PublicBaseURL + "/mcp",
			"headers": map[string]string{"Authorization": "Bearer " + secret},
		},
	})
}

func (s *Server) handleRotateMCPGrant(response http.ResponseWriter, request *http.Request) {
	var input rotateMCPGrantRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	grant, secret, err := s.store.RotateMCPGrant(
		request.Context(), currentUser(request.Context()).ID,
		chi.URLParam(request, "grantID"), time.Duration(input.ExpiresDays)*24*time.Hour,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Pragma", "no-cache")
	writeJSON(response, http.StatusOK, map[string]any{
		"grant": grant, "token": secret,
		"endpoint": s.config.PublicBaseURL + "/mcp",
	})
}

func (s *Server) handleRevokeMCPGrant(response http.ResponseWriter, request *http.Request) {
	if err := s.store.RevokeMCPGrant(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "grantID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
