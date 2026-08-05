package export

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"writers.studio/internal/editor"
	"writers.studio/internal/markdowntree"
)

type markdownTreePath struct {
	File   string
	Folder string
}

func renderMarkdownTree(snapshot Snapshot, assets map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	err := RenderMarkdownTreeStream(&output, snapshot, func(media SnapshotMedia) ([]byte, error) {
		content, exists := assets[media.ID]
		if !exists {
			return nil, fmt.Errorf("markdown tree media %s is unavailable", media.ID)
		}
		return content, nil
	})
	return output.Bytes(), err
}

// RenderMarkdownTreeStream writes a portable vault directly to writer and only
// retains one media object at a time through loadMedia.
func RenderMarkdownTreeStream(
	writer io.Writer,
	snapshot Snapshot,
	loadMedia func(SnapshotMedia) ([]byte, error),
) error {
	if len(snapshot.TreeNodes) == 0 {
		return fmt.Errorf("markdown tree contains no nodes")
	}
	root := markdowntree.PortableSegment(snapshot.ProjectTitle)
	paths, directories := allocateMarkdownTreePaths(root, snapshot.TreeNodes)
	mediaPaths := allocateMarkdownMediaPaths(root, snapshot.Media, paths, directories)

	archive := zip.NewWriter(writer)
	directoryNames := make([]string, 0, len(directories))
	for directory := range directories {
		directoryNames = append(directoryNames, directory)
	}
	sort.Strings(directoryNames)
	for _, directory := range directoryNames {
		header := &zip.FileHeader{Name: strings.TrimSuffix(directory, "/") + "/", Method: zip.Store}
		if _, err := archive.CreateHeader(header); err != nil {
			return err
		}
	}

	nodes := append([]SnapshotTreeNode(nil), snapshot.TreeNodes...)
	sort.Slice(nodes, func(left int, right int) bool {
		return paths[nodes[left].NodeID].File < paths[nodes[right].NodeID].File
	})
	for _, node := range nodes {
		target := paths[node.NodeID].File
		if target == "" {
			continue
		}
		content, err := rewriteMarkdownTreeContent(node.Content, node.NodeID, paths, mediaPaths)
		if err != nil {
			return fmt.Errorf("rewrite markdown tree node %s: %w", node.NodeID, err)
		}
		markdown, err := editor.Markdown(content)
		if err != nil {
			markdown = node.PlainText
		}
		body := strings.TrimSpace(markdown)
		if node.Frontmatter != "" {
			body = node.Frontmatter + strings.TrimLeft(body, "\r\n")
		}
		entry, err := archive.Create(target)
		if err != nil {
			return err
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			return err
		}
	}

	for _, media := range snapshot.Media {
		target := mediaPaths[media.ID]
		if target == "" {
			continue
		}
		content, err := loadMedia(media)
		if err != nil {
			return err
		}
		if media.SHA256 != "" {
			checksum := sha256.Sum256(content)
			if hex.EncodeToString(checksum[:]) != media.SHA256 {
				return fmt.Errorf("markdown tree media %s checksum mismatch", media.ID)
			}
		}
		entry, err := archive.Create(target)
		if err != nil {
			return err
		}
		if _, err := entry.Write(content); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return nil
}

func allocateMarkdownTreePaths(
	root string,
	nodes []SnapshotTreeNode,
) (map[string]markdownTreePath, map[string]bool) {
	byParent := make(map[string][]SnapshotTreeNode)
	known := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		known[node.NodeID] = true
	}
	for _, node := range nodes {
		parent := ""
		if node.ParentID != nil && known[*node.ParentID] {
			parent = *node.ParentID
		}
		byParent[parent] = append(byParent[parent], node)
	}
	for parent := range byParent {
		sort.Slice(byParent[parent], func(left int, right int) bool {
			if byParent[parent][left].Position != byParent[parent][right].Position {
				return byParent[parent][left].Position < byParent[parent][right].Position
			}
			return strings.ToLower(byParent[parent][left].Title) <
				strings.ToLower(byParent[parent][right].Title)
		})
	}
	result := make(map[string]markdownTreePath, len(nodes))
	directories := map[string]bool{root: true}
	type pendingItem struct {
		ParentID string
		Folder   string
	}
	pending := []pendingItem{{Folder: root}}
	visited := map[string]bool{}
	for len(pending) > 0 {
		item := pending[0]
		pending = pending[1:]
		used := map[string]bool{}
		for _, node := range byParent[item.ParentID] {
			if visited[node.NodeID] {
				continue
			}
			visited[node.NodeID] = true
			base := allocatePortableBase(node.Title, used)
			target := markdownTreePath{}
			children := len(byParent[node.NodeID]) > 0
			if node.ContentBearing {
				target.File = path.Join(item.Folder, base+".md")
			}
			if !node.ContentBearing || children {
				target.Folder = path.Join(item.Folder, base)
				directories[target.Folder] = true
			}
			result[node.NodeID] = target
			if target.Folder != "" {
				pending = append(pending, pendingItem{ParentID: node.NodeID, Folder: target.Folder})
			}
		}
	}
	for _, node := range nodes {
		if visited[node.NodeID] {
			continue
		}
		base := allocatePortableBase(node.Title, map[string]bool{})
		target := markdownTreePath{}
		if node.ContentBearing {
			target.File = path.Join(root, base+".md")
		} else {
			target.Folder = path.Join(root, base)
			directories[target.Folder] = true
		}
		result[node.NodeID] = target
	}
	return result, directories
}

