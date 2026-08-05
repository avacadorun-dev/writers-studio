package httpapi

import (
	"context"
	"net/http"
	"net/mail"
	"strings"

	"writers.studio/internal/stt"
)

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(response http.ResponseWriter, request *http.Request) {
	s.authenticate(response, request, false)
}

func (s *Server) handleRegister(response http.ResponseWriter, request *http.Request) {
	s.authenticate(response, request, true)
}

func (s *Server) authenticate(response http.ResponseWriter, request *http.Request, register bool) {
	var input authRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if _, err := mail.ParseAddress(input.Email); err != nil || len(input.Password) < 8 {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_credentials",
			"Укажите корректную почту и пароль не короче 8 символов",
		)
		return
	}

	user, token, err := s.store.Authenticate(
		request.Context(),
		input.Email,
		input.Password,
		s.config.SessionTTL,
		register,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	http.SetCookie(response, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.config.AppEnv == "production",
		MaxAge:   int(s.config.SessionTTL.Seconds()),
	})
	writeJSON(response, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(response http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(request.Context(), cookie.Value)
	}
	http.SetCookie(response, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	response.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{"user": currentUser(request.Context())})
}

func (s *Server) handleSTTCapabilities(response http.ResponseWriter, request *http.Request) {
	finalCapabilities := s.transcriber.Capabilities(request.Context())
	var liveCapabilities *stt.Capabilities
	if s.liveSTT != nil {
		value := s.liveSTT.Capabilities(request.Context())
		liveCapabilities = &value
	}
	capabilities := combineSTTCapabilities(
		finalCapabilities,
		liveCapabilities,
		s.config.LiveSTTAdapter,
	)
	writeJSON(response, http.StatusOK, capabilities)
}

func combineSTTCapabilities(
	final stt.Capabilities,
	live *stt.Capabilities,
	liveAdapter string,
) stt.Capabilities {
	fileAvailable := final.Available && containsString(final.Modes, "file")
	fileOnly := func() stt.Capabilities {
		if !fileAvailable {
			return stt.NewUnavailable().Capabilities(context.Background())
		}
		result := final
		result.Modes = []string{"file"}
		result.StreamingProtocol = "unavailable"
		return result
	}

	switch liveAdapter {
	case "deepgram":
		if live == nil || !live.Available {
			return fileOnly()
		}
		result := *live
		result.Modes = []string{"realtime"}
		if fileAvailable {
			result.Modes = append(result.Modes, "file")
			result.SupportsLongAudio = result.SupportsLongAudio || final.SupportsLongAudio
		}
		return result
	case "openrouter":
		if !final.Available {
			return fileOnly()
		}
		result := final
		result.Modes = []string{"chunked_live", "file"}
		result.StreamingProtocol = "wav_chunks"
		return result
	case "fake":
		if !final.Available {
			return fileOnly()
		}
		result := final
		result.Modes = []string{"realtime", "file"}
		result.StreamingProtocol = "fixture"
		return result
	default:
		return fileOnly()
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
