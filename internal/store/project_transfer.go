package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"writers.studio/internal/assistant"
	canvasdocument "writers.studio/internal/canvas"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	exportformat "writers.studio/internal/export"
	"writers.studio/internal/markdowntree"
)

const (
	ProjectTransferFormat         = "writers-studio-project"
	ProjectTransferFormatVersion  = 6
	ProjectTransferMinimumVersion = 1
)

var ErrInvalidProjectTransfer = errors.New("invalid writers studio project file")

type ProjectTransfer struct {
	Format                   string                              `json:"format"`
	FormatVersion            int                                 `json:"formatVersion"`
	ExportedAt               time.Time                           `json:"exportedAt"`
	Project                  ProjectTransferProject              `json:"project"`
	Nodes                    []ProjectTransferNode               `json:"nodes"`
	DocumentRevisions        []ProjectTransferDocumentRevision   `json:"documentRevisions"`
	NamedVersions            []ProjectTransferNamedVersion       `json:"namedVersions"`
	DictionaryTerms          []ProjectTransferDictionaryTerm     `json:"dictionaryTerms"`
	DocumentAnnotations      []ProjectTransferDocumentAnnotation `json:"documentAnnotations,omitempty"`
	DictationSessions        []ProjectTransferDictationSession   `json:"dictationSessions"`
	TranscriptArtifacts      []ProjectTransferArtifact           `json:"transcriptArtifacts"`
	TranscriptDrafts         []ProjectTransferDraft              `json:"transcriptDrafts"`
	TranscriptDraftRevisions []ProjectTransferDraftRevision      `json:"transcriptDraftRevisions"`
	Audio                    []ProjectTransferAudio              `json:"audio"`
	Media                    []ProjectTransferMedia              `json:"media,omitempty"`
	Canvases                 []ProjectTransferCanvas             `json:"canvases"`
	CanvasRevisions          []ProjectTransferCanvasRevision     `json:"canvasRevisions"`
	AssistantGuide           *ProjectTransferAssistantGuide      `json:"assistantGuide,omitempty"`
	ProjectMemory            []ProjectTransferMemoryRecord       `json:"projectMemory"`
	ProjectMemoryRevisions   []ProjectTransferMemoryRevision     `json:"projectMemoryRevisions"`
}

type ProjectTransferProject struct {
	ID                    string                  `json:"id"`
	Title                 string                  `json:"title"`
	Kind                  string                  `json:"kind"`
	Language              string                  `json:"language"`
	Synopsis              string                  `json:"synopsis"`
	WordGoal              *int                    `json:"wordGoal,omitempty"`
	AssistantStepLimit    *int                    `json:"assistantStepLimit,omitempty"`
	StructureRevision     int                     `json:"structureRevision"`
	Structure             domain.ProjectStructure `json:"structure"`
	ExportProfile         domain.ExportProfile    `json:"exportProfile,omitempty"`
	ExportProfileRevision int                     `json:"exportProfileRevision,omitempty"`
	CreatedAt             time.Time               `json:"createdAt"`
	UpdatedAt             time.Time               `json:"updatedAt"`
}

