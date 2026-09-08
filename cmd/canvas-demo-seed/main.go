package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/store"
)

const (
	defaultOwnerID   = "5f09b150-b0da-4402-8667-a4bc6bce14a0"
	defaultProjectID = "0c1b1115-9818-41cc-8c82-e26ed20549d3"
	mainCanvasID     = "10f3ba15-5233-4b25-8f51-d86aa970c8a5"
	moodboardID      = "5565ef47-278a-48d0-8903-e477645161b6"
	showcaseCanvasID = "9a6d378a-01b7-4c42-8f51-1e20cbe65001"
	guideDocumentID  = "2e18f6b2-3d7d-44d7-8c9b-11f8c5e23001"
	imageMediaID     = "9fdf8472-89e1-4fa7-9d25-13c967364285"
	pdfMediaID       = "65f36ac4-d1ee-4a11-8ef3-1091234abcd1"
	audioMediaID     = "65f36ac4-d1ee-4a11-8ef3-1091234abcd2"
	videoMediaID     = "65f36ac4-d1ee-4a11-8ef3-1091234abcd3"
)

type canvasDoc struct {
	Nodes     []map[string]any `json:"nodes"`
	Edges     []map[string]any `json:"edges"`
	Extension map[string]any   `json:"x-writers-studio"`
}

func main() {
	ownerID := flag.String("owner", defaultOwnerID, "owner user UUID")
	projectID := flag.String("project", defaultProjectID, "demo project UUID")
	cookieFile := flag.String("cookie-file", "", "write a one-hour localhost session cookie for transfer verification")
	validateArchive := flag.String("validate-archive", "", "validate a .wstudio project manifest and exit")
	flag.Parse()
	if *validateArchive != "" {
		if err := validateProjectArchive(*validateArchive); err != nil {
			log.Fatal(err)
		}
		fmt.Println("project archive manifest is valid")
		return
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	dataStore, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer dataStore.Close()
	if err := seed(ctx, dataStore, *ownerID, *projectID); err != nil {
		log.Fatal(err)
	}
	if *cookieFile != "" {
		if err := writeSessionCookie(ctx, dataStore, *ownerID, *cookieFile); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("Canvas demo ready: /projects/%s/canvases/%s\n", *projectID, mainCanvasID)
}

func validateProjectArchive(path string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, item := range archive.File {
		if item.Name != "project.json" {
			continue
		}
		reader, err := item.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			return readErr
		}
		var transfer store.ProjectTransfer
		if err := json.Unmarshal(data, &transfer); err != nil {
			return err
		}
		if err := store.ValidateProjectTransfer(transfer); err != nil {
			return err
		}
		return validateDemoFeatureMatrix(transfer)
	}
	return errors.New("project.json is missing")
}

func validateDemoFeatureMatrix(transfer store.ProjectTransfer) error {
	if transfer.FormatVersion != store.ProjectTransferFormatVersion {
		return fmt.Errorf("demo archive format version = %d, want %d", transfer.FormatVersion, store.ProjectTransferFormatVersion)
	}
	if len(transfer.Nodes) < 9 || len(transfer.DocumentRevisions) < 9 || len(transfer.Canvases) < 3 || len(transfer.CanvasRevisions) < 3 {
		return errors.New("demo archive does not contain the complete document and Canvas set")
	}
	mediaTypes := make(map[string]bool, len(transfer.Media))
	for _, item := range transfer.Media {
		mediaTypes[item.MediaType] = true
	}
	if err := requireDemoValues("media types", mediaTypes, []string{"image/png", "application/pdf", "audio/mpeg", "video/mp4"}); err != nil {
		return err
	}
	if transfer.CanvasSchema == nil || len(transfer.CanvasSchema.Revisions) < 2 {
		return errors.New("demo archive does not contain Canvas schema history")
	}
	latest := transfer.CanvasSchema.Revisions[len(transfer.CanvasSchema.Revisions)-1].Schema
	cardTypes := make(map[string]bool, len(latest.CardTypes))
	fieldTypes := make(map[string]bool)
	for _, card := range latest.CardTypes {
		cardTypes[card.ID] = true
		for _, field := range card.Fields {
			fieldTypes[field.Type] = true
		}
	}
	if err := requireDemoValues("card types", cardTypes, []string{
		"idea", "question", "source", "character", "location", "beat", "conflict", "theme",
		"mechanic", "quest", "dialogue", "state", "level", "resource", "playtest-hypothesis", "evidence",
	}); err != nil {
		return err
	}
	if err := requireDemoValues("field types", fieldTypes, []string{
		"short-text", "markdown", "number", "checkbox", "single-select", "multi-select", "date", "duration", "document", "media", "canvas",
	}); err != nil {
		return err
	}
	relationTypes := make(map[string]bool, len(latest.RelationTypes))
	for _, relation := range latest.RelationTypes {
		relationTypes[relation.ID] = true
	}
	if err := requireDemoValues("relation types", relationTypes, []string{
		"relates", "causes", "contradicts", "depends_on", "blocks", "reveals", "transitions_to", "contains", "consumes", "produces", "rewards", "validates",
	}); err != nil {
		return err
	}
	var canvasJSON strings.Builder
	for _, revision := range transfer.CanvasRevisions {
		var snapshot any
		if err := json.Unmarshal(revision.Content, &snapshot); err != nil {
			return fmt.Errorf("decode demo Canvas revision: %w", err)
		}
		normalized, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("normalize demo Canvas revision: %w", err)
		}
		canvasJSON.Write(normalized)
	}
	content := canvasJSON.String()
	for _, marker := range []string{
		`"type":"link"`, `"kind":"stroke"`, `"kind":"shape"`, `"kind":"frame"`,
		`"kind":"canvas"`, `"kind":"media"`, `"promotedDocumentId"`, `"documentRef"`,
		`"mediaRef"`, `"canvasRef"`, `"locked":true`, `"opaqueExtensionPreserved":true`,
	} {
		if !strings.Contains(content, marker) {
			return fmt.Errorf("demo Canvas snapshots do not contain %s", marker)
		}
	}
	return nil
}

