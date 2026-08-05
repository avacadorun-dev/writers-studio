//go:build integration

package assistantapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	"writers.studio/internal/llm"
	"writers.studio/internal/projectmcp"
	"writers.studio/internal/store"
)

func TestAssistantBackendAcrossDynamicProjectStructures(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dataStore, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dataStore.Close)
	if err := dataStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, _, err := dataStore.Authenticate(
		ctx,
		"assistant-backend-"+uuid.NewString()+"@example.test",
		"writers-studio",
		time.Hour,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = dataStore.Pool.Exec(cleanupContext, "DELETE FROM users WHERE id=$1", user.ID)
	})

	backend := NewProjectBackend(dataStore)
	for _, projectKind := range []string{"novel", "screenplay", "game_design", "free"} {
		t.Run(projectKind, func(t *testing.T) {
			workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
				Title: "Assistant " + projectKind, Kind: projectKind, Language: "ru",
			})
			if err != nil {
				t.Fatal(err)
			}
			node := saveNodeText(t, ctx, dataStore, user.ID, workspace.Nodes[0], "Начало")
			execution := createBackendExecution(
				t,
				ctx,
				dataStore,
				user.ID,
				workspace.Project.ID,
				&node.ID,
				"Допиши текущий документ",
			)
			structure, err := backend.Call(ctx, execution, "project.get_structure", map[string]any{})
			if err != nil || structure["structure"] == nil {
				t.Fatalf("dynamic structure was not readable: %#v %v", structure, err)
			}
			if _, err := backend.Call(ctx, execution, "document.append", map[string]any{
				"nodeId": node.ID, "baseRevision": node.CurrentRevision,
				"content": assistantBlocks("Продолжение"),
			}); err != nil {
				t.Fatal(err)
			}
			changeSet, err := backend.FinalizeChangeSet(
				ctx,
				execution,
				"Дополнение документа",
				"Подготовлен новый абзац",
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := dataStore.ApplyAssistantChangeSet(
				ctx,
				user.ID,
				execution.RunID,
				changeSet.ID,
				nil,
			); err != nil {
				t.Fatal(err)
			}
			updated, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
			if err != nil || !strings.Contains(updated.PlainText, "Продолжение") {
				t.Fatalf("assistant did not edit %s project: %#v %v", projectKind, updated, err)
			}
		})
	}

	t.Run("native high-risk actions hand off without mutating project state", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Нативные действия", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Nodes[0],
			"Текст остаётся без изменений.",
		)
		version, err := dataStore.CreateNamedVersion(
			ctx,
			user.ID,
			node.ID,
			"До нативного действия",
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		session, err := dataStore.CreateDictationSession(
			ctx,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			uuid.NewString(),
			"ru",
		)
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			"Подготовь опасные действия",
		)
		cases := []struct {
			tool            string
			arguments       map[string]any
			kind            string
			destinationType string
		}{
			{
				tool: "tree.prepare_archive", arguments: map[string]any{"nodeId": node.ID},
				kind: "archive_node", destinationType: "project_node",
			},
			{
				tool: "version.prepare_restore",
				arguments: map[string]any{
					"versionId": version.ID, "expectedCurrentRevision": node.CurrentRevision,
				},
				kind: "restore_version", destinationType: "node_versions",
			},
			{
				tool: "transcript.prepare_retranscribe",
				arguments: map[string]any{
					"sessionId": session.ID,
				},
				kind: "retranscribe", destinationType: "dictation_review",
			},
			{
				tool: "export.prepare",
				arguments: map[string]any{
					"format": "docx", "nodeIds": []any{node.ID}, "chapterTitles": true,
				},
				kind: "export", destinationType: "project_export",
			},
		}
		for _, testCase := range cases {
			result, err := backend.Call(
				ctx,
				execution,
				testCase.tool,
				testCase.arguments,
			)
			if err != nil {
				t.Fatalf("%s handoff failed: %v", testCase.tool, err)
			}
			if result["prepared"] != true {
				t.Fatalf("%s was not prepared: %#v", testCase.tool, result)
			}
			flow, ok := result["nativeFlow"].(map[string]any)
			if !ok || flow["kind"] != testCase.kind {
				t.Fatalf("%s returned an invalid native flow: %#v", testCase.tool, result)
			}
			destination, ok := flow["destination"].(map[string]any)
			if !ok || destination["type"] != testCase.destinationType {
				t.Fatalf("%s returned an invalid destination: %#v", testCase.tool, result)
			}
			switch testCase.tool {
			case "version.prepare_restore":
				if destination["versionId"] != version.ID ||
					destination["expectedCurrentRevision"] != node.CurrentRevision {
					t.Fatalf("restore handoff lost its exact prefill: %#v", destination)
				}
			case "export.prepare":
				nodeIDs, valid := destination["nodeIds"].([]string)
				if destination["format"] != "docx" ||
					destination["chapterTitles"] != true ||
					!valid || len(nodeIDs) != 1 || nodeIDs[0] != node.ID {
					t.Fatalf("export handoff lost its exact prefill: %#v", destination)
				}
			}
		}
		if _, err := backend.Call(
			ctx,
			execution,
			"version.prepare_restore",
			map[string]any{
				"versionId": version.ID, "expectedCurrentRevision": node.CurrentRevision - 1,
			},
		); !errors.Is(err, assistant.ErrStaleChangeSet) {
			t.Fatalf("stale restore handoff was accepted: %v", err)
		}
		changeSet, err := backend.FinalizeChangeSet(
			ctx,
			execution,
			"Нативные действия",
			"",
		)
		if err != nil || changeSet != nil {
			t.Fatalf("native handoff staged a canonical mutation: %#v %v", changeSet, err)
		}
		unchanged, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if err != nil ||
			unchanged.CurrentRevision != node.CurrentRevision ||
			unchanged.PlainText != node.PlainText {
			t.Fatalf("native handoff changed the document: %#v %v", unchanged, err)
		}

		otherWorkspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Чужая область", Kind: "free", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := backend.Call(
			ctx,
			execution,
			"tree.prepare_archive",
			map[string]any{"nodeId": otherWorkspace.Nodes[0].ID},
		); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("cross-project native target was accepted: %v", err)
		}
	})

	t.Run("free nested multi-file review transfer and context", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Свободная база", Kind: "free", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		first := saveNodeText(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Nodes[0],
			"Старое имя входит в комнату.",
		)
		folder, err := dataStore.CreateDocumentNode(
			ctx,
			user.ID,
			workspace.Project.ID,
			nil,
			"folder",
			"Вложенная папка",
		)
		if err != nil {
			t.Fatal(err)
		}
		second, err := dataStore.CreateDocumentNode(
			ctx,
			user.ID,
			workspace.Project.ID,
			&folder.ID,
			"document",
			"Вложенный документ",
		)
		if err != nil {
			t.Fatal(err)
		}
		second = saveNodeText(
			t,
			ctx,
			dataStore,
			user.ID,
			second,
			"Старое имя отвечает.",
		)

		replaceExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Во всех документах замени старое имя на Илару",
		)
		result, err := backend.Call(
			ctx,
			replaceExecution,
			"document.prepare_replace_all",
			map[string]any{
				"query": "Старое имя", "replacement": "Илара", "nodeIds": []any{},
			},
		)
		if err != nil || result["operationCount"] != 2 {
			t.Fatalf("multi-file replacement was not staged: %#v %v", result, err)
		}
		replaceSet, err := backend.FinalizeChangeSet(
			ctx,
			replaceExecution,
			"Замена имени",
			"Два независимых документа",
		)
		if err != nil || len(replaceSet.Operations) != 2 {
			t.Fatalf("bad replacement change set: %#v %v", replaceSet, err)
		}
		selectedGroup := replaceSet.Operations[0].GroupKey
		selectedTarget := *replaceSet.Operations[0].TargetID
		unselectedTarget := *replaceSet.Operations[1].TargetID
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			replaceExecution.RunID,
			replaceSet.ID,
			[]string{selectedGroup},
		); err != nil {
			t.Fatal(err)
		}
		selectedNode, _ := dataStore.GetDocumentNode(ctx, user.ID, selectedTarget)
		unselectedNode, _ := dataStore.GetDocumentNode(ctx, user.ID, unselectedTarget)
		if !strings.Contains(selectedNode.PlainText, "Илара") ||
			!strings.Contains(unselectedNode.PlainText, "Старое имя") {
			t.Fatalf(
				"partial apply crossed group boundaries: selected=%q unselected=%q",
				selectedNode.PlainText,
				unselectedNode.PlainText,
			)
		}
		if _, err := dataStore.UndoAssistantChangeSet(
			ctx,
			user.ID,
			replaceExecution.RunID,
			replaceSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		selectedNode, _ = dataStore.GetDocumentNode(ctx, user.ID, selectedTarget)
		if !strings.Contains(selectedNode.PlainText, "Старое имя") {
			t.Fatalf("multi-file undo did not restore the selected document: %q", selectedNode.PlainText)
		}

		first, _ = dataStore.GetDocumentNode(ctx, user.ID, first.ID)
		second, _ = dataStore.GetDocumentNode(ctx, user.ID, second.ID)
		transferExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&first.ID,
			"Перенеси вложенный документ в текущий",
		)
		if _, err := backend.Call(
			ctx,
			transferExecution,
			"document.prepare_transfer",
			map[string]any{
				"sourceNodeId": second.ID, "destinationNodeId": first.ID,
				"baseRevision": first.CurrentRevision,
			},
		); err != nil {
			t.Fatal(err)
		}
		transferSet, err := backend.FinalizeChangeSet(
			ctx,
			transferExecution,
			"Перенос материала",
			"Источник связан с предложением",
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			transferExecution.RunID,
			transferSet.ID,
			nil,
		); err != nil {
			t.Fatal(err)
		}
		first, _ = dataStore.GetDocumentNode(ctx, user.ID, first.ID)
		if !strings.Contains(first.PlainText, "отвечает") {
			t.Fatalf("generic free-form transfer failed: %q", first.PlainText)
		}

		session, err := dataStore.CreateDictationSession(
			ctx,
			user.ID,
			workspace.Project.ID,
			&first.ID,
			uuid.NewString(),
			"ru",
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
			SessionID: session.ID, Content: testDocument("Фрагмент из стенограммы"),
			PlainText: "Фрагмент из стенограммы", ManuallyEdited: true,
		}); err != nil {
			t.Fatal(err)
		}
		first, _ = dataStore.GetDocumentNode(ctx, user.ID, first.ID)
		transcriptExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&first.ID,
			"Перенеси стенограмму в текущий документ",
		)
		if _, err := backend.Call(
			ctx,
			transcriptExecution,
			"transcript.prepare_transfer",
			map[string]any{
				"sessionId": session.ID, "nodeId": first.ID,
				"baseRevision": first.CurrentRevision,
			},
		); err != nil {
			t.Fatal(err)
		}
		transcriptSet, err := backend.FinalizeChangeSet(
			ctx,
			transcriptExecution,
			"Перенос стенограммы",
			"Стенограмма добавлена с provenance",
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			transcriptExecution.RunID,
			transcriptSet.ID,
			nil,
		); err != nil {
			t.Fatal(err)
		}
		first, _ = dataStore.GetDocumentNode(ctx, user.ID, first.ID)
		if !strings.Contains(first.PlainText, "Фрагмент из стенограммы") {
			t.Fatalf("transcript transfer failed: %q", first.PlainText)
		}

		contextExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Добавь имя в словарь, запомни факт и создай руководство",
		)
		mutations := []struct {
			name string
			args map[string]any
		}{
			{
				"dictionary.add",
				map[string]any{
					"canonical": "Илара", "variants": []any{"Иллара"},
					"kind": "character", "language": "ru",
					"pronunciation": "И-ла-ра", "active": true,
				},
			},
			{
				"project_memory.add",
				map[string]any{
					"kind": "character", "statement": "Илара боится глубокой воды.",
					"scope": "project", "scopeNodeId": nil,
					"sourceReferences": []any{}, "tags": []any{"Илара"},
					"active": true, "alwaysRelevant": true,
				},
			},
			{
				"assistant_guide.propose_create",
				map[string]any{"content": "# Стиль\n\nКороткие фразы, настоящее время."},
			},
		}
		for _, mutation := range mutations {
			if _, err := backend.Call(ctx, contextExecution, mutation.name, mutation.args); err != nil {
				t.Fatalf("%s: %v", mutation.name, err)
			}
		}
		contextSet, err := backend.FinalizeChangeSet(
			ctx,
			contextExecution,
			"Контекст проекта",
			"Три независимые группы",
		)
		if err != nil || len(contextSet.Operations) != 3 {
			t.Fatalf("context change set: %#v %v", contextSet, err)
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			contextExecution.RunID,
			contextSet.ID,
			[]string{"dictionary", "memory"},
		); err != nil {
			t.Fatal(err)
		}
		terms, err := dataStore.ListDictionaryTerms(
			ctx,
			user.ID,
			workspace.Project.ID,
		)
		if err != nil || len(terms) != 1 || terms[0].Canonical != "Илара" {
			t.Fatalf("dictionary group not applied: %#v %v", terms, err)
		}
		memory, err := dataStore.ListProjectMemory(
			ctx,
			user.ID,
			workspace.Project.ID,
			false,
		)
		if err != nil || len(memory) != 1 || !memory[0].AlwaysRelevant {
			t.Fatalf("memory group not applied: %#v %v", memory, err)
		}
		dictionaryConflict, err := backend.Call(
			ctx,
			contextExecution,
			"dictionary.add",
			map[string]any{
				"canonical": "Другое имя", "variants": []any{" илара "},
				"kind": "character", "language": "ru",
				"pronunciation": "", "active": true,
			},
		)
		if err != nil || dictionaryConflict["conflict"] != true ||
			dictionaryConflict["staged"] != false {
			t.Fatalf("dictionary duplicate was staged silently: %#v %v", dictionaryConflict, err)
		}
		memoryConflict, err := backend.Call(
			ctx,
			contextExecution,
			"project_memory.add",
			map[string]any{
				"kind": "character", "statement": "  илара БОИТСЯ глубокой воды. ",
				"scope": "project", "scopeNodeId": nil,
				"sourceReferences": []any{}, "tags": []any{"Илара"},
				"active": true, "alwaysRelevant": true,
			},
		)
		if err != nil || memoryConflict["conflict"] != true ||
			memoryConflict["staged"] != false {
			t.Fatalf("memory duplicate was staged silently: %#v %v", memoryConflict, err)
		}
		if duplicateSet, err := backend.FinalizeChangeSet(
			ctx,
			contextExecution,
			"Дубликаты",
			"Не должно создаться",
		); err != nil || duplicateSet != nil {
			t.Fatalf("duplicate conflicts created operations: %#v %v", duplicateSet, err)
		}
		if _, err := dataStore.GetAssistantGuide(
			ctx,
			user.ID,
			workspace.Project.ID,
			false,
		); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("excluded guide group changed canonical state: %v", err)
		}

		readExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&first.ID,
			"Проверь проект",
		)
		readCalls := []struct {
			name string
			args map[string]any
		}{
			{"assistant.get_recent_change_set", map[string]any{}},
			{"project.get_overview", map[string]any{}},
			{"project.list_recent", map[string]any{"limit": 5}},
			{"project.search", map[string]any{"query": "Илара"}},
			{"tree.list", map[string]any{}},
			{"tree.get_node", map[string]any{"nodeId": first.ID}},
			{"document.get", map[string]any{"nodeId": first.ID}},
			{"document.find_exact", map[string]any{
				"nodeId": first.ID, "revision": first.CurrentRevision,
				"text": "Фрагмент", "blockId": nil,
			}},
			{"document.find_exact_batch", map[string]any{
				"nodeId": first.ID, "revision": first.CurrentRevision,
				"queries": []any{
					map[string]any{"text": "Фрагмент", "blockId": nil},
					map[string]any{"text": "Илара", "blockId": nil},
				},
			}},
			{"document.get_range", map[string]any{
				"nodeId": first.ID, "revision": first.CurrentRevision, "start": 0, "end": 5,
			}},
			{"document.list_references", map[string]any{"nodeId": first.ID}},
			{"dictionary.list", map[string]any{}},
			{"dictionary.find", map[string]any{"query": "Илар"}},
			{"mentions.find", map[string]any{"query": "Илара"}},
			{"project_memory.list", map[string]any{"includeInactive": false}},
			{"project_memory.get", map[string]any{"memoryId": memory[0].ID}},
			{"project_memory.search", map[string]any{"query": "воды"}},
			{"dictation.list_sessions", map[string]any{"limit": 10}},
			{"transcript.get", map[string]any{"sessionId": session.ID}},
			{"transcript.get_uncertain", map[string]any{
				"sessionId": session.ID, "maximumConfidence": 0.8,
			}},
			{"export.preview", map[string]any{
				"format": "txt", "nodeIds": []any{first.ID}, "chapterTitles": true,
			}},
			{"recovery.get_status", map[string]any{}},
			{"settings.get_writer_preferences", map[string]any{}},
			{"assistant.get_capabilities", map[string]any{}},
		}
		for _, readCall := range readCalls {
			result, err := backend.Call(ctx, readExecution, readCall.name, readCall.args)
			if err != nil {
				t.Fatalf("read tool %s failed: %v", readCall.name, err)
			}
			if readCall.name == "settings.get_writer_preferences" {
				preferences, ok := result["preferences"].(map[string]any)
				if !ok || preferences["language"] != "ru" ||
					preferences["commandPrefix"] != "Команда" ||
					preferences["assistantSilenceCheckpointMs"] != float64(5000) ||
					result["capturedAt"] != "assistant_run_start" {
					t.Fatalf("writer preferences snapshot is incomplete: %#v", result)
				}
				if _, exposed := preferences["microphoneId"]; exposed {
					t.Fatalf("hardware identifier was exposed to the model: %#v", preferences)
				}
				if _, exposed := preferences["audioRetentionDays"]; exposed {
					t.Fatalf("privacy preference was exposed to the model: %#v", preferences)
				}
			}
		}
	})

	t.Run("orchestrator tool loop stages linked changes", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Оркестратор", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Nodes[0],
			"Марина вошла.",
		)
		instruction := "Допиши сцену, занеси имя Илара в словарь и запомни, что она боится воды"
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			instruction,
		)
		appendArguments := mustJSON(map[string]any{
			"nodeId": node.ID, "baseRevision": node.CurrentRevision,
			"content": assistantBlocks("Илара закрыла дверь."),
		})
		dictionaryArguments := mustJSON(map[string]any{
			"canonical": "Илара", "variants": []string{},
			"kind": "character", "language": "ru",
			"pronunciation": "И-ла-ра", "active": true,
		})
		memoryArguments := mustJSON(map[string]any{
			"kind": "character", "statement": "Илара боится воды.",
			"scope": "project", "scopeNodeId": nil,
			"sourceReferences": []any{}, "tags": []string{"Илара"},
			"active": true, "alwaysRelevant": true,
		})
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "response-declaration",
				ToolCalls: []llm.ToolCall{{
					ID: "call-begin", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Связанные изменения проекта",
					}),
				}},
			},
			{
				ResponseID: "response-tools",
				ToolCalls: []llm.ToolCall{
					{ID: "call-document", Name: "document.append", Arguments: appendArguments},
					{ID: "call-dictionary", Name: "dictionary.add", Arguments: dictionaryArguments},
					{ID: "call-memory", Name: "project_memory.add", Arguments: memoryArguments},
				},
			},
			{
				ResponseID: "response-final",
				Text:       "Подготовил продолжение, словарную карточку и память проекта.",
			},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 3 ||
			result.ToolCallCount != 4 {
			t.Fatalf("unexpected orchestrator result: %#v", result)
		}
		if len(provider.requests) != 3 ||
			provider.requests[0].ToolChoice != "assistant.begin_turn" ||
			provider.requests[1].ToolChoice != "" ||
			!strings.Contains(provider.requests[1].Instructions, `"template":"manuscript"`) {
			t.Fatalf("dynamic schema was not pinned into the prompt: %#v", provider.requests)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || run.Status != assistant.RunAwaitingReview ||
			run.PendingChangeSet == nil {
			t.Fatalf("run did not enter review state: %#v %v", run, err)
		}
		events, err := dataStore.ListAssistantEvents(
			ctx,
			user.ID,
			execution.RunID,
			0,
			100,
		)
		if err != nil || len(events) < 10 {
			t.Fatalf("durable activity trace is incomplete: %d %v", len(events), err)
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			execution.RunID,
			result.ChangeSet.ID,
			nil,
		); err != nil {
			t.Fatal(err)
		}
		afterApply, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || afterApply.Status != assistant.RunActive ||
			afterApply.PendingChangeSet != nil || afterApply.LatestTurn == nil ||
			afterApply.LatestTurn.State != assistant.TurnCompleted {
			t.Fatalf("applied review did not return to an active run: %#v, err=%v", afterApply, err)
		}
		node, _ = dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if !strings.Contains(node.PlainText, "закрыла дверь") {
			t.Fatalf("orchestrated document proposal was not applied: %q", node.PlainText)
		}
		if _, err := dataStore.UndoAssistantChangeSet(
			ctx,
			user.ID,
			execution.RunID,
			result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		node, _ = dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if strings.Contains(node.PlainText, "закрыла дверь") {
			t.Fatalf("orchestrated grouped undo failed: %q", node.PlainText)
		}
	})

	t.Run("structured mutations recover from an incorrect answer declaration", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Восстановление режима памяти", Kind: "story", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Зафиксируй три факта в памяти проекта",
		)
		memoryCall := func(id, statement string) llm.ToolCall {
			return llm.ToolCall{
				ID: id, Name: "project_memory.add",
				Arguments: mustJSON(map[string]any{
					"kind": "plot", "statement": statement, "scope": "project",
					"scopeNodeId": nil, "sourceReferences": []any{},
					"tags": []string{"сюжет"}, "active": true, "alwaysRelevant": true,
				}),
			}
		}
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "incorrect-answer-declaration",
				ToolCalls: []llm.ToolCall{{
					ID: "begin-answer-then-mutate", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "answer", "summary": "Зафиксировать факты",
					}),
				}},
			},
			{
				ResponseID: "parallel-memory-mutations",
				ToolCalls: []llm.ToolCall{
					memoryCall("memory-one", "Поезд отправляется в полночь."),
					memoryCall("memory-two", "Адресат ждёт на следующей станции."),
					memoryCall("memory-three", "Герой остаётся на платформе."),
				},
			},
			{ResponseID: "memory-final", Text: "Факты подготовлены."},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 3 {
			t.Fatalf("memory mutations were not staged together: %#v", result)
		}
		events, err := dataStore.ListAssistantEvents(
			ctx,
			user.ID,
			execution.RunID,
			0,
			100,
		)
		if err != nil {
			t.Fatal(err)
		}
		escalations := 0
		for _, event := range events {
			if event.Type == assistant.EventTurnModeEscalated {
				escalations++
			}
			if event.Type == assistant.EventToolCallEnd &&
				strings.Contains(string(event.Data), `"isError":true`) {
				t.Fatalf("a staged memory call remained failed: %s", event.Data)
			}
		}
		if escalations != 1 {
			t.Fatalf("expected one durable mode escalation, got %d", escalations)
		}
	})

	t.Run("long read sequence can still stage the requested mutation", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Длинная последовательность чтения", Kind: "story", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Прочитай проект и запомни, что финальная сцена происходит ночью",
		)
		completions := []llm.Completion{{
			ResponseID: "long-begin",
			ToolCalls: []llm.ToolCall{{
				ID: "long-begin-call", Name: "assistant.begin_turn",
				Arguments: mustJSON(map[string]any{
					"mode": "change", "summary": "Прочитать проект и обновить память",
				}),
			}},
		}}
		for index := range 25 {
			completions = append(completions, llm.Completion{
				ResponseID: "long-read-response-" + fmt.Sprint(index),
				ToolCalls: []llm.ToolCall{{
					ID:        "long-read-call-" + fmt.Sprint(index),
					Name:      "project.get_overview",
					Arguments: mustJSON(map[string]any{}),
				}},
			})
		}
		completions = append(completions,
			llm.Completion{
				ResponseID: "long-mutation",
				ToolCalls: []llm.ToolCall{{
					ID: "long-memory-call", Name: "project_memory.add",
					Arguments: mustJSON(map[string]any{
						"kind": "plot", "statement": "Финальная сцена происходит ночью.",
						"scope": "project", "scopeNodeId": nil,
						"sourceReferences": []any{}, "tags": []string{"финал"},
						"active": true, "alwaysRelevant": true,
					}),
				}},
			},
			llm.Completion{ResponseID: "long-final", Text: "Подготовил факт для памяти проекта."},
		)
		provider := &scriptedProvider{completions: completions}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 1 ||
			result.ToolCallCount != 27 || len(provider.requests) != 28 {
			t.Fatalf("long tool loop did not reach review: result=%#v requests=%d", result, len(provider.requests))
		}
		if _, err := dataStore.RejectAssistantChangeSet(
			ctx,
			user.ID,
			execution.RunID,
			result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		afterReject, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || afterReject.Status != assistant.RunActive ||
			afterReject.PendingChangeSet != nil || afterReject.LatestTurn == nil ||
			afterReject.LatestTurn.State != assistant.TurnCompleted {
			t.Fatalf("rejected review did not return to an active run: %#v, err=%v", afterReject, err)
		}
	})

	t.Run("project step limit stops an over-budget tool loop", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Ограниченная последовательность", Kind: "story", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		limit := 2
		if _, err := dataStore.UpdateProjectAssistantSettings(
			ctx,
			user.ID,
			workspace.Project.ID,
			store.ProjectAssistantSettings{StepLimit: &limit},
		); err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Прочитай проект",
		)
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "limited-begin",
				ToolCalls: []llm.ToolCall{{
					ID: "limited-begin-call", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "answer", "summary": "Прочитать проект",
					}),
				}},
			},
			{
				ResponseID: "limited-read-1",
				ToolCalls: []llm.ToolCall{{
					ID: "limited-read-call-1", Name: "project.get_overview",
					Arguments: mustJSON(map[string]any{}),
				}},
			},
			{
				ResponseID: "limited-read-2",
				ToolCalls: []llm.ToolCall{{
					ID: "limited-read-call-2", Name: "project.get_overview",
					Arguments: mustJSON(map[string]any{}),
				}},
			},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		if _, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		); err == nil || !strings.Contains(err.Error(), "tool_limit_exceeded") {
			t.Fatalf("ExecuteTurn() error = %v, want tool_limit_exceeded", err)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if run.LatestTurn == nil || run.LatestTurn.ErrorCode == nil ||
			*run.LatestTurn.ErrorCode != "tool_limit_exceeded" {
			t.Fatalf("limited run did not persist the error code: %#v", run.LatestTurn)
		}
	})

	t.Run("content nodes are created in order with one compact batch call", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Пакетная структура", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		anchor := workspace.Nodes[0]
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Создай шесть соседних разделов сразу с коротким планом каждого",
		)
		items := make([]map[string]any, 0, 6)
		for index := range 6 {
			items = append(items, map[string]any{
				"kind": anchor.Kind, "title": fmt.Sprintf("Раздел %d", index+2),
				"content": assistantBlocks(fmt.Sprintf("Короткий план раздела %d.", index+2)),
			})
		}
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "node-batch-begin",
				ToolCalls: []llm.ToolCall{{
					ID: "node-batch-begin-call", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Создать разделы с исходным содержимым",
					}),
				}},
			},
			{
				ResponseID: "node-batch-stage",
				ToolCalls: []llm.ToolCall{{
					ID: "node-batch-call", Name: "tree.create_nodes_batch",
					Arguments: mustJSON(map[string]any{
						"parentId": anchor.ParentID, "positionAfterId": anchor.ID,
						"items": items,
					}),
				}},
			},
			{ResponseID: "node-batch-final", Text: "Подготовил разделы для проверки."},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx, user.ID, execution.RunID, execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 6 ||
			result.ToolCallCount != 2 || len(provider.requests) != 3 {
			t.Fatalf("content node batch did not stay compact: %#v", result)
		}
		for index, operation := range result.ChangeSet.Operations {
			if operation.ToolName != "tree.create_node" || operation.Position != index {
				t.Fatalf("node batch item %d is not a normal review operation: %#v", index, operation)
			}
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx, user.ID, execution.RunID, result.ChangeSet.ID, nil,
		); err != nil {
			t.Fatal(err)
		}
		nodes, err := dataStore.ListDocumentNodes(
			ctx, user.ID, workspace.Project.ID, false,
		)
		if err != nil || len(nodes) != 7 {
			t.Fatalf("node batch apply lost sections: nodes=%d error=%v", len(nodes), err)
		}
		sort.Slice(nodes, func(left, right int) bool { return nodes[left].Position < nodes[right].Position })
		for index, node := range nodes[1:] {
			if node.Title != fmt.Sprintf("Раздел %d", index+2) ||
				!strings.Contains(node.PlainText, "Короткий план") {
				t.Fatalf("node batch order/content drifted at %d: %#v", index, node)
			}
		}
		if _, err := dataStore.UndoAssistantChangeSet(
			ctx, user.ID, execution.RunID, result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		nodes, _ = dataStore.ListDocumentNodes(ctx, user.ID, workspace.Project.ID, false)
		if len(nodes) != 1 {
			t.Fatalf("node batch undo left created sections: %d", len(nodes))
		}
	})

	t.Run("long-form chunks accumulate into one reviewable document revision", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Chunked long form", Kind: "story", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t, ctx, dataStore, user.ID, workspace.Nodes[0], "Исходное начало.",
		)
		initialRevision := node.CurrentRevision
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			"Напиши большой черновик примерно на сто слов",
		)
		firstText := "НАЧАЛО_КОТОРОЕ_НЕЛЬЗЯ_ПЕРЕСЫЛАТЬ " + strings.TrimSpace(
			strings.Repeat("протяжённый-первый-фрагмент ", 49),
		)
		secondText := strings.TrimSpace(strings.Repeat("второй-фрагмент ", 50))
		firstArguments := map[string]any{
			"nodeId": node.ID, "baseRevision": node.CurrentRevision,
			"sequence": 1, "targetWords": 100, "finalChunk": false,
			"text": firstText,
		}
		secondArguments := map[string]any{
			"nodeId": node.ID, "baseRevision": node.CurrentRevision,
			"sequence": 2, "targetWords": 100, "finalChunk": true,
			"text": secondText,
		}
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "chunk-begin",
				ToolCalls: []llm.ToolCall{{
					ID: "chunk-begin-call", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Написать большой черновик",
					}),
				}},
			},
			chunkCompletion("chunk-1", firstArguments),
			chunkCompletion("chunk-2", secondArguments),
			{ResponseID: "chunk-final", Text: "Черновик подготовлен по частям."},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx, user.ID, execution.RunID, execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 1 ||
			result.ChangeSet.Operations[0].ToolName != "document.append_chunk" ||
			result.ToolCallCount != 3 || len(provider.requests) != 4 {
			t.Fatalf("long-form chunks did not coalesce: %#v", result)
		}
		thirdInput := string(mustJSON(provider.requests[2].Input))
		if strings.Contains(thirdInput, "НАЧАЛО_КОТОРОЕ_НЕЛЬЗЯ_ПЕРЕСЫЛАТЬ") ||
			!strings.Contains(thirdInput, "staged long-form chunk omitted") {
			t.Fatalf("long-form replay was not compacted: %s", thirdInput)
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx, user.ID, execution.RunID, result.ChangeSet.ID, nil,
		); err != nil {
			t.Fatal(err)
		}
		updated, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if err != nil || updated.CurrentRevision != initialRevision+1 ||
			!strings.Contains(updated.PlainText, "НАЧАЛО_КОТОРОЕ") ||
			!strings.Contains(updated.PlainText, "второй-фрагмент") {
			t.Fatalf("chunked Apply was not one complete revision: %#v error=%v", updated, err)
		}
		if _, err := dataStore.UndoAssistantChangeSet(
			ctx, user.ID, execution.RunID, result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		restored, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if err != nil || restored.PlainText != "Исходное начало." {
			t.Fatalf("chunked Undo did not restore the document: %#v error=%v", restored, err)
		}
	})

	t.Run("provider timeout preserves an already staged partial long-form draft", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Partial timeout", Kind: "story", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := workspace.Nodes[0]
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			"Добавь короткий фрагмент и подготовь его для проверки",
		)
		provider := &scriptedProvider{
			completions: []llm.Completion{
				{
					ResponseID: "partial-begin",
					ToolCalls: []llm.ToolCall{{
						ID: "partial-begin-call", Name: "assistant.begin_turn",
						Arguments: mustJSON(map[string]any{
							"mode": "change", "summary": "Дополнить текущий документ",
						}),
					}},
				},
				chunkCompletion("partial-stage", map[string]any{
					"nodeId": node.ID, "baseRevision": node.CurrentRevision,
					"sequence": 1, "targetWords": 100, "finalChunk": false,
					"text": strings.TrimSpace(strings.Repeat("частичный-фрагмент ", 25)),
				}),
			},
			err: context.DeadlineExceeded,
		}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, executeErr := orchestrator.ExecuteTurn(
			ctx, user.ID, execution.RunID, execution.TurnID,
		)
		if executeErr == nil || !strings.Contains(executeErr.Error(), "provider_timeout") ||
			result.ChangeSet == nil || len(result.ChangeSet.Operations) != 1 {
			t.Fatalf("partial timeout was discarded: result=%#v error=%v", result, executeErr)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || run.Status != assistant.RunAwaitingReview || run.LatestTurn == nil ||
			run.LatestTurn.State != assistant.TurnAwaitingReview ||
			run.LatestTurn.ErrorCode == nil || *run.LatestTurn.ErrorCode != "provider_timeout" ||
			run.PendingChangeSet == nil {
			t.Fatalf("partial timeout did not remain reviewable: run=%#v error=%v", run, err)
		}
		if result.ChangeSet.Operations[0].ToolName != "document.append_chunk" {
			t.Fatalf("partial long-form proposal used the wrong operation: %#v", result.ChangeSet.Operations[0])
		}
		unchanged, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if err != nil || unchanged.CurrentRevision != node.CurrentRevision ||
			strings.Contains(unchanged.PlainText, "частичный-фрагмент") {
			t.Fatalf("partial timeout changed canonical node: node=%#v error=%v", unchanged, err)
		}
		if _, err := dataStore.RejectAssistantChangeSet(
			ctx, user.ID, execution.RunID, result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("high-cardinality context update fits in two batch tool calls", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Пакетное обновление контекста", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Добавь большой набор терминов и фактов в контекст проекта",
		)
		dictionaryItems := make([]map[string]any, 0, 42)
		for index := range 40 {
			dictionaryItems = append(dictionaryItems, map[string]any{
				"canonical": fmt.Sprintf("Термин %02d", index),
				"variants":  []string{}, "kind": "term", "language": "ru",
				"pronunciation": "", "active": true,
			})
		}
		dictionaryItems = append(dictionaryItems,
			map[string]any{
				"canonical": "Термин 00", "variants": []string{},
				"kind": "term", "language": "ru", "pronunciation": "", "active": true,
			},
			map[string]any{
				"canonical": "Другое написание", "variants": []string{" термин 01 "},
				"kind": "term", "language": "ru", "pronunciation": "", "active": true,
			},
		)
		memoryItems := make([]map[string]any, 0, 42)
		for index := range 40 {
			memoryItems = append(memoryItems, map[string]any{
				"kind": "fact", "statement": fmt.Sprintf("Проверяемый факт номер %02d.", index),
				"scope": "project", "scopeNodeId": nil,
				"sourceReferences": []any{}, "tags": []string{"пакет"},
				"active": true, "alwaysRelevant": false,
			})
		}
		memoryItems = append(memoryItems,
			map[string]any{
				"kind": "fact", "statement": "Проверяемый факт номер 00.",
				"scope": "project", "scopeNodeId": nil, "sourceReferences": []any{},
				"tags": []string{"дубликат"}, "active": true, "alwaysRelevant": false,
			},
			map[string]any{
				"kind": "fact", "statement": "  ПРОВЕРЯЕМЫЙ ФАКТ НОМЕР 01.  ",
				"scope": "project", "scopeNodeId": nil, "sourceReferences": []any{},
				"tags": []string{"дубликат"}, "active": true, "alwaysRelevant": false,
			},
		)
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "batch-begin",
				ToolCalls: []llm.ToolCall{{
					ID: "batch-begin-call", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Пакетно обновить контекст проекта",
					}),
				}},
			},
			{
				ResponseID: "batch-stage",
				ToolCalls: []llm.ToolCall{
					{
						ID: "batch-dictionary", Name: "dictionary.add_batch",
						Arguments: mustJSON(map[string]any{"items": dictionaryItems}),
					},
					{
						ID: "batch-memory", Name: "project_memory.add_batch",
						Arguments: mustJSON(map[string]any{"items": memoryItems}),
					},
				},
			},
			{ResponseID: "batch-final", Text: "Подготовил пакетное обновление контекста."},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 80 ||
			result.ToolCallCount != 3 || len(provider.requests) != 3 {
			t.Fatalf(
				"batch update did not fit in the safe loop: result=%#v requests=%d",
				result,
				len(provider.requests),
			)
		}
		for index, operation := range result.ChangeSet.Operations {
			if operation.Position != index ||
				(operation.ToolName != "dictionary.add" && operation.ToolName != "project_memory.add") {
				t.Fatalf("batch operation %d is not independently reviewable: %#v", index, operation)
			}
		}
		if _, err := dataStore.ApplyAssistantChangeSet(
			ctx,
			user.ID,
			execution.RunID,
			result.ChangeSet.ID,
			nil,
		); err != nil {
			t.Fatal(err)
		}
		terms, termsErr := dataStore.ListDictionaryTerms(
			ctx, user.ID, workspace.Project.ID,
		)
		memory, memoryErr := dataStore.ListProjectMemory(
			ctx, user.ID, workspace.Project.ID, false,
		)
		if termsErr != nil || memoryErr != nil || len(terms) != 40 || len(memory) != 40 {
			t.Fatalf(
				"batch apply lost context records: terms=%d memory=%d errors=%v/%v",
				len(terms), len(memory), termsErr, memoryErr,
			)
		}
		if _, err := dataStore.UndoAssistantChangeSet(
			ctx,
			user.ID,
			execution.RunID,
			result.ChangeSet.ID,
		); err != nil {
			t.Fatal(err)
		}
		terms, _ = dataStore.ListDictionaryTerms(ctx, user.ID, workspace.Project.ID)
		memory, _ = dataStore.ListProjectMemory(ctx, user.ID, workspace.Project.ID, false)
		activeTerms := 0
		for _, term := range terms {
			if term.Active {
				activeTerms++
			}
		}
		if activeTerms != 0 || len(memory) != 0 {
			t.Fatalf(
				"batch undo left active canonical records: active terms=%d memory=%d",
				activeTerms,
				len(memory),
			)
		}
	})

	t.Run("orchestrator completes a native handoff without a change set", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Оркестратор нативного действия", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Подготовь экспорт проекта в DOCX",
		)
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "native-declaration",
				ToolCalls: []llm.ToolCall{{
					ID: "native-begin", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Подготовить экспорт",
					}),
				}},
			},
			{
				ResponseID: "native-tool",
				ToolCalls: []llm.ToolCall{{
					ID: "native-export", Name: "export.prepare",
					Arguments: mustJSON(map[string]any{
						"format": "docx", "nodeIds": []string{},
						"chapterTitles": true,
					}),
				}},
			},
			{ResponseID: "native-final"},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet != nil ||
			result.AssistantText !=
				"Действие подготовлено. Откройте штатный экран ниже и подтвердите его там." {
			t.Fatalf("native handoff produced an invalid result: %#v", result)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || run.Status != assistant.RunActive || run.PendingChangeSet != nil {
			t.Fatalf("native handoff left the run in an invalid state: %#v %v", run, err)
		}
		events, err := dataStore.ListAssistantEvents(
			ctx,
			user.ID,
			execution.RunID,
			0,
			100,
		)
		if err != nil {
			t.Fatal(err)
		}
		nativeResultFound := false
		for _, event := range events {
			if event.Type != assistant.EventToolCallResult {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data["toolCallId"] == "native-export" &&
				strings.Contains(string(event.Data), `"project_export"`) {
				nativeResultFound = true
			}
		}
		if !nativeResultFound {
			t.Fatal("durable native handoff event was not recorded")
		}
	})

	t.Run("orchestrator stages complete combined review", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Контракт review", Kind: "game_design", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := workspace.Nodes[0]
		instruction := "Напиши сцену, добавь имя Лиора в словарь и запомни факт в память проекта. Сначала предложи для проверки."
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			&node.ID,
			instruction,
		)
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "declare-change",
				ToolCalls: []llm.ToolCall{{
					ID: "begin-change", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "change", "summary": "Подготовить три правки",
					}),
				}},
			},
			{
				ResponseID: "read",
				Text:       "Сначала прочитаю документ и перечислю технические ID.",
				ToolCalls: []llm.ToolCall{{
					ID: "read-document", Name: "document.get",
					Arguments: mustJSON(map[string]any{"nodeId": node.ID}),
				}},
			},
			{
				ResponseID: "stage",
				Text:       "Вызываю три мутации и покажу их аргументы.",
				ToolCalls: []llm.ToolCall{
					{
						ID: "stage-document", Name: "document.append",
						Arguments: mustJSON(map[string]any{
							"nodeId": node.ID, "baseRevision": node.CurrentRevision,
							"content": assistantBlocks("Лиора нашла ключ."),
						}),
					},
					{
						ID: "stage-dictionary", Name: "dictionary.add",
						Arguments: mustJSON(map[string]any{
							"canonical": "Лиора", "variants": []string{},
							"kind": "character", "language": "ru",
							"pronunciation": "Ли-о-ра", "active": true,
						}),
					},
					{
						ID: "stage-memory", Name: "project_memory.add",
						Arguments: mustJSON(map[string]any{
							"kind": "character", "statement": "Лиора боится глубокой воды.",
							"scope": "project", "scopeNodeId": nil,
							"sourceReferences": []any{}, "tags": []string{"Лиора"},
							"active": true, "alwaysRelevant": true,
						}),
					},
				},
			},
			{
				ResponseID: "final",
				Text: "<analysis>do not expose</analysis>## Change set\n\n" +
					"| internal id | value |\n|---|---|\n| node | secret-id |",
			},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.ChangeSet == nil || len(result.ChangeSet.Operations) != 3 {
			t.Fatalf("combined proposal was not staged: %#v", result)
		}
		if result.AssistantText !=
			"Подготовил изменение для проверки. Ничего не применено — откройте точный diff, "+
				"чтобы принять всё или только нужную часть." {
			t.Fatalf("provider narration or technical detail leaked: %q", result.AssistantText)
		}
		if len(provider.requests) != 4 ||
			len(provider.requests[0].Tools) != 1 ||
			len(provider.requests[1].Tools) != len(projectmcp.PublicCatalog())-1 {
			t.Fatalf("complete semantic tool surface was not exposed: %#v", provider.requests)
		}
	})

	t.Run("orchestrator text-only and provider failure states", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Чат без правок", Kind: "screenplay", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Объясни структуру проекта, ничего не меняй",
		)
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: &scriptedProvider{
				completions: []llm.Completion{
					{
						ResponseID: "declare-answer",
						ToolCalls: []llm.ToolCall{{
							ID: "begin-answer", Name: "assistant.begin_turn",
							Arguments: mustJSON(map[string]any{
								"mode": "answer", "summary": "Объяснить структуру",
							}),
						}},
					},
					{ResponseID: "text-only", Text: "У проекта сценарная структура."},
				},
			}},
		)
		result, err := orchestrator.ExecuteTurn(
			ctx,
			user.ID,
			execution.RunID,
			execution.TurnID,
		)
		if err != nil || result.ChangeSet != nil ||
			result.AssistantText != "У проекта сценарная структура." {
			t.Fatalf("text-only turn failed: %#v %v", result, err)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || run.Status != assistant.RunActive ||
			run.LatestTurn == nil || run.LatestTurn.State != assistant.TurnCompleted {
			t.Fatalf("text-only run has wrong state: %#v %v", run, err)
		}

		clarifyExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Продолжи это",
		)
		clarifyProvider := &scriptedProvider{completions: []llm.Completion{{
			ResponseID: "declare-clarify",
			ToolCalls: []llm.ToolCall{{
				ID: "begin-clarify", Name: "assistant.begin_turn",
				Arguments: mustJSON(map[string]any{
					"mode":    "clarify",
					"summary": "Какой именно материал нужно продолжить?",
				}),
			}},
		}}}
		clarifyOrchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: clarifyProvider},
		)
		clarified, err := clarifyOrchestrator.ExecuteTurn(
			ctx,
			user.ID,
			clarifyExecution.RunID,
			clarifyExecution.TurnID,
		)
		if err != nil || clarified.ChangeSet != nil ||
			clarified.AssistantText != "Какой именно материал нужно продолжить?" ||
			len(clarifyProvider.requests) != 1 {
			t.Fatalf("clarification was not isolated from stale chat: %#v %v", clarified, err)
		}

		failedExecution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Проверь проект",
		)
		failed := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{err: errors.New("provider offline")},
		)
		if _, err := failed.ExecuteTurn(
			ctx,
			user.ID,
			failedExecution.RunID,
			failedExecution.TurnID,
		); err == nil || !strings.Contains(err.Error(), "provider_unavailable") {
			t.Fatalf("provider failure was not mapped: %v", err)
		}
		failedRun, err := dataStore.GetAssistantRun(ctx, user.ID, failedExecution.RunID)
		if err != nil || failedRun.LatestTurn == nil ||
			failedRun.LatestTurn.State != assistant.TurnFailed {
			t.Fatalf("failed turn was not persisted: %#v %v", failedRun, err)
		}
	})

	t.Run("orchestrator pins the active guide for each confirmed turn", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Guide per turn", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		guide, err := dataStore.SaveAssistantGuide(
			ctx,
			user.ID,
			workspace.Project.ID,
			"# GUIDE-REVISION-ONE\n\nПервая редакция.",
			"manual",
			nil,
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t,
			ctx,
			dataStore,
			user.ID,
			workspace.Project.ID,
			nil,
			"Опиши стиль проекта",
		)
		baseRevision := guide.Revision
		guide, err = dataStore.SaveAssistantGuide(
			ctx,
			user.ID,
			workspace.Project.ID,
			"# GUIDE-REVISION-TWO\n\nВторая редакция применяется к следующему turn.",
			"manual",
			nil,
			nil,
			&baseRevision,
		)
		if err != nil {
			t.Fatal(err)
		}
		provider := &scriptedProvider{completions: []llm.Completion{
			{
				ResponseID: "guide-declare-answer",
				ToolCalls: []llm.ToolCall{{
					ID: "guide-begin-answer", Name: "assistant.begin_turn",
					Arguments: mustJSON(map[string]any{
						"mode": "answer", "summary": "Описать стиль проекта",
					}),
				}},
			},
			{ResponseID: "guide-answer", Text: "Стиль проекта описан в руководстве."},
		}}
		orchestrator := NewOrchestrator(
			dataStore,
			backend,
			staticProviderResolver{provider: provider},
		)
		if _, err := orchestrator.ExecuteTurn(
			ctx, user.ID, execution.RunID, execution.TurnID,
		); err != nil {
			t.Fatal(err)
		}
		if len(provider.requests) != 2 ||
			!strings.Contains(provider.requests[1].Instructions, "GUIDE-REVISION-TWO") ||
			strings.Contains(provider.requests[1].Instructions, "GUIDE-REVISION-ONE") {
			t.Fatalf("confirmed turn received the wrong guide revision: %#v", provider.requests)
		}
		run, err := dataStore.GetAssistantRun(ctx, user.ID, execution.RunID)
		if err != nil || run.LatestTurn == nil || run.LatestTurn.GuideRevision == nil ||
			*run.LatestTurn.GuideRevision != guide.Revision {
			t.Fatalf("turn did not persist its guide revision: run=%+v guide=%+v err=%v", run, guide, err)
		}
		if _, err := dataStore.PinAssistantTurnGuideRevision(
			ctx, user.ID, execution.RunID, execution.TurnID,
		); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("completed turn guide revision could be repinned: %v", err)
		}
		pinnedRevision := guide.Revision
		guide, err = dataStore.SaveAssistantGuide(
			ctx,
			user.ID,
			workspace.Project.ID,
			"# GUIDE-REVISION-THREE\n\nНе должна менять уже подтверждённый turn.",
			"manual",
			nil,
			nil,
			&pinnedRevision,
		)
		if err != nil {
			t.Fatal(err)
		}
		guideResult, err := backend.Call(
			ctx, execution, "assistant_guide.get", map[string]any{},
		)
		pinnedGuide, validGuide := guideResult["guide"].(assistant.Guide)
		if err != nil || !validGuide || pinnedGuide.Revision != pinnedRevision ||
			strings.Contains(pinnedGuide.Content, "GUIDE-REVISION-THREE") ||
			guideResult["pinnedToTurn"] != true {
			t.Fatalf(
				"guide tool drifted from the confirmed turn: result=%+v active=%+v err=%v",
				guideResult,
				guide,
				err,
			)
		}
	})

	t.Run("every catalog domain executes representative uncovered tools", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Полный каталог", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t, ctx, dataStore, user.ID, workspace.Nodes[0], "Первая редакция о Лиоре.",
		)
		node = saveNodeText(
			t, ctx, dataStore, user.ID, node, "Вторая редакция о Лиоре и башне.",
		)
		part, err := dataStore.CreateDocumentNode(
			ctx, user.ID, workspace.Project.ID, nil, "part", "Часть вторая",
		)
		if err != nil {
			t.Fatal(err)
		}
		draft, err := dataStore.CreateDocumentNode(
			ctx, user.ID, workspace.Project.ID, nil, "draft", "Черновик сцены",
		)
		if err != nil {
			t.Fatal(err)
		}
		draft = saveNodeText(t, ctx, dataStore, user.ID, draft, "Материал из черновика.")
		note, err := dataStore.CreateDocumentNode(
			ctx, user.ID, workspace.Project.ID, nil, "note", "Заметка об атмосфере",
		)
		if err != nil {
			t.Fatal(err)
		}
		note = saveNodeText(t, ctx, dataStore, user.ID, note, "Холодный свет.")
		term, err := dataStore.CreateDictionaryTerm(
			ctx, user.ID, workspace.Project.ID, "Лиор", "character", "ru", []string{"Лиора"},
		)
		if err != nil {
			t.Fatal(err)
		}
		memory, err := dataStore.SaveProjectMemory(ctx, user.ID, assistant.MemoryRecord{
			ProjectID: workspace.Project.ID, Kind: assistant.MemoryCharacter,
			Statement: "Лиор боится высоты.", Scope: "project",
			SourceReferences: []assistant.SourceReference{{
				Kind: "note", ID: note.ID, Label: note.Title,
			}},
			Tags: []string{"Лиор"}, Active: true, Source: "manual",
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		guide, err := dataStore.SaveAssistantGuide(
			ctx, user.ID, workspace.Project.ID,
			"# Голос проекта\n\nПишите сдержанно и сохраняйте имена из словаря.",
			"manual", nil, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		version, err := dataStore.CreateNamedVersion(
			ctx, user.ID, node.ID, "Версия перед аудитом", "Проверка каталога",
		)
		if err != nil {
			t.Fatal(err)
		}
		session, err := dataStore.CreateDictationSession(
			ctx, user.ID, workspace.Project.ID, &node.ID, uuid.NewString(), "ru",
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dataStore.StopDictation(
			ctx, user.ID, session.ID, 6000, 6000, mustJSON(map[string]any{"chunks": []any{}}),
		); err != nil {
			t.Fatal(err)
		}
		if err := dataStore.SaveAudioAsset(ctx, store.AudioAsset{
			ID: uuid.NewString(), SessionID: session.ID,
			SHA256: strings.Repeat("a", 64), StorageKey: "integration/audio.wav",
			ByteSize: 1024, DurationMS: 6000,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := dataStore.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
			SessionID: session.ID, BaseSourceText: "Скрытый колокол звучит в башне.",
			Content:   testDocument("Скрытый колокол звучит в башне."),
			PlainText: "Скрытый колокол звучит в башне.", ManuallyEdited: true,
			Source: "integration_test",
		}); err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t, ctx, dataStore, user.ID, workspace.Project.ID, nil,
			"Проверь и подготовь разные изменения проекта",
		)
		if _, err := dataStore.AppendAssistantEvent(ctx, assistant.Event{
			RunID: execution.RunID, TurnID: &execution.TurnID,
			Type: "STEP_STARTED", Data: mustJSON(map[string]any{"safe": true}),
		}); err != nil {
			t.Fatal(err)
		}

		reads := []struct {
			name      string
			arguments map[string]any
			resultKey string
		}{
			{"assistant_guide.get", map[string]any{}, "guide"},
			{"audio.get_source_range", map[string]any{
				"sessionId": session.ID, "startMs": 100, "endMs": 900,
			}, "available"},
			{"project.get_activity", map[string]any{"limit": 30}, "items"},
			{"version.list", map[string]any{"nodeId": node.ID}, "versions"},
			{"version.compare", map[string]any{
				"nodeId": node.ID, "leftRevision": node.CurrentRevision - 1,
				"rightRevision": node.CurrentRevision,
			}, "rightText"},
			{"assistant.get_capabilities", map[string]any{}, "toolCount"},
		}
		for _, read := range reads {
			result, err := backend.Call(ctx, execution, read.name, read.arguments)
			if err != nil || result[read.resultKey] == nil {
				t.Fatalf("%s did not execute: %#v %v", read.name, result, err)
			}
		}
		for _, search := range []struct {
			name, resultKey string
		}{
			{"project.search", "results"},
			{"mentions.find", "mentions"},
		} {
			result, err := backend.Call(
				ctx, execution, search.name, map[string]any{"query": "Скрытый колокол"},
			)
			matches, valid := result[search.resultKey].([]map[string]any)
			if err != nil || !valid || len(matches) != 1 ||
				matches[0]["sourceScope"] != "transcript" || matches[0]["sessionId"] != session.ID {
				t.Fatalf("%s did not search transcript context: %#v %v", search.name, result, err)
			}
		}
		if _, err := backend.Call(ctx, execution, "version.compare", map[string]any{
			"nodeId": node.ID, "leftRevision": node.CurrentRevision,
			"rightRevision": node.CurrentRevision + 100,
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("version.compare accepted a missing revision: %v", err)
		}

		sourceReference := []any{map[string]any{
			"kind": "note", "id": note.ID, "label": note.Title,
			"quote": "Холодный свет.", "start": 0, "end": 15,
		}}
		var nodeDocument editor.Node
		if err := json.Unmarshal(node.Content, &nodeDocument); err != nil ||
			len(nodeDocument.Content) == 0 {
			t.Fatalf("test document is not addressable: %#v %v", nodeDocument, err)
		}
		blockID, _ := nodeDocument.Content[0].Attrs["blockId"].(string)
		if blockID == "" {
			t.Fatal("test document block id is missing")
		}
		mutations := []struct {
			name      string
			arguments map[string]any
		}{
			{"tree.create_node", map[string]any{
				"parentId": nil, "kind": "chapter", "title": "Новая глава",
				"positionAfterId": node.ID,
			}},
			{"document.insert", map[string]any{
				"nodeId": node.ID, "baseRevision": node.CurrentRevision, "position": 0,
				"content": map[string]any{
					"type":    "paragraph",
					"content": []any{map[string]any{"type": "text", "text": "Перед главой."}},
				},
			}},
			{"document.replace_range", map[string]any{
				"nodeId": node.ID, "baseRevision": node.CurrentRevision,
				"edits": []any{map[string]any{
					"blockId": blockID, "start": 0, "end": 6,
					"expectedText": "Вторая", "replacement": "Следующая",
				}},
				"protectedRanges": []any{},
			}},
			{"document.apply_marks", map[string]any{
				"nodeId": node.ID, "baseRevision": node.CurrentRevision,
				"blockId": blockID, "start": 0, "end": 6,
				"marks": []any{map[string]any{"type": "bold", "href": nil}}, "protectedRanges": []any{},
			}},
			{"dictionary.set_active", map[string]any{
				"termId": term.ID, "active": false, "baseRevision": term.Revision,
			}},
			{"dictionary.update", map[string]any{
				"termId": term.ID, "canonical": "Лиор", "variants": []any{"Лиора", "Лиору"},
				"kind": "character", "language": "ru", "pronunciation": "ли-ОР",
				"active": true, "baseRevision": term.Revision,
			}},
			{"document.create_draft", map[string]any{
				"title": "Новый черновик", "content": assistantBlocks("Новая сцена."),
			}},
			{"document.set_title", map[string]any{
				"nodeId": node.ID, "title": "Новое название главы",
			}},
			{"draft.prepare_transfer", map[string]any{
				"draftNodeId": draft.ID, "destinationNodeId": node.ID,
				"baseRevision": node.CurrentRevision,
			}},
			{"note.create", map[string]any{
				"title": "Новая заметка", "content": assistantBlocks("Следить за светом."),
			}},
			{"note.update", map[string]any{
				"nodeId": note.ID, "baseRevision": note.CurrentRevision,
				"content": assistantBlocks("Тёплый янтарный свет."),
			}},
			{"project_memory.update", map[string]any{
				"memoryId": memory.ID, "baseRevision": memory.Revision,
				"kind": "character", "statement": "Лиор преодолевает страх высоты.",
				"scope": "project", "scopeNodeId": nil, "sourceReferences": sourceReference,
				"tags": []any{"Лиор", "башня"}, "active": true, "alwaysRelevant": false,
			}},
			{"project_memory.set_active", map[string]any{
				"memoryId": memory.ID, "active": false, "baseRevision": memory.Revision,
			}},
			{"project_memory.set_always_relevant", map[string]any{
				"memoryId": memory.ID, "alwaysRelevant": true, "baseRevision": memory.Revision,
			}},
			{"assistant_guide.propose_update", map[string]any{
				"baseRevision": guide.Revision,
				"content":      "# Голос проекта\n\nПишите ещё сдержаннее.",
			}},
			{"tree.duplicate_node", map[string]any{"nodeId": node.ID}},
			{"tree.move_node", map[string]any{
				"nodeId": node.ID, "parentId": part.ID, "positionAfterId": nil,
			}},
			{"tree.rename_node", map[string]any{
				"nodeId": node.ID, "title": "Переименованная глава",
			}},
			{"tree.set_status", map[string]any{
				"nodeId": node.ID, "status": "editing",
			}},
			{"version.create_snapshot", map[string]any{
				"nodeId": node.ID, "name": "Снимок ассистента", "note": "Перед правкой",
			}},
		}
		var memoryUpdateFound bool
		for index, mutation := range mutations {
			mutationExecution := execution
			if index > 0 {
				mutationExecution = createBackendExecution(
					t, ctx, dataStore, user.ID, workspace.Project.ID, nil,
					"Подготовь одно изолированное изменение проекта",
				)
			}
			result, err := backend.Call(
				ctx, mutationExecution, mutation.name, mutation.arguments,
			)
			if err != nil || result["staged"] != true {
				t.Fatalf("%s did not stage: %#v %v", mutation.name, result, err)
			}
			changeSet, err := backend.FinalizeChangeSet(
				ctx, mutationExecution, "Изолированное изменение каталога", "",
			)
			if err != nil || changeSet == nil || len(changeSet.Operations) != 1 {
				t.Fatalf("%s did not reach native review: %#v %v", mutation.name, changeSet, err)
			}
			operation := changeSet.Operations[0]
			if operation.ToolName == "project_memory.update" {
				var proposal assistant.MemoryRecord
				if json.Unmarshal(operation.ProposedSnapshot, &proposal) != nil ||
					len(proposal.SourceReferences) != 1 || proposal.SourceReferences[0].ID != note.ID {
					t.Fatalf("memory source reference was lost: %s", operation.ProposedSnapshot)
				}
				memoryUpdateFound = true
			}
		}
		if !memoryUpdateFound {
			t.Fatal("project_memory.update proposal was not found")
		}
		unchangedNode, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		unchangedMemory, memoryErr := dataStore.GetProjectMemory(ctx, user.ID, memory.ID)
		terms, termErr := dataStore.ListDictionaryTerms(ctx, user.ID, workspace.Project.ID)
		if err != nil || memoryErr != nil || termErr != nil ||
			unchangedNode.CurrentRevision != node.CurrentRevision ||
			unchangedNode.Title != node.Title || unchangedMemory.Revision != memory.Revision ||
			len(terms) != 1 || terms[0].Revision != term.Revision ||
			guide.Revision != 1 || version.Revision != node.CurrentRevision {
			t.Fatalf(
				"staging changed canonical state: node=%#v memory=%#v terms=%#v errors=%v/%v/%v",
				unchangedNode, unchangedMemory, terms, err, memoryErr, termErr,
			)
		}
	})

	t.Run("inline annotation MCP reads and stages server-owned anchors", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Заметки для ассистента", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t, ctx, dataStore, user.ID, workspace.Nodes[0], "Старая формулировка.",
		)
		var document editor.Node
		if err := json.Unmarshal(node.Content, &document); err != nil {
			t.Fatal(err)
		}
		blockID, _ := document.Content[0].Attrs["blockId"].(string)
		inlineNote, err := dataStore.CreateDocumentAnnotation(
			ctx, user.ID, node.ID, store.CreateAnnotationInput{
				Kind: "note", Note: "Сделать конкретнее", BaseRevision: node.CurrentRevision,
				Anchor: domain.AnnotationAnchor{
					BlockID: blockID, Start: 0, End: 6, ExactText: "Старая",
					Suffix: " формулировка.",
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		secondNote, err := dataStore.CreateDocumentAnnotation(
			ctx, user.ID, node.ID, store.CreateAnnotationInput{
				Kind: "note", Note: "Убрать канцеляризм", BaseRevision: node.CurrentRevision,
				Anchor: domain.AnnotationAnchor{
					BlockID: blockID, Start: 7, End: 19, ExactText: "формулировка",
					Prefix: "Старая ", Suffix: ".",
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		execution := createBackendExecution(
			t, ctx, dataStore, user.ID, workspace.Project.ID, nil,
			"Исправь все открытые заметки",
		)
		listed, err := backend.Call(ctx, execution, "annotation.list", map[string]any{
			"kind": "note", "status": "open", "anchorStatus": "attached",
			"nodeId": nil, "limit": 100,
		})
		annotations, valid := listed["annotations"].([]domain.DocumentAnnotation)
		listedIDs := map[string]bool{}
		for _, annotation := range annotations {
			listedIDs[annotation.ID] = true
		}
		if err != nil || !valid || len(annotations) != 2 ||
			!listedIDs[inlineNote.ID] || !listedIDs[secondNote.ID] {
			t.Fatalf("annotation.list = %#v, err=%v", listed, err)
		}
		got, err := backend.Call(ctx, execution, "annotation.get", map[string]any{
			"annotationId": inlineNote.ID,
		})
		if err != nil || got["annotation"] == nil || got["history"] == nil {
			t.Fatalf("annotation.get = %#v, err=%v", got, err)
		}
		staged, err := backend.Call(ctx, execution, "annotation.propose_fixes", map[string]any{
			"items": []any{
				map[string]any{
					"annotationId": inlineNote.ID,
					"baseRevision": inlineNote.Revision,
					"expectedText": "Старая",
					"replacement":  "Точная",
					"resolve":      true,
				},
				map[string]any{
					"annotationId": secondNote.ID,
					"baseRevision": secondNote.Revision,
					"expectedText": "формулировка",
					"replacement":  "фраза",
					"resolve":      true,
				},
			},
		})
		if err != nil || staged["staged"] != true || staged["annotationCount"] != 2 {
			t.Fatalf("annotation.propose_fixes = %#v, err=%v", staged, err)
		}
		changeSet, err := backend.FinalizeChangeSet(ctx, execution, "Исправить заметки", "")
		if err != nil || changeSet == nil || len(changeSet.Operations) != 1 ||
			changeSet.Operations[0].ToolName != "annotation.propose_fixes" {
			t.Fatalf("annotation change set = %#v, err=%v", changeSet, err)
		}
		var arguments struct {
			Edits []struct {
				BlockID string `json:"blockId"`
				Start   int    `json:"start"`
				End     int    `json:"end"`
			} `json:"edits"`
		}
		if err := json.Unmarshal(changeSet.Operations[0].Arguments, &arguments); err != nil ||
			len(arguments.Edits) != 2 || arguments.Edits[0].BlockID != blockID ||
			arguments.Edits[0].Start != 0 || arguments.Edits[0].End != 6 {
			t.Fatalf("server did not derive the annotation range: %#v, err=%v", arguments, err)
		}
		applied, err := dataStore.ApplyAssistantChangeSet(
			ctx, user.ID, execution.RunID, changeSet.ID, []string{"annotation:" + node.ID},
		)
		if err != nil || applied.State != assistant.ChangeApplied {
			t.Fatalf("same-block annotation batch was not applicable: %#v, err=%v", applied, err)
		}
		updatedNode, err := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		updatedFirst, firstErr := dataStore.GetDocumentAnnotation(ctx, user.ID, inlineNote.ID)
		updatedSecond, secondErr := dataStore.GetDocumentAnnotation(ctx, user.ID, secondNote.ID)
		if err != nil || firstErr != nil || secondErr != nil ||
			updatedNode.PlainText != "Точная фраза." ||
			updatedFirst.Status != "resolved" || updatedSecond.Status != "resolved" ||
			updatedFirst.BaseRevision != updatedNode.CurrentRevision ||
			updatedSecond.BaseRevision != updatedNode.CurrentRevision {
			t.Fatalf(
				"same-block annotation apply is inconsistent: node=%#v first=%#v second=%#v errors=%v/%v/%v",
				updatedNode, updatedFirst, updatedSecond, err, firstErr, secondErr,
			)
		}
	})

	t.Run("repeated exact edits coalesce into one applicable document operation", func(t *testing.T) {
		workspace, err := dataStore.CreateProject(ctx, user.ID, store.CreateProjectInput{
			Title: "Атомарные правки", Kind: "novel", Language: "ru",
		})
		if err != nil {
			t.Fatal(err)
		}
		node := saveNodeText(
			t, ctx, dataStore, user.ID, workspace.Nodes[0], "Первая старая. Вторая старая.",
		)
		var document editor.Node
		if err := json.Unmarshal(node.Content, &document); err != nil {
			t.Fatal(err)
		}
		blockID, _ := document.Content[0].Attrs["blockId"].(string)
		nodeID := node.ID
		execution := createBackendExecution(
			t, ctx, dataStore, user.ID, workspace.Project.ID, &nodeID,
			"Исправь обе формулировки",
		)
		first, err := backend.Call(ctx, execution, "document.replace_range", map[string]any{
			"nodeId": node.ID, "baseRevision": node.CurrentRevision,
			"protectedRanges": []any{},
			"edits": []any{map[string]any{
				"blockId": blockID, "start": 7, "end": 13,
				"expectedText": "старая", "replacement": "новая",
			}},
		})
		if err != nil || first["staged"] != true || first["merged"] != false {
			t.Fatalf("first exact edit = %#v, err=%v", first, err)
		}
		second, err := backend.Call(ctx, execution, "document.replace_range", map[string]any{
			"nodeId": node.ID, "baseRevision": node.CurrentRevision,
			"protectedRanges": []any{},
			"edits": []any{map[string]any{
				"blockId": blockID, "start": 22, "end": 28,
				"expectedText": "старая", "replacement": "свежая",
			}},
		})
		if err != nil || second["staged"] != true || second["merged"] != true ||
			backend.PendingCount(execution.TurnID) != 1 {
			t.Fatalf("second exact edit was not coalesced: %#v, err=%v", second, err)
		}
		changeSet, err := backend.FinalizeChangeSet(ctx, execution, "Исправить текст", "")
		if err != nil || changeSet == nil || len(changeSet.Operations) != 1 {
			t.Fatalf("coalesced change set = %#v, err=%v", changeSet, err)
		}
		var arguments struct {
			Edits []documentRangeEdit `json:"edits"`
		}
		if err := json.Unmarshal(changeSet.Operations[0].Arguments, &arguments); err != nil ||
			len(arguments.Edits) != 2 {
			t.Fatalf("coalesced edits = %#v, err=%v", arguments, err)
		}
		applied, err := dataStore.ApplyAssistantChangeSet(
			ctx, user.ID, execution.RunID, changeSet.ID, []string{"document"},
		)
		updated, getErr := dataStore.GetDocumentNode(ctx, user.ID, node.ID)
		if err != nil || getErr != nil || applied.State != assistant.ChangeApplied ||
			updated.CurrentRevision != node.CurrentRevision+1 ||
			updated.PlainText != "Первая новая. Вторая свежая." {
			t.Fatalf("coalesced apply = %#v node=%#v errors=%v/%v", applied, updated, err, getErr)
		}
	})
}

type staticProviderResolver struct {
	provider llm.Provider
	err      error
}

func (r staticProviderResolver) ProviderForRun(
	context.Context,
	string,
	assistant.Run,
) (llm.Provider, error) {
	return r.provider, r.err
}

type scriptedProvider struct {
	completions []llm.Completion
	requests    []llm.Request
	err         error
}

func chunkCompletion(responseID string, arguments map[string]any) llm.Completion {
	callID := responseID + "-call"
	encodedArguments := mustJSON(arguments)
	return llm.Completion{
		ResponseID: responseID,
		ToolCalls: []llm.ToolCall{{
			ID: callID, Name: "document.append_chunk", Arguments: encodedArguments,
		}},
		Continuation: []json.RawMessage{mustJSON(map[string]any{
			"role": "assistant",
			"tool_calls": []any{map[string]any{
				"id": callID, "type": "function",
				"function": map[string]any{
					"name": "document_append_chunk", "arguments": string(encodedArguments),
				},
			}},
		})},
	}
}

func (p *scriptedProvider) Complete(_ context.Context, request llm.Request) (llm.Completion, error) {
	p.requests = append(p.requests, request)
	if len(p.completions) == 0 {
		if p.err != nil {
			return llm.Completion{}, p.err
		}
		return llm.Completion{}, errors.New("script exhausted")
	}
	completion := p.completions[0]
	p.completions = p.completions[1:]
	return completion, nil
}

func createBackendExecution(
	t *testing.T,
	ctx context.Context,
	dataStore *store.Store,
	ownerID string,
	projectID string,
	targetNodeID *string,
	instruction string,
) projectmcp.ExecutionContext {
	t.Helper()
	targetKind := assistant.TargetProject
	if targetNodeID != nil {
		targetKind = assistant.TargetNode
	}
	run, err := dataStore.CreateAssistantRun(ctx, assistant.Run{
		OwnerUserID: ownerID, ProjectID: projectID,
		TargetKind: targetKind, TargetNodeID: targetNodeID,
		InputMode: assistant.InputChat, SilenceCheckpointMS: 5000,
		Provider: assistant.ProviderRef{
			Kind: assistant.ProviderSystem, Name: "Test", Model: "test",
			Dialect: assistant.DialectChatCompletions,
			Capabilities: assistant.CapabilitySnapshot{
				Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
			},
		},
		ReadContext: map[string]any{
			"test": true,
			"writerPreferences": assistant.WriterPreferencesSnapshot{
				Language: "ru", CommandPrefix: "Команда", CommandsEnabled: true,
				LiveTranscript: true, AssistantSilenceCheckpointMS: 5000,
				FontFamily: "sofia", FontSize: 19, LineHeight: 1.75,
				ManuscriptWidth: 70, Theme: "system", Spellcheck: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := dataStore.CreateAssistantTurn(
		ctx,
		ownerID,
		run.ID,
		assistant.InputChat,
		instruction,
		assistant.TurnQueued,
	)
	if err != nil {
		t.Fatal(err)
	}
	return projectmcp.ExecutionContext{
		OwnerUserID: ownerID, ProjectID: projectID,
		RunID: run.ID, TurnID: turn.ID, TargetNodeID: targetNodeID,
		Instruction: instruction,
	}
}

func saveNodeText(
	t *testing.T,
	ctx context.Context,
	dataStore *store.Store,
	ownerID string,
	node domain.DocumentNode,
	text string,
) domain.DocumentNode {
	t.Helper()
	content := testDocument(text)
	plain, err := editor.PlainText(content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataStore.SaveRevision(ctx, ownerID, node.ID, store.SaveRevisionInput{
		BaseRevision: node.CurrentRevision, Content: content,
		PlainText: plain, Source: "integration_test",
	}); err != nil {
		t.Fatal(err)
	}
	node, err = dataStore.GetDocumentNode(ctx, ownerID, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func testDocument(text string) json.RawMessage {
	return mustJSON(editor.Node{
		Type: "doc",
		Content: []editor.Node{{
			Type: "paragraph",
			Attrs: map[string]any{
				"blockId": uuid.NewString(), "sourceRefs": []string{},
			},
			Content: []editor.Node{{Type: "text", Text: text}},
		}},
	})
}

func assistantBlocks(text string) map[string]any {
	return map[string]any{
		"blocks": []any{map[string]any{
			"type": "paragraph",
			"text": text,
		}},
	}
}
