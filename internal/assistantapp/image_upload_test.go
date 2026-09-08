package assistantapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"writers.studio/internal/domain"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/projectmcp"
	"writers.studio/internal/projectmedia"
	"writers.studio/internal/store"
)

type imageObjectStoreProbe struct {
	putErr, deleteErr error
	objects           map[string][]byte
}

func (s *imageObjectStoreProbe) Put(
	_ context.Context, key, _ string, data []byte,
) error {
	if s.putErr != nil {
		return s.putErr
	}
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = append([]byte(nil), data...)
	return nil
}

func (s *imageObjectStoreProbe) PutReader(
	ctx context.Context, key, mediaType string, reader io.Reader, _ int64,
) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return s.Put(ctx, key, mediaType, data)
}

func (s *imageObjectStoreProbe) Get(_ context.Context, key string) ([]byte, error) {
	data, exists := s.objects[key]
	if !exists {
		return nil, errors.New("object not found")
	}
	return append([]byte(nil), data...), nil
}

func (s *imageObjectStoreProbe) Delete(_ context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, key)
	return nil
}

func (s *imageObjectStoreProbe) PresignedGet(
	context.Context, string, time.Duration,
) (string, error) {
	return "", errors.New("not implemented")
}

func TestProjectUploadImageStoresScopedAssetAndReturnsCanonicalBlock(t *testing.T) {
	t.Parallel()
	data := testPNG(t)
	objects := objectstore.NewMemory()
	var captured store.CreateProjectMediaInput
	backend := &ProjectBackend{
		store: &store.Store{}, objects: objects,
		createProjectMedia: func(
			_ context.Context, ownerID string, input store.CreateProjectMediaInput,
		) (domain.ProjectMedia, error) {
			if ownerID != "owner" || input.ProjectID != "project" {
				t.Fatalf("upload escaped execution scope: owner=%q project=%q", ownerID, input.ProjectID)
			}
			captured = input
			return domain.ProjectMedia{
				ID: "asset", ProjectID: input.ProjectID, MediaType: input.MediaType,
				OriginalName: input.OriginalName, SHA256: input.SHA256,
				ByteSize: input.ByteSize, Width: input.Width, Height: input.Height,
				ContentURL: "/api/media/asset/content",
			}, nil
		},
	}
	output, err := backend.Call(context.Background(), projectmcp.ExecutionContext{
		OwnerUserID: "owner", ProjectID: "project",
	}, "project.upload_image", map[string]any{
		"filename": "../../cover.png", "mediaType": "image/png",
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	if output["assetId"] != "asset" || output["uploaded"] != true {
		t.Fatalf("unexpected upload response: %#v", output)
	}
	block, ok := output["imageBlock"].(map[string]any)
	if !ok || block["type"] != "image" || block["assetId"] != "asset" {
		t.Fatalf("canonical image block missing: %#v", output["imageBlock"])
	}
	if captured.OriginalName != "cover.png" || captured.MediaType != "image/png" ||
		captured.Width == nil || *captured.Width != 2 ||
		captured.Height == nil || *captured.Height != 1 ||
		!strings.HasPrefix(captured.StorageKey, "media/project/") {
		t.Fatalf("unexpected stored metadata: %#v", captured)
	}
	stored, err := objects.Get(context.Background(), captured.StorageKey)
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("stored image differs: %v", err)
	}
}

func TestProjectUploadImageValidatesBytesAndCleansFailedMetadata(t *testing.T) {
	t.Parallel()
	data := testPNG(t)
	objects := objectstore.NewMemory()
	var storageKey string
	backend := &ProjectBackend{
		store: &store.Store{}, objects: objects,
		createProjectMedia: func(
			_ context.Context, _ string, input store.CreateProjectMediaInput,
		) (domain.ProjectMedia, error) {
			storageKey = input.StorageKey
			return domain.ProjectMedia{}, errors.New("database rejected media")
		},
	}
	execution := projectmcp.ExecutionContext{OwnerUserID: "owner", ProjectID: "project"}
	for name, arguments := range map[string]map[string]any{
		"invalid base64": {
			"filename": "x.png", "mediaType": "image/png", "dataBase64": "***",
		},
		"invalid image": {
			"filename": "x.png", "mediaType": "image/png",
			"dataBase64": base64.StdEncoding.EncodeToString([]byte("not image")),
		},
		"mismatched MIME": {
			"filename": "x.jpg", "mediaType": "image/jpeg",
			"dataBase64": base64.RawStdEncoding.EncodeToString(data),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := backend.Call(
				context.Background(), execution, "project.upload_image", arguments,
			); err == nil {
				t.Fatal("invalid upload was accepted")
			}
		})
	}
	_, err := backend.Call(context.Background(), execution, "project.upload_image", map[string]any{
		"filename": "x.png", "mediaType": "image/png",
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
	if err == nil || !strings.Contains(err.Error(), "database rejected media") {
		t.Fatalf("metadata failure was not surfaced: %v", err)
	}
	if storageKey == "" {
		t.Fatal("storage key was not captured")
	}
	if _, err := objects.Get(context.Background(), storageKey); err == nil {
		t.Fatal("orphaned object was not cleaned up")
	}

	unavailable := &ProjectBackend{store: &store.Store{}}
	if _, err := unavailable.Call(
		context.Background(), execution, "project.upload_image", map[string]any{},
	); err == nil || !strings.Contains(err.Error(), "storage is unavailable") {
		t.Fatalf("missing storage error = %v", err)
	}
}

func TestCanonicalImageReferencesMustBelongToOpenProject(t *testing.T) {
	t.Parallel()
	const assetID = "11111111-1111-1111-1111-111111111111"
	execution := projectmcp.ExecutionContext{OwnerUserID: "owner", ProjectID: "project"}
	content := map[string]any{"blocks": []any{
		map[string]any{"type": "image", "assetId": assetID, "alt": "Карта", "caption": "Мир"},
		map[string]any{"type": "image", "assetId": assetID, "alt": "Карта 2", "caption": "Мир 2"},
	}}
	lookups := 0
	backend := &ProjectBackend{getProjectMedia: func(
		_ context.Context, ownerID, requestedID string,
	) (domain.ProjectMedia, error) {
		lookups++
		if ownerID != "owner" || requestedID != assetID {
			t.Fatalf("unscoped media lookup: owner=%q asset=%q", ownerID, requestedID)
		}
		return domain.ProjectMedia{
			ID: assetID, ProjectID: "project", MediaType: "image/png",
		}, nil
	}}
	if err := backend.validateContentImageAssets(
		context.Background(), execution, content,
	); err != nil {
		t.Fatal(err)
	}
	if lookups != 1 {
		t.Fatalf("duplicate asset was looked up %d times", lookups)
	}
	backend.getProjectMedia = func(
		context.Context, string, string,
	) (domain.ProjectMedia, error) {
		return domain.ProjectMedia{ProjectID: "other", MediaType: "image/png"}, nil
	}
	if err := backend.validateContentImageAssets(
		context.Background(), execution, content,
	); err == nil {
		t.Fatal("foreign-project image was accepted")
	}
	backend.getProjectMedia = func(
		context.Context, string, string,
	) (domain.ProjectMedia, error) {
		return domain.ProjectMedia{ProjectID: "project", MediaType: "application/pdf"}, nil
	}
	if err := backend.validateContentImageAssets(
		context.Background(), execution, content,
	); err == nil {
		t.Fatal("non-image asset was accepted as an image block")
	}
	backend.getProjectMedia = nil
	if err := backend.validateContentImageAssets(
		context.Background(), execution, map[string]any{
			"blocks": []any{map[string]any{"type": "paragraph", "text": "Без картинки"}},
		},
	); err != nil {
		t.Fatalf("content without assets required media storage: %v", err)
	}
	backend.getProjectMedia = func(
		context.Context, string, string,
	) (domain.ProjectMedia, error) {
		return domain.ProjectMedia{}, errors.New("lookup failed")
	}
	if err := backend.validateContentImageAssets(
		context.Background(), execution, content,
	); err == nil {
		t.Fatal("failed asset lookup was accepted")
	}
	if err := backend.validateContentImageAssets(
		context.Background(), execution, map[string]any{"blocks": []any{}},
	); err == nil {
		t.Fatal("malformed canonical content was accepted")
	}
}

func TestProjectUploadImageCoversStorageFailureBoundaries(t *testing.T) {
	t.Parallel()
	data := testPNG(t)
	execution := projectmcp.ExecutionContext{OwnerUserID: "owner", ProjectID: "project"}
	encoded := base64.StdEncoding.EncodeToString(data)
	create := func(
		context.Context, string, store.CreateProjectMediaInput,
	) (domain.ProjectMedia, error) {
		return domain.ProjectMedia{}, errors.New("metadata failed")
	}

	backend := &ProjectBackend{
		store: &store.Store{}, objects: &imageObjectStoreProbe{}, createProjectMedia: create,
	}
	if _, err := backend.callDirect(
		context.Background(), execution, "unknown.direct", nil,
	); err == nil {
		t.Fatal("unknown direct tool was accepted")
	}
	if _, err := backend.Call(
		context.Background(), execution, "project.upload_image", map[string]any{
			"filename": "x.png", "mediaType": "image/png", "dataBase64": "",
		},
	); err == nil {
		t.Fatal("empty upload was accepted")
	}
	if _, err := backend.Call(
		context.Background(), execution, "project.upload_image", map[string]any{
			"filename": "x.png", "mediaType": "image/png",
			"dataBase64": strings.Repeat("A", projectmedia.MaxBase64Length+1),
		},
	); !errors.Is(err, projectmedia.ErrImageTooLarge) {
		t.Fatalf("oversized base64 error = %v", err)
	}

	backend.objects = &imageObjectStoreProbe{putErr: errors.New("storage failed")}
	if _, err := backend.Call(
		context.Background(), execution, "project.upload_image", map[string]any{
			"filename": "x.png", "mediaType": "image/png", "dataBase64": encoded,
		},
	); err == nil || !strings.Contains(err.Error(), "store project image") {
		t.Fatalf("storage failure error = %v", err)
	}

	backend.objects = &imageObjectStoreProbe{deleteErr: errors.New("cleanup failed")}
	if _, err := backend.Call(
		context.Background(), execution, "project.upload_image", map[string]any{
			"filename": "x.png", "mediaType": "image/png", "dataBase64": encoded,
		},
	); err == nil || !strings.Contains(err.Error(), "cleanup object") {
		t.Fatalf("cleanup failure error = %v", err)
	}

	for name, expected := range map[string]bool{
		"document.append": true, "note.create": true, "document.set_title": false,
	} {
		if got := mutationAcceptsCanonicalContent(name); got != expected {
			t.Fatalf("mutationAcceptsCanonicalContent(%q) = %v", name, got)
		}
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 1))
	picture.Set(0, 0, color.RGBA{G: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