func requireDemoValues(label string, actual map[string]bool, expected []string) error {
	for _, value := range expected {
		if !actual[value] {
			return fmt.Errorf("demo archive %s do not contain %s", label, value)
		}
	}
	return nil
}

func writeSessionCookie(ctx context.Context, dataStore *store.Store, ownerID, path string) error {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().Add(time.Hour)
	if _, err := dataStore.Pool.Exec(ctx, `
		INSERT INTO auth_sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4)
	`, uuid.NewString(), ownerID, hex.EncodeToString(sum[:]), expires); err != nil {
		return err
	}
	line := strings.Join([]string{"localhost", "FALSE", "/", "FALSE", fmt.Sprint(expires.Unix()), "writers_studio_session", token}, "\t") + "\n"
	return os.WriteFile(path, []byte("# Netscape HTTP Cookie File\n"+line), 0o600)
}

func seed(ctx context.Context, dataStore *store.Store, ownerID, projectID string) error {
	var exists bool
	if err := dataStore.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1 AND owner_user_id=$2)
	`, projectID, ownerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("demo project %s does not belong to %s", projectID, ownerID)
	}
	if err := seedGuideDocument(ctx, dataStore, projectID); err != nil {
		return err
	}
	if err := seedMedia(ctx, dataStore, ownerID, projectID); err != nil {
		return err
	}
	if err := seedSchema(ctx, dataStore, ownerID, projectID); err != nil {
		return err
	}
	if err := putCanvas(ctx, dataStore, ownerID, projectID, showcaseCanvasID, "Canvas 0.5/0.6 — лаборатория", 3000, showcaseDocument()); err != nil {
		return err
	}
	if err := enrichMainCanvas(ctx, dataStore, ownerID); err != nil {
		return err
	}
	_, err := dataStore.Pool.Exec(ctx, `
		UPDATE projects SET synopsis=$2, updated_at=now() WHERE id=$1
	`, projectID, "Демонстрационный игровой концепт: документы, whiteboard, типизированный GDD, play-through, анализ графа и переносимые media cards.")
	return err
}

func seedGuideDocument(ctx context.Context, dataStore *store.Store, projectID string) error {
	content := map[string]any{
		"type": "doc", "attrs": map[string]any{"schemaVersion": 3},
		"content": []any{
			block("heading", "Как устроен Canvas-демо проект", 2, "20000000-0000-4000-8000-000000000001"),
			block("paragraph", "«Эхо Предела» — компактный, но реалистичный game-design пример. Начните с доски Vertical Slice, затем откройте вложенную лабораторию Canvas 0.5/0.6.", 0, "20000000-0000-4000-8000-000000000002"),
			block("heading", "Что здесь можно попробовать", 2, "20000000-0000-4000-8000-000000000003"),
			block("paragraph", "Перо и маркер, фигуры и frames, соединители, группы, направляющие, раскладки, поиск, presentation mode, ссылки, PDF, аудио, видео и вложенные доски.", 0, "20000000-0000-4000-8000-000000000004"),
			block("paragraph", "Типизированные карточки охватывают идеи, персонажей, локации, биты, механики, квесты, диалоги, состояния, уровни и ресурсы. Переключайте Timeline, Swimlanes и Dependency graph, запускайте диагностику и ручной play-through.", 0, "20000000-0000-4000-8000-000000000005"),
			block("heading", "Проверка переноса", 2, "20000000-0000-4000-8000-000000000006"),
			block("paragraph", "Экспортируйте проект в .wstudio и импортируйте как независимую копию. Документы, все ревизии досок, типы, изображения, PDF, audio/video и ссылки на вложенные Canvas должны сохраниться с переназначенными ID.", 0, "20000000-0000-4000-8000-000000000007"),
		},
	}
	raw, _ := json.Marshal(content)
	plain := "Как устроен Canvas-демо проект\nЭхо Предела — реалистичный game-design пример. Whiteboard, типизированный GDD, play-through, анализ графа, PDF, audio/video и перенос .wstudio."
	return pgx.BeginFunc(ctx, dataStore.Pool, func(tx pgx.Tx) error {
		var current int
		err := tx.QueryRow(ctx, `SELECT current_revision FROM document_nodes WHERE id=$1`, guideDocumentID).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `
				INSERT INTO document_nodes(id, project_id, kind, title, status, position, current_revision)
				VALUES($1,$2,'note','Как устроен Canvas-демо проект','ready',9000,1)
			`, guideDocumentID, projectID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO document_revisions(id,node_id,revision,base_revision,schema_version,content,plain_text,source)
				VALUES($1,$2,1,0,3,$3,$4,'demo_seed')
			`, uuid.NewString(), guideDocumentID, raw, plain)
			return err
		}
		if err != nil {
			return err
		}
		var stored []byte
		if err := tx.QueryRow(ctx, `SELECT content FROM document_revisions WHERE node_id=$1 AND revision=$2`, guideDocumentID, current).Scan(&stored); err != nil {
			return err
		}
		if jsonEqual(stored, raw) {
			return nil
		}
		next := current + 1
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_revisions(id,node_id,revision,base_revision,schema_version,content,plain_text,source)
			VALUES($1,$2,$3,$4,3,$5,$6,'demo_seed')
		`, uuid.NewString(), guideDocumentID, next, current, raw, plain); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE document_nodes SET current_revision=$2,status='ready',updated_at=now() WHERE id=$1`, guideDocumentID, next)
		return err
	})
}

