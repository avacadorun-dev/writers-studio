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
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/store"
)

const (
	maxProjectImageBytes = 12 << 20
	maxProjectPDFBytes   = 25 << 20
	maxProjectMediaBytes = 100 << 20
)

var supportedProjectImages = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
}

var supportedProjectMedia = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
	"audio/mpeg":      ".mp3",
	"audio/mp4":       ".m4a",
	"audio/ogg":       ".ogg",
	"audio/wav":       ".wav",
	"video/mp4":       ".mp4",
	"video/webm":      ".webm",
}

func (s *Server) handleUploadProjectMedia(response http.ResponseWriter, request *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(response, request.Body, maxProjectMediaBytes))
	if err != nil {
		writeError(response, http.StatusRequestEntityTooLarge, "media_too_large", "Файл больше 100 МБ")
		return
	}
	if len(data) == 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_media", "Выберите непустой файл")
		return
	}
	mediaType := normalizedUploadedMediaType(http.DetectContentType(data), request.Header.Get("Content-Type"), data)
	extension, supported := supportedProjectMedia[mediaType]
	if !supported {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media", "Поддерживаются изображения, PDF, MP3/M4A/OGG/WAV и MP4/WebM")
		return
	}
	var width, height *int
	if _, imageMedia := supportedProjectImages[mediaType]; imageMedia {
		if len(data) > maxProjectImageBytes {
			writeError(response, http.StatusRequestEntityTooLarge, "image_too_large", "Изображение больше 12 МБ")
			return
		}
		configuration, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil || configuration.Width < 1 || configuration.Height < 1 ||
			configuration.Width > 20_000 || configuration.Height > 20_000 {
			writeError(response, http.StatusUnprocessableEntity, "invalid_image", "Файл изображения повреждён или имеет недопустимый размер")
			return
		}
		widthValue, heightValue := configuration.Width, configuration.Height
		width, height = &widthValue, &heightValue
	} else if mediaType == "application/pdf" && !validPDF(data) {
		writeError(response, http.StatusUnprocessableEntity, "invalid_pdf", "PDF повреждён или имеет недопустимый формат")
		return
	} else if strings.HasPrefix(mediaType, "audio/") && len(data) > 50<<20 {
		writeError(response, http.StatusRequestEntityTooLarge, "audio_too_large", "Аудио больше 50 МБ")
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
			Width:        width,
			Height:       height,
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

func normalizedUploadedMediaType(detected, declared string, data []byte) string {
	declared = strings.TrimSpace(strings.SplitN(declared, ";", 2)[0])
	if detected == "application/ogg" && (declared == "audio/ogg" || declared == "video/ogg") {
		return "audio/ogg"
	}
	if detected == "audio/x-wav" || detected == "audio/wave" {
		return "audio/wav"
	}
	if detected == "application/octet-stream" && len(data) >= 12 && string(data[4:8]) == "ftyp" {
		if declared == "audio/mp4" {
			return declared
		}
		return "video/mp4"
	}
	if detected == "video/mp4" && declared == "audio/mp4" {
		return declared
	}
	return detected
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
		writeError(response, http.StatusServiceUnavailable, "media_unavailable", "Файл временно недоступен")
		return
	}
	checksum := sha256.Sum256(data)
	if int64(len(data)) != media.ByteSize || hex.EncodeToString(checksum[:]) != media.SHA256 {
		writeError(response, http.StatusConflict, "media_checksum_mismatch", "Проверка целостности файла не пройдена")
		return
	}
	response.Header().Set("Content-Type", media.MediaType)
	response.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	response.Header().Set("ETag", `"`+media.SHA256+`"`)
	response.Header().Set("Accept-Ranges", "bytes")
	response.Header().Set(
		"Content-Disposition",
		"inline; filename*=UTF-8''"+strings.ReplaceAll(url.QueryEscape(media.OriginalName), "+", "%20"),
	)
	http.ServeContent(response, request, media.OriginalName, media.CreatedAt, bytes.NewReader(data))
}

func validPDF(data []byte) bool {
	if len(data) < 12 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return false
	}
	tail := data
	if len(tail) > 4_096 {
		tail = tail[len(tail)-4_096:]
	}
	return bytes.Contains(tail, []byte("%%EOF"))
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
