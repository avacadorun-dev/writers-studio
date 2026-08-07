package projectmedia

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
)

func TestInspectImageAndSafeFileName(t *testing.T) {
	var encoded bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 3, 2))
	picture.Set(1, 1, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	metadata, err := InspectImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.MediaType != "image/png" || metadata.Extension != ".png" ||
		metadata.Width != 3 || metadata.Height != 2 {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	for name, test := range map[string]struct {
		value string
		want  string
	}{
		"empty":      {value: "", want: "image.png"},
		"path":       {value: "../../cover.png", want: "cover.png"},
		"windows":    {value: `..\\cover.png`, want: "cover.png"},
		"controls":   {value: "co\x00ver.png", want: "cover.png"},
		"whitespace": {value: "  map.png  ", want: "map.png"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SafeFileName(test.value, ".png"); got != test.want {
				t.Fatalf("SafeFileName() = %q, want %q", got, test.want)
			}
		})
	}
	long := strings.Repeat("я", 300)
	if got := SafeFileName(long, ".png"); len([]rune(got)) != 255 {
		t.Fatalf("long name has %d runes", len([]rune(got)))
	}
}

func TestInspectImageRejectsInvalidInputs(t *testing.T) {
	if _, err := InspectImage(nil); !errors.Is(err, ErrEmptyImage) {
		t.Fatalf("empty image error = %v", err)
	}
	if _, err := InspectImage(make([]byte, MaxImageBytes+1)); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("large image error = %v", err)
	}
	if _, err := InspectImage([]byte("not an image")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("invalid image error = %v", err)
	}
	image.RegisterFormat(
		"unsupported-test", "UNSP", func(_ io.Reader) (image.Image, error) {
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		func(_ io.Reader) (image.Config, error) {
			return image.Config{Width: 1, Height: 1}, nil
		},
	)
	if _, err := InspectImage([]byte("UNSP")); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("unsupported image error = %v", err)
	}
	image.RegisterFormat(
		"png", "WIDE", func(_ io.Reader) (image.Image, error) {
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		func(_ io.Reader) (image.Config, error) {
			return image.Config{Width: MaxImageDimension + 1, Height: 1}, nil
		},
	)
	if _, err := InspectImage([]byte("WIDE")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("oversized dimensions error = %v", err)
	}
	for format, want := range map[string]string{
		"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp",
	} {
		mediaType, extension, ok := imageFormat(format)
		if !ok || mediaType != want || extension == "" {
			t.Fatalf("format %q = %q %q %v", format, mediaType, extension, ok)
		}
	}
	if _, _, ok := imageFormat("tiff"); ok {
		t.Fatal("unknown format was accepted")
	}
}
