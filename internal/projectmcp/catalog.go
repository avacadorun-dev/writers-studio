package projectmcp

import (
	"writers.studio/internal/assistant"
	"writers.studio/internal/llm"
	"writers.studio/internal/projectmedia"
)

type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

func Catalog() []ToolSpec {
	noArgs := strictSchema(nil)
	projectQuery := strictSchema(map[string]any{
		"query": stringSchema(),
	})
	nodeID := strictSchema(map[string]any{"nodeId": stringSchema()})
	exactQuery := strictSchema(map[string]any{
		"text": stringSchema(), "blockId": nullableStringSchema(),
	})
	content := assistantContentSchema()
	documentMutation := strictSchema(map[string]any{
		"nodeId":       stringSchema(),
		"baseRevision": integerSchema(),
		"content":      content,
	})
	nodeCreateWithContent := strictSchema(map[string]any{
		"kind": stringSchema(), "title": stringSchema(), "content": content,
	})
	protectedRanges := arraySchema(strictSchema(map[string]any{
		"blockId": stringSchema(), "start": integerSchema(),
		"end": integerSchema(), "reason": stringSchema(),
	}))
	mark := strictSchema(map[string]any{
		"type": enumSchema("italic", "bold", "underline", "strike", "link", "code"),
		"href": nullableStringSchema(),
	})
	sourceReferences := arraySchema(strictSchema(map[string]any{
		"kind": stringSchema(), "id": stringSchema(),
		"label": nullableStringSchema(), "quote": nullableStringSchema(),
		"start": nullableIntegerSchema(), "end": nullableIntegerSchema(),
	}))
	memoryID := strictSchema(map[string]any{"memoryId": stringSchema()})
	dictionaryAdd := strictSchema(map[string]any{
		"canonical": stringSchema(), "variants": arraySchema(stringSchema()),
		"kind":     enumSchema("character", "place", "term", "other"),
		"language": enumSchema("ru", "en"), "pronunciation": stringSchema(),
		"active": booleanSchema(),
	})
	memoryAdd := strictSchema(map[string]any{
		"kind":      enumSchema("fact", "character", "world_rule", "plot", "style", "constraint", "decision", "other"),
		"statement": stringSchema(), "scope": enumSchema("project", "node"),
		"scopeNodeId": nullableStringSchema(), "sourceReferences": sourceReferences,
		"tags": arraySchema(stringSchema()), "active": booleanSchema(),
		"alwaysRelevant": booleanSchema(),
	})
	canvasID := strictSchema(map[string]any{"canvasId": stringSchema()})
	canvasPatchOperation := strictSchema(map[string]any{
		"op":        enumSchema("add", "update", "delete"),
		"kind":      enumSchema("node", "edge"),
		"id":        stringSchema(),
		"valueJson": stringSchema(),
	})
	return []ToolSpec{
		spec(
			"assistant.begin_turn",
			"Required exactly once at the start of every turn. Semantically classify the current "+
				"author request without lexical rules: answer reads/explains, change stages one or "+
				"more project mutations, clarify asks for missing information.",
			strictSchema(map[string]any{
				"mode":    enumSchema("answer", "change", "clarify"),
				"summary": stringSchema(),
			}),
		),
		spec(
			"assistant.get_recent_change_set",
			"Read the latest native change set for this run, including rejected or applied state. "+
				"Use it when revising an earlier proposal; historical chat prose is not authoritative.",
			noArgs,
		),
		spec("project.get_overview", "Read project metadata, structure summary and current target.", noArgs),
		spec("project.get_structure", "Read project-defined node types, labels and nesting rules.", noArgs),
		spec("project.list_media", "List project media that may be referenced by canonical image blocks. Use an image assetId from this result.", noArgs),
		spec(
			"project.upload_image",
			"Upload one PNG, JPEG, GIF or WebP image directly into the open project and return its assetId plus a ready canonical image block. Pass raw base64 only, without a data-URL prefix. This stores the otherwise unattached asset immediately; document insertion remains a separately reviewed change.",
			strictSchema(map[string]any{
				"filename": boundedStringSchema(1, 255),
				"mediaType": enumSchema(
					"image/png", "image/jpeg", "image/gif", "image/webp",
				),
				"dataBase64": boundedStringSchema(1, projectmedia.MaxBase64Length),
			}),
		),
		spec("project.list_recent", "List recently changed project artifacts.", strictSchema(map[string]any{"limit": integerSchema()})),
		spec("project.search", "Search documents, notes, drafts and transcripts in this project.", projectQuery),
		spec("project.get_activity", "Read recent native and assistant project activity.", strictSchema(map[string]any{"limit": integerSchema()})),
		spec("canvas.list", "List active Canvas boards in this project.", noArgs),
		spec("canvas.get", "Read one Canvas snapshot with its current revision and resolved project resources.", canvasID),
		spec("canvas.search", "Search standalone Canvas text, typed fields and linked metadata in this project.", projectQuery),
		spec("canvas.analyze_graph", "Analyze one Canvas graph for components, orphans, roots, leaves and cycles without changing it.", canvasID),
		spec("canvas.propose_patch", "Stage typed add, update and delete operations for one Canvas revision. valueJson must contain one complete JSON Canvas node or edge for add/update and be an empty string for delete. Canonical Canvas stays unchanged until Apply.", strictSchema(map[string]any{
			"canvasId": stringSchema(), "baseRevision": integerSchema(),
			"operations": boundedArraySchema(canvasPatchOperation, 1, 500),
		})),
		spec("canvas.promote_cards", "Stage atomic promotion of selected Canvas text cards into project documents. Canonical Canvas and project tree stay unchanged until Apply.", strictSchema(map[string]any{
			"canvasId": stringSchema(), "baseRevision": integerSchema(),
			"items": boundedArraySchema(strictSchema(map[string]any{
				"nodeId": stringSchema(), "title": stringSchema(), "kind": stringSchema(),
				"parentId": nullableStringSchema(),
			}), 1, 50),
		})),
		spec("tree.list", "List the complete project tree without document bodies.", noArgs),
		spec("tree.get_node", "Read node metadata and revision.", nodeID),
		spec("tree.create_node", "Stage creation of a project node.", strictSchema(map[string]any{
			"parentId": nullableStringSchema(), "kind": stringSchema(),
			"title": stringSchema(), "positionAfterId": nullableStringSchema(),
		})),
		spec("tree.create_nodes_batch", "Stage 1 to 50 project nodes together with their complete initial content. Use this for every new content-bearing node and for every multi-node request; never call tree.create_node and then document.append for an unapplied node. All items share one canonical parent and are inserted in array order after positionAfterId. Each item becomes an independent tree.create_node review operation.", strictSchema(map[string]any{
			"parentId": nullableStringSchema(), "positionAfterId": nullableStringSchema(),
			"items": boundedArraySchema(nodeCreateWithContent, 1, 50),
		})),
		spec("tree.rename_node", "Stage a node title change.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "title": stringSchema(),
		})),
		spec("tree.move_node", "Stage moving a node in the project tree.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "parentId": nullableStringSchema(),
			"positionAfterId": nullableStringSchema(),
		})),
		spec("tree.set_status", "Stage a node workflow status change.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "status": enumSchema("idea", "draft", "editing", "ready"),
		})),
		spec("tree.duplicate_node", "Stage duplication of a node.", nodeID),
		spec("tree.prepare_archive", "Prepare a high-risk archive operation for review.", nodeID),
		spec("document.get", "Read the current structured document, canonical blocks and plain text. Blocks preserve native code, lists, separators, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts without flattening them into paragraphs.", nodeID),
		spec(
			"document.find_exact",
			"Locate an exact literal substring and return verified block-local Unicode anchors for "+
				"document.replace_range. This is deterministic indexing, not semantic inference. "+
				"Use it instead of counting offsets.",
			strictSchema(map[string]any{
				"nodeId": stringSchema(), "revision": integerSchema(),
				"text": stringSchema(), "blockId": nullableStringSchema(),
			}),
		),
		spec(
			"document.find_exact_batch",
			"Locate several exact literal substrings in one document revision and return verified "+
				"block-local Unicode anchors for one atomic document.replace_range call. Use this "+
				"instead of one document.find_exact call per paragraph.",
			strictSchema(map[string]any{
				"nodeId": stringSchema(), "revision": integerSchema(),
				"queries": arraySchema(exactQuery),
			}),
		),
		spec("document.get_range", "Read an exact text range from a document revision.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "revision": integerSchema(),
			"start": integerSchema(), "end": integerSchema(),
		})),
		spec("document.list_references", "List source references attached to document blocks.", nodeID),
		spec("document.insert", "Stage structured content insertion at an exact position. Content supports canonical prose, native code, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "baseRevision": integerSchema(),
			"position": integerSchema(), "content": content,
		})),
		spec(
			"document.replace_range",
			"Stage one atomic replacement of one or more exact block-local text ranges. "+
				"Every anchor must come from document.find_exact or document.find_exact_batch; "+
				"never calculate offsets manually. "+
				"Combine every edit for the same document in this call so review, apply and undo stay atomic.",
			strictSchema(map[string]any{
				"nodeId": stringSchema(), "baseRevision": integerSchema(),
				"edits": arraySchema(strictSchema(map[string]any{
					"blockId": stringSchema(), "start": integerSchema(),
					"end": integerSchema(), "expectedText": stringSchema(),
					"replacement": stringSchema(),
				})),
				"protectedRanges": protectedRanges,
			}),
		),
		spec("document.append", "Stage appending canonical prose, native code, tables, project images, Mermaid diagrams or info/idea/warning/todo callouts to a document.", documentMutation),
		spec(
			"document.append_chunk",
			"Stage one bounded long-form chunk in an existing document. Use this repeatedly instead of document.append whenever the requested new prose may exceed 1500 words. Chunks are accumulated in sequence into one reviewable document operation; canonical state remains unchanged until Apply. Keep each chunk between roughly 600 and 1200 words, copy the author's approximate total into targetWords, start sequence at 1, and set finalChunk only when the accumulated draft reaches the target.",
			strictSchema(map[string]any{
				"nodeId": stringSchema(), "baseRevision": integerSchema(),
				"sequence":    boundedIntegerSchema(1, 16),
				"targetWords": boundedIntegerSchema(100, 20_000),
				"finalChunk":  booleanSchema(),
				"text":        boundedStringSchema(1, 9_000),
			}),
		),
		spec("document.set_title", "Stage a document title change.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "title": stringSchema(),
		})),
		spec("document.apply_marks", "Stage formatting marks on an exact range.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "baseRevision": integerSchema(),
			"blockId": stringSchema(), "start": integerSchema(), "end": integerSchema(),
			"marks": arraySchema(mark), "protectedRanges": protectedRanges,
		})),
		spec("document.create_draft", "Stage a new draft document with canonical prose, native code, tables, project images, Mermaid diagrams and info/idea/warning/todo callouts.", strictSchema(map[string]any{
			"title": stringSchema(), "content": content,
			"parentId": nullableStringSchema(), "positionAfterId": nullableStringSchema(),
		})),
		spec("note.create", "Stage a project note.", strictSchema(map[string]any{
			"title": stringSchema(), "content": content,
			"parentId": nullableStringSchema(), "positionAfterId": nullableStringSchema(),
		})),
		spec("note.update", "Stage a note update.", documentMutation),
		spec(
			"annotation.list",
			"List inline manuscript annotations across the whole project. Use this before a project-wide note cleanup; results include immutable annotation revisions and exact server-owned anchors.",
			strictSchema(map[string]any{
				"kind":         enumSchema("all", "note", "dictionary_link"),
				"status":       enumSchema("all", "open", "resolved", "needs_attention"),
				"anchorStatus": enumSchema("all", "attached", "detached"),
				"nodeId":       nullableStringSchema(),
				"limit":        boundedIntegerSchema(1, 500),
			}),
		),
		spec(
			"annotation.get",
			"Read one inline annotation together with its versioned Assistant reanchoring history.",
			strictSchema(map[string]any{"annotationId": stringSchema()}),
		),
		spec(
			"annotation.propose_fixes",
			"Stage exact manuscript fixes for 1 to 50 inline notes. Supply annotation IDs and revisions from annotation.list/get; the server derives every text range from the current anchor, groups edits atomically by document, and reanchors notes itself. Never supply offsets. resolve=true marks the note resolved only after Apply; notes are never deleted.",
			strictSchema(map[string]any{
				"items": boundedArraySchema(strictSchema(map[string]any{
					"annotationId": stringSchema(),
					"baseRevision": integerSchema(),
					"expectedText": stringSchema(),
					"replacement":  stringSchema(),
					"resolve":      booleanSchema(),
				}), 1, 50),
			}),
		),
		spec("document.prepare_transfer", "Prepare copying content between any two content-bearing project nodes.", strictSchema(map[string]any{
			"sourceNodeId": stringSchema(), "destinationNodeId": stringSchema(),
			"baseRevision": integerSchema(),
		})),
		spec("draft.prepare_transfer", "Prepare transferring a draft into a manuscript node.", strictSchema(map[string]any{
			"draftNodeId": stringSchema(), "destinationNodeId": stringSchema(),
			"baseRevision": integerSchema(),
		})),
		spec("document.prepare_replace_all", "Prepare a high-risk project-wide replacement.", strictSchema(map[string]any{
			"query": stringSchema(), "replacement": stringSchema(),
			"nodeIds": arraySchema(stringSchema()),
		})),
		spec("dictionary.list", "List active and inactive project dictionary terms.", noArgs),
		spec("dictionary.find", "Find dictionary terms by canonical name or variant.", projectQuery),
		spec("dictionary.add", "Stage one new dictionary term. If a canonical name or variant already exists, returns a typed conflict and does not stage a duplicate. Use dictionary.add_batch whenever the request contains two or more new terms.", dictionaryAdd),
		spec("dictionary.add_batch", "Stage 2 to 100 new dictionary terms in one call. Prefer this tool for every multi-term request so large author instructions stay within the safe tool-call limit. Each staged item remains an independent reviewable operation; duplicates return typed per-item conflicts.", strictSchema(map[string]any{
			"items": boundedArraySchema(dictionaryAdd, 2, 100),
		})),
		spec("dictionary.update", "Stage an update to a dictionary term.", strictSchema(map[string]any{
			"termId": stringSchema(), "canonical": stringSchema(),
			"variants": arraySchema(stringSchema()),
			"kind":     enumSchema("character", "place", "term", "other"),
			"language": enumSchema("ru", "en"), "pronunciation": stringSchema(),
			"active": booleanSchema(), "baseRevision": integerSchema(),
		})),
		spec("dictionary.set_active", "Stage activating or deactivating a dictionary term.", strictSchema(map[string]any{
			"termId": stringSchema(), "active": booleanSchema(), "baseRevision": integerSchema(),
		})),
		spec("mentions.find", "Find mentions of a term across project text.", projectQuery),
		spec("assistant_guide.get", "Read the pinned assistant writing guide revision.", noArgs),
		spec("assistant_guide.propose_create", "Stage creation of the project assistant guide.", strictSchema(map[string]any{"content": stringSchema()})),
		spec("assistant_guide.propose_update", "Stage a new guide revision.", strictSchema(map[string]any{
			"baseRevision": integerSchema(), "content": stringSchema(),
		})),
		spec("project_memory.list", "List project memory records.", strictSchema(map[string]any{"includeInactive": booleanSchema()})),
		spec("project_memory.search", "Search project memory records.", projectQuery),
		spec("project_memory.get", "Read one project memory record.", memoryID),
		spec("project_memory.add", "Stage one project memory record. If the same normalized statement already exists, returns a typed conflict and does not stage a duplicate. Use project_memory.add_batch whenever the request contains two or more new records.", memoryAdd),
		spec("project_memory.add_batch", "Stage 2 to 100 project memory records in one call. Prefer this tool for every multi-record request so large author instructions stay within the safe tool-call limit. Each staged item remains an independent reviewable operation; duplicates return typed per-item conflicts.", strictSchema(map[string]any{
			"items": boundedArraySchema(memoryAdd, 2, 100),
		})),
		spec("project_memory.update", "Stage an update to a project memory record.", strictSchema(map[string]any{
			"memoryId": stringSchema(), "baseRevision": integerSchema(),
			"kind":      enumSchema("fact", "character", "world_rule", "plot", "style", "constraint", "decision", "other"),
			"statement": stringSchema(), "scope": enumSchema("project", "node"),
			"scopeNodeId": nullableStringSchema(), "sourceReferences": sourceReferences,
			"tags": arraySchema(stringSchema()), "active": booleanSchema(),
			"alwaysRelevant": booleanSchema(),
		})),
		spec("project_memory.set_active", "Stage activating or deactivating memory.", strictSchema(map[string]any{
			"memoryId": stringSchema(), "active": booleanSchema(), "baseRevision": integerSchema(),
		})),
		spec("project_memory.set_always_relevant", "Stage always-relevant memory status.", strictSchema(map[string]any{
			"memoryId": stringSchema(), "alwaysRelevant": booleanSchema(),
			"baseRevision": integerSchema(),
		})),
		spec("version.list", "List document named versions and revisions.", nodeID),
		spec("version.compare", "Compare two document revisions.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "leftRevision": integerSchema(),
			"rightRevision": integerSchema(),
		})),
		spec("version.create_snapshot", "Stage creation of a named document snapshot.", strictSchema(map[string]any{
			"nodeId": stringSchema(), "name": stringSchema(), "note": stringSchema(),
		})),
		spec("version.prepare_restore", "Prepare a high-risk version restore.", strictSchema(map[string]any{
			"versionId": stringSchema(), "expectedCurrentRevision": integerSchema(),
		})),
		spec("dictation.list_sessions", "List dictation sessions linked to this project.", strictSchema(map[string]any{"limit": integerSchema()})),
		spec("transcript.get", "Read transcript and draft metadata.", strictSchema(map[string]any{"sessionId": stringSchema()})),
		spec("transcript.get_uncertain", "Read low-confidence transcript ranges.", strictSchema(map[string]any{
			"sessionId": stringSchema(), "maximumConfidence": numberSchema(),
		})),
		spec("audio.get_source_range", "Get authorized audio source range metadata.", strictSchema(map[string]any{
			"sessionId": stringSchema(), "startMs": integerSchema(), "endMs": integerSchema(),
		})),
		spec("transcript.prepare_transfer", "Prepare transferring a transcript draft.", strictSchema(map[string]any{
			"sessionId": stringSchema(), "nodeId": stringSchema(), "baseRevision": integerSchema(),
		})),
		spec("transcript.prepare_retranscribe", "Prepare explicit transcript retranscription.", strictSchema(map[string]any{
			"sessionId": stringSchema(),
		})),
		spec("export.preview", "Preview a project export without creating it.", strictSchema(map[string]any{
			"format":  enumSchema("docx", "epub", "pdf", "markdown", "txt"),
			"nodeIds": arraySchema(stringSchema()), "chapterTitles": booleanSchema(),
		})),
		spec("export.prepare", "Prepare creating an export artifact.", strictSchema(map[string]any{
			"format":  enumSchema("docx", "epub", "pdf", "markdown", "txt"),
			"nodeIds": arraySchema(stringSchema()), "chapterTitles": booleanSchema(),
		})),
		spec("recovery.get_status", "Read recovery and unsynchronized-data status.", noArgs),
		spec(
			"settings.get_writer_preferences",
			"Read the run-pinned non-secret writer preferences snapshot.",
			noArgs,
		),
		spec("assistant.get_capabilities", "Read assistant/provider/tool capabilities.", noArgs),
	}
}