func block(kind, text string, level int, blockID string) map[string]any {
	attrs := map[string]any{"blockId": blockID, "sourceRefs": []any{}}
	if kind == "heading" {
		attrs["level"] = level
	}
	return map[string]any{"type": kind, "attrs": attrs, "content": []any{map[string]any{"type": "text", "text": text}}}
}

func seedMedia(ctx context.Context, dataStore *store.Store, ownerID, projectID string) error {
	items := []struct {
		id, key, mediaType, name, hash string
		size, duration                 int64
		pages                          any
	}{
		{pdfMediaID, "media/demo/echo-frontier/playtest.pdf", "application/pdf", "echo-frontier-playtest.pdf", "6603d39e2c58579df6e9ab5428e52ee87d003053acbee46a1b68092990a3263c", 76424, 0, 1},
		{audioMediaID, "media/demo/echo-frontier/signal.mp3", "audio/mpeg", "echo-frontier-signal.mp3", "137de0dc87c3d886ce6eeeb8d5f8f76e8e596e85ece8c34aad693a19b55e40df", 48946, 4000, nil},
		{videoMediaID, "media/demo/echo-frontier/field-log.mp4", "video/mp4", "echo-frontier-field-log.mp4", "29d37a534fec7fadd925d5e8cd63a0c7e408fd4fb27093f3e69f2bf39f5f1127", 40072, 4000, nil},
	}
	for _, item := range items {
		var duration any
		if item.duration > 0 {
			duration = item.duration
		}
		_, err := dataStore.Pool.Exec(ctx, `
			INSERT INTO project_media(id,project_id,owner_user_id,storage_key,media_type,original_name,sha256,byte_size,width,height,page_count,duration_ms)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULL,NULL,$9,$10)
			ON CONFLICT(id) DO UPDATE SET storage_key=EXCLUDED.storage_key,media_type=EXCLUDED.media_type,
				original_name=EXCLUDED.original_name,sha256=EXCLUDED.sha256,byte_size=EXCLUDED.byte_size,
				page_count=EXCLUDED.page_count,duration_ms=EXCLUDED.duration_ms
		`, item.id, projectID, ownerID, item.key, item.mediaType, item.name, item.hash, item.size, item.pages, duration)
		if err != nil {
			return err
		}
	}
	return nil
}

func seedSchema(ctx context.Context, dataStore *store.Store, ownerID, projectID string) error {
	schema := demoSchema()
	current, err := dataStore.GetCanvasTypeSchema(ctx, ownerID, projectID)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.CardTypes, schema.CardTypes) && reflect.DeepEqual(current.RelationTypes, schema.RelationTypes) {
		return nil
	}
	_, err = dataStore.SaveCanvasTypeSchema(ctx, ownerID, projectID, current.Revision, schema, "demo_seed")
	return err
}