type ProjectTransferNode struct {
	ID                string     `json:"id"`
	ParentID          *string    `json:"parentId,omitempty"`
	Kind              string     `json:"kind"`
	Title             string     `json:"title"`
	Status            string     `json:"status"`
	Position          float64    `json:"position"`
	CurrentRevision   int        `json:"currentRevision"`
	SourceFrontmatter string     `json:"sourceFrontmatter,omitempty"`
	TrashedAt         *time.Time `json:"trashedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type ProjectTransferDocumentRevision struct {
	ID            string          `json:"id"`
	NodeID        string          `json:"nodeId"`
	Revision      int             `json:"revision"`
	BaseRevision  int             `json:"baseRevision"`
	SchemaVersion int             `json:"schemaVersion"`
	Content       json.RawMessage `json:"content"`
	PlainText     string          `json:"plainText"`
	Source        string          `json:"source"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type ProjectTransferNamedVersion struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	Revision  int       `json:"revision"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type ProjectTransferDictionaryTerm struct {
	ID            string    `json:"id"`
	Canonical     string    `json:"canonical"`
	Variants      []string  `json:"variants"`
	Kind          string    `json:"kind"`
	Language      string    `json:"language"`
	Pronunciation string    `json:"pronunciation"`
	Active        bool      `json:"active"`
	Revision      int       `json:"revision"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ProjectTransferDocumentAnnotation struct {
	ID               string                  `json:"id"`
	NodeID           string                  `json:"nodeId"`
	Kind             string                  `json:"kind"`
	DictionaryTermID *string                 `json:"dictionaryTermId,omitempty"`
	Status           string                  `json:"status"`
	AnchorStatus     string                  `json:"anchorStatus,omitempty"`
	Anchor           domain.AnnotationAnchor `json:"anchor"`
	Note             string                  `json:"note,omitempty"`
	TargetSnapshot   json.RawMessage         `json:"targetSnapshot"`
	BaseRevision     int                     `json:"baseRevision"`
	Revision         int                     `json:"revision,omitempty"`
	ResolvedAt       *time.Time              `json:"resolvedAt,omitempty"`
	CreatedAt        time.Time               `json:"createdAt"`
	UpdatedAt        time.Time               `json:"updatedAt"`
}

type ProjectTransferDictationSession struct {
	ID                    string          `json:"id"`
	DestinationNodeID     *string         `json:"destinationNodeId,omitempty"`
	Language              string          `json:"language"`
	Status                string          `json:"status"`
	DurationMS            int64           `json:"durationMs"`
	RecordedDurationMS    int64           `json:"recordedDurationMs"`
	Manifest              json.RawMessage `json:"manifest"`
	LiveFinalizedAt       *time.Time      `json:"liveFinalizedAt,omitempty"`
	CommandsEnabled       bool            `json:"commandsEnabled"`
	CommandPrefix         string          `json:"commandPrefix"`
	RetranscriptionStatus string          `json:"retranscriptionStatus,omitempty"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type ProjectTransferArtifact struct {
	ID              string          `json:"id"`
	SessionID       string          `json:"sessionId"`
	Kind            string          `json:"kind"`
	PipelineVersion string          `json:"pipelineVersion"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	Language        string          `json:"language"`
	Text            string          `json:"text"`
	Segments        json.RawMessage `json:"segments"`
	AudioSHA256     *string         `json:"audioSha256,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type ProjectTransferDraft struct {
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
}

type ProjectTransferDraftRevision struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Revision  int             `json:"revision"`
	Content   json.RawMessage `json:"content"`
	PlainText string          `json:"plainText"`
	Source    string          `json:"source"`
	CreatedAt time.Time       `json:"createdAt"`
}

type ProjectTransferAudio struct {
	SessionID  string `json:"sessionId"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	ByteSize   int64  `json:"byteSize"`
	DurationMS int64  `json:"durationMs"`
	MediaType  string `json:"mediaType"`
	StorageKey string `json:"-"`
}

type ProjectTransferMedia struct {
	ID           string  `json:"id"`
	Path         string  `json:"path"`
	SHA256       string  `json:"sha256"`
	ByteSize     int64   `json:"byteSize"`
	MediaType    string  `json:"mediaType"`
	OriginalName string  `json:"originalName"`
	SourcePath   *string `json:"sourcePath,omitempty"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	StorageKey   string  `json:"-"`
}

type ProjectTransferCanvas struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Position        float64    `json:"position"`
	CurrentRevision int        `json:"currentRevision"`
	TrashedAt       *time.Time `json:"trashedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ProjectTransferCanvasRevision struct {
	ID            string          `json:"id"`
	CanvasID      string          `json:"canvasId"`
	Revision      int             `json:"revision"`
	BaseRevision  int             `json:"baseRevision"`
	SchemaVersion int             `json:"schemaVersion"`
	Content       json.RawMessage `json:"content"`
	Source        string          `json:"source"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type ProjectTransferAssistantGuide struct {
	CurrentRevision int                                     `json:"currentRevision"`
	Active          bool                                    `json:"active"`
	CreatedAt       time.Time                               `json:"createdAt"`
	UpdatedAt       time.Time                               `json:"updatedAt"`
	Revisions       []ProjectTransferAssistantGuideRevision `json:"revisions"`
}

type ProjectTransferAssistantGuideRevision struct {
	ID                   string    `json:"id"`
	Revision             int       `json:"revision"`
	Content              string    `json:"content"`
	Source               string    `json:"source"`
	RestoredFromRevision *int      `json:"restoredFromRevision,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
}

type ProjectTransferMemoryRecord struct {
	ID               string                      `json:"id"`
	Kind             assistant.MemoryKind        `json:"kind"`
	Statement        string                      `json:"statement"`
	Scope            string                      `json:"scope"`
	ScopeNodeID      *string                     `json:"scopeNodeId,omitempty"`
	SourceReferences []assistant.SourceReference `json:"sourceReferences"`
	Tags             []string                    `json:"tags"`
	Active           bool                        `json:"active"`
	AlwaysRelevant   bool                        `json:"alwaysRelevant"`
	Revision         int                         `json:"revision"`
	Source           string                      `json:"source"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
}

type ProjectTransferMemoryRevision struct {
	ID        string                      `json:"id"`
	MemoryID  string                      `json:"memoryId"`
	Revision  int                         `json:"revision"`
	Snapshot  ProjectTransferMemoryRecord `json:"snapshot"`
	Source    string                      `json:"source"`
	CreatedAt time.Time                   `json:"createdAt"`
}

type ProjectTransferImportResult struct {
	ProjectID   string `json:"projectId"`
	FirstNodeID string `json:"firstNodeId"`
	Title       string `json:"title"`
}

func (s *Store) BuildProjectTransfer(
	ctx context.Context,
	ownerID string,
	projectID string,
) (ProjectTransfer, error) {
	workspace, err := s.GetProject(ctx, ownerID, projectID)
	if err != nil {
		return ProjectTransfer{}, err
	}
	result := ProjectTransfer{
		Format:                   ProjectTransferFormat,
		FormatVersion:            ProjectTransferFormatVersion,
		ExportedAt:               time.Now().UTC(),
		DocumentRevisions:        make([]ProjectTransferDocumentRevision, 0),
		NamedVersions:            make([]ProjectTransferNamedVersion, 0),
		DictionaryTerms:          make([]ProjectTransferDictionaryTerm, 0),
		DocumentAnnotations:      make([]ProjectTransferDocumentAnnotation, 0),
		DictationSessions:        make([]ProjectTransferDictationSession, 0),
		TranscriptArtifacts:      make([]ProjectTransferArtifact, 0),
		TranscriptDrafts:         make([]ProjectTransferDraft, 0),
		TranscriptDraftRevisions: make([]ProjectTransferDraftRevision, 0),
		Audio:                    make([]ProjectTransferAudio, 0),
		Media:                    make([]ProjectTransferMedia, 0),
		Canvases:                 make([]ProjectTransferCanvas, 0),
		CanvasRevisions:          make([]ProjectTransferCanvasRevision, 0),
		ProjectMemory:            make([]ProjectTransferMemoryRecord, 0),
		ProjectMemoryRevisions:   make([]ProjectTransferMemoryRevision, 0),
	}
	project := workspace.Project
	assistantSettings, err := s.GetProjectAssistantSettings(ctx, ownerID, projectID)
	if err != nil {
		return ProjectTransfer{}, err
	}
	exportProfile, exportProfileRevision, err := s.GetExportProfile(ctx, ownerID, projectID)
	if err != nil {
		return ProjectTransfer{}, err
	}
	result.Project = ProjectTransferProject{
		ID: project.ID, Title: project.Title, Kind: project.Kind, Language: project.Language,
		Synopsis: project.Synopsis, WordGoal: project.WordGoal,
		AssistantStepLimit: assistantSettings.StepLimit,
		StructureRevision:  project.StructureRevision, Structure: project.Structure,
		ExportProfile: exportProfile, ExportProfileRevision: exportProfileRevision,
		CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt,
	}
	result.Nodes = make([]ProjectTransferNode, 0, len(workspace.Nodes))
	for _, node := range workspace.Nodes {
		result.Nodes = append(result.Nodes, ProjectTransferNode{
			ID: node.ID, ParentID: node.ParentID, Kind: node.Kind, Title: node.Title,
			Status: node.Status, Position: node.Position, CurrentRevision: node.CurrentRevision,
			SourceFrontmatter: node.SourceFrontmatter,
			TrashedAt:         node.TrashedAt, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt,
		})
	}

	if err := s.loadProjectTransferDocuments(ctx, projectID, &result); err != nil {
		return ProjectTransfer{}, err
	}
	if err := s.loadProjectTransferCanvases(ctx, projectID, &result); err != nil {
		return ProjectTransfer{}, err
	}
	if err := s.loadProjectTransferDictations(ctx, projectID, &result); err != nil {
		return ProjectTransfer{}, err
	}
	if err := s.loadProjectTransferAssistant(ctx, projectID, &result); err != nil {
		return ProjectTransfer{}, err
	}
	if err := ValidateProjectTransfer(result); err != nil {
		return ProjectTransfer{}, fmt.Errorf("validate generated project transfer: %w", err)
	}
	return result, nil
}

func (s *Store) loadProjectTransferCanvases(
	ctx context.Context,
	projectID string,
	result *ProjectTransfer,
) error {
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, title, position::float8, current_revision, trashed_at,
			created_at, updated_at
		FROM canvases WHERE project_id=$1 ORDER BY position, created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProjectTransferCanvas
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Position, &item.CurrentRevision,
			&item.TrashedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return err
		}
		result.Canvases = append(result.Canvases, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	revisionRows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.canvas_id::text, r.revision, r.base_revision,
			r.schema_version, r.content, r.source, r.created_at
		FROM canvas_revisions r
		JOIN canvases c ON c.id=r.canvas_id
		WHERE c.project_id=$1 ORDER BY c.created_at, r.revision
	`, projectID)
	if err != nil {
		return err
	}
	defer revisionRows.Close()
	for revisionRows.Next() {
		var item ProjectTransferCanvasRevision
		if err := revisionRows.Scan(
			&item.ID, &item.CanvasID, &item.Revision, &item.BaseRevision,
			&item.SchemaVersion, &item.Content, &item.Source, &item.CreatedAt,
		); err != nil {
			return err
		}
		result.CanvasRevisions = append(result.CanvasRevisions, item)
	}
	return revisionRows.Err()
}