func PublicCatalog() []ToolSpec {
	return Catalog()
}

// RemoteCatalog is deliberately a strict superset of PublicCatalog. The
// built-in assistant consumes PublicCatalog and therefore remains insulated
// from remote workspace/session and review lifecycle tools.
func RemoteCatalog() []ToolSpec {
	result := append([]ToolSpec(nil), PublicCatalog()...)
	annotationResolution := strictSchema(map[string]any{
		"annotationId": stringSchema(), "resolved": booleanSchema(),
	})
	result = append(result,
		spec("tree.archive", "Stage a high-risk node archive operation. Canonical state changes only after change_set.apply and supports Undo.", strictSchema(map[string]any{
			"nodeId": stringSchema(),
		})),
		spec("version.restore", "Stage restoring a named version as a new document revision. Requires the exact current revision and supports Undo.", strictSchema(map[string]any{
			"versionId": stringSchema(), "expectedCurrentRevision": integerSchema(),
		})),
		spec("workspace.list_projects", "List projects allowed by this access grant. Trash is never exposed.", strictSchema(map[string]any{
			"status": map[string]any{"type": []string{"string", "null"}, "enum": []any{"active", "archived", nil}},
		})),
		spec("workspace.open_project", "Open one grant-authorized project in this MCP session. The server binds all subsequent project tools to it.", strictSchema(map[string]any{
			"projectId": stringSchema(), "targetNodeId": nullableStringSchema(),
		})),
		spec("workspace.get_context", "Read the server-bound project, target, turn and grant context for this session.", strictSchema(nil)),
		spec("workspace.close_project", "Close the active project after its turn has been finished or discarded.", strictSchema(nil)),
		spec("assistant.finish_turn", "Finish the remote turn and atomically convert its durable draft into a normal Assistant change set.", strictSchema(map[string]any{
			"title": stringSchema(), "summary": stringSchema(),
		})),
		spec("assistant.discard_turn", "Discard only the current unapplied durable draft and close the active turn.", strictSchema(map[string]any{
			"reason": stringSchema(),
		})),
		spec("change_set.list", "List change sets created by this grant. An empty projectId lists all authorized projects.", strictSchema(map[string]any{
			"projectId": stringSchema(), "limit": integerSchema(),
		})),
		spec("change_set.get", "Get one change set created by this grant.", strictSchema(map[string]any{
			"changeSetId": stringSchema(),
		})),
		spec("change_set.review", "Read exact operation groups and snapshots, then receive a one-time 10-minute review token bound to the current content and state.", strictSchema(map[string]any{
			"changeSetId": stringSchema(), "action": enumSchema("apply", "reject", "undo"),
		})),
		spec("change_set.apply", "Atomically consume a review token and apply selected operation groups with stale and dependency checks.", strictSchema(map[string]any{
			"changeSetId": stringSchema(), "reviewToken": stringSchema(),
			"selectedGroups":        arraySchema(stringSchema()),
			"annotationResolutions": arraySchema(annotationResolution),
		})),
		spec("change_set.reject", "Consume a review token and reject a staged change set.", strictSchema(map[string]any{
			"changeSetId": stringSchema(), "reviewToken": stringSchema(),
		})),
		spec("change_set.undo", "Consume a fresh review token for an applied change set and undo it.", strictSchema(map[string]any{
			"changeSetId": stringSchema(), "reviewToken": stringSchema(),
		})),
		spec("transcript.retranscribe", "Consume the one-time token from transcript.prepare_retranscribe and queue retranscription idempotently.", strictSchema(map[string]any{
			"sessionId": stringSchema(), "actionToken": stringSchema(),
		})),
		spec("export.create", "Consume the one-time token from export.prepare and create an export job idempotently.", strictSchema(map[string]any{
			"actionToken": stringSchema(),
		})),
		spec("export.get", "Poll an export job created by this grant. A ready job exposes its authorized MCP resource URI.", strictSchema(map[string]any{
			"exportId": stringSchema(),
		})),
	)
	return result
}

