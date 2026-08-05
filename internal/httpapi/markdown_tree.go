package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"writers.studio/internal/markdowntree"
	"writers.studio/internal/store"
)

const maxMarkdownTreeManifestBytes = 16 << 20

type markdownTreeDirectoryManifest struct {
	RootName string                               `json:"rootName"`
	Entries  []markdownTreeDirectoryManifestEntry `json:"entries"`
}

type markdownTreeDirectoryManifestEntry struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}

func (s *Server) handleImportMarkdownTree(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, markdowntree.MaxArchiveBytes+(32<<20))
	reader, err := request.MultipartReader()
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_markdown_tree_request", "Ожидается форма с ZIP или выбранной папкой")
		return
	}
	fields := map[string]string{}
	files := map[string][]byte{}
	manifestPaths := map[string]string{}
	var archive []byte
	var manifestData []byte
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			writeMarkdownTreeReadError(response, partErr)
			return
		}
		name := part.FormName()
		switch {
		case name == "archive":
			archive, err = readMultipartPart(part, markdowntree.MaxArchiveBytes)
		case name == "manifest":
			manifestData, err = readMultipartPart(part, maxMarkdownTreeManifestBytes)
			if err == nil {
				var preview markdownTreeDirectoryManifest
				if json.Unmarshal(manifestData, &preview) == nil {
					for _, item := range preview.Entries {
						manifestPaths[item.ID] = item.Path
					}
				}
			}
		case strings.HasPrefix(name, "entry."):
			if len(files) >= markdowntree.MaxEntries {
				err = fmt.Errorf("too many directory files")
				break
			}
			entryID := strings.TrimPrefix(name, "entry.")
			entryPath := manifestPaths[entryID]
			switch {
			case markdowntree.IsIgnoredPath(entryPath):
				_, err = io.Copy(io.Discard, part)
				files[entryID] = nil
			case markdowntree.IsMarkdown(entryPath):
				files[entryID], err = readMultipartPart(part, markdowntree.MaxMarkdownBytes)
			case markdowntree.SupportedImageExtension(entryPath):
				files[entryID], err = readMultipartPart(part, markdowntree.MaxImageBytes)
			case entryPath != "":
				_, err = io.Copy(io.Discard, part)
				files[entryID] = nil
			default:
				files[entryID], err = readMultipartPart(part, markdowntree.MaxImageBytes)
			}
		default:
			var value []byte
			value, err = readMultipartPart(part, 1<<10)
			if err == nil {
				fields[name] = string(value)
			}
		}
		part.Close()
		if err != nil {
			writeMarkdownTreeReadError(response, err)
			return
		}
	}

	title := strings.TrimSpace(fields["title"])
	if title == "" {
		title = "Markdown project"
	}
	language := fields["language"]
	if language == "" {
		language = "ru"
	}
	var entries []markdowntree.Entry
	switch fields["source"] {
	case "zip":
		entries, err = markdowntree.ReadZIP(archive)
	case "directory":
		entries, err = directoryManifestEntries(manifestData, files)
	default:
		err = fmt.Errorf("unknown markdown tree source")
	}
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_markdown_tree", markdownTreeErrorMessage(err))
		return
	}
	prepared, err := markdowntree.BuildProject(title, language, entries)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_markdown_tree", markdownTreeErrorMessage(err))
		return
	}

	uploadedKeys := make([]string, 0, len(prepared.Media))
	storageKeys := make(map[string]string, len(prepared.Media))
	for _, media := range prepared.Media {
		storageKey := path.Join("media", prepared.ID, media.ID+media.Extension)
		if err := s.objects.Put(request.Context(), storageKey, media.MediaType, media.Content); err != nil {
			cleanupObjectKeys(request, s, uploadedKeys)
			writeError(response, http.StatusServiceUnavailable, "markdown_tree_media_failed", "Импорт не завершён: изображения не удалось сохранить")
			return
		}
		uploadedKeys = append(uploadedKeys, storageKey)
		storageKeys[media.ID] = storageKey
	}
	result, err := s.store.ImportMarkdownTree(
		request.Context(),
		currentUser(request.Context()).ID,
		store.MarkdownTreeImportInput{Project: prepared, MediaStorageKeys: storageKeys},
	)
	if err != nil {
		cleanupObjectKeys(request, s, uploadedKeys)
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"project": result,
		"report":  prepared.Report,
		"notice":  "Markdown-дерево импортировано в новый проект",
	})
}

func directoryManifestEntries(
	manifestData []byte,
	files map[string][]byte,
) ([]markdowntree.Entry, error) {
	var manifest markdownTreeDirectoryManifest
	if len(manifestData) == 0 || json.Unmarshal(manifestData, &manifest) != nil ||
		len(manifest.Entries) == 0 || len(manifest.Entries) > markdowntree.MaxEntries {
		return nil, fmt.Errorf("invalid directory manifest")
	}
	result := make([]markdowntree.Entry, 0, len(manifest.Entries)+1)
	if manifest.RootName != "" {
		result = append(result, markdowntree.Entry{Path: manifest.RootName, Directory: true})
	}
	seenIDs := map[string]bool{}
	for _, item := range manifest.Entries {
		if item.ID == "" || seenIDs[item.ID] || item.Size < 0 {
			return nil, fmt.Errorf("invalid directory manifest entry")
		}
		seenIDs[item.ID] = true
		entry := markdowntree.Entry{Path: item.Path, Directory: item.Kind == "directory"}
		if item.Kind != "directory" {
			if item.Kind != "file" {
				return nil, fmt.Errorf("invalid directory manifest kind")
			}
			content, exists := files[item.ID]
			if !exists || (!markdowntree.IsIgnoredPath(item.Path) && (markdowntree.IsMarkdown(item.Path) ||
				markdowntree.SupportedImageExtension(item.Path)) && int64(len(content)) != item.Size) {
				return nil, fmt.Errorf("directory file is missing")
			}
			entry.Content = content
		}
		result = append(result, entry)
	}
	return result, nil
}

func readMultipartPart(part *multipart.Part, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("multipart part exceeds limit")
	}
	return content, nil
}

func writeMarkdownTreeReadError(response http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) || strings.Contains(strings.ToLower(err.Error()), "limit") {
		writeError(response, http.StatusRequestEntityTooLarge, "markdown_tree_too_large", "Markdown-хранилище превышает допустимый размер")
		return
	}
	writeError(response, http.StatusBadRequest, "markdown_tree_unreadable", "Не удалось прочитать Markdown-хранилище")
}

func markdownTreeErrorMessage(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "no documents"):
		return "В выбранном хранилище нет Markdown-документов"
	case strings.Contains(message, "limit"), strings.Contains(message, "too many"), strings.Contains(message, "exceeds"):
		return "Хранилище превышает допустимый размер, глубину или число файлов"
	case strings.Contains(message, "symlink"), strings.Contains(message, "path"), strings.Contains(message, "duplicate"):
		return "Хранилище содержит небезопасные или конфликтующие пути"
	default:
		return "Хранилище повреждено или содержит неподдерживаемую структуру"
	}
}