func demoSchema() canvasdocument.TypeSchema {
	summary := canvasdocument.FieldDefinition{ID: "summary", Name: "Кратко", Type: "markdown"}
	card := func(id, name, category, color, icon string, fields ...canvasdocument.FieldDefinition) canvasdocument.CardType {
		return canvasdocument.CardType{ID: id, Name: name, Category: category, Color: color, Icon: icon, Fields: fields}
	}
	relation := func(id, name, color string, directed bool) canvasdocument.RelationType {
		return canvasdocument.RelationType{ID: id, Name: name, Color: color, Directed: directed}
	}
	return canvasdocument.TypeSchema{
		CardTypes: []canvasdocument.CardType{
			card("idea", "Идея", "general", "3", "Lightbulb", summary),
			card("question", "Вопрос", "general", "2", "CircleHelp", summary),
			card("source", "Источник", "general", "5", "Library", summary, canvasdocument.FieldDefinition{ID: "url", Name: "URL", Type: "short-text"}),
			card("character", "Персонаж", "narrative", "6", "UserRound", summary, canvasdocument.FieldDefinition{ID: "goal", Name: "Цель", Type: "markdown"}, canvasdocument.FieldDefinition{ID: "conflict", Name: "Конфликт", Type: "markdown"}),
			card("location", "Локация", "narrative", "4", "MapPin", summary),
			card("beat", "Бит / событие", "narrative", "3", "Clapperboard", summary, canvasdocument.FieldDefinition{ID: "date", Name: "Дата", Type: "date"}, canvasdocument.FieldDefinition{ID: "duration", Name: "Длительность", Type: "duration"}),
			card("conflict", "Конфликт", "narrative", "1", "Swords", summary),
			card("theme", "Тема", "narrative", "5", "Sparkles", summary),
			card("mechanic", "Механика", "game-design", "5", "Gamepad2", summary),
			card("quest", "Квест", "game-design", "3", "ScrollText", summary, canvasdocument.FieldDefinition{ID: "prerequisites", Name: "Предусловия", Type: "multi-select"}),
			card("dialogue", "Диалог", "game-design", "6", "MessagesSquare", canvasdocument.FieldDefinition{ID: "line", Name: "Реплика", Type: "markdown", Required: true}, canvasdocument.FieldDefinition{ID: "terminal", Name: "Завершает диалог", Type: "checkbox"}),
			card("state", "Состояние", "game-design", "2", "Workflow", summary, canvasdocument.FieldDefinition{ID: "initial", Name: "Начальное", Type: "checkbox"}, canvasdocument.FieldDefinition{ID: "terminal", Name: "Конечное", Type: "checkbox"}),
			card("level", "Уровень", "game-design", "4", "Map", summary),
			card("resource", "Ресурс", "game-design", "2", "Gem", summary, canvasdocument.FieldDefinition{ID: "initialAmount", Name: "Стартовое количество", Type: "number"}),
			card("playtest-hypothesis", "Гипотеза плейтеста", "custom", "1", "FlaskConical", summary, canvasdocument.FieldDefinition{ID: "owner", Name: "Ответственный", Type: "short-text"}, canvasdocument.FieldDefinition{ID: "confidence", Name: "Уверенность", Type: "single-select", Options: []string{"low", "medium", "high"}}),
			card("evidence", "Связанное доказательство", "custom", "6", "Paperclip", summary,
				canvasdocument.FieldDefinition{ID: "documentRef", Name: "Документ", Type: "document"},
				canvasdocument.FieldDefinition{ID: "mediaRef", Name: "Медиа", Type: "media"},
				canvasdocument.FieldDefinition{ID: "canvasRef", Name: "Доска", Type: "canvas"}),
		},
		RelationTypes: []canvasdocument.RelationType{
			relation("relates", "Связано с", "5", false), relation("causes", "Вызывает", "3", true),
			relation("contradicts", "Противоречит", "1", false), relation("depends_on", "Зависит от", "2", true),
			relation("blocks", "Блокирует", "1", true), relation("reveals", "Раскрывает", "6", true),
			relation("transitions_to", "Переходит в", "5", true), relation("contains", "Содержит", "4", true),
			relation("consumes", "Потребляет", "2", true), relation("produces", "Производит", "4", true),
			relation("rewards", "Награждает", "3", true), relation("validates", "Проверяет", "4", true),
		},
	}
}