func Definitions(specs []ToolSpec) []llm.ToolDefinition {
	result := make([]llm.ToolDefinition, 0, len(specs))
	for _, tool := range specs {
		result = append(result, llm.ToolDefinition{
			Name: tool.Name, Description: tool.Description, Parameters: tool.Schema,
		})
	}
	return result
}

func spec(name string, description string, schema map[string]any) ToolSpec {
	return ToolSpec{Name: name, Description: description, Schema: schema}
}

func strictSchema(properties map[string]any) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	return map[string]any{
		"type": "object", "properties": properties, "required": required,
		"additionalProperties": false,
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func boundedStringSchema(minimum, maximum int) map[string]any {
	return map[string]any{
		"type": "string", "minLength": minimum, "maxLength": maximum,
	}
}

func nullableStringSchema() map[string]any {
	return map[string]any{"type": []string{"string", "null"}}
}

func integerSchema() map[string]any {
	return map[string]any{"type": "integer"}
}

func boundedIntegerSchema(minimum, maximum int) map[string]any {
	return map[string]any{
		"type": "integer", "minimum": minimum, "maximum": maximum,
	}
}

func nullableIntegerSchema() map[string]any {
	return map[string]any{"type": []string{"integer", "null"}}
}

func numberSchema() map[string]any {
	return map[string]any{"type": "number"}
}

func booleanSchema() map[string]any {
	return map[string]any{"type": "boolean"}
}

func assistantContentSchema() map[string]any {
	inlineText := map[string]any{
		"type":        "string",
		"description": "Prose text; wrap inline code in matching Markdown backticks, for example `go test`. It is stored as a canonical code mark and returned with the language-neutral backtick syntax.",
	}
	proseBlock := strictSchema(map[string]any{
		"type": enumSchema("paragraph", "heading", "blockquote"),
		"text": inlineText,
	})
	codeBlockWithoutLanguage := strictSchema(map[string]any{
		"type": enumSchema("code"), "text": stringSchema(),
	})
	codeBlockWithLanguage := strictSchema(map[string]any{
		"type": enumSchema("code"), "language": stringSchema(), "text": stringSchema(),
	})
	calloutBlock := strictSchema(map[string]any{
		"type": enumSchema("callout"), "kind": enumSchema("info", "idea", "warning", "todo"),
		"text": inlineText,
	})
	mermaidBlock := strictSchema(map[string]any{
		"type": enumSchema("mermaid"), "code": stringSchema(), "caption": stringSchema(),
	})
	imageBlock := strictSchema(map[string]any{
		"type": enumSchema("image"), "assetId": stringSchema(),
		"alt": stringSchema(), "caption": stringSchema(),
	})
	tableCell := strictSchema(map[string]any{
		"text": inlineText, "header": booleanSchema(),
		"colspan": boundedIntegerSchema(1, 100), "rowspan": boundedIntegerSchema(1, 100),
	})
	tableRow := strictSchema(map[string]any{
		"cells": boundedArraySchema(tableCell, 1, 100),
	})
	tableBlock := strictSchema(map[string]any{
		"type": enumSchema("table"), "rows": boundedArraySchema(tableRow, 1, 1_000),
	})
	listBlock := strictSchema(map[string]any{
		"type":  enumSchema("bullet_list", "ordered_list"),
		"items": boundedArraySchema(inlineText, 1, 1_000),
	})
	separatorBlock := strictSchema(map[string]any{
		"type": enumSchema("scene_break", "horizontal_rule"),
	})
	block := map[string]any{"anyOf": []any{
		proseBlock, codeBlockWithoutLanguage, codeBlockWithLanguage,
		calloutBlock, mermaidBlock, imageBlock, tableBlock,
		listBlock, separatorBlock,
	}}
	result := strictSchema(map[string]any{
		"blocks": arraySchema(block),
	})
	result["description"] = "Canonical document blocks. Prose strings support Markdown backtick spans as native inline code; code blocks keep their text byte-for-byte and may declare an optional language."
	return result
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func boundedArraySchema(items map[string]any, minimum, maximum int) map[string]any {
	return map[string]any{
		"type": "array", "items": items, "minItems": minimum, "maxItems": maximum,
	}
}

func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func ToolPolicy(name string) assistant.ToolPolicy {
	return assistant.ClassifyTool(name)
}
