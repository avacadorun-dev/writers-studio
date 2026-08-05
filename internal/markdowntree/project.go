package markdowntree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

type Report struct {
	Directories   int       `json:"directories"`
	Documents     int       `json:"documents"`
	Images        int       `json:"images"`
	ResolvedLinks int       `json:"resolvedLinks"`
	Skipped       int       `json:"skipped"`
	WarningsCount int       `json:"warningsCount"`
	Warnings      []Warning `json:"warnings"`
}

type PreparedProject struct {
	ID          string
	Title       string
	Language    string
	Structure   domain.ProjectStructure
	Nodes       []PreparedNode
	Media       []PreparedMedia
	FirstNodeID string
	Report      Report
}

type PreparedNode struct {
	ID          string
	ParentID    *string
	Kind        string
	Title       string
	Position    float64
	Content     []byte
	PlainText   string
	Frontmatter string
}

type PreparedMedia struct {
	ID           string
	SourcePath   string
	OriginalName string
	MediaType    string
	Extension    string
	SHA256       string
	ByteSize     int64
	Width        int
	Height       int
	Content      []byte
}

type projectResolver struct {
	projectID   string
	documents   map[string][]string
	docByBase   map[string][]string
	images      map[string][]PreparedMedia
	imageByBase map[string][]PreparedMedia
}

func BuildProject(title string, language string, entries []Entry) (PreparedProject, error) {
	if language != "ru" && language != "en" {
		return PreparedProject{}, fmt.Errorf("invalid project language")
	}
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 120 {
		return PreparedProject{}, fmt.Errorf("invalid project title")
	}
	cleanEntries, rootName, report, err := prepareEntries(entries)
	if err != nil {
		return PreparedProject{}, err
	}
	if title == "Markdown project" && rootName != "" {
		title = truncateRunes(rootName, 120)
	}

	projectID := uuid.NewString()
	directories := make(map[string]bool)
	markdown := make(map[string]Entry)
	imageEntries := make(map[string]Entry)
	var contentBytes int64
	for _, entry := range cleanEntries {
		contentBytes += int64(len(entry.Content))
		if contentBytes > MaxArchiveBytes {
			return PreparedProject{}, fmt.Errorf("markdown tree content exceeds limit")
		}
		if int64(len(entry.Content)) > MaxArchiveBytes {
			return PreparedProject{}, fmt.Errorf("markdown tree entry exceeds limit")
		}
		if IsMarkdown(entry.Path) && int64(len(entry.Content)) > MaxMarkdownBytes {
			return PreparedProject{}, fmt.Errorf("markdown file exceeds limit")
		}
		if SupportedImageExtension(entry.Path) && int64(len(entry.Content)) > MaxImageBytes {
			return PreparedProject{}, fmt.Errorf("markdown image exceeds limit")
		}
		if entry.Directory {
			directories[entry.Path] = true
			continue
		}
		addParentDirectories(directories, entry.Path)
		switch {
		case IsMarkdown(entry.Path):
			markdown[entry.Path] = entry
		case SupportedImageExtension(entry.Path):
			imageEntries[entry.Path] = entry
		default:
			report.Skipped++
			addWarning(&report, Warning{Path: entry.Path, Code: "unsupported_file", Message: "Файл этого типа пропущен"})
		}
	}
	if len(markdown) == 0 {
		return PreparedProject{}, fmt.Errorf("markdown tree contains no documents")
	}
	if len(imageEntries) > MaxImages {
		return PreparedProject{}, fmt.Errorf("markdown tree exceeds image limit")
	}

	docIDs := make(map[string]string, len(markdown))
	for value := range markdown {
		docIDs[value] = uuid.NewString()
	}
	folderIDs := make(map[string]string, len(directories))
	containerIDs := make(map[string]string, len(directories))
	for directory := range directories {
		if documentID, paired := docIDs[directory+".md"]; paired {
			containerIDs[directory] = documentID
			continue
		}
		id := uuid.NewString()
		folderIDs[directory] = id
		containerIDs[directory] = id
	}
	if len(markdown)+len(folderIDs) > MaxNodes {
		return PreparedProject{}, fmt.Errorf("markdown tree exceeds node limit")
	}

	media := prepareImages(imageEntries, &report)
	resolver := newProjectResolver(projectID, docIDs, media)
	nodes := make([]PreparedNode, 0, len(folderIDs)+len(markdown))
	for directory, id := range folderIDs {
		nodes = append(nodes, PreparedNode{
			ID: id, ParentID: parentIDForPath(directory, containerIDs),
			Kind: "folder", Title: nodeTitle(path.Base(directory), &report, directory),
		})
	}
	documentPaths := sortedKeys(markdown)
	for _, documentPath := range documentPaths {
		entry := markdown[documentPath]
		result, parseErr := ParseMarkdown(documentPath, entry.Content, resolver)
		if parseErr != nil {
			return PreparedProject{}, fmt.Errorf("parse %q: %w", documentPath, parseErr)
		}
		report.ResolvedLinks += result.ResolvedLinks
		for _, warning := range result.Warnings {
			addWarning(&report, warning)
		}
		nodes = append(nodes, PreparedNode{
			ID: docIDs[documentPath], ParentID: parentIDForPath(documentPath, containerIDs),
			Kind: "document", Title: nodeTitle(strings.TrimSuffix(path.Base(documentPath), path.Ext(documentPath)), &report, documentPath),
			Content: result.Content, PlainText: result.PlainText, Frontmatter: result.Frontmatter,
		})
	}
	if len(nodes) == len(folderIDs) {
		return PreparedProject{}, fmt.Errorf("markdown tree contains no readable documents")
	}
	assignPositions(nodes)
	report.Directories = len(directories)
	report.Documents = len(nodes) - len(folderIDs)
	report.Images = len(media)
	structure := markdownVaultStructure(language)
	firstNodeID := ""
	for _, documentPath := range documentPaths {
		id := docIDs[documentPath]
		for _, node := range nodes {
			if node.ID == id {
				firstNodeID = id
				break
			}
		}
		if firstNodeID != "" {
			break
		}
	}
	return PreparedProject{
		ID: projectID, Title: title, Language: language, Structure: structure,
		Nodes: nodes, Media: media, FirstNodeID: firstNodeID, Report: report,
	}, nil
}

