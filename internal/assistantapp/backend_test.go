package assistantapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"writers.studio/internal/assistant"
	"writers.studio/internal/config"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	"writers.studio/internal/llm"
	"writers.studio/internal/projectmcp"
)

type providerProbeSequence struct {
	values []any
}

func (p *providerProbeSequence) Complete(
	_ context.Context,
	_ llm.Request,
) (llm.Completion, error) {
	if len(p.values) == 0 {
		return llm.Completion{}, errors.New("probe sequence exhausted")
	}
	value := p.values[0]
	p.values = p.values[1:]
	switch typed := value.(type) {
	case llm.Completion:
		return typed, nil
	case error:
		return llm.Completion{}, typed
	default:
		return llm.Completion{}, errors.New("invalid probe sequence value")
	}
}

func TestTransferContentPreservesProvenanceAndCreatesIndependentBlocks(t *testing.T) {
	t.Parallel()
	source := json.RawMessage(`{
		"type":"doc",
		"content":[
			{"type":"paragraph","attrs":{"blockId":"original","sourceRefs":["segment-1"]},
			 "content":[{"type":"text","text":"Первый абзац"}]},
			{"type":"blockquote","attrs":{"blockId":"quote"},
			 "content":[{"type":"text","text":"Цитата"}]}
		]
	}`)
	cloned, err := cloneDocumentForTransfer(source, "document:source@3")
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.Validate(cloned); err != nil {
		t.Fatal(err)
	}
	var document editor.Node
	if err := json.Unmarshal(cloned, &document); err != nil {
		t.Fatal(err)
	}
	for _, block := range document.Content {
		if block.Attrs["blockId"] == "original" || block.Attrs["blockId"] == "quote" {
			t.Fatal("transferred block retained a source block ID")
		}
		references := stringValues(block.Attrs["sourceRefs"])
		if !sliceContainsFold(references, "document:source@3") {
			t.Fatalf("source provenance missing: %#v", references)
		}
	}
	if !strings.Contains(string(source), `"blockId":"original"`) {
		t.Fatal("source document was mutated")
	}
	if _, err := cloneDocumentForTransfer(json.RawMessage(`{"type":"paragraph"}`), "x"); err == nil {
		t.Fatal("non-document transfer source was accepted")
	}
}

