package projectmedia

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path"
	"strings"
	"unicode"

	_ "golang.org/x/image/webp"
)

const (
	MaxImageBytes     = 12 << 20
	MaxImageDimension = 20_000
	MaxBase64Length   = 4 * ((MaxImageBytes + 2) / 3)
)

var (
	ErrEmptyImage       = errors.New("image is empty")
	ErrImageTooLarge    = errors.New("image is too large")
	ErrInvalidImage     = errors.New("image is invalid")
	ErrUnsupportedImage = errors.New("image format is unsupported")
)

type ImageMetadata struct {
	MediaType string
	Extension string
	Width     int
	Height    int
}

// InspectImage derives the media type from decoded bytes rather than trusting
// caller-supplied metadata. DecodeConfig validates the image header without
// allocating a full uncompressed bitmap.
func InspectImage(data []byte) (ImageMetadata, error) {
	if len(data) == 0 {
		return ImageMetadata{}, ErrEmptyImage
	}
	if len(data) > MaxImageBytes {
		return ImageMetadata{}, ErrImageTooLarge
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageMetadata{}, ErrInvalidImage
	}
	mediaType, extension, supported := imageFormat(format)
	if !supported {
		return ImageMetadata{}, ErrUnsupportedImage
	}
	if configuration.Width < 1 || configuration.Height < 1 ||
		configuration.Width > MaxImageDimension || configuration.Height > MaxImageDimension {
		return ImageMetadata{}, ErrInvalidImage
	}
	return ImageMetadata{
		MediaType: mediaType,
		Extension: extension,
		Width:     configuration.Width,
		Height:    configuration.Height,
	}, nil
}

func imageFormat(format string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png", ".png", true
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "gif":
		return "image/gif", ".gif", true
	case "webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}

func SafeFileName(value, extension string) string {
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
