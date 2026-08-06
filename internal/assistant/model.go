package assistant

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	MaxGuideCharacters    = 24_000
	MaxInstructionChars   = 32_000
	MaxCommandPrefixRunes = 40
)

var (
	ErrInvalidRun         = errors.New("invalid assistant run")
	ErrInvalidTurn        = errors.New("invalid assistant turn")
	ErrInvalidMemory      = errors.New("invalid project memory")
	ErrInvalidChangeSet   = errors.New("invalid assistant change set")
	ErrConfirmationNeeded = errors.New("explicit confirmation is required")
	ErrStaleChangeSet     = errors.New("assistant change set is stale")
	ErrRunBusy            = errors.New("assistant run is working")
)

type ProviderDialect string

const (
	DialectResponses       ProviderDialect = "responses"
	DialectChatCompletions ProviderDialect = "chat_completions"
	DialectAuto            ProviderDialect = "auto"
	DialectMCP             ProviderDialect = "mcp"
)

type ProviderKind string

const (
	ProviderSystem     ProviderKind = "system"
	ProviderConnection ProviderKind = "connection"
	ProviderMCP        ProviderKind = "mcp"
)

type InputMode string

const (
	InputVoice InputMode = "voice"
	InputChat  InputMode = "chat"
	InputMCP   InputMode = "mcp"
)

type RunOrigin string

const (
	RunOriginInternal  RunOrigin = "internal"
	RunOriginRemoteMCP RunOrigin = "remote_mcp"
)

type AgentRef struct {
	GrantID    string `json:"grantId"`
	ClientName string `json:"clientName"`
}

// WriterPreferencesSnapshot contains only non-secret preferences that can help
// the assistant interpret the current run. Hardware identifiers, credentials,
// privacy consent and account controls deliberately do not belong here.
type WriterPreferencesSnapshot struct {
	Language                     string  `json:"language"`
	CommandPrefix                string  `json:"commandPrefix"`
	CommandsEnabled              bool    `json:"commandsEnabled"`
	LiveTranscript               bool    `json:"liveTranscript"`
	AutoPlaySource               bool    `json:"autoPlaySource"`
	AssistantSilenceCheckpointMS int     `json:"assistantSilenceCheckpointMs"`
	FontFamily                   string  `json:"fontFamily"`
	FontSize                     int     `json:"fontSize"`
	LineHeight                   float64 `json:"lineHeight"`
	ManuscriptWidth              int     `json:"manuscriptWidth"`
	Theme                        string  `json:"theme"`
	Spellcheck                   bool    `json:"spellcheck"`
	WordGoal                     int     `json:"wordGoal"`
}

func (p WriterPreferencesSnapshot) Normalized() WriterPreferencesSnapshot {
	if p.Language != "ru" && p.Language != "en" {
		p.Language = "ru"
	}
	p.CommandPrefix = strings.TrimSpace(p.CommandPrefix)
	commandPrefixRunes := []rune(p.CommandPrefix)
	if len(commandPrefixRunes) > MaxCommandPrefixRunes {
		p.CommandPrefix = string(commandPrefixRunes[:MaxCommandPrefixRunes])
	}
	if p.CommandPrefix == "" {
		p.CommandPrefix = "Команда"
	}
	switch p.AssistantSilenceCheckpointMS {
	case 0, 3000, 5000, 8000:
	default:
		p.AssistantSilenceCheckpointMS = 5000
	}
	switch p.FontFamily {
	case "sofia", "inter", "source-serif":
	default:
		p.FontFamily = "sofia"
	}
	switch p.FontSize {
	case 17, 19, 21:
	default:
		p.FontSize = 19
	}
	switch p.LineHeight {
	case 1.5, 1.75, 2:
	default:
		p.LineHeight = 1.75
	}
	switch p.ManuscriptWidth {
	case 60, 70, 80:
	default:
		p.ManuscriptWidth = 70
	}
	switch p.Theme {
	case "light", "dark", "system":
	default:
		p.Theme = "system"
	}
	if p.WordGoal < 0 {
		p.WordGoal = 0
	} else if p.WordGoal > 1_000_000 {
		p.WordGoal = 1_000_000
	}
	return p
}

type TargetKind string

const (
	TargetProject TargetKind = "project"
	TargetNode    TargetKind = "node"
	TargetCanvas  TargetKind = "canvas"
)

