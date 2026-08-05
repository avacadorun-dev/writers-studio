package markdowntree

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

type Entry struct {
	Path      string
	Directory bool
	Content   []byte
}

func ReadZIP(data []byte) ([]Entry, error) {
	if len(data) == 0 || int64(len(data)) > MaxArchiveBytes {
		return nil, fmt.Errorf("markdown archive exceeds limit")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open markdown archive: %w", err)
	}
	if len(archive.File) == 0 || len(archive.File) > MaxEntries {
		return nil, fmt.Errorf("invalid markdown archive entry count")
	}
	entries := make([]Entry, 0, len(archive.File))
	seen := make(map[string]bool, len(archive.File))
	var total int64
	for _, item := range archive.File {
		if item.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("markdown archive contains symlink")
		}
		normalized, err := NormalizePath(item.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid archive path %q: %w", item.Name, err)
		}
		directory := item.FileInfo().IsDir() || strings.HasSuffix(item.Name, "/")
		if !directory {
			if item.UncompressedSize64 > uint64(MaxArchiveBytes) {
				return nil, fmt.Errorf("markdown archive entry exceeds limit")
			}
			total += int64(item.UncompressedSize64)
			if total > MaxArchiveBytes {
				return nil, fmt.Errorf("uncompressed markdown archive exceeds limit")
			}
		}
		if IsIgnoredPath(normalized) {
			continue
		}
		key := CollisionKey(normalized)
		if seen[key] {
			return nil, fmt.Errorf("duplicate markdown archive path")
		}
		seen[key] = true
		entry := Entry{Path: normalized, Directory: directory}
		if !directory {
			if IsMarkdown(normalized) || SupportedImageExtension(normalized) {
				limit := MaxMarkdownBytes
				if SupportedImageExtension(normalized) {
					limit = MaxImageBytes
				}
				content, err := readZIPEntry(item, limit)
				if err != nil {
					return nil, fmt.Errorf("read %q: %w", normalized, err)
				}
				entry.Content = content
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readZIPEntry(item *zip.File, limit int64) ([]byte, error) {
	reader, err := item.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("entry exceeds limit")
	}
	return content, nil
}
