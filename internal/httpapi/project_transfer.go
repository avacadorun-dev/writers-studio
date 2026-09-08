package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/store"
)

const (
	projectTransferMIME            = "application/vnd.writers-studio.project+zip"
	maxProjectTransferBytes        = 512 << 20
	maxProjectTransferManifestSize = 64 << 20
)

func (s *Server) handleDownloadProjectTransfer(response http.ResponseWriter, request *http.Request) {
	transfer, err := s.store.BuildProjectTransfer(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "projectID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}

	fileContent := make(map[string][]byte, len(transfer.Audio)+len(transfer.Media))
	for _, item := range transfer.Audio {
		content, err := s.objects.Get(request.Context(), item.StorageKey)
		if err != nil {
			writeError(
				response,
				http.StatusServiceUnavailable,
				"project_audio_unavailable",
				"Файл проекта не создан: часть исходного аудио временно недоступна",
			)
			return
		}
		if int64(len(content)) != item.ByteSize || sha256Hex(content) != item.SHA256 {
			writeError(
				response,
				http.StatusConflict,
				"project_audio_checksum_mismatch",
				"Файл проекта не создан: проверка исходного аудио не пройдена",
			)
			return
		}
		fileContent[item.Path] = content
	}
	for _, item := range transfer.Media {
		content, err := s.objects.Get(request.Context(), item.StorageKey)
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "project_media_unavailable", "Файл проекта не создан: часть изображений временно недоступна")
			return
		}
		if int64(len(content)) != item.ByteSize || sha256Hex(content) != item.SHA256 {
			writeError(response, http.StatusConflict, "project_media_checksum_mismatch", "Файл проекта не создан: проверка изображений не пройдена")
			return
		}
		fileContent[item.Path] = content
	}

	data, err := encodeProjectTransferArchive(transfer, fileContent)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "project_export_failed", "Не удалось собрать файл проекта")
		return
	}
	response.Header().Set("Content-Type", projectTransferMIME)
	response.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="writers-studio-%s.wstudio"`, transfer.Project.ID),
	)
	response.Header().Set("Cache-Control", "private, no-store")
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}

func (s *Server) handleImportProjectTransfer(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, maxProjectTransferBytes)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(response, http.StatusRequestEntityTooLarge, "project_file_too_large", "Файл проекта больше 512 МБ")
			return
		}
		writeError(response, http.StatusBadRequest, "project_file_unreadable", "Не удалось прочитать файл проекта")
		return
	}
	transfer, fileContent, err := decodeProjectTransferArchive(data)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_project_file", projectTransferErrorMessage(err))
		return
	}

	importID := uuid.NewString()
	uploadedKeys := make([]string, 0, len(transfer.Audio)+len(transfer.Media))
	for index := range transfer.Audio {
		item := &transfer.Audio[index]
		extension := path.Ext(item.Path)
		if extension == "" {
			extension = ".webm"
		}
		item.StorageKey = path.Join("audio", "imports", importID, uuid.NewString()+extension)
		if err := s.objects.Put(request.Context(), item.StorageKey, item.MediaType, fileContent[item.Path]); err != nil {
			cleanupObjectKeys(request, s, uploadedKeys)
			writeError(response, http.StatusServiceUnavailable, "project_audio_import_failed", "Импорт не завершён: не удалось сохранить аудио")
			return
		}
		uploadedKeys = append(uploadedKeys, item.StorageKey)
	}
	for index := range transfer.Media {
		item := &transfer.Media[index]
		extension := path.Ext(item.Path)
		item.StorageKey = path.Join("media", "imports", importID, uuid.NewString()+extension)
		if err := s.objects.Put(request.Context(), item.StorageKey, item.MediaType, fileContent[item.Path]); err != nil {
			cleanupObjectKeys(request, s, uploadedKeys)
			writeError(response, http.StatusServiceUnavailable, "project_media_import_failed", "Импорт не завершён: не удалось сохранить изображения")
			return
		}
		uploadedKeys = append(uploadedKeys, item.StorageKey)
	}

	result, err := s.store.ImportProjectTransfer(
		request.Context(),
		currentUser(request.Context()).ID,
		transfer,
	)
	if err != nil {
		cleanupObjectKeys(request, s, uploadedKeys)
		if errors.Is(err, store.ErrInvalidProjectTransfer) {
			writeError(response, http.StatusUnprocessableEntity, "invalid_project_file", projectTransferErrorMessage(err))
			return
		}
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"project": result,
		"notice":  "Проект импортирован как независимая копия",
	})
}

func cleanupObjectKeys(request *http.Request, server *Server, keys []string) {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), 30*time.Second)
	defer cancel()
	for _, key := range keys {
		if err := server.objects.Delete(cleanupContext, key); err != nil {
			server.logger.Warn("cleanup failed project import object", "storageKey", key)
		}
	}
}

func encodeProjectTransferArchive(
	transfer store.ProjectTransfer,
	fileContent map[string][]byte,
) ([]byte, error) {
	manifest, err := json.MarshalIndent(transfer, "", "  ")
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	metadata, err := archive.Create("project.json")
	if err != nil {
		return nil, err
	}
	if _, err := metadata.Write(manifest); err != nil {
		return nil, err
	}
	for _, item := range transfer.Audio {
		content, ok := fileContent[item.Path]
		if !ok {
			return nil, fmt.Errorf("missing audio %s", item.Path)
		}
		entry, err := archive.Create(item.Path)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(content); err != nil {
			return nil, err
		}
	}
	for _, item := range transfer.Media {
		content, ok := fileContent[item.Path]
		if !ok {
			return nil, fmt.Errorf("missing media %s", item.Path)
		}
		entry, err := archive.Create(item.Path)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(content); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func decodeProjectTransferArchive(
	data []byte,
) (store.ProjectTransfer, map[string][]byte, error) {
	if len(data) == 0 {
		return store.ProjectTransfer{}, nil, fmt.Errorf("empty archive")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return store.ProjectTransfer{}, nil, fmt.Errorf("open zip: %w", err)
	}
	if len(archive.File) == 0 || len(archive.File) > 40_002 {
		return store.ProjectTransfer{}, nil, fmt.Errorf("invalid archive entry count")
	}
	entries := make(map[string]*zip.File, len(archive.File))
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !safeArchivePath(entry.Name) || entries[entry.Name] != nil {
			return store.ProjectTransfer{}, nil, fmt.Errorf("unsafe or duplicate archive path")
		}
		entries[entry.Name] = entry
	}
	manifestEntry := entries["project.json"]
	if manifestEntry == nil || manifestEntry.UncompressedSize64 > maxProjectTransferManifestSize {
		return store.ProjectTransfer{}, nil, fmt.Errorf("missing or oversized manifest")
	}
	manifest, err := readZipEntry(manifestEntry, maxProjectTransferManifestSize)
	if err != nil {
		return store.ProjectTransfer{}, nil, fmt.Errorf("read manifest: %w", err)
	}
	var transfer store.ProjectTransfer
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&transfer); err != nil {
		return store.ProjectTransfer{}, nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := store.ValidateProjectTransfer(transfer); err != nil {
		return store.ProjectTransfer{}, nil, err
	}
	if len(entries) != len(transfer.Audio)+len(transfer.Media)+1 {
		return store.ProjectTransfer{}, nil, fmt.Errorf("archive contains undeclared entries")
	}

	fileContent := make(map[string][]byte, len(transfer.Audio)+len(transfer.Media))
	var totalUncompressed int64 = int64(len(manifest))
	for _, item := range transfer.Audio {
		entry := entries[item.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(item.ByteSize) {
			return store.ProjectTransfer{}, nil, fmt.Errorf("audio size mismatch")
		}
		if item.ByteSize > maxProjectTransferBytes || totalUncompressed > maxProjectTransferBytes-item.ByteSize {
			return store.ProjectTransfer{}, nil, fmt.Errorf("uncompressed project exceeds limit")
		}
		content, err := readZipEntry(entry, item.ByteSize)
		if err != nil {
			return store.ProjectTransfer{}, nil, fmt.Errorf("read audio: %w", err)
		}
		if sha256Hex(content) != strings.ToLower(item.SHA256) {
			return store.ProjectTransfer{}, nil, fmt.Errorf("audio checksum mismatch")
		}
		totalUncompressed += item.ByteSize
		fileContent[item.Path] = content
	}
	for _, item := range transfer.Media {
		entry := entries[item.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(item.ByteSize) {
			return store.ProjectTransfer{}, nil, fmt.Errorf("media size mismatch")
		}
		if item.ByteSize > maxProjectTransferBytes || totalUncompressed > maxProjectTransferBytes-item.ByteSize {
			return store.ProjectTransfer{}, nil, fmt.Errorf("uncompressed project exceeds limit")
		}
		content, err := readZipEntry(entry, item.ByteSize)
		if err != nil {
			return store.ProjectTransfer{}, nil, fmt.Errorf("read media: %w", err)
		}
		if sha256Hex(content) != strings.ToLower(item.SHA256) {
			return store.ProjectTransfer{}, nil, fmt.Errorf("media checksum mismatch")
		}
		if http.DetectContentType(content) != item.MediaType {
			return store.ProjectTransfer{}, nil, fmt.Errorf("media content mismatch")
		}
		if item.MediaType == "application/pdf" {
			if !validPDF(content) || item.Width != nil || item.Height != nil {
				return store.ProjectTransfer{}, nil, fmt.Errorf("media content mismatch")
			}
		} else if !isTimedProjectMedia(item.MediaType) {
			configuration, _, decodeErr := image.DecodeConfig(bytes.NewReader(content))
			if decodeErr != nil || item.Width == nil || item.Height == nil ||
				configuration.Width != *item.Width || configuration.Height != *item.Height {
				return store.ProjectTransfer{}, nil, fmt.Errorf("media content mismatch")
			}
		}
		totalUncompressed += item.ByteSize
		fileContent[item.Path] = content
	}
	return transfer, fileContent, nil
}

func isTimedProjectMedia(mediaType string) bool {
	return strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/")
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("entry exceeds declared limit")
	}
	return content, nil
}

func safeArchivePath(value string) bool {
	return value != "" && path.Clean(value) == value && !strings.HasPrefix(value, "/") &&
		!strings.Contains(value, "\\") && !strings.Contains(value, "../")
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func projectTransferErrorMessage(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unsupported format version"):
		return "Эта версия файла создана более новой версией приложения"
	case strings.Contains(message, "checksum"):
		return "Проверка целостности файлов проекта не пройдена"
	case strings.Contains(message, "limit"), strings.Contains(message, "oversized"):
		return "Файл проекта превышает допустимый размер"
	default:
		return "Файл повреждён или не является проектом «Студии автора»"
	}
}
