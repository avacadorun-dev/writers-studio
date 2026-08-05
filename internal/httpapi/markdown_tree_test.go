package httpapi

import (
	"encoding/json"
	"testing"
)

func TestDirectoryManifestEntriesPreservesExplicitEmptyDirectories(t *testing.T) {
	t.Parallel()
	manifest, err := json.Marshal(markdownTreeDirectoryManifest{
		RootName: "Vault",
		Entries: []markdownTreeDirectoryManifestEntry{
			{ID: "folder", Path: "Vault/empty", Kind: "directory"},
			{ID: "note", Path: "Vault/note.md", Kind: "file", Size: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := directoryManifestEntries(manifest, map[string][]byte{"note": []byte("# Note")})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || !entries[0].Directory || entries[1].Path != "Vault/empty" ||
		!entries[1].Directory || string(entries[2].Content) != "# Note" {
		t.Fatalf("manifest changed: %+v", entries)
	}
}

func TestDirectoryManifestEntriesRejectsMissingOrMismatchedParts(t *testing.T) {
	t.Parallel()
	manifest := []byte(`{"entries":[{"id":"note","path":"note.md","kind":"file","size":10}]}`)
	if _, err := directoryManifestEntries(manifest, map[string][]byte{"note": []byte("short")}); err == nil {
		t.Fatal("mismatched multipart file size was accepted")
	}
}
