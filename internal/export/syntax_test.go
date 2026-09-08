package export

import (
	"errors"
	"html"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"writers.studio/internal/editor"
)

func TestHighlightCodePreservesEveryCharacterAndRecognizesGoAliases(t *testing.T) {
	t.Parallel()
	source := "package main  \n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	for _, language := range []string{"go", "GoLang", "unknown-language", ""} {
		tokens := highlightCode(language, source)
		var reconstructed strings.Builder
		for _, token := range tokens {
			reconstructed.WriteString(token.Text)
			if strings.TrimSpace(token.Text) == "" && token.Colour != "" {
				t.Fatalf("%s whitespace received an invisible foreground: %#v", language, token)
			}
		}
		if reconstructed.String() != source {
			t.Fatalf("%s highlighting changed source: %q", language, reconstructed.String())
		}
	}

	goTokens := highlightCode("golang", source)
	if len(goTokens) < 4 || !containsColouredSyntaxToken(goTokens, "package") || !containsColouredSyntaxToken(goTokens, "func") {
		t.Fatalf("Go syntax was not highlighted: %#v", goTokens)
	}
}

func TestHighlightedCodeHTMLIsSafeAndKeepsStyledTokens(t *testing.T) {
	t.Parallel()
	source := "if value < 3 {\n\tprintln(\"<ok>\")\n}"
	output := highlightedCodeHTML("go", source)
	if strings.Contains(output, "<ok>") || !strings.Contains(output, "&lt;ok&gt;") || !strings.Contains(output, `style="color:#`) {
		t.Fatalf("highlighted HTML is unsafe or unstyled: %s", output)
	}
	if text := html.UnescapeString(stripSyntaxSpans(output)); text != source {
		t.Fatalf("highlighted HTML changed source: %q", text)
	}
	if codeLanguageLabel("") != "code" || codeLanguageLabel("  Go ") != "Go" {
		t.Fatal("language labels are not normalized for display")
	}
	allStyles := syntaxTokensHTML([]syntaxToken{
		{Text: "plain"},
		{Text: "styled", Colour: "#123456", Bold: true, Italic: true, Underline: true},
	})
	for _, expected := range []string{"color:#123456", "font-weight:600", "font-style:italic", "text-decoration:underline"} {
		if !strings.Contains(allStyles, expected) {
			t.Fatalf("syntax style %q is missing: %s", expected, allStyles)
		}
	}
}

func TestHighlightCodeFallsBackWhenLexerFailsOrChangesText(t *testing.T) {
	t.Parallel()
	for _, lexer := range []chroma.Lexer{
		&testLexer{err: errors.New("broken lexer")},
		&testLexer{tokens: []chroma.Token{{Type: chroma.Text, Value: "changed"}}},
	} {
		tokens := highlightCodeWithLexer(lexer, "source")
		if len(tokens) != 1 || tokens[0].Text != "source" || tokens[0].Colour != "" {
			t.Fatalf("unsafe lexer output was not discarded: %#v", tokens)
		}
	}
	if tokens := highlightCodeWithLexer(&testLexer{}, ""); len(tokens) != 1 || tokens[0].Text != "" {
		t.Fatalf("empty source fallback changed: %#v", tokens)
	}
}

func TestCodeNodeTextPreservesNestedTextAndHardBreaks(t *testing.T) {
	t.Parallel()
	nodes := []editor.Node{
		{Type: "text", Text: "one\t"},
		{Type: "hardBreak"},
		{Type: "wrapper", Content: []editor.Node{{Type: "text", Text: "  two"}}},
	}
	if got := codeNodeText(nodes); got != "one\t\n  two" {
		t.Fatalf("code tree changed: %q", got)
	}
}

func containsColouredSyntaxToken(tokens []syntaxToken, text string) bool {
	for _, token := range tokens {
		if token.Text == text && token.Colour != "" {
			return true
		}
	}
	return false
}

func stripSyntaxSpans(value string) string {
	for {
		start := strings.Index(value, `<span style="`)
		if start < 0 {
			return strings.ReplaceAll(value, "</span>", "")
		}
		end := strings.Index(value[start:], `">`)
		if end < 0 {
			return value
		}
		value = value[:start] + value[start+end+2:]
	}
}

type testLexer struct {
	tokens []chroma.Token
	err    error
}

func (lexer *testLexer) Config() *chroma.Config { return &chroma.Config{Name: "test"} }

func (lexer *testLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	if lexer.err != nil {
		return nil, lexer.err
	}
	index := 0
	return func() chroma.Token {
		if index >= len(lexer.tokens) {
			return chroma.EOF
		}
		token := lexer.tokens[index]
		index++
		return token
	}, nil
}

func (lexer *testLexer) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return lexer }
func (lexer *testLexer) SetAnalyser(func(string) float32) chroma.Lexer  { return lexer }
func (lexer *testLexer) AnalyseText(string) float32                     { return 0 }