type RunStatus string

const (
	RunActive         RunStatus = "active"
	RunWorking        RunStatus = "working"
	RunAwaitingReview RunStatus = "awaiting_review"
	RunCompleted      RunStatus = "completed"
	RunCancelled      RunStatus = "cancelled"
	RunFailed         RunStatus = "failed"
)

type TurnState string

const (
	TurnCollecting           TurnState = "collecting"
	TurnAwaitingConfirmation TurnState = "awaiting_confirmation"
	TurnQueued               TurnState = "queued"
	TurnWorking              TurnState = "working"
	TurnAwaitingReview       TurnState = "awaiting_review"
	TurnCompleted            TurnState = "completed"
	TurnFailed               TurnState = "failed"
	TurnCancelled            TurnState = "cancelled"
)

type CapabilitySnapshot struct {
	Text                 bool     `json:"text"`
	StrictTools          bool     `json:"strictTools"`
	ToolResults          bool     `json:"toolResults"`
	SequentialToolCalls  bool     `json:"sequentialToolCalls"`
	Streaming            bool     `json:"streaming"`
	ParallelToolCalls    bool     `json:"parallelToolCalls"`
	SupportedDialects    []string `json:"supportedDialects,omitempty"`
	Limitations          []string `json:"limitations,omitempty"`
	CheckedWithModel     string   `json:"checkedWithModel,omitempty"`
	ProbeProtocolVersion string   `json:"probeProtocolVersion,omitempty"`
}

func (c CapabilitySnapshot) CanRunAssistant() bool {
	return c.Text && c.StrictTools && c.ToolResults && c.SequentialToolCalls
}

type ProviderRef struct {
	Kind         ProviderKind       `json:"kind"`
	ConnectionID *string            `json:"connectionId,omitempty"`
	Name         string             `json:"name"`
	Model        string             `json:"model"`
	Dialect      ProviderDialect    `json:"dialect"`
	Capabilities CapabilitySnapshot `json:"capabilities"`
}

func (p ProviderRef) Valid() bool {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Model) == "" {
		return false
	}
	if p.Kind == ProviderMCP {
		return p.ConnectionID == nil && p.Dialect == DialectMCP
	}
	if p.Dialect != DialectResponses && p.Dialect != DialectChatCompletions {
		return false
	}
	if p.Kind == ProviderSystem {
		return p.ConnectionID == nil
	}
	return p.Kind == ProviderConnection && p.ConnectionID != nil &&
		strings.TrimSpace(*p.ConnectionID) != ""
}

type Run struct {
	ID                  string              `json:"id"`
	OwnerUserID         string              `json:"-"`
	ProjectID           string              `json:"projectId"`
	Title               string              `json:"title"`
	TargetKind          TargetKind          `json:"targetKind"`
	TargetNodeID        *string             `json:"targetNodeId,omitempty"`
	TargetCanvasID      *string             `json:"targetCanvasId,omitempty"`
	InputMode           InputMode           `json:"inputMode"`
	Status              RunStatus           `json:"status"`
	SilenceCheckpointMS int                 `json:"silenceCheckpointMs"`
	Origin              RunOrigin           `json:"origin"`
	Agent               *AgentRef           `json:"agent,omitempty"`
	Provider            ProviderRef         `json:"provider"`
	GuideRevision       *int                `json:"guideRevision,omitempty"`
	ReadContext         map[string]any      `json:"readContext"`
	ErrorCode           *string             `json:"errorCode,omitempty"`
	StartedAt           time.Time           `json:"startedAt"`
	UpdatedAt           time.Time           `json:"updatedAt"`
	LastViewedAt        time.Time           `json:"lastViewedAt"`
	Unread              bool                `json:"unread"`
	EndedAt             *time.Time          `json:"endedAt,omitempty"`
	LatestTurn          *Turn               `json:"latestTurn,omitempty"`
	PendingChangeSet    *ChangeSet          `json:"pendingChangeSet,omitempty"`
	LatestChangeSet     *ChangeSet          `json:"latestChangeSet,omitempty"`
	Activity            []Activity          `json:"activity,omitempty"`
	Capabilities        AssistantCapability `json:"assistantCapabilities"`
}

