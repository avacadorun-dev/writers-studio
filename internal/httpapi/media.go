package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/store"
)

const maxProjectImageBytes = 12 << 20

var supportedProjectImages = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
}

func (s *Server) handleUploadProjectMedia(response http.ResponseWriter, request *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(response, request.Body, maxProjectImageBytes))
	if err != nil {
		writeError(response, http.StatusRequestEntityTooLarge, "image_too_large", "Изображение больше 12 МБ")
		return
	}
	if len(data) == 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_image", "Выберите непустой файл изображения")
		return
	}
	mediaType := http.DetectContentType(data)
	extension, supported := supportedProjectImages[mediaType]
	if !supported {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_image", "Поддерживаются PNG, JPEG и GIF")
		return
	}
	configuration, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 ||
		configuration.Width > 20_000 || configuration.Height > 20_000 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_image", "Файл изображения повреждён или имеет недопустимый размер")
		return
	}
	projectID := chi.URLParam(request, "projectID")
	storageKey := path.Join("media", projectID, uuid.NewString()+extension)
	checksum := sha256.Sum256(data)
	if err := s.objects.Put(request.Context(), storageKey, mediaType, data); err != nil {
		writeError(response, http.StatusServiceUnavailable, "image_storage_unavailable", "Не удалось сохранить изображение")
		return
	}
	media, err := s.store.CreateProjectMedia(
		request.Context(),
		currentUser(request.Context()).ID,
		store.CreateProjectMediaInput{
			ProjectID:    projectID,
			MediaType:    mediaType,
			OriginalName: safeUploadedFileName(request.Header.Get("X-File-Name"), extension),
			SHA256:       hex.EncodeToString(checksum[:]),
			ByteSize:     int64(len(data)),
			Width:        configuration.Width,
			Height:       configuration.Height,
			StorageKey:   storageKey,
		},
	)
	if err != nil {
		if cleanupErr := s.objects.Delete(request.Context(), storageKey); cleanupErr != nil {
			s.logger.Warn("cleanup failed media upload", "storageKey", storageKey)
		}
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"media": media})
}

func (s *Server) handleProjectMediaContent(response http.ResponseWriter, request *http.Request) {
	media, err := s.store.GetProjectMedia(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "mediaID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if request.Header.Get("If-None-Match") == `"`+media.SHA256+`"` {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	data, err := s.objects.Get(request.Context(), media.StorageKey)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "image_unavailable", "Изображение временно недоступно")
		return
	}
	checksum := sha256.Sum256(data)
	if int64(len(data)) != media.ByteSize || hex.EncodeToString(checksum[:]) != media.SHA256 {
		writeError(response, http.StatusConflict, "image_checksum_mismatch", "Проверка целостности изображения не пройдена")
		return
	}
	response.Header().Set("Content-Type", media.MediaType)
	response.Header().Set("Content-Length", strconv.Itoa(len(data)))
	response.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	response.Header().Set("ETag", `"`+media.SHA256+`"`)
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}

func safeUploadedFileName(headerValue, extension string) string {
	value, err := url.QueryUnescape(headerValue)
	if err != nil {
		value = ""
	}
	value = path.Base(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "image" + extension
	}
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	return value
}