func (s *Store) loadProjectTransferDocuments(
	ctx context.Context,
	projectID string,
	result *ProjectTransfer,
) error {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.node_id::text, r.revision, r.base_revision, r.schema_version,
			r.content, r.plain_text, r.source, r.created_at
		FROM document_revisions r
		JOIN document_nodes n ON n.id=r.node_id
		WHERE n.project_id=$1
		ORDER BY n.created_at, r.revision
	`, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProjectTransferDocumentRevision
		if err := rows.Scan(&item.ID, &item.NodeID, &item.Revision, &item.BaseRevision,
			&item.SchemaVersion, &item.Content, &item.PlainText, &item.Source,
			&item.CreatedAt); err != nil {
			return err
		}
		result.DocumentRevisions = append(result.DocumentRevisions, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	versionRows, err := s.Pool.Query(ctx, `
		SELECT v.id::text, v.node_id::text, v.revision, v.name, v.note, v.created_at
		FROM named_versions v JOIN document_nodes n ON n.id=v.node_id
		WHERE n.project_id=$1 ORDER BY v.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer versionRows.Close()
	for versionRows.Next() {
		var item ProjectTransferNamedVersion
		if err := versionRows.Scan(&item.ID, &item.NodeID, &item.Revision, &item.Name,
			&item.Note, &item.CreatedAt); err != nil {
			return err
		}
		result.NamedVersions = append(result.NamedVersions, item)
	}
	if err := versionRows.Err(); err != nil {
		return err
	}

	termRows, err := s.Pool.Query(ctx, `
		SELECT id::text, canonical, variants, kind, language, pronunciation, active,
			revision, source, created_at, updated_at
		FROM dictionary_terms WHERE project_id=$1 ORDER BY created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer termRows.Close()
	for termRows.Next() {
		var item ProjectTransferDictionaryTerm
		var variants []byte
		if err := termRows.Scan(&item.ID, &item.Canonical, &variants, &item.Kind,
			&item.Language, &item.Pronunciation, &item.Active, &item.Revision,
			&item.Source, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		if err := json.Unmarshal(variants, &item.Variants); err != nil {
			return err
		}
		result.DictionaryTerms = append(result.DictionaryTerms, item)
	}
	if err := termRows.Err(); err != nil {
		return err
	}
	termRows.Close()

	annotationRows, err := s.Pool.Query(ctx, `
		SELECT id::text,node_id::text,kind,dictionary_term_id::text,status,anchor_status,block_id,
			start_offset,end_offset,exact_text,prefix,suffix,note,target_snapshot,
			base_revision,revision,resolved_at,created_at,updated_at
		FROM document_annotations WHERE project_id=$1 ORDER BY created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer annotationRows.Close()
	for annotationRows.Next() {
		var item ProjectTransferDocumentAnnotation
		if err := annotationRows.Scan(
			&item.ID, &item.NodeID, &item.Kind, &item.DictionaryTermID, &item.Status, &item.AnchorStatus,
			&item.Anchor.BlockID, &item.Anchor.Start, &item.Anchor.End, &item.Anchor.ExactText,
			&item.Anchor.Prefix, &item.Anchor.Suffix, &item.Note, &item.TargetSnapshot,
			&item.BaseRevision, &item.Revision, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return err
		}
		result.DocumentAnnotations = append(result.DocumentAnnotations, item)
	}
	return annotationRows.Err()
}

func (s *Store) loadProjectTransferDictations(
	ctx context.Context,
	projectID string,
	result *ProjectTransfer,
) error {
	rows, err := s.Pool.Query(ctx, `
		SELECT session.id::text, session.destination_node_id::text, session.language,
			session.status, session.duration_ms, session.recorded_duration_ms,
			session.manifest, session.live_finalized_at, session.commands_enabled,
			session.command_prefix, COALESCE((
				SELECT job.status FROM background_jobs job
				WHERE job.kind='retranscribe_dictation'
				  AND job.payload->>'sessionId'=session.id::text
				ORDER BY job.created_at DESC LIMIT 1
			), ''), session.created_at, session.updated_at
		FROM dictation_sessions session
		WHERE session.project_id=$1 ORDER BY session.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProjectTransferDictationSession
		if err := rows.Scan(&item.ID, &item.DestinationNodeID, &item.Language, &item.Status,
			&item.DurationMS, &item.RecordedDurationMS, &item.Manifest,
			&item.LiveFinalizedAt, &item.CommandsEnabled, &item.CommandPrefix,
			&item.RetranscriptionStatus, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		result.DictationSessions = append(result.DictationSessions, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	artifactRows, err := s.Pool.Query(ctx, `
		SELECT a.id::text, a.session_id::text, a.kind, a.pipeline_version, a.provider,
			a.model, a.language, a.text, a.segments, a.audio_sha256, a.created_at
		FROM transcript_artifacts a JOIN dictation_sessions s ON s.id=a.session_id
		WHERE s.project_id=$1 ORDER BY a.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer artifactRows.Close()
	for artifactRows.Next() {
		var item ProjectTransferArtifact
		if err := artifactRows.Scan(&item.ID, &item.SessionID, &item.Kind,
			&item.PipelineVersion, &item.Provider, &item.Model, &item.Language, &item.Text,
			&item.Segments, &item.AudioSHA256, &item.CreatedAt); err != nil {
			return err
		}
		result.TranscriptArtifacts = append(result.TranscriptArtifacts, item)
	}
	if err := artifactRows.Err(); err != nil {
		return err
	}

	draftRows, err := s.Pool.Query(ctx, `
		SELECT d.id::text, d.session_id::text, d.source_artifact_id::text, d.revision,
			d.base_source_text, d.content, d.plain_text, d.manually_edited, d.created_at, d.updated_at
		FROM transcript_drafts d JOIN dictation_sessions s ON s.id=d.session_id
		WHERE s.project_id=$1 ORDER BY d.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer draftRows.Close()
	for draftRows.Next() {
		var item ProjectTransferDraft
		if err := draftRows.Scan(&item.ID, &item.SessionID, &item.SourceArtifactID,
			&item.Revision, &item.BaseSourceText, &item.Content, &item.PlainText,
			&item.ManuallyEdited, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		result.TranscriptDrafts = append(result.TranscriptDrafts, item)
	}
	if err := draftRows.Err(); err != nil {
		return err
	}

	draftRevisionRows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.session_id::text, r.revision, r.content, r.plain_text,
			r.source, r.created_at
		FROM transcript_draft_revisions r JOIN dictation_sessions s ON s.id=r.session_id
		WHERE s.project_id=$1 ORDER BY r.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer draftRevisionRows.Close()
	for draftRevisionRows.Next() {
		var item ProjectTransferDraftRevision
		if err := draftRevisionRows.Scan(&item.ID, &item.SessionID, &item.Revision,
			&item.Content, &item.PlainText, &item.Source, &item.CreatedAt); err != nil {
			return err
		}
		result.TranscriptDraftRevisions = append(result.TranscriptDraftRevisions, item)
	}
	if err := draftRevisionRows.Err(); err != nil {
		return err
	}

	audioRows, err := s.Pool.Query(ctx, `
		SELECT a.session_id::text, a.sha256, a.storage_key, a.byte_size, a.duration_ms
		FROM audio_assets a JOIN dictation_sessions s ON s.id=a.session_id
		WHERE s.project_id=$1 ORDER BY s.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer audioRows.Close()
	for audioRows.Next() {
		var item ProjectTransferAudio
		if err := audioRows.Scan(&item.SessionID, &item.SHA256, &item.StorageKey,
			&item.ByteSize, &item.DurationMS); err != nil {
			return err
		}
		extension := audioExtension(item.StorageKey)
		item.Path = "audio/" + item.SessionID + "." + extension
		item.MediaType = audioMediaType(extension)
		result.Audio = append(result.Audio, item)
	}
	if err := audioRows.Err(); err != nil {
		return err
	}

	mediaRows, err := s.Pool.Query(ctx, `
		SELECT id::text, sha256, storage_key, byte_size, media_type, original_name,
			width, height, source_path
		FROM project_media
		WHERE project_id=$1 ORDER BY created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer mediaRows.Close()
	for mediaRows.Next() {
		var item ProjectTransferMedia
		if err := mediaRows.Scan(&item.ID, &item.SHA256, &item.StorageKey, &item.ByteSize,
			&item.MediaType, &item.OriginalName, &item.Width, &item.Height, &item.SourcePath); err != nil {
			return err
		}
		item.Path = "media/" + item.ID + mediaExtension(item.MediaType)
		result.Media = append(result.Media, item)
	}
	return mediaRows.Err()
}

func (s *Store) loadProjectTransferAssistant(
	ctx context.Context,
	projectID string,
	result *ProjectTransfer,
) error {
	var guide ProjectTransferAssistantGuide
	err := s.Pool.QueryRow(ctx, `
		SELECT current_revision, active, created_at, updated_at
		FROM assistant_guides WHERE project_id=$1
	`, projectID).Scan(&guide.CurrentRevision, &guide.Active, &guide.CreatedAt, &guide.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		guide.Revisions = make([]ProjectTransferAssistantGuideRevision, 0)
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, revision, content, source, restored_from_revision, created_at
			FROM assistant_guide_revisions WHERE project_id=$1 ORDER BY revision
		`, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ProjectTransferAssistantGuideRevision
			if err := rows.Scan(&item.ID, &item.Revision, &item.Content, &item.Source,
				&item.RestoredFromRevision, &item.CreatedAt); err != nil {
				return err
			}
			guide.Revisions = append(guide.Revisions, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		result.AssistantGuide = &guide
	}

	memoryRows, err := s.Pool.Query(ctx, `
		SELECT id::text, kind, statement, scope, scope_node_id::text, source_references,
			tags, active, always_relevant, revision, source, created_at, updated_at
		FROM project_memory_records WHERE project_id=$1 ORDER BY created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer memoryRows.Close()
	for memoryRows.Next() {
		var item ProjectTransferMemoryRecord
		var references, tags []byte
		if err := memoryRows.Scan(&item.ID, &item.Kind, &item.Statement, &item.Scope,
			&item.ScopeNodeID, &references, &tags, &item.Active, &item.AlwaysRelevant,
			&item.Revision, &item.Source, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		if err := json.Unmarshal(references, &item.SourceReferences); err != nil {
			return err
		}
		if err := json.Unmarshal(tags, &item.Tags); err != nil {
			return err
		}
		result.ProjectMemory = append(result.ProjectMemory, item)
	}
	if err := memoryRows.Err(); err != nil {
		return err
	}

	revisionRows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.memory_id::text, r.revision, r.snapshot, r.source, r.created_at
		FROM project_memory_revisions r
		JOIN project_memory_records m ON m.id=r.memory_id
		WHERE m.project_id=$1 ORDER BY r.created_at
	`, projectID)
	if err != nil {
		return err
	}
	defer revisionRows.Close()
	for revisionRows.Next() {
		var item ProjectTransferMemoryRevision
		var snapshot []byte
		if err := revisionRows.Scan(&item.ID, &item.MemoryID, &item.Revision, &snapshot,
			&item.Source, &item.CreatedAt); err != nil {
			return err
		}
		if err := json.Unmarshal(snapshot, &item.Snapshot); err != nil {
			return err
		}
		result.ProjectMemoryRevisions = append(result.ProjectMemoryRevisions, item)
	}
	return revisionRows.Err()
}

func (s *Store) ImportProjectTransfer(
	ctx context.Context,
	ownerID string,
	transfer ProjectTransfer,
) (ProjectTransferImportResult, error) {
	if err := ValidateProjectTransfer(transfer); err != nil {
		return ProjectTransferImportResult{}, err
	}

	projectID := uuid.NewString()
	nodeIDs := remapIDs(transferNodeIDs(transfer.Nodes))
	termIDs := remapIDs(transferTermIDs(transfer.DictionaryTerms))
	sessionIDs := remapIDs(transferSessionIDs(transfer.DictationSessions))
	artifactIDs := remapIDs(transferArtifactIDs(transfer.TranscriptArtifacts))
	memoryIDs := remapIDs(transferMemoryIDs(transfer.ProjectMemory))
	mediaIDs := remapIDs(transferMediaIDs(transfer.Media))
	canvasIDs := remapIDs(transferCanvasIDs(transfer.Canvases))
	exportProfile := transfer.Project.ExportProfile
	exportProfileRevision := transfer.Project.ExportProfileRevision
	if transfer.FormatVersion < 4 || exportProfile.SchemaVersion == 0 {
		exportProfile = exportformat.DefaultProfile(transfer.Project.Title, transfer.Project.Synopsis)
		exportProfileRevision = 1
	}
	if exportProfile.Metadata.CoverAssetID != nil {
		mapped, exists := mediaIDs[*exportProfile.Metadata.CoverAssetID]
		if !exists {
			return ProjectTransferImportResult{}, fmt.Errorf("%w: export profile cover is missing", ErrInvalidProjectTransfer)
		}
		exportProfile.Metadata.CoverAssetID = &mapped
	}
	exportProfileJSON, err := json.Marshal(exportProfile)
	if err != nil {
		return ProjectTransferImportResult{}, err
	}
	orderedNodes := append([]ProjectTransferNode(nil), transfer.Nodes...)
	sort.SliceStable(orderedNodes, func(i, j int) bool {
		return nodeDepth(orderedNodes[i], transfer.Nodes) < nodeDepth(orderedNodes[j], transfer.Nodes)
	})

	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		structure, err := json.Marshal(transfer.Project.Structure)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO projects(
				id, owner_user_id, title, kind, language, synopsis, word_goal,
				assistant_step_limit, status,
				structure_revision, structure_schema, export_profile,
				export_profile_revision, created_at, updated_at
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$10,$11,$12,now(),now())
		`, projectID, ownerID, transfer.Project.Title, transfer.Project.Kind,
			transfer.Project.Language, transfer.Project.Synopsis, transfer.Project.WordGoal,
			transfer.Project.AssistantStepLimit, transfer.Project.StructureRevision,
			structure, exportProfileJSON, exportProfileRevision); err != nil {
			return err
		}
		for _, media := range transfer.Media {
			if _, err := tx.Exec(ctx, `
				INSERT INTO project_media(
					id,project_id,owner_user_id,storage_key,media_type,original_name,
					sha256,byte_size,width,height,source_path
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			`, mediaIDs[media.ID], projectID, ownerID, media.StorageKey, media.MediaType,
				media.OriginalName, media.SHA256, media.ByteSize, media.Width, media.Height, media.SourcePath); err != nil {
				return err
			}
		}

		for _, node := range orderedNodes {
			var parentID *string
			if node.ParentID != nil {
				mapped := nodeIDs[*node.ParentID]
				parentID = &mapped
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_nodes(
					id, project_id, parent_id, kind, title, status, position,
					current_revision, trashed_at, created_at, updated_at, source_frontmatter
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			`, nodeIDs[node.ID], projectID, parentID, node.Kind, node.Title, node.Status,
				node.Position, node.CurrentRevision, node.TrashedAt, node.CreatedAt,
				node.UpdatedAt, node.SourceFrontmatter); err != nil {
				return err
			}
		}
		for _, revision := range transfer.DocumentRevisions {
			content, err := remapDocumentMedia(revision.Content, mediaIDs)
			if err != nil {
				return err
			}
			content, err = remapDocumentLinks(content, transfer.Project.ID, projectID, nodeIDs)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_revisions(
					id, node_id, revision, base_revision, schema_version, content,
					plain_text, source, operation_id, assistant_run_id,
					assistant_change_set_id, created_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULL,NULL,NULL,$9)
			`, uuid.NewString(), nodeIDs[revision.NodeID], revision.Revision,
				revision.BaseRevision, revision.SchemaVersion, content,
				revision.PlainText, revision.Source, revision.CreatedAt); err != nil {
				return err
			}
		}
		for _, canvas := range transfer.Canvases {
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvases(
					id,project_id,title,position,current_revision,trashed_at,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			`, canvasIDs[canvas.ID], projectID, canvas.Title, canvas.Position,
				canvas.CurrentRevision, canvas.TrashedAt, canvas.CreatedAt, canvas.UpdatedAt); err != nil {
				return err
			}
		}
		for _, revision := range transfer.CanvasRevisions {
			content, err := remapCanvasResources(revision.Content, nodeIDs, mediaIDs)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvas_revisions(
					id,canvas_id,revision,base_revision,schema_version,content,source,created_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			`, uuid.NewString(), canvasIDs[revision.CanvasID], revision.Revision,
				revision.BaseRevision, revision.SchemaVersion, content,
				revision.Source, revision.CreatedAt); err != nil {
				return err
			}
		}
		for _, version := range transfer.NamedVersions {
			if _, err := tx.Exec(ctx, `
				INSERT INTO named_versions(id,node_id,revision,name,note,created_at)
				VALUES($1,$2,$3,$4,$5,$6)
			`, uuid.NewString(), nodeIDs[version.NodeID], version.Revision, version.Name,
				version.Note, version.CreatedAt); err != nil {
				return err
			}
		}
		for _, term := range transfer.DictionaryTerms {
			variants, err := json.Marshal(term.Variants)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO dictionary_terms(
					id,project_id,canonical,variants,kind,language,pronunciation,active,
					revision,source,operation_id,assistant_run_id,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL,NULL,$11,$12)
			`, termIDs[term.ID], projectID, term.Canonical, variants, term.Kind,
				term.Language, term.Pronunciation, term.Active, term.Revision, term.Source,
				term.CreatedAt, term.UpdatedAt); err != nil {
				return err
			}
		}
		for _, annotation := range transfer.DocumentAnnotations {
			targetSnapshot, err := remapDocumentMedia(annotation.TargetSnapshot, mediaIDs)
			if err != nil {
				return err
			}
			targetSnapshot, err = remapDocumentLinks(targetSnapshot, transfer.Project.ID, projectID, nodeIDs)
			if err != nil {
				return err
			}
			if annotation.AnchorStatus == "" {
				annotation.AnchorStatus = "attached"
				if annotation.Status == "needs_attention" {
					annotation.AnchorStatus = "detached"
				}
			}
			if annotation.Revision < 1 {
				annotation.Revision = 1
			}
			var termID *string
			if annotation.DictionaryTermID != nil {
				mapped := termIDs[*annotation.DictionaryTermID]
				termID = &mapped
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_annotations(
					id,project_id,node_id,kind,dictionary_term_id,status,anchor_status,block_id,
					start_offset,end_offset,exact_text,prefix,suffix,note,target_snapshot,
					base_revision,revision,resolved_at,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
			`, uuid.NewString(), projectID, nodeIDs[annotation.NodeID], annotation.Kind,
				termID, annotation.Status, annotation.AnchorStatus, annotation.Anchor.BlockID, annotation.Anchor.Start,
				annotation.Anchor.End, annotation.Anchor.ExactText, annotation.Anchor.Prefix,
				annotation.Anchor.Suffix, annotation.Note, targetSnapshot,
				annotation.BaseRevision, annotation.Revision, annotation.ResolvedAt, annotation.CreatedAt,
				annotation.UpdatedAt); err != nil {
				return err
			}
		}

		draftSessions := map[string]bool{}
		for _, draft := range transfer.TranscriptDrafts {
			draftSessions[draft.SessionID] = true
		}
		for _, session := range transfer.DictationSessions {
			var destination *string
			if session.DestinationNodeID != nil {
				mapped := nodeIDs[*session.DestinationNodeID]
				destination = &mapped
			}
			status := normalizedImportedDictationStatus(session.Status, draftSessions[session.ID])
			if _, err := tx.Exec(ctx, `
				INSERT INTO dictation_sessions(
					id,owner_user_id,project_id,destination_node_id,client_session_id,
					language,status,duration_ms,recorded_duration_ms,manifest,
					live_finalized_at,commands_enabled,command_prefix,
					created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			`, sessionIDs[session.ID], ownerID, projectID, destination,
				"import:"+sessionIDs[session.ID], session.Language, status,
				session.DurationMS, session.RecordedDurationMS, session.Manifest,
				session.LiveFinalizedAt, session.CommandsEnabled, session.CommandPrefix,
				session.CreatedAt, session.UpdatedAt); err != nil {
				return err
			}
		}
		for _, artifact := range transfer.TranscriptArtifacts {
			if _, err := tx.Exec(ctx, `
				INSERT INTO transcript_artifacts(
					id,session_id,kind,pipeline_version,provider,model,language,text,
					segments,audio_sha256,created_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			`, artifactIDs[artifact.ID], sessionIDs[artifact.SessionID], artifact.Kind,
				artifact.PipelineVersion, artifact.Provider, artifact.Model,
				artifact.Language, artifact.Text, artifact.Segments, artifact.AudioSHA256,
				artifact.CreatedAt); err != nil {
				return err
			}
		}
		for _, draft := range transfer.TranscriptDrafts {
			var sourceArtifactID *string
			if draft.SourceArtifactID != nil {
				mapped := artifactIDs[*draft.SourceArtifactID]
				sourceArtifactID = &mapped
			}
			content, err := remapDocumentMedia(draft.Content, mediaIDs)
			if err != nil {
				return err
			}
			content, err = remapDocumentLinks(content, transfer.Project.ID, projectID, nodeIDs)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO transcript_drafts(
					id,session_id,source_artifact_id,revision,base_source_text,content,
					plain_text,manually_edited,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			`, uuid.NewString(), sessionIDs[draft.SessionID], sourceArtifactID,
				draft.Revision, draft.BaseSourceText, content, draft.PlainText,
				draft.ManuallyEdited, draft.CreatedAt, draft.UpdatedAt); err != nil {
				return err
			}
		}
		for _, revision := range transfer.TranscriptDraftRevisions {
			content, err := remapDocumentMedia(revision.Content, mediaIDs)
			if err != nil {
				return err
			}
			content, err = remapDocumentLinks(content, transfer.Project.ID, projectID, nodeIDs)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO transcript_draft_revisions(
					id,session_id,revision,content,plain_text,source,created_at
				) VALUES($1,$2,$3,$4,$5,$6,$7)
			`, uuid.NewString(), sessionIDs[revision.SessionID], revision.Revision,
				content, revision.PlainText, revision.Source,
				revision.CreatedAt); err != nil {
				return err
			}
		}
		for _, audio := range transfer.Audio {
			if _, err := tx.Exec(ctx, `
				INSERT INTO audio_assets(id,session_id,sha256,storage_key,byte_size,duration_ms)
				VALUES($1,$2,$3,$4,$5,$6)
			`, uuid.NewString(), sessionIDs[audio.SessionID], audio.SHA256,
				audio.StorageKey, audio.ByteSize, audio.DurationMS); err != nil {
				return err
			}
		}

		if transfer.AssistantGuide != nil {
			guide := transfer.AssistantGuide
			if _, err := tx.Exec(ctx, `
				INSERT INTO assistant_guides(
					project_id,current_revision,updated_by_user_id,active,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6)
			`, projectID, guide.CurrentRevision, ownerID, guide.Active,
				guide.CreatedAt, guide.UpdatedAt); err != nil {
				return err
			}
			for _, revision := range guide.Revisions {
				if _, err := tx.Exec(ctx, `
					INSERT INTO assistant_guide_revisions(
						id,project_id,revision,content,source,restored_from_revision,
						assistant_run_id,created_by_user_id,created_at
					) VALUES($1,$2,$3,$4,$5,$6,NULL,$7,$8)
				`, uuid.NewString(), projectID, revision.Revision, revision.Content,
					revision.Source, revision.RestoredFromRevision, ownerID,
					revision.CreatedAt); err != nil {
					return err
				}
			}
		}
		for _, memory := range transfer.ProjectMemory {
			var scopeNodeID *string
			if memory.ScopeNodeID != nil {
				mapped := nodeIDs[*memory.ScopeNodeID]
				scopeNodeID = &mapped
			}
			references := remapSourceReferences(memory.SourceReferences, nodeIDs)
			referencesJSON, err := json.Marshal(references)
			if err != nil {
				return err
			}
			tags, err := json.Marshal(append([]string{}, memory.Tags...))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO project_memory_records(
					id,project_id,kind,statement,scope,scope_node_id,source_references,
					tags,active,always_relevant,revision,source,operation_id,
					assistant_run_id,created_by_user_id,created_at,updated_at
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULL,NULL,$13,$14,$15)
			`, memoryIDs[memory.ID], projectID, memory.Kind, memory.Statement,
				memory.Scope, scopeNodeID, referencesJSON, tags, memory.Active,
				memory.AlwaysRelevant, memory.Revision, memory.Source, ownerID,
				memory.CreatedAt, memory.UpdatedAt); err != nil {
				return err
			}
		}
		for _, revision := range transfer.ProjectMemoryRevisions {
			snapshot := revision.Snapshot
			snapshot.ID = memoryIDs[revision.MemoryID]
			if snapshot.ScopeNodeID != nil {
				mapped := nodeIDs[*snapshot.ScopeNodeID]
				snapshot.ScopeNodeID = &mapped
			}
			snapshot.SourceReferences = remapSourceReferences(snapshot.SourceReferences, nodeIDs)
			snapshot.Tags = append([]string{}, snapshot.Tags...)
			snapshotJSON, err := json.Marshal(snapshot)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO project_memory_revisions(
					id,memory_id,revision,snapshot,source,operation_id,
					assistant_run_id,created_at
				) VALUES($1,$2,$3,$4,$5,NULL,NULL,$6)
			`, uuid.NewString(), memoryIDs[revision.MemoryID], revision.Revision,
				snapshotJSON, revision.Source, revision.CreatedAt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ProjectTransferImportResult{}, fmt.Errorf("import project transfer: %w", err)
	}

	firstNodeID := ""
	contentBearing := make(map[string]bool, len(transfer.Project.Structure.NodeTypes))
	for _, nodeType := range transfer.Project.Structure.NodeTypes {
		contentBearing[nodeType.Key] = nodeType.ContentBearing
	}
	for _, node := range transfer.Nodes {
		if node.TrashedAt == nil && contentBearing[node.Kind] {
			firstNodeID = nodeIDs[node.ID]
			break
		}
	}
	if firstNodeID == "" {
		for _, node := range transfer.Nodes {
			if node.TrashedAt == nil {
				firstNodeID = nodeIDs[node.ID]
				break
			}
		}
	}
	return ProjectTransferImportResult{
		ProjectID: projectID, FirstNodeID: firstNodeID, Title: transfer.Project.Title,
	}, nil
}

func ValidateProjectTransfer(transfer ProjectTransfer) error {
	invalid := func(message string) error {
		return fmt.Errorf("%w: %s", ErrInvalidProjectTransfer, message)
	}
	if transfer.Format != ProjectTransferFormat {
		return invalid("unknown format")
	}
	if transfer.FormatVersion < ProjectTransferMinimumVersion ||
		transfer.FormatVersion > ProjectTransferFormatVersion {
		return invalid("unsupported format version")
	}
	if length := len([]rune(strings.TrimSpace(transfer.Project.Title))); length < 1 || length > 120 {
		return invalid("invalid project title")
	}
	if transfer.Project.Language != "ru" && transfer.Project.Language != "en" {
		return invalid("invalid project language")
	}
	validKinds := map[string]bool{"novel": true, "story": true, "screenplay": true, "nonfiction": true, "game_design": true, "free": true, "custom": true}
	if !validKinds[transfer.Project.Kind] {
		return invalid("invalid project kind")
	}
	if transfer.Project.WordGoal != nil && (*transfer.Project.WordGoal < 1 || *transfer.Project.WordGoal > 10_000_000) {
		return invalid("invalid word goal")
	}
	if !ValidProjectAssistantStepLimit(transfer.Project.AssistantStepLimit) {
		return invalid("invalid assistant step limit")
	}
	if transfer.Project.StructureRevision < 1 || ValidateProjectStructure(transfer.Project.Structure) != nil {
		return invalid("invalid project structure")
	}
	if transfer.FormatVersion >= 4 && transfer.Project.ExportProfile.SchemaVersion != 0 {
		if transfer.Project.ExportProfileRevision < 1 || exportformat.ValidateProfile(transfer.Project.ExportProfile) != nil {
			return invalid("invalid export profile")
		}
	}
	if len(transfer.Nodes) == 0 || len(transfer.Nodes) > 20_000 {
		return invalid("invalid document count")
	}
	if len(transfer.DocumentRevisions) > 100_000 || len(transfer.DictationSessions) > 20_000 ||
		len(transfer.Media) > 20_000 || len(transfer.Canvases) > 2_000 ||
		len(transfer.CanvasRevisions) > 100_000 {
		return invalid("project exceeds transfer limits")
	}

	nodes := map[string]ProjectTransferNode{}
	allowedNodeKinds := map[string]bool{}
	for _, nodeType := range transfer.Project.Structure.NodeTypes {
		allowedNodeKinds[nodeType.Key] = true
	}
	for _, node := range transfer.Nodes {
		if node.ID == "" || nodes[node.ID].ID != "" {
			return invalid("duplicate document id")
		}
		if !allowedNodeKinds[node.Kind] {
			return invalid("unknown document kind")
		}
		if length := len([]rune(strings.TrimSpace(node.Title))); length < 1 || length > 200 {
			return invalid("invalid document title")
		}
		if node.Status != "idea" && node.Status != "draft" && node.Status != "editing" && node.Status != "ready" {
			return invalid("invalid document status")
		}
		if node.CurrentRevision < 0 {
			return invalid("invalid current revision")
		}
		if len(node.SourceFrontmatter) > int(markdowntree.MaxMarkdownBytes) ||
			(transfer.FormatVersion < 5 && node.SourceFrontmatter != "") {
			return invalid("invalid source frontmatter")
		}
		nodes[node.ID] = node
	}
	for _, node := range transfer.Nodes {
		if node.ParentID != nil {
			if *node.ParentID == node.ID || nodes[*node.ParentID].ID == "" {
				return invalid("invalid document parent")
			}
		}
		seen := map[string]bool{node.ID: true}
		cursor := node.ParentID
		for cursor != nil {
			if seen[*cursor] {
				return invalid("document tree contains a cycle")
			}
			seen[*cursor] = true
			parent := nodes[*cursor]
			cursor = parent.ParentID
		}
	}

	revisionKeys := map[string]bool{}
	revisionContent := map[string]json.RawMessage{}
	maxRevision := map[string]int{}
	for _, revision := range transfer.DocumentRevisions {
		schemaVersion, schemaErr := editor.SchemaVersion(revision.Content)
		if nodes[revision.NodeID].ID == "" || revision.Revision < 1 || revision.BaseRevision < 0 ||
			revision.SchemaVersion < 1 || revision.SchemaVersion > editor.CurrentSchemaVersion ||
			schemaErr != nil || schemaVersion != revision.SchemaVersion || editor.Validate(revision.Content) != nil {
			return invalid("invalid document revision")
		}
		key := fmt.Sprintf("%s:%d", revision.NodeID, revision.Revision)
		if revisionKeys[key] {
			return invalid("duplicate document revision")
		}
		revisionKeys[key] = true
		revisionContent[key] = revision.Content
		if revision.Revision > maxRevision[revision.NodeID] {
			maxRevision[revision.NodeID] = revision.Revision
		}
	}
	for _, node := range transfer.Nodes {
		if node.CurrentRevision != maxRevision[node.ID] {
			return invalid("current revision does not match revision history")
		}
	}
	for _, version := range transfer.NamedVersions {
		if !revisionKeys[fmt.Sprintf("%s:%d", version.NodeID, version.Revision)] ||
			len([]rune(strings.TrimSpace(version.Name))) < 1 || len([]rune(version.Name)) > 160 {
			return invalid("invalid named version")
		}
	}

	sessions := map[string]bool{}
	for _, session := range transfer.DictationSessions {
		if session.ID == "" || sessions[session.ID] {
			return invalid("duplicate dictation id")
		}
		sessions[session.ID] = true
		if session.DestinationNodeID != nil && nodes[*session.DestinationNodeID].ID == "" {
			return invalid("dictation destination is missing")
		}
		if session.Language != "ru" && session.Language != "en" {
			return invalid("invalid dictation language")
		}
		if !json.Valid(session.Manifest) {
			return invalid("invalid dictation manifest")
		}
	}
	artifacts := map[string]bool{}
	for _, artifact := range transfer.TranscriptArtifacts {
		if artifact.ID == "" || artifacts[artifact.ID] || !sessions[artifact.SessionID] ||
			!json.Valid(artifact.Segments) {
			return invalid("invalid transcript artifact")
		}
		if artifact.Kind != "live_final" && artifact.Kind != "batch_source" && artifact.Kind != "cleaned_suggestion" {
			return invalid("invalid transcript artifact kind")
		}
		artifacts[artifact.ID] = true
	}
	draftSessions := map[string]bool{}
	for _, draft := range transfer.TranscriptDrafts {
		if !sessions[draft.SessionID] || draftSessions[draft.SessionID] || draft.Revision < 1 || !json.Valid(draft.Content) {
			return invalid("invalid transcript draft")
		}
		if draft.SourceArtifactID != nil && !artifacts[*draft.SourceArtifactID] {
			return invalid("transcript draft source is missing")
		}
		draftSessions[draft.SessionID] = true
	}
	draftRevisionKeys := map[string]bool{}
	for _, revision := range transfer.TranscriptDraftRevisions {
		key := fmt.Sprintf("%s:%d", revision.SessionID, revision.Revision)
		if !sessions[revision.SessionID] || revision.Revision < 1 || !json.Valid(revision.Content) || draftRevisionKeys[key] {
			return invalid("invalid transcript draft revision")
		}
		draftRevisionKeys[key] = true
	}
	audioSessions := map[string]bool{}
	archivePaths := map[string]bool{}
	for _, audio := range transfer.Audio {
		if !sessions[audio.SessionID] || audioSessions[audio.SessionID] || archivePaths[audio.Path] ||
			!validTransferPath(audio.Path) || len(audio.SHA256) != 64 || audio.ByteSize < 0 || audio.DurationMS < 0 {
			return invalid("invalid audio entry")
		}
		audioSessions[audio.SessionID] = true
		archivePaths[audio.Path] = true
	}
	if transfer.FormatVersion < 3 && len(transfer.Media) > 0 {
		return invalid("media requires format version 3")
	}
	mediaIDs := map[string]bool{}
	for _, media := range transfer.Media {
		if media.ID == "" || mediaIDs[media.ID] || archivePaths[media.Path] ||
			!validTransferMediaPath(media.Path) || len(media.SHA256) != 64 || media.ByteSize < 1 ||
			media.ByteSize > 12<<20 || (media.MediaType != "image/png" &&
			media.MediaType != "image/jpeg" && media.MediaType != "image/gif") ||
			len([]rune(media.OriginalName)) > 255 || media.Width < 1 || media.Width > 20_000 ||
			media.Height < 1 || media.Height > 20_000 {
			return invalid("invalid media entry")
		}
		if media.SourcePath != nil {
			if transfer.FormatVersion < 5 {
				return invalid("media source path requires format version 5")
			}
			if _, err := markdowntree.NormalizePath(*media.SourcePath); err != nil {
				return invalid("invalid media source path")
			}
		}
		mediaIDs[media.ID] = true
		archivePaths[media.Path] = true
	}
	if transfer.FormatVersion >= 4 && transfer.Project.ExportProfile.Metadata.CoverAssetID != nil &&
		!mediaIDs[*transfer.Project.ExportProfile.Metadata.CoverAssetID] {
		return invalid("export profile cover is missing")
	}
	for _, revision := range transfer.DocumentRevisions {
		for _, assetID := range documentMediaIDs(revision.Content) {
			if !mediaIDs[assetID] {
				return invalid("document image asset is missing")
			}
		}
	}
	if transfer.FormatVersion < 6 &&
		(len(transfer.Canvases) > 0 || len(transfer.CanvasRevisions) > 0) {
		return invalid("canvases require format version 6")
	}
	canvases := map[string]ProjectTransferCanvas{}
	for _, item := range transfer.Canvases {
		if item.ID == "" || canvases[item.ID].ID != "" ||
			len([]rune(strings.TrimSpace(item.Title))) < 1 ||
			len([]rune(item.Title)) > 160 || item.CurrentRevision < 1 ||
			math.IsNaN(item.Position) || math.IsInf(item.Position, 0) ||
			item.Position < -1e12 || item.Position > 1e12 {
			return invalid("invalid canvas")
		}
		canvases[item.ID] = item
	}
	canvasRevisionKeys := map[string]bool{}
	maxCanvasRevision := map[string]int{}
	for _, revision := range transfer.CanvasRevisions {
		references, err := canvasdocument.Validate(revision.Content)
		key := fmt.Sprintf("%s:%d", revision.CanvasID, revision.Revision)
		if canvases[revision.CanvasID].ID == "" || revision.Revision < 1 ||
			revision.BaseRevision < 0 || revision.BaseRevision >= revision.Revision ||
			revision.SchemaVersion != canvasdocument.SchemaVersion || err != nil ||
			canvasRevisionKeys[key] || strings.TrimSpace(revision.Source) == "" ||
			len(revision.Source) > 80 {
			return invalid("invalid canvas revision")
		}
		for _, nodeID := range references.DocumentIDs {
			if nodes[nodeID].ID == "" {
				return invalid("canvas document reference is missing")
			}
		}
		for _, mediaID := range references.MediaIDs {
			if !mediaIDs[mediaID] {
				return invalid("canvas media reference is missing")
			}
		}
		canvasRevisionKeys[key] = true
		if revision.Revision > maxCanvasRevision[revision.CanvasID] {
			maxCanvasRevision[revision.CanvasID] = revision.Revision
		}
	}
	for _, item := range transfer.Canvases {
		if item.CurrentRevision != maxCanvasRevision[item.ID] {
			return invalid("canvas current revision does not match revision history")
		}
	}
	terms := map[string]bool{}
	for _, term := range transfer.DictionaryTerms {
		if term.ID == "" || terms[term.ID] || strings.TrimSpace(term.Canonical) == "" || (term.Language != "ru" && term.Language != "en") ||
			(term.Kind != "character" && term.Kind != "place" && term.Kind != "term" && term.Kind != "other") || term.Revision < 1 {
			return invalid("invalid dictionary term")
		}
		terms[term.ID] = true
	}
	if transfer.FormatVersion == 1 && len(transfer.DocumentAnnotations) > 0 {
		return invalid("annotations require format version 2")
	}
	seenAnnotations := map[string]bool{}
	for _, annotation := range transfer.DocumentAnnotations {
		if annotation.ID == "" || seenAnnotations[annotation.ID] || nodes[annotation.NodeID].ID == "" ||
			(annotation.Kind != "note" && annotation.Kind != "dictionary_link") ||
			!validAnnotationStatus(annotation.Status) || annotation.BaseRevision < 1 ||
			(annotation.AnchorStatus != "" && annotation.AnchorStatus != "attached" &&
				annotation.AnchorStatus != "detached") || annotation.Revision < 0 ||
			annotation.BaseRevision > nodes[annotation.NodeID].CurrentRevision ||
			annotation.Anchor.BlockID == "" || annotation.Anchor.ExactText == "" ||
			annotation.Anchor.Start < 0 || annotation.Anchor.End <= annotation.Anchor.Start ||
			!json.Valid(annotation.TargetSnapshot) {
			return invalid("invalid document annotation")
		}
		if annotation.Kind == "note" && strings.TrimSpace(annotation.Note) == "" {
			return invalid("invalid document note")
		}
		if annotation.DictionaryTermID != nil && !terms[*annotation.DictionaryTermID] {
			return invalid("annotation dictionary term is missing")
		}
		if annotation.Kind == "dictionary_link" && annotation.DictionaryTermID == nil &&
			string(annotation.TargetSnapshot) == "{}" {
			return invalid("dictionary annotation target is missing")
		}
		content := revisionContent[fmt.Sprintf("%s:%d", annotation.NodeID, annotation.BaseRevision)]
		if len(content) == 0 || editor.ValidateAnchor(content, annotation.Anchor) != nil {
			return invalid("annotation anchor does not match its base revision")
		}
		seenAnnotations[annotation.ID] = true
	}
	if transfer.AssistantGuide != nil {
		guide := transfer.AssistantGuide
		seen := map[int]bool{}
		for _, revision := range guide.Revisions {
			if revision.Revision < 1 || seen[revision.Revision] || assistant.ValidateGuide(revision.Content) != nil ||
				(revision.Source != "manual" && revision.Source != "assistant" && revision.Source != "restore") {
				return invalid("invalid assistant guide")
			}
			seen[revision.Revision] = true
		}
		if guide.CurrentRevision > 0 && !seen[guide.CurrentRevision] {
			return invalid("assistant guide revision is missing")
		}
	}
	memories := map[string]bool{}
	for _, memory := range transfer.ProjectMemory {
		if memory.ID == "" || memories[memory.ID] {
			return invalid("duplicate memory id")
		}
		if memory.ScopeNodeID != nil && nodes[*memory.ScopeNodeID].ID == "" {
			return invalid("memory node is missing")
		}
		candidate := assistant.MemoryRecord{Kind: memory.Kind, Statement: memory.Statement,
			Scope: memory.Scope, ScopeNodeID: memory.ScopeNodeID}
		if candidate.Validate() != nil || memory.Revision < 1 || (memory.Source != "manual" && memory.Source != "assistant") {
			return invalid("invalid project memory")
		}
		memories[memory.ID] = true
	}
	memoryRevisionKeys := map[string]bool{}
	for _, revision := range transfer.ProjectMemoryRevisions {
		key := fmt.Sprintf("%s:%d", revision.MemoryID, revision.Revision)
		if !memories[revision.MemoryID] || revision.Revision < 1 || memoryRevisionKeys[key] ||
			(revision.Source != "manual" && revision.Source != "assistant") {
			return invalid("invalid project memory revision")
		}
		memoryRevisionKeys[key] = true
	}
	return nil
}

func validTransferPath(value string) bool {
	return strings.HasPrefix(value, "audio/") && !strings.Contains(value, "..") &&
		!strings.Contains(value, "\\") && !strings.HasPrefix(value, "/")
}

func validTransferMediaPath(value string) bool {
	return strings.HasPrefix(value, "media/") && !strings.Contains(value, "..") &&
		!strings.Contains(value, "\\") && !strings.HasPrefix(value, "/")
}

func documentMediaIDs(raw json.RawMessage) []string {
	var document editor.Node
	if json.Unmarshal(raw, &document) != nil || document.Type != "doc" {
		return nil
	}
	result := make([]string, 0)
	var visit func(editor.Node)
	visit = func(node editor.Node) {
		if node.Type == "imageBlock" {
			if assetID, ok := node.Attrs["assetId"].(string); ok && assetID != "" {
				result = append(result, assetID)
			}
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(document)
	return result
}

func remapIDs(ids []string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = uuid.NewString()
	}
	return result
}

func transferNodeIDs(items []ProjectTransferNode) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferTermIDs(items []ProjectTransferDictionaryTerm) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferSessionIDs(items []ProjectTransferDictationSession) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferArtifactIDs(items []ProjectTransferArtifact) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferMemoryIDs(items []ProjectTransferMemoryRecord) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferMediaIDs(items []ProjectTransferMedia) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func transferCanvasIDs(items []ProjectTransferCanvas) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func remapCanvasResources(
	raw json.RawMessage,
	nodeIDs map[string]string,
	mediaIDs map[string]string,
) (json.RawMessage, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("%w: invalid canvas content", ErrInvalidProjectTransfer)
	}
	nodes, _ := document["nodes"].([]any)
	for _, value := range nodes {
		node, _ := value.(map[string]any)
		if node["type"] != "file" {
			continue
		}
		extension, _ := node["x-writers-studio"].(map[string]any)
		resource, _ := extension["resource"].(map[string]any)
		kind, _ := resource["kind"].(string)
		id, _ := resource["id"].(string)
		var mapped, prefix string
		var exists bool
		switch kind {
		case "document":
			mapped, exists = nodeIDs[id]
			prefix = "writers-studio://documents/"
		case "media":
			mapped, exists = mediaIDs[id]
			prefix = "writers-studio://media/"
		default:
			return nil, fmt.Errorf("%w: invalid canvas resource kind", ErrInvalidProjectTransfer)
		}
		if !exists {
			return nil, fmt.Errorf("%w: canvas resource is missing", ErrInvalidProjectTransfer)
		}
		resource["id"] = mapped
		node["file"] = prefix + mapped
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func remapDocumentMedia(raw json.RawMessage, mediaIDs map[string]string) (json.RawMessage, error) {
	if len(raw) == 0 || len(mediaIDs) == 0 {
		return raw, nil
	}
	var document editor.Node
	if err := json.Unmarshal(raw, &document); err != nil || document.Type != "doc" {
		return raw, nil
	}
	var remap func(*editor.Node) error
	remap = func(node *editor.Node) error {
		if node.Type == "imageBlock" {
			assetID, _ := node.Attrs["assetId"].(string)
			if assetID != "" {
				mapped, exists := mediaIDs[assetID]
				if !exists {
					return fmt.Errorf("%w: image asset is missing", ErrInvalidProjectTransfer)
				}
				node.Attrs["assetId"] = mapped
				node.Attrs["src"] = "/api/media/" + mapped + "/content"
			}
		}
		for index := range node.Content {
			if err := remap(&node.Content[index]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := remap(&document); err != nil {
		return nil, err
	}
	return json.Marshal(document)
}

func remapDocumentLinks(
	raw json.RawMessage,
	sourceProjectID string,
	targetProjectID string,
	nodeIDs map[string]string,
) (json.RawMessage, error) {
	if len(raw) == 0 || len(nodeIDs) == 0 {
		return raw, nil
	}
	var document editor.Node
	if err := json.Unmarshal(raw, &document); err != nil || document.Type != "doc" {
		return raw, nil
	}
	changed := false
	var visit func(*editor.Node) error
	visit = func(node *editor.Node) error {
		for index := range node.Marks {
			if node.Marks[index].Type != "link" {
				continue
			}
			href, _ := node.Marks[index].Attrs["href"].(string)
			parsed, err := url.Parse(href)
			if err != nil {
				continue
			}
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) != 4 || parts[0] != "projects" || parts[1] != sourceProjectID ||
				parts[2] != "studio" {
				continue
			}
			mapped, exists := nodeIDs[parts[3]]
			if !exists {
				return fmt.Errorf("%w: internal document link is missing", ErrInvalidProjectTransfer)
			}
			mappedHref := "/projects/" + targetProjectID + "/studio/" + mapped
			if parsed.Fragment != "" {
				mappedHref += "#" + url.PathEscape(parsed.Fragment)
			}
			node.Marks[index].Attrs["href"] = mappedHref
			changed = true
		}
		for index := range node.Content {
			if err := visit(&node.Content[index]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(&document); err != nil {
		return nil, err
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(document)
}

func nodeDepth(node ProjectTransferNode, nodes []ProjectTransferNode) int {
	byID := make(map[string]ProjectTransferNode, len(nodes))
	for _, item := range nodes {
		byID[item.ID] = item
	}
	depth := 0
	cursor := node.ParentID
	for cursor != nil {
		depth++
		parent := byID[*cursor]
		cursor = parent.ParentID
	}
	return depth
}

func normalizedImportedDictationStatus(status string, hasDraft bool) string {
	if status == "trashed" {
		return "trashed"
	}
	if hasDraft {
		return "review_ready"
	}
	return "finalization_failed"
}

func remapSourceReferences(
	references []assistant.SourceReference,
	nodeIDs map[string]string,
) []assistant.SourceReference {
	result := make([]assistant.SourceReference, len(references))
	copy(result, references)
	for index := range result {
		if mapped, ok := nodeIDs[result[index].ID]; ok {
			result[index].ID = mapped
		}
	}
	return result
}

func audioExtension(storageKey string) string {
	index := strings.LastIndex(storageKey, ".")
	if index < 0 || index == len(storageKey)-1 {
		return "webm"
	}
	extension := strings.ToLower(storageKey[index+1:])
	switch extension {
	case "webm", "wav", "mp3", "m4a", "ogg":
		return extension
	}
	return "webm"
}

func audioMediaType(extension string) string {
	switch extension {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "m4a":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	default:
		return "audio/webm"
	}
}

func mediaExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