func (r Run) Validate() error {
	if r.Origin == "" {
		r.Origin = RunOriginInternal
	}
	if strings.TrimSpace(r.ProjectID) == "" || !r.Provider.Valid() {
		return ErrInvalidRun
	}
	if len([]rune(strings.TrimSpace(r.Title))) > 120 {
		return ErrInvalidRun
	}
	if r.TargetKind == TargetProject {
		if r.TargetNodeID != nil || r.TargetCanvasID != nil {
			return ErrInvalidRun
		}
	} else if r.TargetKind == TargetNode {
		if r.TargetNodeID == nil || strings.TrimSpace(*r.TargetNodeID) == "" || r.TargetCanvasID != nil {
			return ErrInvalidRun
		}
	} else if r.TargetKind == TargetCanvas {
		if r.TargetCanvasID == nil || strings.TrimSpace(*r.TargetCanvasID) == "" || r.TargetNodeID != nil {
			return ErrInvalidRun
		}
	} else {
		return ErrInvalidRun
	}
	if r.InputMode != InputVoice && r.InputMode != InputChat && r.InputMode != InputMCP {
		return ErrInvalidRun
	}
	if r.Origin != RunOriginInternal && r.Origin != RunOriginRemoteMCP {
		return ErrInvalidRun
	}
	if r.Origin == RunOriginRemoteMCP {
		if r.InputMode != InputMCP || r.Provider.Kind != ProviderMCP || r.Agent == nil ||
			strings.TrimSpace(r.Agent.GrantID) == "" || strings.TrimSpace(r.Agent.ClientName) == "" {
			return ErrInvalidRun
		}
	} else if r.Agent != nil {
		return ErrInvalidRun
	}
	switch r.SilenceCheckpointMS {
	case 0, 3000, 5000, 8000:
	default:
		return ErrInvalidRun
	}
	return nil
}

type Turn struct {
	ID                  string               `json:"id"`
	RunID               string               `json:"runId"`
	Sequence            int                  `json:"sequence"`
	InputMode           InputMode            `json:"inputMode"`
	State               TurnState            `json:"state"`
	Transcript          string               `json:"transcript"`
	Instruction         string               `json:"instruction"`
	AssistantText       string               `json:"assistantText"`
	SelectionAttachment *SelectionAttachment `json:"selectionAttachment,omitempty"`
	GuideRevision       *int                 `json:"guideRevision,omitempty"`
	ProviderResponseID  string               `json:"providerResponseId,omitempty"`
	ErrorCode           *string              `json:"errorCode,omitempty"`
	CreatedAt           time.Time            `json:"createdAt"`
	ConfirmedAt         *time.Time           `json:"confirmedAt,omitempty"`
	StartedAt           *time.Time           `json:"startedAt,omitempty"`
	CompletedAt         *time.Time           `json:"completedAt,omitempty"`
}

func (t Turn) ValidateInstruction() error {
	instruction := strings.TrimSpace(t.Instruction)
	if instruction == "" || len([]rune(instruction)) > MaxInstructionChars {
		return ErrInvalidTurn
	}
	if t.InputMode != InputVoice && t.InputMode != InputChat && t.InputMode != InputMCP {
		return ErrInvalidTurn
	}
	return nil
}

type Guide struct {
	ProjectID string          `json:"projectId"`
	Revision  int             `json:"revision"`
	Content   string          `json:"content"`
	Source    string          `json:"source"`
	History   []GuideRevision `json:"history,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type GuideRevision struct {
	ID                   string    `json:"id"`
	Revision             int       `json:"revision"`
	Content              string    `json:"content"`
	Source               string    `json:"source"`
	RestoredFromRevision *int      `json:"restoredFromRevision,omitempty"`
	AssistantRunID       *string   `json:"assistantRunId,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
}

func ValidateGuide(content string) error {
	if len([]rune(content)) > MaxGuideCharacters {
		return errors.New("assistant guide exceeds 24000 characters")
	}
	return nil
}

type MemoryKind string

const (
	MemoryFact       MemoryKind = "fact"
	MemoryCharacter  MemoryKind = "character"
	MemoryWorldRule  MemoryKind = "world_rule"
	MemoryPlot       MemoryKind = "plot"
	MemoryStyle      MemoryKind = "style"
	MemoryConstraint MemoryKind = "constraint"
	MemoryDecision   MemoryKind = "decision"
	MemoryOther      MemoryKind = "other"
)