func putCanvas(ctx context.Context, dataStore *store.Store, ownerID, projectID, canvasID, title string, position float64, document canvasDoc) error {
	raw, _ := json.Marshal(document)
	references, err := canvasdocument.Validate(raw)
	if err != nil {
		return fmt.Errorf("validate %s: %w", title, err)
	}
	if err := dataStore.ValidateCanvasResources(ctx, ownerID, projectID, references); err != nil {
		return fmt.Errorf("validate %s resources: %w", title, err)
	}
	var current int
	var stored []byte
	err = dataStore.Pool.QueryRow(ctx, `
		SELECT c.current_revision,r.content FROM canvases c
		JOIN canvas_revisions r ON r.canvas_id=c.id AND r.revision=c.current_revision
		WHERE c.id=$1 AND c.project_id=$2
	`, canvasID, projectID).Scan(&current, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.BeginFunc(ctx, dataStore.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvases(id,project_id,title,position,current_revision) VALUES($1,$2,$3,$4,1)
			`, canvasID, projectID, title, position); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO canvas_revisions(id,canvas_id,revision,base_revision,schema_version,content,source)
				VALUES($1,$2,1,0,2,$3,'demo_seed')
			`, uuid.NewString(), canvasID, raw)
			return err
		})
	}
	if err != nil {
		return err
	}
	if jsonEqual(stored, raw) {
		_, err = dataStore.Pool.Exec(ctx, `UPDATE canvases SET title=$2,position=$3,trashed_at=NULL WHERE id=$1`, canvasID, title, position)
		return err
	}
	_, err = dataStore.SaveCanvasRevision(ctx, ownerID, canvasID, store.SaveCanvasRevisionInput{
		BaseRevision: current, SchemaVersion: 2, Content: raw, Source: "demo_seed", References: references,
	})
	return err
}

func enrichMainCanvas(ctx context.Context, dataStore *store.Store, ownerID string) error {
	workspace, err := dataStore.GetCanvasWorkspace(ctx, ownerID, mainCanvasID)
	if err != nil {
		return err
	}
	var document canvasDoc
	if err := json.Unmarshal(workspace.Revision.Content, &document); err != nil {
		return err
	}
	for _, item := range document.Nodes {
		if item["id"] == "frame-showcase" {
			return nil
		}
	}
	document.Nodes = append(document.Nodes,
		frame("frame-showcase", 1520, 940, 700, 600, "Попробовать всё", 5),
		fileNode("showcase-card", 1560, 1020, 300, 190, "canvas", showcaseCanvasID, "5", "Лаборатория всех возможностей", "source"),
		fileNode("guide-card", 1880, 1020, 300, 190, "document", guideDocumentID, "4", "Как устроен демо-проект", "source"),
		typedText("assistant-note", 1560, 1260, 620, 190, "# Ассистент на доске\n\nВыделите карточки и откройте Ассистента: анализ, предложение связей и раскладки попадут в review как ghost overlay до Apply.", "idea", "3", map[string]any{"summary": "Canvas target + безопасный preview"}),
	)
	document.Edges = append(document.Edges,
		edge("e-showcase-guide", "showcase-card", "guide-card", "reveals", "что попробовать", "6", "right", "left"),
		edge("e-guide-assistant", "guide-card", "assistant-note", "contains", "следующий шаг", "4", "bottom", "right"),
	)
	raw, _ := json.Marshal(document)
	references, err := canvasdocument.Validate(raw)
	if err != nil {
		return err
	}
	if err := dataStore.ValidateCanvasResources(ctx, ownerID, workspace.Canvas.ProjectID, references); err != nil {
		return err
	}
	_, err = dataStore.SaveCanvasRevision(ctx, ownerID, mainCanvasID, store.SaveCanvasRevisionInput{
		BaseRevision: workspace.Canvas.CurrentRevision, SchemaVersion: 2, Content: raw, Source: "demo_seed", References: references,
	})
	return err
}

