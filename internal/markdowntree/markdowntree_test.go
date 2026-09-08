package markdowntree

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"writers.studio/internal/editor"
)

func TestNormalizePathRejectsUnsafeAndConflictingShapes(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"../note.md", "/note.md", `C:\note.md`, "a//b.md", "a/./b.md"} {
		if _, err := NormalizePath(value); err == nil {
			t.Fatalf("NormalizePath(%q) accepted an unsafe path", value)
		}
	}
	if normalized, err := NormalizePath(`chapters\one.md`); err != nil || normalized != "chapters/one.md" {
		t.Fatalf("Windows separators were not normalized: %q, %v", normalized, err)
	}
	if got := PortableSegment("CON"); got != "_CON" {
		t.Fatalf("reserved Windows name was not protected: %q", got)
	}
	composed, err := NormalizePath("Cafe\u0301.md")
	if err != nil || composed != "Café.md" {
		t.Fatalf("Unicode path was not normalized to NFC: %q, %v", composed, err)
	}
}

func TestIsIgnoredPathRecognizesMacOSMetadata(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"Vault/._Note.md",
		"Vault/__MACOSX/._Note.md",
		"Vault/__macosx/nested/Note.md",
	} {
		if !IsIgnoredPath(value) {
			t.Fatalf("IsIgnoredPath(%q) did not recognize macOS metadata", value)
		}
	}
	for _, value := range []string{"Vault/Note.md", "Vault/Note._draft.md", "Vault/__MACOSX.md"} {
		if IsIgnoredPath(value) {
			t.Fatalf("IsIgnoredPath(%q) ignored a user document", value)
		}
	}
}

func TestSplitFrontmatterPreservesRawBytes(t *testing.T) {
	t.Parallel()
	frontmatter := "---\r\ntitle:  Test\r\naliases: [A, B]\r\n---\r\n"
	got, body := SplitFrontmatter(frontmatter + "# Heading\r\n")
	if got != frontmatter || body != "# Heading\r\n" {
		t.Fatalf("frontmatter changed: %q body=%q", got, body)
	}
	withoutClosing := "---\ntitle: broken\nbody"
	if got, body := SplitFrontmatter(withoutClosing); got != "" || body != withoutClosing {
		t.Fatal("unterminated frontmatter must remain Markdown body")
	}
}

