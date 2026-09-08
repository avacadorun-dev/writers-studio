package export

import (
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"writers.studio/internal/editor"
)

type syntaxToken struct {
	Text      string
	Colour    string
	Bold      bool
	Italic    bool
	Underline bool
}

var exportLanguageAliases = map[string]string{
	"c#":         "csharp",
	"c++":        "cpp",
	"golang":     "go",
	"html":       "html",
	"js":         "javascript",
	"jsx":        "react",
	"md":         "markdown",
	"py":         "python",
	"rb":         "ruby",
	"sh":         "bash",
	"shell":      "bash",
	"ts":         "typescript",
	"tsx":        "typescript",
	"yml":        "yaml",
	"plaintext":  "text",
	"plain text": "text",
}

func highlightCode(language, source string) []syntaxToken {
	lexerName := strings.ToLower(strings.TrimSpace(language))
	if alias := exportLanguageAliases[lexerName]; alias != "" {
		lexerName = alias
	}
	lexer := lexers.Get(lexerName)
	if lexer == nil || lexerName == "" {
		lexer = lexers.Fallback
	}
	return highlightCodeWithLexer(lexer, source)
}

func highlightCodeWithLexer(lexer chroma.Lexer, source string) []syntaxToken {
	tokens, err := chroma.Tokenise(lexer, nil, source)
	if err != nil {
		return []syntaxToken{{Text: source}}
	}
	var reconstructed strings.Builder
	for _, token := range tokens {
		reconstructed.WriteString(token.Value)
	}
	if reconstructed.String() != source {
		return []syntaxToken{{Text: source}}
	}

	style := styles.Get("github")
	result := make([]syntaxToken, 0, len(tokens))
	for _, token := range tokens {
		if token.Value == "" {
			continue
		}
		entry := style.Get(token.Type)
		current := syntaxToken{
			Text:      token.Value,
			Bold:      entry.Bold == chroma.Yes,
			Italic:    entry.Italic == chroma.Yes,
			Underline: entry.Underline == chroma.Yes,
		}
		if entry.Colour.IsSet() {
			current.Colour = entry.Colour.String()
		}
		if strings.TrimSpace(token.Value) == "" {
			current.Colour = ""
			current.Bold = false
			current.Italic = false
			current.Underline = false
		}
		if len(result) > 0 && sameSyntaxStyle(result[len(result)-1], current) {
			result[len(result)-1].Text += current.Text
			continue
		}
		result = append(result, current)
	}
	if len(result) == 0 {
		return []syntaxToken{{Text: source}}
	}
	return result
}

func sameSyntaxStyle(left, right syntaxToken) bool {
	return left.Colour == right.Colour && left.Bold == right.Bold && left.Italic == right.Italic && left.Underline == right.Underline
}

func highlightedCodeHTML(language, source string) string {
	return syntaxTokensHTML(highlightCode(language, source))
}

func syntaxTokensHTML(tokens []syntaxToken) string {
	var output strings.Builder
	for _, token := range tokens {
		styles := make([]string, 0, 4)
		if token.Colour != "" {
			styles = append(styles, "color:"+token.Colour)
		}
		if token.Bold {
			styles = append(styles, "font-weight:600")
		}
		if token.Italic {
			styles = append(styles, "font-style:italic")
		}
		if token.Underline {
			styles = append(styles, "text-decoration:underline")
		}
		if len(styles) == 0 {
			output.WriteString(html.EscapeString(token.Text))
			continue
		}
		output.WriteString(`<span style="`)
		output.WriteString(strings.Join(styles, ";"))
		output.WriteString(`">`)
		output.WriteString(html.EscapeString(token.Text))
		output.WriteString(`</span>`)
	}
	return output.String()
}

func codeLanguageLabel(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "code"
	}
	return language
}

func codeNodeText(nodes []editor.Node) string {
	var output strings.Builder
	var visit func(editor.Node)
	visit = func(node editor.Node) {
		switch node.Type {
		case "text":
			output.WriteString(node.Text)
		case "hardBreak":
			output.WriteByte('\n')
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	return output.String()
}