func showcaseDocument() canvasDoc {
	d := canvasDoc{Extension: map[string]any{
		"schemaVersion": 2, "activeView": "free",
		"demo":   map[string]any{"kind": "canvas-feature-showcase", "title": "Эхо Предела — лаборатория"},
		"future": map[string]any{"opaqueExtensionPreserved": true, "drawingLayer": "svg"},
	}}
	d.Nodes = append(d.Nodes,
		frame("frame-whiteboard", 0, 0, 1360, 900, "1 · Whiteboard и быстрый захват", 1),
		frame("frame-media", 1420, 0, 1360, 900, "2 · Research и media", 2),
		frame("frame-types", 0, 960, 1360, 1040, "3 · Типизированный нарратив и GDD", 3),
		frame("frame-play", 1420, 960, 1360, 1040, "4 · Graph analysis и play-through", 4),
	)
	d.Nodes = append(d.Nodes,
		typedText("brainstorm", 50, 80, 300, 190, "# Brain dump\n\nКак превратить звук одновременно в карту, риск и рассказ?", "idea", "3", map[string]any{"summary": "Центральная идея vertical slice"}),
		typedText("question", 380, 80, 300, 190, "# Вопрос\n\nКогда игрок впервые понимает, что громкое эхо опасно?", "question", "2", map[string]any{"summary": "Проверить без туториального текста"}),
		group("affinity", 720, 50, 570, 260, "Affinity cluster · обратная связь", "6"),
		typedText("affinity-a", 750, 100, 240, 160, "# Наблюдение\n\nИгрок ждёт анимацию карты", "playtest-hypothesis", "1", map[string]any{"summary": "Видимость ответа", "owner": "UX", "confidence": "medium"}),
		typedText("affinity-b", 1020, 100, 240, 160, "# Наблюдение\n\nЗвук читается раньше UI", "playtest-hypothesis", "4", map[string]any{"summary": "Аудио как сигнал", "owner": "Audio", "confidence": "high"}),
		shape("shape-rect", 70, 370, 190, 100, "rectangle", "EXPLORE", "4", "transparent", "dashed", 0),
		shape("shape-ellipse", 330, 370, 190, 100, "ellipse", "LISTEN", "5", "transparent", "solid", 0),
		shape("shape-diamond", 590, 350, 190, 140, "diamond", "RISK?", "2", "transparent", "dotted", 0),
		shape("shape-line", 850, 390, 180, 60, "line", "route", "6", "transparent", "dashed", -4),
		shape("shape-arrow", 1080, 370, 190, 100, "arrow", "RETURN", "3", "transparent", "solid", 0),
		stroke("pen-stroke", 70, 570, 550, 120, "pen", "5", 5, .95, [][]float64{{8, 68, .2}, {70, 30, .5}, {145, 82, .8}, {220, 25, .4}, {305, 80, .75}, {395, 30, .5}, {540, 65, .25}}),
		stroke("marker-stroke", 690, 580, 570, 70, "highlighter", "3", 24, .28, [][]float64{{8, 35}, {130, 29}, {250, 38}, {380, 27}, {555, 34}}),
		lockedShape("rotated-shape", 1040, 700, 180, 100, "rectangle", "LOCKED + HEX", "#c26b3a", "#17324d", "dashed", 8),
	)
	d.Edges = append(d.Edges,
		edge("wb-1", "brainstorm", "question", "causes", "порождает", "3", "right", "left"),
		edge("wb-2", "shape-rect", "shape-ellipse", "transitions_to", "", "4", "right", "left"),
		edge("wb-3", "shape-ellipse", "shape-diamond", "transitions_to", "", "5", "right", "left"),
		edge("wb-4", "shape-diamond", "shape-arrow", "transitions_to", "решение", "3", "right", "left"),
	)
	d.Nodes = append(d.Nodes,
		fileNode("media-image", 1470, 80, 390, 250, "media", imageMediaID, "6", "Концепт-арт острова", "source"),
		fileNode("media-pdf", 1900, 80, 360, 250, "media", pdfMediaID, "5", "PDF · протокол плейтеста", "source"),
		fileNode("media-audio", 2300, 80, 410, 170, "media", audioMediaID, "2", "Audio · сигнал резонатора", "source"),
		fileNode("media-video", 1470, 390, 500, 300, "media", videoMediaID, "4", "Video · полевой лог", "source"),
		linkNode("research-link", 2020, 390, 330, 180, "https://jsoncanvas.org/", "JSON Canvas 1.0", "Открытый переносимый формат досок.", "5"),
		fileNode("nested-moodboard", 2390, 390, 300, 180, "canvas", moodboardID, "6", "Вложенная доска · moodboard", "source"),
		fileNode("demo-guide", 2020, 620, 330, 190, "document", guideDocumentID, "4", "Документ · гид по примеру", "source"),
	)
	d.Edges = append(d.Edges,
		edge("media-1", "media-pdf", "demo-guide", "reveals", "методика", "5", "bottom", "top"),
		edge("media-2", "media-image", "media-video", "relates", "визуальный язык", "6", "bottom", "left"),
		edge("media-3", "research-link", "nested-moodboard", "contains", "референсы", "4", "right", "left"),
	)
	types := []struct {
		id, title, kind, color string
		fields                 map[string]any
	}{
		{"type-idea", "Идея · остров слушает игрока", "idea", "3", map[string]any{"summary": "Фантазия исследования"}},
		{"type-question", "Вопрос · цена импульса?", "question", "2", map[string]any{"summary": "Проверить на плейтесте"}},
		{"type-source", "Источник · акустика пещер", "source", "5", map[string]any{"summary": "Research", "url": "https://example.com/acoustics"}},
		{"type-character", "Персонаж · картограф Лея", "character", "6", map[string]any{"summary": "Главная героиня", "goal": "Вернуть карту", "conflict": "Не доверяет собственному слуху"}},
		{"type-location", "Локация · немой маяк", "location", "4", map[string]any{"summary": "Хаб и финальная цель"}},
		{"type-beat", "Beat · первый двойной отклик", "beat", "3", map[string]any{"summary": "Необратимое открытие", "date": "2027-01-12", "duration": 180}},
		{"type-conflict", "Конфликт · карта против памяти", "conflict", "1", map[string]any{"summary": "Внешняя и внутренняя правда расходятся"}},
		{"type-theme", "Тема · доверие восприятию", "theme", "5", map[string]any{"summary": "Слушать важнее, чем видеть"}},
		{"type-mechanic", "Механика · направленное эхо", "mechanic", "5", map[string]any{"summary": "Раскрывает геометрию и создаёт угрозу"}},
		{"type-quest", "Квест · Первый резонанс", "quest", "3", map[string]any{"summary": "Обучающая экспедиция", "prerequisites": []string{"echo-tutorial"}}},
		{"type-dialogue", "Диалог · Не доверяй контуру", "dialogue", "6", map[string]any{"line": "Не доверяй контуру, если он звучит дважды", "terminal": false}},
		{"type-state", "State · настороженность", "state", "2", map[string]any{"summary": "AI услышал импульс", "initial": true, "terminal": false}},
		{"type-level", "Level · затопленные арки", "level", "4", map[string]any{"summary": "Два маршрута и общий финал"}},
		{"type-resource", "Resource · заряд резонатора", "resource", "2", map[string]any{"summary": "Тратится на направленное эхо", "initialAmount": 2}},
		{"type-custom", "Custom · гипотеза плейтеста", "playtest-hypothesis", "1", map[string]any{"summary": "Игрок сам объясняет цену эха", "owner": "Design", "confidence": "medium"}},
		{"type-evidence", "Refs · документ + media + Canvas", "evidence", "6", map[string]any{"summary": "Все типы ссылочных полей", "documentRef": guideDocumentID, "mediaRef": pdfMediaID, "canvasRef": moodboardID}},
	}
	for index, item := range types {
		x := 50 + (index%4)*325
		y := 1040 + (index/4)*220
		node := typedText(item.id, x, y, 285, 175, "# "+item.title, item.kind, item.color, item.fields)
		if item.id == "type-evidence" {
			extension := node["x-writers-studio"].(map[string]any)
			card := extension["card"].(map[string]any)
			card["promotedDocumentId"] = guideDocumentID
		}
		d.Nodes = append(d.Nodes, node)
	}
	d.Edges = append(d.Edges,
		edge("type-e1", "type-character", "type-quest", "reveals", "мотивация", "6", "bottom", "top"),
		edge("type-e2", "type-mechanic", "type-resource", "consumes", "заряд", "2", "bottom", "top"),
		edge("type-e3", "type-beat", "type-conflict", "causes", "обостряет", "3", "right", "left"),
		edge("type-e4", "type-custom", "type-mechanic", "validates", "проверяет", "4", "top", "bottom"),
		edge("type-e5", "type-theme", "type-conflict", "contradicts", "напряжение", "1", "right", "left"),
		edge("type-e6", "type-question", "type-quest", "blocks", "до ответа", "1", "bottom", "top"),
		edge("type-e7", "type-mechanic", "type-resource", "produces", "в убежище", "4", "right", "left"),
		edge("type-e8", "type-quest", "type-resource", "rewards", "за экспедицию", "3", "bottom", "top"),
		edge("type-e9", "type-evidence", "type-source", "relates", "подтверждает", "5", "top", "bottom"),
	)
	d.Nodes = append(d.Nodes,
		typedText("quest-start", 1470, 1040, 300, 170, "# Квест · Найти маяк", "quest", "3", map[string]any{"summary": "Вход в цепочку", "prerequisites": []string{}}),
		typedText("quest-tune", 1840, 1040, 300, 170, "# Квест · Настроить резонатор", "quest", "3", map[string]any{"summary": "Требует образец", "prerequisites": []string{"quest-start"}}),
		typedText("quest-return", 2210, 1040, 300, 170, "# Квест · Вернуться в убежище", "quest", "4", map[string]any{"summary": "Награда: новая карта", "prerequisites": []string{"quest-tune"}}),
		typedText("dialogue-entry", 1470, 1300, 300, 170, "# Лея\n\nКонтур звучит дважды.", "dialogue", "6", map[string]any{"line": "Контур звучит дважды", "terminal": false}),
		typedText("dialogue-trust", 1840, 1260, 300, 170, "# Довериться карте\n\nИдём по старому следу.", "dialogue", "4", map[string]any{"line": "Идём по старому следу", "terminal": true}),
		typedText("dialogue-listen", 1840, 1460, 300, 170, "# Довериться слуху\n\nПроверим новый отклик.", "dialogue", "3", map[string]any{"line": "Проверим новый отклик", "terminal": true}),
		typedText("state-silent", 1470, 1740, 270, 160, "# Silent", "state", "5", map[string]any{"summary": "Угроза спокойна", "initial": true, "terminal": false}),
		typedText("state-alert", 1810, 1740, 270, 160, "# Alert", "state", "2", map[string]any{"summary": "Слышит импульс", "initial": false, "terminal": false}),
		typedText("state-hunt", 2150, 1740, 270, 160, "# Hunt", "state", "1", map[string]any{"summary": "Преследует", "initial": false, "terminal": false}),
		typedText("state-safe", 2490, 1740, 240, 160, "# Safe", "state", "4", map[string]any{"summary": "Игрок в убежище", "initial": false, "terminal": true}),
	)
	d.Edges = append(d.Edges,
		edge("quest-e1", "quest-start", "quest-tune", "depends_on", "образец", "2", "right", "left"),
		edge("quest-e2", "quest-tune", "quest-return", "depends_on", "заряд", "2", "right", "left"),
		edge("dialogue-e1", "dialogue-entry", "dialogue-trust", "transitions_to", "выбрать карту", "5", "right", "left"),
		edge("dialogue-e2", "dialogue-entry", "dialogue-listen", "transitions_to", "проверить слух", "5", "right", "left"),
		edge("state-e1", "state-silent", "state-alert", "transitions_to", "шум", "5", "right", "left"),
		edge("state-e2", "state-alert", "state-hunt", "transitions_to", "цель видна", "5", "right", "left"),
		edge("state-e3", "state-hunt", "state-safe", "transitions_to", "убежище", "5", "right", "left"),
	)
	return d
}