type MemoryRecord struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"projectId"`
	Kind             MemoryKind        `json:"kind"`
	Statement        string            `json:"statement"`
	Scope            string            `json:"scope"`
	ScopeNodeID      *string           `json:"scopeNodeId,omitempty"`
	SourceReferences []SourceReference `json:"sourceReferences"`
	Tags             []string          `json:"tags"`
	Active           bool              `json:"active"`
	AlwaysRelevant   bool              `json:"alwaysRelevant"`
	Revision         int               `json:"revision"`
	Source           string            `json:"source"`
	OperationID      *string           `json:"operationId,omitempty"`
	AssistantRunID   *string           `json:"assistantRunId,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

type MemoryRevision struct {
	ID             string       `json:"id"`
	MemoryID       string       `json:"memoryId"`
	Revision       int          `json:"revision"`
	Snapshot       MemoryRecord `json:"snapshot"`
	Source         string       `json:"source"`
	OperationID    *string      `json:"operationId,omitempty"`
	AssistantRunID *string      `json:"assistantRunId,omitempty"`
	CreatedAt      time.Time    `json:"createdAt"`
}

type SourceReference struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Quote string `json:"quote,omitempty"`
	Start *int   `json:"start,omitempty"`
	End   *int   `json:"end,omitempty"`
}

