package domain

import (
	"encoding/json"
	"time"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Project struct {
	ID                string           `json:"id"`
	Title             string           `json:"title"`
	Kind              string           `json:"kind"`
	Language          string           `json:"language"`
	Synopsis          string           `json:"synopsis"`
	WordGoal          *int             `json:"wordGoal,omitempty"`
	Status            string           `json:"status"`
	StructureRevision int              `json:"structureRevision"`
	Structure         ProjectStructure `json:"structure"`
	TrashedAt         *time.Time       `json:"trashedAt,omitempty"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

type ProjectStructure struct {
	SchemaVersion int               `json:"schemaVersion"`
	Template      string            `json:"template"`
	RootLabel     string            `json:"rootLabel"`
	NodeTypes     []ProjectNodeType `json:"nodeTypes"`
}

type ProjectNodeType struct {
	Key             string   `json:"key"`
	Label           string   `json:"label"`
	PluralLabel     string   `json:"pluralLabel"`
	Icon            string   `json:"icon"`
	ContentBearing  bool     `json:"contentBearing"`
	AllowedChildren []string `json:"allowedChildren"`
	CanBeRoot       bool     `json:"canBeRoot"`
}

type DocumentNode struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"projectId"`
	ParentID          *string         `json:"parentId,omitempty"`
	Kind              string          `json:"kind"`
	Title             string          `json:"title"`
	Status            string          `json:"status"`
	Position          float64         `json:"position"`
	CurrentRevision   int             `json:"currentRevision"`
	Content           json.RawMessage `json:"content,omitempty"`
	PlainText         string          `json:"plainText,omitempty"`
	SourceFrontmatter string          `json:"-"`
	TrashedAt         *time.Time      `json:"trashedAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type DocumentRevision struct {
	ID            string          `json:"id"`
	NodeID        string          `json:"nodeId"`
	Revision      int             `json:"revision"`
	BaseRevision  int             `json:"baseRevision"`
	SchemaVersion int             `json:"schemaVersion"`
	Content       json.RawMessage `json:"content"`
	PlainText     string          `json:"plainText"`
	Source        string          `json:"source"`
	OperationID   *string         `json:"operationId,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type ProjectMedia struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"projectId"`
	MediaType    string    `json:"mediaType"`
	OriginalName string    `json:"originalName"`
	SourcePath   *string   `json:"sourcePath,omitempty"`
	SHA256       string    `json:"sha256"`
	ByteSize     int64     `json:"byteSize"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	ContentURL   string    `json:"contentUrl"`
	StorageKey   string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ExportMetadata struct {
	Title        string  `json:"title"`
	Subtitle     string  `json:"subtitle"`
	Author       string  `json:"author"`
	Annotation   string  `json:"annotation"`
	CoverAssetID *string `json:"coverAssetId,omitempty"`
}

type ExportStyle struct {
	Preset          string  `json:"preset"`
	FontFamily      string  `json:"fontFamily"`
	FontSizePT      int     `json:"fontSizePt"`
	LineHeight      float64 `json:"lineHeight"`
	ParagraphMode   string  `json:"paragraphMode"`
	FirstLineIndent float64 `json:"firstLineIndentEm"`
	ParagraphSpace  float64 `json:"paragraphSpaceEm"`
	TextAlign       string  `json:"textAlign"`
	PageSize        string  `json:"pageSize"`
	MarginMM        int     `json:"marginMm"`
	SceneSeparator  string  `json:"sceneSeparator"`
}

type ExportProfile struct {
	SchemaVersion    int            `json:"schemaVersion"`
	Metadata         ExportMetadata `json:"metadata"`
	Style            ExportStyle    `json:"style"`
	TitlePage        bool           `json:"titlePage"`
	TableOfContents  bool           `json:"tableOfContents"`
	ChapterTitles    bool           `json:"chapterTitles"`
	ChapterPageBreak bool           `json:"chapterPageBreak"`
	IncludeNotes     bool           `json:"includeNotes"`
	LastFormat       string         `json:"lastFormat"`
}

type NamedVersion struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	Revision  int       `json:"revision"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type DictionaryTerm struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"projectId"`
	Canonical      string    `json:"canonical"`
	Variants       []string  `json:"variants"`
	Kind           string    `json:"kind"`
	Language       string    `json:"language"`
	Pronunciation  string    `json:"pronunciation"`
	Active         bool      `json:"active"`
	Revision       int       `json:"revision"`
	Source         string    `json:"source"`
	OperationID    *string   `json:"operationId,omitempty"`
	AssistantRunID *string   `json:"assistantRunId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AnnotationAnchor struct {
	BlockID   string `json:"blockId"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	ExactText string `json:"exactText"`
	Prefix    string `json:"prefix"`
	Suffix    string `json:"suffix"`
}

type AnnotationTerm struct {
	ID        *string `json:"id,omitempty"`
	Canonical string  `json:"canonical"`
	Kind      string  `json:"kind"`
	Language  string  `json:"language"`
	Active    bool    `json:"active"`
}

type DocumentAnnotation struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"projectId"`
	NodeID           string           `json:"nodeId"`
	NodeTitle        string           `json:"nodeTitle"`
	Kind             string           `json:"kind"`
	Status           string           `json:"status"`
	AnchorStatus     string           `json:"anchorStatus"`
	DictionaryTermID *string          `json:"dictionaryTermId,omitempty"`
	DictionaryTerm   *AnnotationTerm  `json:"dictionaryTerm,omitempty"`
	Anchor           AnnotationAnchor `json:"anchor"`
	Note             string           `json:"note,omitempty"`
	BaseRevision     int              `json:"baseRevision"`
	Revision         int              `json:"revision"`
	ResolvedAt       *time.Time       `json:"resolvedAt,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}

type DocumentAnnotationHistory struct {
	ID                   string             `json:"id"`
	AnnotationID         string             `json:"annotationId"`
	AnnotationRevision   int                `json:"annotationRevision"`
	EventKind            string             `json:"eventKind"`
	Before               DocumentAnnotation `json:"before"`
	After                DocumentAnnotation `json:"after"`
	Source               string             `json:"source"`
	OperationID          string             `json:"operationId"`
	ChangeOperationID    *string            `json:"changeOperationId,omitempty"`
	AssistantRunID       *string            `json:"assistantRunId,omitempty"`
	AssistantChangeSetID *string            `json:"assistantChangeSetId,omitempty"`
	DocumentRevision     int                `json:"documentRevision"`
	CreatedAt            time.Time          `json:"createdAt"`
}

type TranscriptWord struct {
	Text       string   `json:"text"`
	StartMS    int64    `json:"startMs"`
	EndMS      int64    `json:"endMs"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type TranscriptSegment struct {
	ID         string           `json:"id"`
	Text       string           `json:"text"`
	StartMS    int64            `json:"startMs"`
	EndMS      int64            `json:"endMs"`
	Confidence *float64         `json:"confidence,omitempty"`
	Words      []TranscriptWord `json:"words,omitempty"`
	Command    *CommandEvent    `json:"command,omitempty"`
}

type CommandEvent struct {
	Kind     string `json:"kind"`
	RawText  string `json:"rawText"`
	Executed bool   `json:"executed"`
	Target   string `json:"target,omitempty"`
	Ordinal  int    `json:"ordinal,omitempty"`
}

type DictationSession struct {
	ID                    string                    `json:"id"`
	ProjectID             string                    `json:"projectId"`
	DestinationNodeID     *string                   `json:"destinationNodeId,omitempty"`
	ClientSessionID       string                    `json:"clientSessionId"`
	Language              string                    `json:"language"`
	Status                string                    `json:"status"`
	DurationMS            int64                     `json:"durationMs"`
	RecordedDurationMS    int64                     `json:"recordedDurationMs"`
	Manifest              json.RawMessage           `json:"manifest"`
	LiveArtifact          *TranscriptArtifact       `json:"liveArtifact,omitempty"`
	BatchArtifact         *TranscriptArtifact       `json:"batchArtifact,omitempty"`
	SuggestionArtifact    *TranscriptArtifact       `json:"suggestionArtifact,omitempty"`
	RetranscriptionStatus string                    `json:"retranscriptionStatus,omitempty"`
	Draft                 *TranscriptDraft          `json:"draft,omitempty"`
	DraftRevisions        []TranscriptDraftRevision `json:"draftRevisions,omitempty"`
	CreatedAt             time.Time                 `json:"createdAt"`
	UpdatedAt             time.Time                 `json:"updatedAt"`
}

type TranscriptArtifact struct {
	ID              string              `json:"id"`
	SessionID       string              `json:"sessionId"`
	Kind            string              `json:"kind"`
	PipelineVersion string              `json:"pipelineVersion"`
	Provider        string              `json:"provider"`
	Model           string              `json:"model"`
	Language        string              `json:"language"`
	Text            string              `json:"text"`
	Segments        []TranscriptSegment `json:"segments"`
	AudioSHA256     *string             `json:"audioSha256,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
}

type TranscriptDraft struct {
	ID               string          `json:"id"`
	SessionID        string          `json:"sessionId"`
	SourceArtifactID *string         `json:"sourceArtifactId,omitempty"`
	Revision         int             `json:"revision"`
	BaseSourceText   string          `json:"baseSourceText"`
	Content          json.RawMessage `json:"content"`
	PlainText        string          `json:"plainText"`
	ManuallyEdited   bool            `json:"manuallyEdited"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	Source           string          `json:"-"`
}

type TranscriptDraftRevision struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Revision  int             `json:"revision"`
	Content   json.RawMessage `json:"content"`
	PlainText string          `json:"plainText"`
	Source    string          `json:"source"`
	CreatedAt time.Time       `json:"createdAt"`
}

type SearchResult struct {
	NodeID      string `json:"nodeId"`
	NodeKind    string `json:"nodeKind"`
	Breadcrumb  string `json:"breadcrumb"`
	Snippet     string `json:"snippet"`
	MatchCount  int    `json:"matchCount"`
	Revision    int    `json:"revision"`
	SourceScope string `json:"sourceScope"`
}

type RecoveryItem struct {
	SessionID        string    `json:"sessionId"`
	ProjectID        string    `json:"projectId"`
	ProjectTitle     string    `json:"projectTitle"`
	DestinationTitle *string   `json:"destinationTitle,omitempty"`
	Status           string    `json:"status"`
	DurationMS       int64     `json:"durationMs"`
	LocalChunkCount  int       `json:"localChunkCount"`
	RemoteChunkCount int       `json:"remoteChunkCount"`
	CreatedAt        time.Time `json:"createdAt"`
}

type ExportJob struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"projectId"`
	Format      string          `json:"format"`
	Status      string          `json:"status"`
	Snapshot    json.RawMessage `json:"snapshot"`
	StorageKey  *string         `json:"storageKey,omitempty"`
	DownloadURL *string         `json:"downloadUrl,omitempty"`
	ErrorCode   *string         `json:"errorCode,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}
