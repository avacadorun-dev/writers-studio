package markdowntree

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxArchiveBytes  = int64(512 << 20)
	MaxMarkdownBytes = int64(10 << 20)
	MaxImageBytes    = int64(12 << 20)
	MaxNodes         = 20_000
	MaxImages        = 20_000
	MaxEntries       = 40_000
	MaxDepth         = 512
	MaxPathBytes     = 4_096
	MaxSegmentBytes  = 255
)

var (
	ErrInvalidPath      = errors.New("invalid markdown tree path")
	reservedWindowsName = regexp.MustCompile("(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\\..*)?$")
	pathFold            = cases.Fold()
)

func NormalizePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "\\") || (len(value) >= 3 && value[1] == ':' &&
		(value[2] == '/' || value[2] == '\\')) {
		return "", ErrInvalidPath
	}
	if !utf8.ValidString(value) {
		return "", ErrInvalidPath
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = norm.NFC.String(value)
	if len(value) > MaxPathBytes {
		return "", ErrInvalidPath
	}
	parts := strings.Split(strings.TrimSuffix(value, "/"), "/")
	if len(parts) == 0 || len(parts) > MaxDepth {
		return "", ErrInvalidPath
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || len(part) > MaxSegmentBytes {
			return "", ErrInvalidPath
		}
	}
	normalized := path.Clean(strings.Join(parts, "/"))
	if normalized == "." || strings.HasPrefix(normalized, "../") {
		return "", ErrInvalidPath
	}
	return normalized, nil
}

func CollisionKey(value string) string {
	return pathFold.String(norm.NFC.String(value))
}

func IsIgnoredPath(value string) bool {
	for _, part := range strings.Split(value, "/") {
		folded := strings.ToLower(part)
		switch folded {
		case ".obsidian", ".trash", ".git", ".ds_store", "thumbs.db", "__macosx":
			return true
		}
		// Finder and macOS archive tools may add AppleDouble sidecar files
		// containing binary metadata next to otherwise valid Markdown files.
		if strings.HasPrefix(folded, "._") {
			return true
		}
	}
	return false
}

func IsMarkdown(value string) bool {
	return strings.EqualFold(path.Ext(value), ".md")
}

func SupportedImageExtension(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}

func PortableSegment(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || strings.ContainsRune("<>:\"/\\|?*", character) {
			return '-'
		}
		return character
	}, strings.TrimSpace(value))
	value = strings.TrimRight(value, ". ")
	if value == "" || value == "." || value == ".." {
		value = "untitled"
	}
	if reservedWindowsName.MatchString(value) {
		value = "_" + value
	}
	for len([]byte(value)) > 220 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func AllocatePortableName(title string, extension string, used map[string]bool) string {
	base := PortableSegment(title)
	for index := 1; ; index++ {
		candidate := base
		if index > 1 {
			candidate = fmt.Sprintf("%s (%d)", base, index)
		}
		name := candidate + extension
		key := CollisionKey(name)
		if !used[key] {
			used[key] = true
			return name
		}
	}
}

func StripCommonRoot(paths []string) (string, []string) {
	if len(paths) == 0 {
		return "", paths
	}
	root := strings.SplitN(paths[0], "/", 2)[0]
	for _, value := range paths {
		parts := strings.SplitN(value, "/", 2)
		if len(parts) < 2 || parts[0] != root {
			return "", paths
		}
	}
	result := make([]string, len(paths))
	for index, value := range paths {
		result[index] = strings.TrimPrefix(value, root+"/")
	}
	return root, result
}