func prepareEntries(entries []Entry) ([]Entry, string, Report, error) {
	if len(entries) == 0 || len(entries) > MaxEntries {
		return nil, "", Report{}, fmt.Errorf("invalid markdown tree entry count")
	}
	result := make([]Entry, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	// Keep the wire representation stable for clients: an import without
	// warnings must return [] rather than JSON null.
	report := Report{Warnings: make([]Warning, 0)}
	for _, entry := range entries {
		normalized, err := NormalizePath(entry.Path)
		if err != nil {
			return nil, "", Report{}, err
		}
		if IsIgnoredPath(normalized) {
			continue
		}
		key := CollisionKey(normalized)
		if seen[key] {
			return nil, "", Report{}, fmt.Errorf("duplicate markdown tree path")
		}
		seen[key] = true
		entry.Path = normalized
		result = append(result, entry)
	}
	root := commonEntryRoot(result)
	if root != "" {
		next := make([]Entry, 0, len(result))
		for _, entry := range result {
			if entry.Path == root && entry.Directory {
				continue
			}
			entry.Path = strings.TrimPrefix(entry.Path, root+"/")
			next = append(next, entry)
		}
		result = next
	}
	return result, root, report, nil
}

func commonEntryRoot(entries []Entry) string {
	root := ""
	for _, entry := range entries {
		parts := strings.SplitN(entry.Path, "/", 2)
		if len(parts) == 1 {
			if entry.Directory {
				continue
			}
			return ""
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return ""
		}
	}
	return root
}

func prepareImages(entries map[string]Entry, report *Report) []PreparedMedia {
	paths := sortedKeys(entries)
	result := make([]PreparedMedia, 0, len(paths))
	for _, sourcePath := range paths {
		entry := entries[sourcePath]
		if len(entry.Content) == 0 || int64(len(entry.Content)) > MaxImageBytes {
			report.Skipped++
			addWarning(report, Warning{Path: sourcePath, Code: "invalid_image", Message: "Изображение пустое или слишком большое"})
			continue
		}
		mediaType := http.DetectContentType(entry.Content)
		extension := ""
		switch mediaType {
		case "image/png":
			extension = ".png"
		case "image/jpeg":
			extension = ".jpg"
		case "image/gif":
			extension = ".gif"
		default:
			report.Skipped++
			addWarning(report, Warning{Path: sourcePath, Code: "invalid_image", Message: "Фактический формат изображения не поддерживается"})
			continue
		}
		configuration, _, err := image.DecodeConfig(bytes.NewReader(entry.Content))
		if err != nil || configuration.Width < 1 || configuration.Height < 1 ||
			configuration.Width > 20_000 || configuration.Height > 20_000 {
			report.Skipped++
			addWarning(report, Warning{Path: sourcePath, Code: "invalid_image", Message: "Изображение повреждено или имеет недопустимый размер"})
			continue
		}
		checksum := sha256.Sum256(entry.Content)
		item := PreparedMedia{
			ID: uuid.NewString(), SourcePath: sourcePath, OriginalName: path.Base(sourcePath),
			MediaType: mediaType, Extension: extension, SHA256: hex.EncodeToString(checksum[:]),
			ByteSize: int64(len(entry.Content)), Width: configuration.Width, Height: configuration.Height,
			Content: entry.Content,
		}
		result = append(result, item)
	}
	return result
}

func newProjectResolver(projectID string, documents map[string]string, media []PreparedMedia) *projectResolver {
	result := &projectResolver{
		projectID: projectID, documents: map[string][]string{}, docByBase: map[string][]string{},
		images: map[string][]PreparedMedia{}, imageByBase: map[string][]PreparedMedia{},
	}
	for sourcePath, id := range documents {
		key := CollisionKey(sourcePath)
		result.documents[key] = append(result.documents[key], id)
		base := CollisionKey(strings.TrimSuffix(path.Base(sourcePath), path.Ext(sourcePath)))
		result.docByBase[base] = append(result.docByBase[base], id)
	}
	for _, item := range media {
		key := CollisionKey(item.SourcePath)
		result.images[key] = append(result.images[key], item)
		result.imageByBase[CollisionKey(path.Base(item.SourcePath))] =
			append(result.imageByBase[CollisionKey(path.Base(item.SourcePath))], item)
	}
	return result
}

func (r *projectResolver) Document(sourcePath string, target string) (string, bool, bool) {
	target, _ = url.PathUnescape(strings.TrimSpace(target))
	targetPath, fragment, _ := strings.Cut(target, "#")
	targetPath = strings.TrimSuffix(targetPath, "/")
	candidates := candidatePaths(sourcePath, targetPath, true)
	for _, candidate := range candidates {
		ids := r.documents[CollisionKey(candidate)]
		if len(ids) == 1 {
			return internalDocumentLink(r.projectID, ids[0], fragment), true, false
		}
		if len(ids) > 1 {
			return "", false, true
		}
	}
	base := CollisionKey(strings.TrimSuffix(path.Base(targetPath), path.Ext(targetPath)))
	ids := r.docByBase[base]
	if len(ids) == 1 {
		return internalDocumentLink(r.projectID, ids[0], fragment), true, false
	}
	return "", false, len(ids) > 1
}

func (r *projectResolver) Image(sourcePath string, target string) (Image, bool, bool) {
	target, _ = url.PathUnescape(strings.TrimSpace(target))
	target, _, _ = strings.Cut(target, "#")
	for _, candidate := range candidatePaths(sourcePath, target, false) {
		items := r.images[CollisionKey(candidate)]
		if len(items) == 1 {
			return preparedImage(items[0]), true, false
		}
		if len(items) > 1 {
			return Image{}, false, true
		}
	}
	items := r.imageByBase[CollisionKey(path.Base(target))]
	if len(items) == 1 {
		return preparedImage(items[0]), true, false
	}
	return Image{}, false, len(items) > 1
}

func preparedImage(item PreparedMedia) Image {
	return Image{
		AssetID: item.ID, Source: "/api/media/" + item.ID + "/content",
		Alt: item.OriginalName, Width: item.Width, Height: item.Height,
	}
}

func candidatePaths(sourcePath string, target string, markdown bool) []string {
	target = strings.ReplaceAll(target, "\\", "/")
	var candidate string
	if strings.HasPrefix(target, "/") {
		candidate = strings.TrimPrefix(path.Clean(target), "/")
	} else {
		candidate = path.Clean(path.Join(path.Dir(sourcePath), target))
	}
	result := []string{candidate}
	if markdown && path.Ext(candidate) == "" {
		result = append(result, candidate+".md")
	}
	return result
}

func internalDocumentLink(projectID string, nodeID string, fragment string) string {
	result := "/projects/" + projectID + "/studio/" + nodeID
	if fragment != "" {
		result += "#" + url.PathEscape(fragment)
	}
	return result
}

func addParentDirectories(directories map[string]bool, filePath string) {
	for parent := path.Dir(filePath); parent != "." && parent != "/"; parent = path.Dir(parent) {
		directories[parent] = true
	}
}

func parentIDForPath(value string, containers map[string]string) *string {
	parent := path.Dir(value)
	if parent == "." {
		return nil
	}
	id, exists := containers[parent]
	if !exists {
		return nil
	}
	return &id
}

func assignPositions(nodes []PreparedNode) {
	sort.Slice(nodes, func(left int, right int) bool {
		leftParent, rightParent := "", ""
		if nodes[left].ParentID != nil {
			leftParent = *nodes[left].ParentID
		}
		if nodes[right].ParentID != nil {
			rightParent = *nodes[right].ParentID
		}
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		if nodes[left].Kind != nodes[right].Kind {
			return nodes[left].Kind == "folder"
		}
		return strings.ToLower(nodes[left].Title) < strings.ToLower(nodes[right].Title)
	})
	positions := map[string]int{}
	for index := range nodes {
		parent := ""
		if nodes[index].ParentID != nil {
			parent = *nodes[index].ParentID
		}
		positions[parent]++
		nodes[index].Position = float64(positions[parent] * 1_000)
	}
}

func markdownVaultStructure(language string) domain.ProjectStructure {
	localize := func(ru string, en string) string {
		if language == "en" {
			return en
		}
		return ru
	}
	return domain.ProjectStructure{
		SchemaVersion: 1, Template: "markdown_vault", RootLabel: localize("Хранилище", "Vault"),
		NodeTypes: []domain.ProjectNodeType{
			{
				Key: "folder", Label: localize("Папка", "Folder"),
				PluralLabel: localize("Папки", "Folders"), Icon: "folder",
				AllowedChildren: []string{"folder", "document"}, CanBeRoot: true,
			},
			{
				Key: "document", Label: localize("Документ", "Document"),
				PluralLabel: localize("Документы", "Documents"), Icon: "file-text",
				ContentBearing: true, AllowedChildren: []string{"folder", "document"}, CanBeRoot: true,
			},
		},
	}
}

func nodeTitle(value string, report *Report, sourcePath string) string {
	result := strings.TrimSpace(value)
	if result == "" {
		result = "Untitled"
	}
	truncated := truncateRunes(result, 200)
	if truncated != result {
		addWarning(report, Warning{Path: sourcePath, Code: "title_truncated", Message: "Название сокращено до 200 символов"})
	}
	return truncated
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func addWarning(report *Report, warning Warning) {
	report.WarningsCount++
	if len(report.Warnings) < 100 {
		report.Warnings = append(report.Warnings, warning)
	}
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Slice(result, func(left int, right int) bool {
		lowerLeft, lowerRight := strings.ToLower(result[left]), strings.ToLower(result[right])
		if lowerLeft == lowerRight {
			return result[left] < result[right]
		}
		return lowerLeft < lowerRight
	})
	return result
}