func frame(id string, x, y, width, height int, label string, order int) map[string]any {
	return map[string]any{"id": id, "type": "group", "x": x, "y": y, "width": width, "height": height, "label": label,
		"x-writers-studio": map[string]any{"element": map[string]any{"kind": "frame", "order": order, "label": label}}}
}

func group(id string, x, y, width, height int, label, color string) map[string]any {
	return map[string]any{"id": id, "type": "group", "x": x, "y": y, "width": width, "height": height, "label": label, "color": color}
}

func typedText(id string, x, y, width, height int, text, typeID, color string, fields map[string]any) map[string]any {
	return map[string]any{"id": id, "type": "text", "x": x, "y": y, "width": width, "height": height, "text": text, "color": color,
		"x-writers-studio": map[string]any{"card": map[string]any{"typeId": typeID, "fields": fields}}}
}

func fileNode(id string, x, y, width, height int, kind, resourceID, color, summary, typeID string) map[string]any {
	pathKind := map[string]string{"document": "documents", "media": "media", "canvas": "canvases"}[kind]
	return map[string]any{"id": id, "type": "file", "x": x, "y": y, "width": width, "height": height, "color": color,
		"file": fmt.Sprintf("writers-studio://%s/%s", pathKind, resourceID),
		"x-writers-studio": map[string]any{
			"resource":    map[string]any{"kind": kind, "id": resourceID},
			"card":        map[string]any{"typeId": typeID, "fields": map[string]any{"summary": summary}},
			"nestedTitle": summary,
		}}
}