func TestRichTransferAndAppendPreserveCompatibilityMetadata(t *testing.T) {
	t.Parallel()

	source := json.RawMessage(`{
		"type":"doc","attrs":{"schemaVersion":2},"content":[
			{"type":"callout","attrs":{"kind":"idea","blockId":"callout-original"},"content":[
				{"type":"paragraph","attrs":{"blockId":"paragraph-original","sourceRefs":[]},"content":[{"type":"text","text":"Идея"}]}
			]}
		]
	}`)
	cloned, err := cloneDocumentForTransfer(source, "document:source@2")
	if err != nil {
		t.Fatal(err)
	}
	var clone editor.Node
	if err := json.Unmarshal(cloned, &clone); err != nil {
		t.Fatal(err)
	}
	if clone.Content[0].Attrs["blockId"] == "callout-original" ||
		clone.Content[0].Content[0].Attrs["blockId"] == "paragraph-original" {
		t.Fatal("rich transfer retained source block identities")
	}
	if version, err := editor.SchemaVersion(cloned); err != nil || version != 2 {
		t.Fatalf("rich transfer schema version = %d, %v", version, err)
	}

	legacy := json.RawMessage(
		`{"type":"doc","attrs":{"schemaVersion":1},"content":[{"type":"paragraph","attrs":{"blockId":"legacy","sourceRefs":[]},"content":[{"type":"text","text":"Текст"}]}]}`,
	)
	proposed, err := buildDocumentProposal(
		"document.append",
		legacy,
		map[string]any{"content": json.RawMessage(`{"type":"mermaidBlock","attrs":{"code":"flowchart TD\nA-->B"}}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := editor.SchemaVersion(proposed); err != nil || version != 2 {
		t.Fatalf("rich append schema version = %d, %v", version, err)
	}
}

func TestTranscriptTransferPrefersEditedDraftAndFallsBackToArtifacts(t *testing.T) {
	t.Parallel()
	draftContent, err := documentFromPlainText("Черновик автора.\n\nВторой абзац.", "segment:draft")
	if err != nil {
		t.Fatal(err)
	}
	content, label, err := transcriptTransferContent(domain.DictationSession{
		ID: "session-1",
		Draft: &domain.TranscriptDraft{
			Revision: 4, Content: draftContent, PlainText: "Черновик автора.",
		},
		SuggestionArtifact: &domain.TranscriptArtifact{
			Kind: "suggestion", Text: "Не должен победить черновик",
		},
	})
	if err != nil || label != "draft@4" {
		t.Fatalf("draft transfer: label=%q err=%v", label, err)
	}
	plain, err := editor.PlainText(content)
	if err != nil || !strings.Contains(plain, "Второй абзац") ||
		strings.Contains(plain, "Не должен") {
		t.Fatalf("wrong draft content: %q, %v", plain, err)
	}

	content, label, err = transcriptTransferContent(domain.DictationSession{
		ID: "session-2",
		SuggestionArtifact: &domain.TranscriptArtifact{
			Kind: "suggestion",
			Segments: []domain.TranscriptSegment{{
				ID: "segment-2", Text: "Текст из сегмента",
			}},
		},
	})
	if err != nil || label != "suggestion" {
		t.Fatalf("artifact transfer: label=%q err=%v", label, err)
	}
	plain, _ = editor.PlainText(content)
	if plain != "Текст из сегмента" {
		t.Fatalf("unexpected artifact text: %q", plain)
	}

	content, label, err = transcriptTransferContent(domain.DictationSession{
		ID: "session-3",
		BatchArtifact: &domain.TranscriptArtifact{
			Kind: "batch", Text: "Первый.\r\n\r\nВторой.",
		},
	})
	if err != nil || label != "batch" {
		t.Fatalf("plain artifact transfer: label=%q err=%v", label, err)
	}
	plain, _ = editor.PlainText(content)
	if !strings.Contains(plain, "Первый.") || !strings.Contains(plain, "Второй.") {
		t.Fatalf("plain artifact paragraphs missing: %q", plain)
	}
	if _, _, err := transcriptTransferContent(domain.DictationSession{ID: "empty"}); err == nil {
		t.Fatal("empty transcript was accepted")
	}
	if _, err := documentFromPlainText(" \n\n ", "x"); err == nil {
		t.Fatal("empty plain text was accepted")
	}
}

func TestDocumentProposalSupportsAppendInsertReplaceAndMarks(t *testing.T) {
	t.Parallel()
	current := json.RawMessage(`{
		"type":"doc",
		"content":[{"type":"paragraph","attrs":{"blockId":"p1","sourceRefs":[]},
			"content":[{"type":"text","text":"Один два три"}]}]
	}`)
	appendDoc := map[string]any{
		"type": "doc",
		"content": []any{map[string]any{
			"type":    "paragraph",
			"attrs":   map[string]any{"blockId": "p2", "sourceRefs": []string{}},
			"content": []any{map[string]any{"type": "text", "text": "Четыре"}},
		}},
	}
	appended, err := buildDocumentProposal(
		"document.append",
		current,
		map[string]any{"content": appendDoc},
	)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := editor.PlainText(appended)
	if !strings.Contains(plain, "Четыре") {
		t.Fatalf("append failed: %q", plain)
	}

	inserted, err := buildDocumentProposal(
		"document.insert",
		current,
		map[string]any{
			"position": 0,
			"content": map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": "Перед"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ = editor.PlainText(inserted)
	if !strings.HasPrefix(plain, "Перед") {
		t.Fatalf("insert failed: %q", plain)
	}
	if _, err := buildDocumentProposal(
		"document.insert",
		current,
		map[string]any{"position": 99, "content": map[string]any{"type": "paragraph"}},
	); err == nil {
		t.Fatal("out-of-bounds insertion was accepted")
	}

	replaced, err := buildDocumentProposal(
		"document.replace_range",
		current,
		map[string]any{"edits": []any{
			map[string]any{
				"blockId": "p1", "start": 0, "end": 4,
				"expectedText": "Один", "replacement": "Новый",
			},
			map[string]any{
				"blockId": "p1", "start": 9, "end": 12,
				"expectedText": "три", "replacement": "текст",
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ = editor.PlainText(replaced)
	if plain != "Новый два текст" {
		t.Fatalf("atomic range proposal was not accepted: %q", plain)
	}

	marked, err := buildDocumentProposal(
		"document.apply_marks",
		current,
		map[string]any{
			"blockId": "p1", "start": 4, "end": 7,
			"marks": []any{map[string]any{"type": "italic", "href": nil}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var markedDocument editor.Node
	_ = json.Unmarshal(marked, &markedDocument)
	if len(markedDocument.Content[0].Content) != 3 ||
		len(markedDocument.Content[0].Content[1].Marks) != 1 {
		t.Fatalf("mark did not split the inline text: %#v", markedDocument.Content[0].Content)
	}
	codeMarked, err := buildDocumentProposal(
		"document.apply_marks",
		current,
		map[string]any{
			"blockId": "p1", "start": 4, "end": 7,
			"marks": []any{map[string]any{"type": "code", "href": nil}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := editor.SchemaVersion(codeMarked); err != nil || version != 2 {
		t.Fatalf("inline code schema version = %d, %v", version, err)
	}
	if _, err := applyDocumentMarks(current, map[string]any{
		"blockId": "missing", "start": 0, "end": 1, "marks": []any{},
	}); err == nil {
		t.Fatal("unknown mark block was accepted")
	}

	normalized, err := normalizeAssistantContent(map[string]any{
		"blocks": []any{
			map[string]any{"type": "heading", "text": "Сцена"},
			map[string]any{"type": "paragraph", "text": "Лиора вошла."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err = editor.PlainText(normalized)
	if err != nil || !strings.Contains(plain, "Сцена") ||
		!strings.Contains(plain, "Лиора вошла.") {
		t.Fatalf("assistant blocks were not normalized: %q %v", plain, err)
	}
	var normalizedDocument editor.Node
	if json.Unmarshal(normalized, &normalizedDocument) != nil ||
		normalizedDocument.Content[0].Attrs["blockId"] == "" {
		t.Fatalf("assistant block metadata missing: %#v", normalizedDocument)
	}
	if _, err := normalizeAssistantContent(map[string]any{
		"blocks": []any{map[string]any{"type": "table", "text": "bad"}},
	}); err == nil {
		t.Fatal("unsupported assistant block type was accepted")
	}
}

func TestDocumentRangeReplacementIsAtomicAndPreservesUntouchedContent(t *testing.T) {
	t.Parallel()
	current := json.RawMessage(`{
		"type":"doc",
		"content":[
			{"type":"paragraph","attrs":{"blockId":"intro","sourceRefs":["voice"]},
			 "content":[{"type":"text","text":"Вступление."}]},
			{"type":"paragraph","attrs":{"blockId":"reply-1","sourceRefs":["voice"]},
			 "content":[{"type":"text","text":"— Прямой ","marks":[{"type":"italic"}]},
			            {"type":"text","text":"вопрос."}]},
			{"type":"paragraph","attrs":{"blockId":"reply-2","sourceRefs":["voice"]},
			 "content":[{"type":"text","text":"— Последний ответ."}]}
		]
	}`)
	proposed, err := buildDocumentProposal(
		"document.replace_range",
		current,
		map[string]any{"edits": []any{
			map[string]any{
				"blockId": "reply-1", "start": 2, "end": 15,
				"expectedText": "Прямой вопрос", "replacement": "Ты всё-таки здесь",
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	target, err := assistant.NewExplicitResolvedTarget(
		"node",
		1,
		current,
		"Сделай диалог менее прямолинейным, но не меняй последнюю реплику",
		[]assistant.TextRange{{
			BlockID: "reply-1", Start: 2, End: 15, Reason: "selected reply",
		}},
		[]assistant.TextRange{{
			BlockID: "reply-2", Start: 0, End: 18, Reason: "protected final reply",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := assistant.ValidateProposalScope(current, proposed, target); err != nil {
		t.Fatalf("scoped atomic edit rejected: %v", err)
	}
	var document editor.Node
	if err := json.Unmarshal(proposed, &document); err != nil {
		t.Fatal(err)
	}
	if got := document.Content[0].Attrs["sourceRefs"]; !reflect.DeepEqual(got, []any{"voice"}) &&
		!reflect.DeepEqual(got, []string{"voice"}) {
		t.Fatalf("untouched provenance changed: %#v", got)
	}
	if len(document.Content[1].Content) == 0 ||
		len(document.Content[1].Content[0].Marks) != 1 {
		t.Fatalf("replacement did not preserve local marks: %#v", document.Content[1].Content)
	}
}

func TestDocumentRangeReplacementRejectsInvalidBatches(t *testing.T) {
	t.Parallel()
	current := json.RawMessage(`{
		"type":"doc",
		"content":[{"type":"paragraph","attrs":{"blockId":"p1","sourceRefs":[]},
			"content":[{"type":"text","text":"Один два три"}]}]
	}`)
	tests := []map[string]any{
		{"edits": []any{}},
		{"edits": []any{map[string]any{
			"blockId": "missing", "start": 0, "end": 1,
			"expectedText": "О", "replacement": "x",
		}}},
		{"edits": []any{map[string]any{
			"blockId": "p1", "start": 0, "end": 99,
			"expectedText": "", "replacement": "x",
		}}},
		{"edits": []any{map[string]any{
			"blockId": "p1", "start": 0, "end": 4,
			"expectedText": "Точно не Один", "replacement": "x",
		}}},
		{"edits": []any{
			map[string]any{
				"blockId": "p1", "start": 0, "end": 5,
				"expectedText": "Один ", "replacement": "x",
			},
			map[string]any{
				"blockId": "p1", "start": 4, "end": 7,
				"expectedText": " дв", "replacement": "y",
			},
		}},
		{"edits": []any{map[string]any{
			"blockId": "p1", "start": 1, "end": 1,
			"expectedText": "", "replacement": "",
		}}},
	}
	for index, arguments := range tests {
		if _, err := replaceDocumentRanges(current, arguments); err == nil {
			t.Fatalf("invalid edit batch %d was accepted", index)
		}
	}
}

func TestBackendHelpersAndUnavailableBackend(t *testing.T) {
	t.Parallel()
	var backend *ProjectBackend
	if _, err := backend.Call(
		context.Background(),
		projectmcp.ExecutionContext{},
		"project.get_overview",
		nil,
	); err == nil {
		t.Fatal("nil backend was usable")
	}
	backend = &ProjectBackend{pending: make(map[string][]assistant.ChangeOperation)}
	if backend.PendingCount("turn") != 0 {
		t.Fatal("new backend contains pending mutations")
	}
	backend.pending["turn"] = []assistant.ChangeOperation{{ToolName: "x"}}
	if backend.PendingCount("turn") != 1 {
		t.Fatal("pending mutation was not counted")
	}
	backend.DiscardPending("turn")
	if backend.PendingCount("turn") != 0 {
		t.Fatal("pending mutation was not discarded")
	}

	if boundedInt(0, 7, 1, 10) != 7 ||
		boundedInt(-4, 7, 1, 10) != 1 ||
		boundedInt(99, 7, 1, 10) != 10 {
		t.Fatal("bounded integer normalization failed")
	}
	if intArg(map[string]any{"x": json.Number("12")}, "x") != 12 ||
		intArg(map[string]any{"x": float64(3)}, "x") != 3 {
		t.Fatal("numeric tool arguments were not decoded")
	}
	if nullableStringArg(map[string]any{"x": " "}, "x") != nil ||
		nullableStringArg(map[string]any{"x": " value "}, "x") == nil {
		t.Fatal("nullable string normalization failed")
	}
	if !sameOptionalString(nil, nil) || sameOptionalString(nil, ptrString("x")) {
		t.Fatal("optional string comparison failed")
	}
	if groupKeyForTool("project_memory.add") != "memory" ||
		groupKeyForTool("document.append") != "document" {
		t.Fatal("change grouping failed")
	}
}

func TestProviderRegistrySecurityAndCredentialFlow(t *testing.T) {
	t.Parallel()
	validKey := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	registry, err := NewProviderRegistry(config.Config{
		LLMAdapter:        "openai_compatible",
		LLMBaseURL:        "https://example.com/v1",
		LLMAPIKey:         "system-key",
		LLMModel:          "model",
		LLMDialect:        "chat_completions",
		LLMCredentialKey:  validKey,
		LLMRequestTimeout: time.Second,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.ConnectionsEnabled() {
		t.Fatal("credential-backed connections are disabled")
	}
	reference, ok := registry.SystemProviderRef(context.Background(), false)
	if !ok || reference.Kind != assistant.ProviderSystem ||
		reference.Capabilities.CheckedWithModel != "model" {
		t.Fatalf("bad system provider reference: %#v", reference)
	}
	secret, err := registry.EncryptCredential("connection", "sk-secret")
	if err != nil || secret.Last4 != "cret" {
		t.Fatalf("credential encryption failed: %#v %v", secret, err)
	}
	headers, nonce, err := registry.EncryptHeaders(
		"connection",
		map[string]string{"X-Tenant": "writer"},
	)
	if err != nil || len(headers) == 0 || len(nonce) == 0 {
		t.Fatalf("header encryption failed: %v", err)
	}

	normalized, err := NormalizeProviderBaseURL("https://example.com/v1///")
	if err != nil || normalized != "https://example.com/v1" {
		t.Fatalf("base URL normalization failed: %q %v", normalized, err)
	}
	for _, invalid := range []string{
		"ftp://example.com", "https://user:pass@example.com",
		"https://example.com?v=1", "https://example.com/#fragment", "not a url",
	} {
		if _, err := NormalizeProviderBaseURL(invalid); err == nil {
			t.Fatalf("unsafe base URL accepted: %s", invalid)
		}
	}

	disabled, err := NewProviderRegistry(config.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.ConnectionsEnabled() {
		t.Fatal("connections enabled without a credential key")
	}
	if _, _, err := disabled.EncryptHeaders(
		"id",
		map[string]string{"X-Test": "value"},
	); !errors.Is(err, llm.ErrCredentialKeyMissing) {
		t.Fatalf("missing credential key was not reported: %v", err)
	}
	var missing *ProviderRegistry
	if _, err := missing.ProviderForRun(
		context.Background(),
		"owner",
		assistant.Run{Provider: assistant.ProviderRef{Kind: assistant.ProviderSystem}},
	); err == nil {
		t.Fatal("nil provider registry was usable")
	}
}

func TestProviderProbeRetriesOnceAndReportsInstability(t *testing.T) {
	t.Parallel()
	provider := &providerProbeSequence{values: []any{
		errors.New("temporary outage"),
		llm.Completion{
			ResponseID: "one",
			ToolCalls: []llm.ToolCall{{
				ID: "call-one", Name: "probe_echo",
				Arguments: json.RawMessage(`{"value":"alpha"}`),
			}},
			Continuation: []json.RawMessage{json.RawMessage(`{"role":"assistant"}`)},
		},
		llm.Completion{
			ResponseID: "two",
			ToolCalls: []llm.ToolCall{{
				ID: "call-two", Name: "probe_finish",
				Arguments: json.RawMessage(`{"previous":"alpha"}`),
			}},
			Continuation: []json.RawMessage{json.RawMessage(`{"role":"assistant"}`)},
		},
		llm.Completion{ResponseID: "three", Text: "OK"},
	}}
	report := probeProviderCompatibility(
		context.Background(),
		provider,
		assistant.DialectChatCompletions,
		"model",
	)
	if report.Status != "limited" || !report.Capabilities.CanRunAssistant() ||
		len(report.Capabilities.Limitations) < 2 ||
		!strings.Contains(report.Detail, "со второй попытки") || len(provider.values) != 0 {
		t.Fatalf("transient provider failure was not recovered: %#v", report)
	}
}

func TestProviderProbeDoesNotRepeatAfterContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	provider := &providerProbeSequence{values: []any{
		errors.New("offline"),
		errors.New("must remain unused"),
	}}
	cancel()
	report := probeProviderCompatibility(
		ctx,
		provider,
		assistant.DialectChatCompletions,
		"model",
	)
	if report.Status != "unavailable" || len(provider.values) != 1 {
		t.Fatalf("cancelled probe retried: %#v values=%d", report, len(provider.values))
	}
}

func TestConversationInputKeepsRecentOrderedContext(t *testing.T) {
	t.Parallel()
	turns := make([]assistant.Turn, 0, 14)
	for index := range 14 {
		turns = append(turns, assistant.Turn{
			ID:            "turn-" + string(rune('a'+index)),
			Instruction:   "instruction",
			AssistantText: "answer",
		})
	}
	current := turns[len(turns)-1].ID
	input := conversationInput(turns, current)
	if len(input) != 23 {
		t.Fatalf("unexpected bounded conversation length: %d", len(input))
	}
	if !strings.Contains(string(input[len(input)-1]), "instruction") {
		t.Fatal("current user instruction is missing")
	}
}

func TestAssistantTextSanitization(t *testing.T) {
	t.Parallel()
	raw := "<think>private reasoning</think>\n\nГотово.\n\n\n<analysis>hidden</analysis>"
	if got := sanitizeAssistantText(raw); got != "Готово." {
		t.Fatalf("hidden reasoning was exposed: %q", got)
	}
}

func TestStructuredTurnModeDeclaration(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"answer", "change", "clarify"} {
		got, err := decodeTurnMode(mustJSON(map[string]any{
			"mode": mode, "summary": "Краткий результат",
		}))
		if err != nil || got.Mode != mode || got.Summary != "Краткий результат" {
			t.Fatalf("valid semantic mode %q was rejected: %#v %v", mode, got, err)
		}
	}
	for _, value := range []json.RawMessage{
		json.RawMessage(`{"mode":"unknown","summary":"unsupported"}`),
		json.RawMessage(`{"mode":"answer","summary":" "}`),
		json.RawMessage(`{"mode":`),
	} {
		if _, err := decodeTurnMode(value); err == nil {
			t.Fatalf("invalid semantic declaration was accepted: %s", value)
		}
	}
	nodeID := "node-id"
	nodePrompt := turnModeInstructions(assistant.Run{
		TargetKind: assistant.TargetNode, TargetNodeID: &nodeID,
	})
	if !strings.Contains(nodePrompt, "active node target") ||
		!strings.Contains(nodePrompt, "reviewable proposed difference") ||
		!strings.Contains(nodePrompt, "project-memory record") ||
		!strings.Contains(nodePrompt, "cannot be recovered") {
		t.Fatalf("semantic declaration lacks outcome/scope rules: %q", nodePrompt)
	}
	projectPrompt := turnModeInstructions(assistant.Run{TargetKind: assistant.TargetProject})
	if !strings.Contains(projectPrompt, "whole project") {
		t.Fatalf("project declaration scope is missing: %q", projectPrompt)
	}
	if got := stagedAssistantText(assistant.ChangeSet{RiskLevel: assistant.RiskLow}, false); !strings.Contains(
		got,
		"точный diff",
	) {
		t.Fatalf("low-risk review handoff is unclear: %q", got)
	}
	if got := stagedAssistantText(assistant.ChangeSet{RiskLevel: assistant.RiskHigh}, false); !strings.Contains(
		got,
		"область действия",
	) {
		t.Fatalf("high-risk review warning is missing: %q", got)
	}
	if got := stagedAssistantText(assistant.ChangeSet{RiskLevel: assistant.RiskLow}, true); !strings.Contains(got, "только часть") || !strings.Contains(got, "оставшиеся замечания") {
		t.Fatalf("partial review handoff hides incomplete work: %q", got)
	}
}

func TestStructuredMutationEscalatesAnswerModeWithoutLanguageGuessing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mode      string
		tool      string
		wantMode  string
		escalated bool
		wantError bool
	}{
		{name: "read remains answer", mode: "answer", tool: "project_memory.list", wantMode: "answer"},
		{name: "mutation escalates answer", mode: "answer", tool: "project_memory.add", wantMode: "change", escalated: true},
		{name: "change remains change", mode: "change", tool: "document.append", wantMode: "change"},
		{name: "clarification cannot mutate", mode: "clarify", tool: "dictionary.add", wantMode: "clarify", wantError: true},
		{name: "missing declaration is rejected", tool: "document.append", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, escalated, err := reconcileTurnModeWithTool(test.mode, test.tool)
			if (err != nil) != test.wantError || mode != test.wantMode || escalated != test.escalated {
				t.Fatalf("mode reconciliation = %q/%v/%v", mode, escalated, err)
			}
		})
	}

	first := llm.ToolCall{ID: "one", Name: "project_memory.add", Arguments: json.RawMessage(`{"statement":"one"}`)}
	second := llm.ToolCall{ID: "two", Name: "project_memory.add", Arguments: json.RawMessage(`{"statement":"two"}`)}
	toolError := errors.New("validation failed")
	if repeatedToolFailureKey(first, toolError) == repeatedToolFailureKey(second, toolError) {
		t.Fatal("distinct parallel mutations were counted as one repeated failure")
	}
}

func TestProviderFailureClassificationAndSafeDiagnostics(t *testing.T) {
	t.Parallel()
	timeout := &llm.ProviderHTTPError{
		StatusCode:   http.StatusGatewayTimeout,
		ProviderCode: "upstream_timeout",
		ErrorType:    "provider_error",
		ProviderName: "OpenRouter upstream",
		Message:      "sensitive provider detail",
	}
	if providerRequestErrorCode(timeout) != "provider_timeout" {
		t.Fatalf("gateway timeout was not classified: %v", timeout)
	}
	if providerRequestErrorCode(&llm.ProviderHTTPError{StatusCode: 524}) != "provider_timeout" {
		t.Fatal("OpenRouter/edge 524 was not classified as provider timeout")
	}
	diagnostics := safeProviderFailureData(timeout)
	encoded := string(mustJSON(diagnostics))
	if diagnostics["providerStatus"] != http.StatusGatewayTimeout ||
		diagnostics["providerCode"] != "upstream_timeout" ||
		strings.Contains(encoded, "sensitive provider detail") {
		t.Fatalf("provider diagnostics are unsafe or incomplete: %s", encoded)
	}
	requestFailure := &llm.ProviderHTTPError{StatusCode: http.StatusBadGateway}
	if providerRequestErrorCode(requestFailure) != "provider_request_failed" {
		t.Fatal("generic upstream failure was mislabeled as a timeout")
	}
}

func TestFindExactDocumentAnchorsUsesBlockLocalUnicodeCoordinates(t *testing.T) {
	t.Parallel()
	node := domain.DocumentNode{
		ID:              "node",
		CurrentRevision: 7,
		Content: json.RawMessage(`{
			"type":"doc",
			"content":[
				{"type":"paragraph","attrs":{"blockId":"p1"},
				 "content":[{"type":"text","text":"Сияние и сияние"}]},
				{"type":"paragraph","attrs":{"blockId":"p2"},
				 "content":[{"type":"text","text":"сияние"}]}
			]
		}`),
	}
	result, err := findExactDocumentAnchors(node, "сияние", nil)
	if err != nil {
		t.Fatal(err)
	}
	matches := result["matches"].([]map[string]any)
	if result["matchCount"] != 2 || len(matches) != 2 ||
		matches[0]["blockId"] != "p1" || matches[0]["start"] != 9 ||
		matches[0]["end"] != 15 || matches[0]["expectedText"] != "сияние" ||
		matches[1]["blockId"] != "p2" || matches[1]["start"] != 0 {
		t.Fatalf("exact Unicode anchors are wrong: %#v", result)
	}
	blockID := "p2"
	scoped, err := findExactDocumentAnchors(node, "сияние", &blockID)
	if err != nil || scoped["matchCount"] != 1 {
		t.Fatalf("block-scoped exact lookup failed: %#v %v", scoped, err)
	}
	missing, err := findExactDocumentAnchors(node, "нет", nil)
	if err != nil || missing["matchCount"] != 0 {
		t.Fatalf("missing literal lookup was not an empty success: %#v %v", missing, err)
	}
	if _, err := findExactDocumentAnchors(node, "", nil); err == nil {
		t.Fatal("empty exact lookup was accepted")
	}
}

func TestFindExactDocumentAnchorsSupportsLegacyBlocksWithoutIDs(t *testing.T) {
	t.Parallel()
	node := domain.DocumentNode{
		ID: "legacy", CurrentRevision: 4,
		Content: json.RawMessage(`{
			"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"Старый свет"}]},
				{"type":"paragraph","attrs":{"blockId":"modern"},
				 "content":[{"type":"text","text":"Новый свет"}]}
			]
		}`),
	}
	result, err := findExactDocumentAnchors(node, "Старый", nil)
	if err != nil {
		t.Fatal(err)
	}
	matches := result["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["blockId"] != "__writers_studio_legacy_block_0" {
		t.Fatalf("legacy block did not receive a stable anchor: %#v", result)
	}
	replaced, err := replaceDocumentRanges(node.Content, map[string]any{
		"edits": []any{map[string]any{
			"blockId": matches[0]["blockId"], "start": 0, "end": 6,
			"expectedText": "Старый", "replacement": "Яркий",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := editor.PlainText(replaced)
	if plain != "Яркий свет\n\nНовый свет" {
		t.Fatalf("legacy anchored replacement failed: %q", plain)
	}
	resolved, err := assistant.NewExplicitResolvedTarget(
		node.ID,
		node.CurrentRevision,
		node.Content,
		"замени старый свет",
		[]assistant.TextRange{{
			BlockID: "__writers_studio_legacy_block_0", Start: 0, End: 6,
		}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := assistant.ValidateProposalScope(node.Content, replaced, resolved); err != nil {
		t.Fatalf("legacy proposal scope validation failed: %v", err)
	}
}

func TestFindExactDocumentAnchorsBatchReturnsOrderedVerifiedQueries(t *testing.T) {
	t.Parallel()
	node := domain.DocumentNode{
		ID: "node", CurrentRevision: 3,
		Content: json.RawMessage(`{
			"type":"doc","content":[
				{"type":"paragraph","attrs":{"blockId":"first"},
				 "content":[{"type":"text","text":"Первый абзац"}]},
				{"type":"paragraph","attrs":{"blockId":"second"},
				 "content":[{"type":"text","text":"Второй абзац"}]}
			]
		}`),
	}
	second := "second"
	result, err := findExactDocumentAnchorsBatch(node, []any{
		map[string]any{"text": "абзац", "blockId": nil},
		map[string]any{"text": "Второй", "blockId": second},
	})
	if err != nil {
		t.Fatal(err)
	}
	results := result["results"].([]map[string]any)
	if len(results) != 2 || results[0]["queryIndex"] != 0 ||
		results[0]["matchCount"] != 2 || results[1]["queryIndex"] != 1 ||
		results[1]["matchCount"] != 1 || result["revision"] != 3 {
		t.Fatalf("unexpected exact batch: %#v", result)
	}
	if _, err := findExactDocumentAnchorsBatch(node, []any{}); err == nil {
		t.Fatal("empty exact batch was accepted")
	}
	tooMany := make([]any, 51)
	for index := range tooMany {
		tooMany[index] = map[string]any{"text": "x", "blockId": nil}
	}
	if _, err := findExactDocumentAnchorsBatch(node, tooMany); err == nil {
		t.Fatal("oversized exact batch was accepted")
	}
	if _, err := findExactDocumentAnchorsBatch(node, []any{
		map[string]any{"text": "", "blockId": nil},
	}); err == nil {
		t.Fatal("invalid exact query was accepted")
	}
}

func TestNodeScopedMutationCannotEscapeExactTarget(t *testing.T) {
	t.Parallel()
	target := "target-node"
	execution := projectmcp.ExecutionContext{TargetNodeID: &target}
	other := "other-node"
	if err := validateMutationScope(execution, []assistant.ChangeOperation{{
		TargetKind: "document",
		TargetID:   &other,
	}}); !errors.Is(err, assistant.ErrMutationOutsideScope) {
		t.Fatalf("document scope escape was accepted: %v", err)
	}
	if err := validateMutationScope(execution, []assistant.ChangeOperation{{
		TargetKind: "project_memory",
		TargetID:   &other,
	}}); err != nil {
		t.Fatalf("linked project memory proposal was blocked: %v", err)
	}
	if err := validateMutationScope(execution, []assistant.ChangeOperation{{
		TargetKind:       "version",
		ProposedSnapshot: mustJSON(map[string]any{"nodeId": target}),
	}}); err != nil {
		t.Fatalf("target version proposal was blocked: %v", err)
	}
}

func TestChangeSetSummaryIsDeterministicAndCompact(t *testing.T) {
	t.Parallel()
	summary := summarizeChangeOperations([]assistant.ChangeOperation{
		{GroupKey: "document"},
		{GroupKey: "dictionary"},
		{GroupKey: "memory"},
	}, "very long provider prose")
	for _, expected := range []string{
		"операций: 3",
		"текст и структура · 1",
		"словарь · 1",
		"память проекта · 1",
		"Ничего не применено",
	} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary %q does not contain %q", summary, expected)
		}
	}
	if strings.Contains(summary, "very long") {
		t.Fatalf("provider prose leaked into review summary: %q", summary)
	}
}

func TestDuplicateValueOverlapIsLiteralAndCaseInsensitive(t *testing.T) {
	t.Parallel()
	if !normalizedValuesOverlap(
		[]string{" Элина ", "Эля"},
		[]string{"элина", "Lina"},
	) {
		t.Fatal("literal canonical duplicate was missed")
	}
	if normalizedValuesOverlap(
		[]string{"Элина"},
		[]string{"Элиана"},
	) {
		t.Fatal("nearby but different names were treated as duplicates")
	}
	if normalizedValuesOverlap([]string{"", "  "}, []string{""}) {
		t.Fatal("empty values were treated as duplicates")
	}
}

func TestPreparedNativeFlowRequiresTypedServerDestination(t *testing.T) {
	t.Parallel()
	if !hasPreparedNativeFlow(map[string]any{
		"prepared": true,
		"nativeFlow": map[string]any{
			"kind": "export",
			"destination": map[string]any{
				"type": "project_export", "projectId": "project",
			},
		},
	}) {
		t.Fatal("typed native handoff was not recognized")
	}
	for _, output := range []map[string]any{
		{"prepared": false},
		{"prepared": true, "nativeFlow": "https://example.invalid"},
		{"prepared": true, "nativeFlow": map[string]any{"kind": "export"}},
		{
			"prepared": true,
			"nativeFlow": map[string]any{
				"kind": "", "destination": map[string]any{"type": "project_export"},
			},
		},
	} {
		if hasPreparedNativeFlow(output) {
			t.Fatalf("malformed native handoff was accepted: %#v", output)
		}
	}
}
