package httpapi

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
)

type preferencesRequest struct {
	AudioRetentionDays int `json:"audioRetentionDays"`
}

type purgeAccountRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Server) handleGetPreferences(response http.ResponseWriter, request *http.Request) {
	preferences, err := s.store.GetUserPreferences(
		request.Context(),
		currentUser(request.Context()).ID,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"preferences": preferences})
}

func (s *Server) handleUpdatePreferences(response http.ResponseWriter, request *http.Request) {
	var input preferencesRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.AudioRetentionDays != 30 &&
		input.AudioRetentionDays != 90 &&
		input.AudioRetentionDays != 365 {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"invalid_retention",
			"Выберите доступный срок хранения",
		)
		return
	}
	userID := currentUser(request.Context()).ID
	keys, err := s.store.ListExpiredAudioKeys(
		request.Context(),
		userID,
		input.AudioRetentionDays,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	for _, key := range keys {
		if err := s.objects.Delete(request.Context(), key); err != nil {
			writeError(
				response,
				http.StatusServiceUnavailable,
				"retention_storage_failed",
				"Настройка не применена: не удалось безопасно удалить просроченное аудио",
			)
			return
		}
	}
	if err := s.store.DeleteExpiredAudio(
		request.Context(),
		userID,
		input.AudioRetentionDays,
	); err != nil {
		handleStoreError(response, err)
		return
	}
	preferences, err := s.store.UpdateUserPreferences(
		request.Context(),
		userID,
		input.AudioRetentionDays,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"preferences":    preferences,
		"deletedObjects": len(keys),
	})
}

func (s *Server) handleAccountBackup(response http.ResponseWriter, request *http.Request) {
	data, audio, media, err := s.store.BuildAccountBackup(
		request.Context(),
		currentUser(request.Context()).ID,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	metadata, err := archive.Create("backup.json")
	if err != nil {
		writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
		return
	}
	if _, err := metadata.Write(data); err != nil {
		writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
		return
	}
	for _, item := range audio {
		content, err := s.objects.Get(request.Context(), item.StorageKey)
		if err != nil {
			writeError(
				response,
				http.StatusServiceUnavailable,
				"backup_audio_unavailable",
				"Архив не создан: часть исходного аудио временно недоступна",
			)
			return
		}
		extension := strings.TrimPrefix(path.Ext(item.StorageKey), ".")
		if extension == "" {
			extension = "webm"
		}
		file, err := archive.Create(fmt.Sprintf("audio/%s.%s", item.SessionID, extension))
		if err != nil {
			writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
			return
		}
		if _, err := file.Write(content); err != nil {
			writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
			return
		}
	}
	for _, item := range media {
		content, err := s.objects.Get(request.Context(), item.StorageKey)
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "backup_media_unavailable", "Архив не создан: часть изображений временно недоступна")
			return
		}
		extension := path.Ext(item.StorageKey)
		if extension == "" {
			extension = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif"}[item.MediaType]
		}
		file, err := archive.Create("media/" + item.ID + extension)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
			return
		}
		if _, err := file.Write(content); err != nil {
			writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось собрать архив")
			return
		}
	}
	if err := archive.Close(); err != nil {
		writeError(response, http.StatusInternalServerError, "backup_failed", "Не удалось завершить архив")
		return
	}
	response.Header().Set("Content-Type", "application/zip")
	response.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(
			`attachment; filename="writers-studio-backup-%s.zip"`,
			time.Now().Format("2006-01-02"),
		),
	)
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(output.Bytes())
}

func (s *Server) handlePurgeAccount(response http.ResponseWriter, request *http.Request) {
	var input purgeAccountRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	user := currentUser(request.Context())
	plan, err := s.store.GetAccountPurgePlan(request.Context(), user.ID)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if input.Confirmation != plan.Email {
		writeError(
			response,
			http.StatusUnprocessableEntity,
			"confirmation_mismatch",
			"Введите точный адрес электронной почты",
		)
		return
	}
	for _, key := range plan.ObjectKeys {
		if err := s.objects.Delete(request.Context(), key); err != nil {
			writeError(
				response,
				http.StatusServiceUnavailable,
				"purge_storage_failed",
				"Удаление не завершено. Аккаунт и тексты сохранены; повторите попытку",
			)
			return
		}
	}
	if err := s.store.PurgeAccount(request.Context(), user.ID, len(plan.ObjectKeys)); err != nil {
		handleStoreError(response, err)
		return
	}
	http.SetCookie(response, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(response, http.StatusOK, map[string]any{
		"purged": true,
		"notice": "Рабочие данные аккаунта удалены. Инфраструктурные резервные копии могут храниться по политике оператора развёртывания.",
	})
}