func (m MemoryRecord) Validate() error {
	length := len([]rune(strings.TrimSpace(m.Statement)))
	if length == 0 || length > 12_000 {
		return ErrInvalidMemory
	}
	switch m.Kind {
	case MemoryFact, MemoryCharacter, MemoryWorldRule, MemoryPlot, MemoryStyle,
		MemoryConstraint, MemoryDecision, MemoryOther:
	default:
		return ErrInvalidMemory
	}
	if m.Scope == "project" {
		if m.ScopeNodeID != nil {
			return ErrInvalidMemory
		}
	} else if m.Scope == "node" {
		if m.ScopeNodeID == nil || strings.TrimSpace(*m.ScopeNodeID) == "" {
			return ErrInvalidMemory
		}
	} else {
		return ErrInvalidMemory
	}
	return nil
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ChangeSetState string

const (
	ChangeStaged   ChangeSetState = "staged"
	ChangeApplying ChangeSetState = "applying"
	ChangeApplied  ChangeSetState = "applied"
	ChangeRejected ChangeSetState = "rejected"
	ChangeStale    ChangeSetState = "stale"
	ChangeFailed   ChangeSetState = "failed"
	ChangeUndone   ChangeSetState = "undone"
)

type ChangeSet struct {
	ID              string            `json:"id"`
	RunID           string            `json:"runId"`
	TurnID          string            `json:"turnId"`
	State           ChangeSetState    `json:"state"`
	Title           string            `json:"title"`
	Summary         string            `json:"summary"`
	RiskLevel       RiskLevel         `json:"riskLevel"`
	BaseFingerprint string            `json:"baseFingerprint"`
	OperationID     *string           `json:"operationId,omitempty"`
	Operations      []ChangeOperation `json:"operations"`
	AppliedAt       *time.Time        `json:"appliedAt,omitempty"`
	RejectedAt      *time.Time        `json:"rejectedAt,omitempty"`
	UndoneAt        *time.Time        `json:"undoneAt,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type ChangeOperation struct {
	ID               string          `json:"id"`
	Position         int             `json:"position"`
	GroupKey         string          `json:"groupKey"`
	Selected         bool            `json:"selected"`
	Dependencies     []string        `json:"dependencies"`
	ToolName         string          `json:"toolName"`
	TargetKind       string          `json:"targetKind"`
	TargetID         *string         `json:"targetId,omitempty"`
	BaseRevision     *int            `json:"baseRevision,omitempty"`
	Arguments        json.RawMessage `json:"arguments"`
	BeforeSnapshot   json.RawMessage `json:"beforeSnapshot,omitempty"`
	ProposedSnapshot json.RawMessage `json:"proposedSnapshot"`
	Trace            json.RawMessage `json:"trace,omitempty"`
	RiskLevel        RiskLevel       `json:"riskLevel"`
	Applied          bool            `json:"applied"`
}

func (c ChangeSet) Validate() error {
	if strings.TrimSpace(c.Title) == "" || len(c.BaseFingerprint) != 64 ||
		len(c.Operations) == 0 {
		return ErrInvalidChangeSet
	}
	revisionedTargets := make(map[string]bool)
	for index, operation := range c.Operations {
		if operation.Position != index || strings.TrimSpace(operation.ToolName) == "" ||
			strings.TrimSpace(operation.GroupKey) == "" || len(operation.ProposedSnapshot) == 0 {
			return ErrInvalidChangeSet
		}
		if key := operation.RevisionedTargetKey(); key != "" {
			if revisionedTargets[key] {
				return ErrInvalidChangeSet
			}
			revisionedTargets[key] = true
		}
	}
	return nil
}

// RevisionedTargetKey identifies canonical resources whose optimistic revision
// is consumed by Apply. Two operations with the same key cannot both be based
// on the same canonical snapshot; callers must merge them into one operation.
func (c ChangeOperation) RevisionedTargetKey() string {
	if c.BaseRevision == nil || strings.TrimSpace(c.TargetKind) == "" {
		return ""
	}
	if c.TargetID == nil {
		return c.TargetKind
	}
	targetID := strings.TrimSpace(*c.TargetID)
	if targetID == "" {
		return ""
	}
	return c.TargetKind + ":" + targetID
}

type TextRange struct {
	BlockID string `json:"blockId"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Reason  string `json:"reason"`
}

// SelectionAttachment is an author-created, revision-pinned reference to text
// in the manuscript. The quote is presentation/context data; block/range
// anchors remain the source of truth for provider context and validation.
type SelectionAttachment struct {
	NodeID       string           `json:"nodeId"`
	BaseRevision int              `json:"baseRevision"`
	Ranges       []SelectionRange `json:"ranges"`
	Quote        string           `json:"quote"`
}

type SelectionRange struct {
	BlockID      string `json:"blockId"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	ExpectedText string `json:"expectedText"`
}

func (s SelectionAttachment) Validate() error {
	quoteLength := len([]rune(strings.TrimSpace(s.Quote)))
	if strings.TrimSpace(s.NodeID) == "" || s.BaseRevision < 0 || quoteLength == 0 ||
		quoteLength > 8_000 || len(s.Ranges) == 0 || len(s.Ranges) > 64 {
		return errors.New("invalid selection attachment")
	}
	for _, textRange := range s.Ranges {
		if strings.TrimSpace(textRange.BlockID) == "" || textRange.Start < 0 ||
			textRange.End <= textRange.Start ||
			len([]rune(textRange.ExpectedText)) != textRange.End-textRange.Start {
			return errors.New("invalid selection attachment range")
		}
	}
	return nil
}

type ResolvedTarget struct {
	NodeID          string      `json:"nodeId"`
	BaseRevision    int         `json:"baseRevision"`
	Ranges          []TextRange `json:"ranges"`
	ProtectedRanges []TextRange `json:"protectedRanges"`
	SourcePhrase    string      `json:"sourcePhrase"`
}

func (r ResolvedTarget) Validate() error {
	if strings.TrimSpace(r.NodeID) == "" || r.BaseRevision < 0 ||
		strings.TrimSpace(r.SourcePhrase) == "" || len(r.Ranges) == 0 {
		return errors.New("invalid resolved target")
	}
	for _, textRange := range append(
		append([]TextRange(nil), r.Ranges...),
		r.ProtectedRanges...,
	) {
		if strings.TrimSpace(textRange.BlockID) == "" || textRange.Start < 0 ||
			textRange.End < textRange.Start {
			return errors.New("invalid resolved text range")
		}
	}
	return nil
}

type Activity struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Label     string          `json:"label"`
	Status    string          `json:"status"`
	ToolName  string          `json:"toolName,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	StartedAt time.Time       `json:"startedAt"`
	EndedAt   *time.Time      `json:"endedAt,omitempty"`
}

type AssistantCapability struct {
	VoiceAvailable       bool `json:"voiceAvailable"`
	ChatAvailable        bool `json:"chatAvailable"`
	ProviderAvailable    bool `json:"providerAvailable"`
	GuideAvailable       bool `json:"guideAvailable"`
	MemoryAvailable      bool `json:"memoryAvailable"`
	ChangeReviewRequired bool `json:"changeReviewRequired"`
}