func allocatePortableBase(title string, used map[string]bool) string {
	base := markdowntree.PortableSegment(title)
	for index := 1; ; index++ {
		candidate := base
		if index > 1 {
			candidate = fmt.Sprintf("%s (%d)", base, index)
		}
		key := markdowntree.CollisionKey(candidate)
		if !used[key] {
			used[key] = true
			return candidate
		}
	}
}

func allocateMarkdownMediaPaths(
	root string,
	media []SnapshotMedia,
	nodes map[string]markdownTreePath,
	directories map[string]bool,
) map[string]string {
	used := make(map[string]bool)
	for directory := range directories {
		used[markdowntree.CollisionKey(directory)] = true
	}
	for _, node := range nodes {
		if node.File != "" {
			used[markdowntree.CollisionKey(node.File)] = true
		}
	}
	result := make(map[string]string, len(media))
	attachmentNames := map[string]bool{}
	for _, item := range media {
		candidate := ""
		if item.SourcePath != nil {
			if normalized, err := markdowntree.NormalizePath(*item.SourcePath); err == nil {
				candidate = path.Join(root, normalized)
			}
		}
		if candidate == "" || used[markdowntree.CollisionKey(candidate)] {
			extension := markdownTreeMediaExtension(item.MediaType)
			title := strings.TrimSuffix(item.OriginalName, path.Ext(item.OriginalName))
			name := markdowntree.AllocatePortableName(title, extension, attachmentNames)
			candidate = path.Join(root, "_attachments", name)
			directories[path.Join(root, "_attachments")] = true
		}
		used[markdowntree.CollisionKey(candidate)] = true
		result[item.ID] = candidate
	}
	return result
}

func rewriteMarkdownTreeContent(
	raw json.RawMessage,
	nodeID string,
	paths map[string]markdownTreePath,
	mediaPaths map[string]string,
) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage([]byte("{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\"}]}")), nil
	}
	var document editor.Node
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	currentFile := paths[nodeID].File
	var visit func(*editor.Node)
	visit = func(node *editor.Node) {
		for index := range node.Marks {
			if node.Marks[index].Type != "link" {
				continue
			}
			href, _ := node.Marks[index].Attrs["href"].(string)
			targetID, fragment, ok := internalLinkTarget(href)
			if !ok || paths[targetID].File == "" {
				continue
			}
			node.Marks[index].Attrs["href"] =
				relativeMarkdownPath(currentFile, paths[targetID].File) + fragment
		}
		if node.Type == "imageBlock" {
			assetID, _ := node.Attrs["assetId"].(string)
			if target := mediaPaths[assetID]; target != "" {
				node.Attrs["src"] = relativeMarkdownPath(currentFile, target)
			}
		}
		for index := range node.Content {
			visit(&node.Content[index])
		}
	}
	visit(&document)
	return json.Marshal(document)
}

func internalLinkTarget(href string) (string, string, bool) {
	parsed, err := url.Parse(href)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "studio" {
		return "", "", false
	}
	fragment := ""
	if parsed.Fragment != "" {
		fragment = "#" + url.PathEscape(parsed.Fragment)
	}
	return parts[3], fragment, true
}

func relativeMarkdownPath(source string, target string) string {
	relative, err := filepath.Rel(filepath.FromSlash(path.Dir(source)), filepath.FromSlash(target))
	if err != nil {
		return target
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index, part := range parts {
		if part != ".." && part != "." {
			parts[index] = url.PathEscape(part)
		}
	}
	return strings.Join(parts, "/")
}

func markdownTreeMediaExtension(mediaType string) string {
	switch mediaType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}