func TestBuildProjectPairsDocumentAndFolderAndResolvesVaultLinks(t *testing.T) {
	t.Parallel()
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	frontmatter := "---\ntags: [test, vault]\ncustom:  value\n---\n"
	project, err := BuildProject("Markdown project", "ru", []Entry{
		{Path: "Vault", Directory: true},
		{Path: "Vault/Глава.md", Content: []byte(frontmatter + "# Глава\n\n[[Глава/Глубоко#секция|переход]]\n\n![[assets/pixel.png]]\n")},
		{Path: "Vault/Глава", Directory: true},
		{Path: "Vault/Глава/Глубоко.md", Content: []byte("###### Секция\n\n> [!warning]\n> Осторожно\n\n| A | B |\n|---|---|\n| 1 | 2 |\n")},
		{Path: "Vault/Пустая", Directory: true},
		{Path: "Vault/assets/pixel.png", Content: png},
		{Path: "Vault/.obsidian/workspace.json", Content: []byte("ignored")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Title != "Vault" || project.Structure.Template != "markdown_vault" {
		t.Fatalf("unexpected project metadata: title=%q template=%q", project.Title, project.Structure.Template)
	}
	if project.Report.Documents != 2 || project.Report.Images != 1 || project.Report.ResolvedLinks != 1 {
		t.Fatalf("unexpected report: %+v", project.Report)
	}
	if project.Report.Warnings == nil {
		t.Fatal("an empty warnings collection must be encoded as [] rather than null")
	}
	var chapter, deep *PreparedNode
	for index := range project.Nodes {
		switch project.Nodes[index].Title {
		case "Глава":
			chapter = &project.Nodes[index]
		case "Глубоко":
			deep = &project.Nodes[index]
		}
	}
	if chapter == nil || deep == nil || deep.ParentID == nil || *deep.ParentID != chapter.ID {
		t.Fatalf("file+folder pair was not represented as a content-bearing parent: %+v", project.Nodes)
	}
	if chapter.Frontmatter != frontmatter || !strings.Contains(string(chapter.Content), "/projects/") {
		t.Fatal("frontmatter or canonical internal link was not preserved")
	}
	var document editor.Node
	if err := json.Unmarshal(deep.Content, &document); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(document)
	if !bytes.Contains(encoded, []byte(`"level":6`)) ||
		!bytes.Contains(encoded, []byte(`"type":"callout"`)) ||
		!bytes.Contains(encoded, []byte(`"type":"table"`)) {
		t.Fatalf("GFM/Obsidian blocks were lost: %s", encoded)
	}
}

func TestReadZIPRejectsTraversalAndCaseInsensitiveDuplicates(t *testing.T) {
	t.Parallel()
	makeArchive := func(names ...string) []byte {
		var output bytes.Buffer
		writer := zip.NewWriter(&output)
		for _, name := range names {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = entry.Write([]byte("# note"))
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	if _, err := ReadZIP(makeArchive("../escape.md")); err == nil {
		t.Fatal("ZIP traversal was accepted")
	}
	if _, err := ReadZIP(makeArchive("Note.md", "note.md")); err == nil {
		t.Fatal("case-insensitive duplicate was accepted")
	}
}

func TestFolderAndZIPImportsIgnoreAppleDoubleMarkdown(t *testing.T) {
	t.Parallel()
	appleDouble := []byte{0x00, 0x05, 0x16, 0x07, 0xff, 0xfe}
	directoryProject, err := BuildProject("Vault", "ru", []Entry{
		{Path: "Vault", Directory: true},
		{Path: "Vault/Note.md", Content: []byte("# Real note")},
		{Path: "Vault/._Note.md", Content: appleDouble},
		{Path: "Vault/__MACOSX/._Note.md", Content: appleDouble},
	})
	if err != nil {
		t.Fatalf("folder import rejected AppleDouble metadata: %v", err)
	}
	if directoryProject.Report.Documents != 1 {
		t.Fatalf("folder import created metadata documents: %+v", directoryProject.Report)
	}

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range map[string][]byte{
		"Vault/Note.md":            []byte("# Real note"),
		"Vault/._Note.md":          appleDouble,
		"Vault/__MACOSX/._Note.md": appleDouble,
	} {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write(content); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadZIP(output.Bytes())
	if err != nil {
		t.Fatalf("ZIP import rejected AppleDouble metadata: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "Vault/Note.md" {
		t.Fatalf("ZIP reader retained AppleDouble metadata: %+v", entries)
	}
	archiveProject, err := BuildProject("Vault", "ru", entries)
	if err != nil {
		t.Fatal(err)
	}
	if archiveProject.Report.Documents != 1 {
		t.Fatalf("ZIP import created metadata documents: %+v", archiveProject.Report)
	}
}

func TestMarkdownSemanticRoundTripKeepsListsAndCallouts(t *testing.T) {
	t.Parallel()
	source := []byte("3. Third\n4. Fourth\n\n- Alpha\n- Beta\n\n> [!tip]\n> Portable idea\n")
	parsed, err := ParseMarkdown("note.md", source, nil)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := editor.Markdown(parsed.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(portable, "3. Third") || !strings.Contains(portable, "- Alpha") ||
		!strings.Contains(portable, "> [!TIP]") {
		t.Fatalf("portable Markdown lost semantic blocks: %s", portable)
	}
	reparsed, err := ParseMarkdown("note.md", []byte(portable), nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(reparsed.Content)
	for _, kind := range []string{`"type":"orderedList"`, `"type":"bulletList"`, `"type":"callout"`} {
		if !strings.Contains(encoded, kind) {
			t.Fatalf("round-trip lost %s: %s", kind, encoded)
		}
	}
}

func TestMarkdownCodeRoundTripPreservesLanguageAndWhitespace(t *testing.T) {
	t.Parallel()
	code := "package main  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	raw, err := json.Marshal(editor.Node{
		Type: "doc", Attrs: map[string]any{"schemaVersion": 2},
		Content: []editor.Node{{
			Type: "codeBlock", Attrs: map[string]any{"language": "go", "blockId": "go-code"},
			Content: []editor.Node{{Type: "text", Text: code}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := editor.Markdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMarkdown("main.md", []byte(markdown), nil)
	if err != nil {
		t.Fatal(err)
	}
	var document editor.Node
	if err := json.Unmarshal(parsed.Content, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 1 || document.Content[0].Type != "codeBlock" ||
		document.Content[0].Attrs["language"] != "go" ||
		document.Content[0].Content[0].Text != code {
		t.Fatalf("Markdown code round-trip changed: markdown=%q node=%#v", markdown, document.Content)
	}
}

func TestTrimMarkdownFenceLineEndingRemovesOnlyOneDelimiter(t *testing.T) {
	t.Parallel()
	for input, expected := range map[string]string{
		"code\n":       "code",
		"code\r\n":     "code",
		"code\n\n":     "code\n",
		"code\r\n\r\n": "code\r\n",
		"code":         "code",
	} {
		if got := trimMarkdownFenceLineEnding(input); got != expected {
			t.Fatalf("trimMarkdownFenceLineEnding(%q) = %q, want %q", input, got, expected)
		}
	}
}