func linkNode(id string, x, y, width, height int, url, title, description, color string) map[string]any {
	return map[string]any{"id": id, "type": "link", "x": x, "y": y, "width": width, "height": height, "url": url, "color": color,
		"x-writers-studio": map[string]any{"card": map[string]any{"typeId": "source", "fields": map[string]any{"url": url}},
			"preview": map[string]any{"title": title, "description": description, "siteName": "jsoncanvas.org", "fetchedAt": "2026-08-06T00:00:00Z"}}}
}

func shape(id string, x, y, width, height int, kind, label, color, fill, dash string, rotation int) map[string]any {
	return map[string]any{"id": id, "type": "group", "x": x, "y": y, "width": width, "height": height,
		"x-writers-studio": map[string]any{"element": map[string]any{"kind": "shape", "shape": kind, "label": label,
			"color": color, "fill": fill, "dash": dash, "size": 3, "opacity": .9, "rotation": rotation}}}
}

func lockedShape(id string, x, y, width, height int, kind, label, color, fill, dash string, rotation int) map[string]any {
	result := shape(id, x, y, width, height, kind, label, color, fill, dash, rotation)
	extension := result["x-writers-studio"].(map[string]any)
	extension["locked"] = true
	return result
}

func stroke(id string, x, y, width, height int, brush, color string, size, opacity float64, points [][]float64) map[string]any {
	return map[string]any{"id": id, "type": "group", "x": x, "y": y, "width": width, "height": height,
		"x-writers-studio": map[string]any{"element": map[string]any{"kind": "stroke", "brush": brush, "color": color,
			"size": size, "opacity": opacity, "points": points}}}
}

func edge(id, from, to, relation, label, color, fromSide, toSide string) map[string]any {
	return map[string]any{"id": id, "fromNode": from, "fromSide": fromSide, "fromEnd": "none", "toNode": to, "toSide": toSide,
		"toEnd": "arrow", "label": label, "color": color,
		"x-writers-studio": map[string]any{"relation": map[string]any{"typeId": relation}}}
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
