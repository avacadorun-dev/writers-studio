package assistant

import (
	"strings"
	"testing"
)

func validProvider() ProviderRef {
	return ProviderRef{
		Kind:    ProviderSystem,
		Name:    "System",
		Model:   "writer-model",
		Dialect: DialectResponses,
		Capabilities: CapabilitySnapshot{
			Text: true, StrictTools: true, ToolResults: true, SequentialToolCalls: true,
		},
	}
}

func TestRunValidation(t *testing.T) {
	t.Parallel()
	nodeID := "node"
	canvasID := "canvas"
	tests := []struct {
		name string
		run  Run
		ok   bool
	}{
		{
			name: "project voice run",
			run: Run{
				ProjectID: "project", TargetKind: TargetProject, InputMode: InputVoice,
				SilenceCheckpointMS: 5000, Provider: validProvider(),
			},
			ok: true,
		},
		{
			name: "targeted chat run",
			run: Run{
				ProjectID: "project", TargetKind: TargetNode, TargetNodeID: &nodeID,
				InputMode: InputChat, SilenceCheckpointMS: 0, Provider: validProvider(),
			},
			ok: true,
		},
		{
			name: "canvas chat run",
			run: Run{
				ProjectID: "project", TargetKind: TargetCanvas, TargetCanvasID: &canvasID,
				InputMode: InputChat, SilenceCheckpointMS: 0, Provider: validProvider(),
			},
			ok: true,
		},
		{
			name: "canvas target missing",
			run: Run{
				ProjectID: "project", TargetKind: TargetCanvas,
				InputMode: InputChat, SilenceCheckpointMS: 0, Provider: validProvider(),
			},
		},
		{
			name: "node target missing",
			run: Run{
				ProjectID: "project", TargetKind: TargetNode, InputMode: InputChat,
				SilenceCheckpointMS: 5000, Provider: validProvider(),
			},
		},
		{
			name: "project target has node",
			run: Run{
				ProjectID: "project", TargetKind: TargetProject, TargetNodeID: &nodeID,
				InputMode: InputChat, SilenceCheckpointMS: 5000, Provider: validProvider(),
			},
		},
		{
			name: "invalid checkpoint",
			run: Run{
				ProjectID: "project", TargetKind: TargetProject, InputMode: InputChat,
				SilenceCheckpointMS: 4000, Provider: validProvider(),
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.run.Validate()
			if test.ok && err != nil {
				t.Fatal(err)
			}
			if !test.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestProviderAndCapabilityValidation(t *testing.T) {
	t.Parallel()
	provider := validProvider()
	if !provider.Valid() || !provider.Capabilities.CanRunAssistant() {
		t.Fatal("valid provider was rejected")
	}
	connectionID := "connection"
	provider.Kind = ProviderConnection
	provider.ConnectionID = &connectionID
	provider.Dialect = DialectChatCompletions
	if !provider.Valid() {
		t.Fatal("valid connection was rejected")
	}
	provider.ConnectionID = nil
	if provider.Valid() {
		t.Fatal("connection without id was accepted")
	}
	provider.Capabilities.SequentialToolCalls = false
	if provider.Capabilities.CanRunAssistant() {
		t.Fatal("incomplete capability snapshot was accepted")
	}
	provider = validProvider()
	provider.Name = " "
	if provider.Valid() {
		t.Fatal("provider without a name was accepted")
	}
	provider = validProvider()
	provider.Dialect = DialectAuto
	if provider.Valid() {
		t.Fatal("unresolved provider dialect was accepted")
	}
	provider = validProvider()
	connectionID = " "
	provider.Kind = ProviderConnection
	provider.ConnectionID = &connectionID
	if provider.Valid() {
		t.Fatal("blank connection id was accepted")
	}
	provider = validProvider()
	provider.Kind = "unknown"
	if provider.Valid() {
		t.Fatal("unknown provider kind was accepted")
	}
}

func TestWriterPreferencesSnapshotNormalization(t *testing.T) {
	t.Parallel()
	normalized := (WriterPreferencesSnapshot{
		Language: "de", CommandPrefix: strings.Repeat("я", 50),
		AssistantSilenceCheckpointMS: 4000, FontFamily: "comic",
		FontSize: 99, LineHeight: 3, ManuscriptWidth: 120,
		Theme: "neon", WordGoal: 2_000_000,
	}).Normalized()
	if normalized.Language != "ru" || len([]rune(normalized.CommandPrefix)) != 40 ||
		normalized.AssistantSilenceCheckpointMS != 5000 || normalized.FontFamily != "sofia" ||
		normalized.FontSize != 19 || normalized.LineHeight != 1.75 ||
		normalized.ManuscriptWidth != 70 || normalized.Theme != "system" ||
		normalized.WordGoal != 1_000_000 {
		t.Fatalf("writer preferences were not normalized: %#v", normalized)
	}
	valid := (WriterPreferencesSnapshot{
		Language: "en", CommandPrefix: " Action ", AssistantSilenceCheckpointMS: 0,
		FontFamily: "inter", FontSize: 17, LineHeight: 1.5,
		ManuscriptWidth: 60, Theme: "dark", WordGoal: -1,
	}).Normalized()
	if valid.CommandPrefix != "Action" || valid.AssistantSilenceCheckpointMS != 0 ||
		valid.FontFamily != "inter" || valid.WordGoal != 0 {
		t.Fatalf("valid writer preferences changed unexpectedly: %#v", valid)
	}
}

func TestGuideAndInstructionLimits(t *testing.T) {
	t.Parallel()
	if err := ValidateGuide(strings.Repeat("я", MaxGuideCharacters)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGuide(strings.Repeat("я", MaxGuideCharacters+1)); err == nil {
		t.Fatal("oversized guide was accepted")
	}
	turn := Turn{InputMode: InputVoice, Instruction: "Напиши сцену"}
	if err := turn.ValidateInstruction(); err != nil {
		t.Fatal(err)
	}
	turn.Instruction = ""
	if err := turn.ValidateInstruction(); err == nil {
		t.Fatal("empty instruction was accepted")
	}
	turn.Instruction = strings.Repeat("я", MaxInstructionChars+1)
	if err := turn.ValidateInstruction(); err == nil {
		t.Fatal("oversized instruction was accepted")
	}
	turn.Instruction = "текст"
	turn.InputMode = "unknown"
	if err := turn.ValidateInstruction(); err == nil {
		t.Fatal("unknown input mode was accepted")
	}
}

func TestMemoryValidation(t *testing.T) {
	t.Parallel()
	nodeID := "node"
	valid := MemoryRecord{Kind: MemoryFact, Statement: "Герой боится воды.", Scope: "project"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.Scope = "node"
	valid.ScopeNodeID = &nodeID
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.ScopeNodeID = nil
	if err := valid.Validate(); err == nil {
		t.Fatal("node-scoped memory without node was accepted")
	}
	valid.Scope = "unknown"
	if err := valid.Validate(); err == nil {
		t.Fatal("unknown scope was accepted")
	}
	valid = MemoryRecord{Kind: "unknown", Statement: "Факт", Scope: "project"}
	if err := valid.Validate(); err == nil {
		t.Fatal("unknown memory kind was accepted")
	}
	valid = MemoryRecord{Kind: MemoryFact, Statement: " ", Scope: "project"}
	if err := valid.Validate(); err == nil {
		t.Fatal("empty memory was accepted")
	}
	valid = MemoryRecord{
		Kind: MemoryFact, Statement: strings.Repeat("я", 12_001), Scope: "project",
	}
	if err := valid.Validate(); err == nil {
		t.Fatal("oversized memory was accepted")
	}
	valid = MemoryRecord{
		Kind: MemoryFact, Statement: "Факт", Scope: "project", ScopeNodeID: &nodeID,
	}
	if err := valid.Validate(); err == nil {
		t.Fatal("project memory with node was accepted")
	}
}

func TestChangeSetValidation(t *testing.T) {
	t.Parallel()
	changeSet := ChangeSet{
		Title:           "Обновить сцену",
		BaseFingerprint: strings.Repeat("a", 64),
		Operations: []ChangeOperation{{
			Position: 0, GroupKey: "document", ToolName: "document.append",
			ProposedSnapshot: []byte(`{"text":"x"}`),
		}},
	}
	if err := changeSet.Validate(); err != nil {
		t.Fatal(err)
	}
	revision := 4
	targetID := "scene"
	changeSet.Operations = []ChangeOperation{
		{
			Position: 0, GroupKey: "document", ToolName: "document.replace_range",
			TargetKind: "document", TargetID: &targetID, BaseRevision: &revision,
			ProposedSnapshot: []byte(`{"text":"one"}`),
		},
		{
			Position: 1, GroupKey: "document", ToolName: "document.replace_range",
			TargetKind: "document", TargetID: &targetID, BaseRevision: &revision,
			ProposedSnapshot: []byte(`{"text":"two"}`),
		},
	}
	if err := changeSet.Validate(); err == nil {
		t.Fatal("two optimistic mutations of the same target were accepted")
	}
	otherTarget := "other-scene"
	changeSet.Operations[1].TargetID = &otherTarget
	if err := changeSet.Validate(); err != nil {
		t.Fatalf("independent revisioned targets were rejected: %v", err)
	}
	changeSet.Operations = changeSet.Operations[:1]
	changeSet.Operations[0].Position = 1
	if err := changeSet.Validate(); err == nil {
		t.Fatal("invalid operation order was accepted")
	}
	changeSet.Operations[0].Position = 0
	changeSet.Operations[0].ToolName = ""
	if err := changeSet.Validate(); err == nil {
		t.Fatal("operation without tool was accepted")
	}
	changeSet.Operations[0].ToolName = "document.append"
	changeSet.Operations[0].ProposedSnapshot = nil
	if err := changeSet.Validate(); err == nil {
		t.Fatal("operation without proposal was accepted")
	}
	changeSet.Operations = nil
	if err := changeSet.Validate(); err == nil {
		t.Fatal("empty change set was accepted")
	}
	changeSet.Title = ""
	if err := changeSet.Validate(); err == nil {
		t.Fatal("untitled change set was accepted")
	}
}

func TestToolClassificationCoversCatalog(t *testing.T) {
	t.Parallel()
	names := AllowedToolNames()
	if len(names) < 50 {
		t.Fatalf("tool catalog unexpectedly small: %d", len(names))
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Fatalf("duplicate tool %q", name)
		}
		seen[name] = true
		policy := ClassifyTool(name)
		if policy.Authority == AuthorityNeverExposed {
			t.Fatalf("catalog tool %q has no policy", name)
		}
		if policy.Authority != AuthorityRead && policy.Authority != AuthorityDirectLow &&
			!policy.RequiresReview {
			t.Fatalf("mutation %q does not require review", name)
		}
	}
	if ClassifyTool("secrets.read").Authority != AuthorityNeverExposed {
		t.Fatal("unknown tool was exposed")
	}
	if ClassifyTool(" ").Authority != AuthorityNeverExposed {
		t.Fatal("blank tool was exposed")
	}
	highRisk := ClassifyTool("document.prepare_replace_all")
	if highRisk.Authority != AuthorityStageHigh || !highRisk.RequiresExactRef ||
		highRisk.RiskLevel != RiskHigh {
		t.Fatal("high-risk tool has the wrong policy")
	}
	native := ClassifyTool("export.prepare")
	if native.Authority != AuthorityNativeOnly || !native.RequiresReview {
		t.Fatal("native-only tool has the wrong policy")
	}
	direct := ClassifyTool("project.upload_image")
	if direct.Authority != AuthorityDirectLow || direct.RequiresReview ||
		direct.RiskLevel != RiskLow {
		t.Fatal("direct image upload has the wrong policy")
	}
}

func TestEventHelpers(t *testing.T) {
	t.Parallel()
	if !IsLifecycleEvent(EventRunStarted) || IsLifecycleEvent(EventToolCallStart) {
		t.Fatal("lifecycle event classification is wrong")
	}
	data, err := NewEventData(map[string]string{"status": "ok"})
	if err != nil || string(data) != `{"status":"ok"}` {
		t.Fatalf("unexpected event data: %s, %v", data, err)
	}
}
