package assistant

import "strings"

type ToolAuthority string

const (
	AuthorityRead         ToolAuthority = "A0"
	AuthorityStageLow     ToolAuthority = "A1"
	AuthorityStageHigh    ToolAuthority = "A2"
	AuthorityNativeOnly   ToolAuthority = "A3"
	AuthorityNeverExposed ToolAuthority = "A4"
)

type ToolPolicy struct {
	Name             string        `json:"name"`
	Authority        ToolAuthority `json:"authority"`
	RequiresReview   bool          `json:"requiresReview"`
	RequiresExactRef bool          `json:"requiresExactReference"`
	RiskLevel        RiskLevel     `json:"riskLevel"`
}

var highRiskStagedTools = []string{
	"document.prepare_replace_all",
	"document.prepare_transfer",
	"transcript.prepare_transfer",
	"draft.prepare_transfer",
	"tree.archive",
	"version.restore",
}

var nativeOnlyTools = []string{
	"tree.prepare_archive",
	"version.prepare_restore",
	"transcript.prepare_retranscribe",
	"export.prepare",
}

func ClassifyTool(name string) ToolPolicy {
	name = strings.TrimSpace(name)
	if name == "" {
		return ToolPolicy{Name: name, Authority: AuthorityNeverExposed, RiskLevel: RiskHigh}
	}
	if isReadTool(name) {
		return ToolPolicy{Name: name, Authority: AuthorityRead, RiskLevel: RiskLow}
	}
	for _, toolName := range highRiskStagedTools {
		if name == toolName {
			return ToolPolicy{
				Name:             name,
				Authority:        AuthorityStageHigh,
				RequiresReview:   true,
				RequiresExactRef: true,
				RiskLevel:        RiskHigh,
			}
		}
	}
	for _, toolName := range nativeOnlyTools {
		if name == toolName {
			return ToolPolicy{
				Name:             name,
				Authority:        AuthorityNativeOnly,
				RequiresReview:   true,
				RequiresExactRef: true,
				RiskLevel:        RiskHigh,
			}
		}
	}
	if isStagedMutation(name) {
		return ToolPolicy{
			Name:           name,
			Authority:      AuthorityStageLow,
			RequiresReview: true,
			RiskLevel:      RiskLow,
		}
	}
	return ToolPolicy{Name: name, Authority: AuthorityNeverExposed, RiskLevel: RiskHigh}
}

func isReadTool(name string) bool {
	switch name {
	case "assistant.begin_turn", "assistant.get_recent_change_set",
		"project.get_overview", "project.get_structure",
		"project.list_recent", "project.search",
		"project.get_activity", "tree.list", "tree.get_node", "document.get",
		"document.find_exact", "document.find_exact_batch", "document.get_range",
		"document.list_references", "dictionary.list",
		"dictionary.find", "mentions.find", "annotation.list", "annotation.get", "assistant_guide.get",
		"project_memory.list", "project_memory.search", "project_memory.get",
		"version.list", "version.compare", "dictation.list_sessions",
		"transcript.get", "transcript.get_uncertain", "audio.get_source_range",
		"export.preview", "recovery.get_status", "settings.get_writer_preferences",
		"assistant.get_capabilities":
		return true
	default:
		return false
	}
}

func isStagedMutation(name string) bool {
	switch name {
	case "tree.create_node", "tree.create_nodes_batch", "tree.rename_node", "tree.move_node", "tree.set_status",
		"tree.duplicate_node", "document.insert", "document.replace_range",
		"document.append", "document.append_chunk", "document.set_title", "document.apply_marks",
		"document.create_draft", "note.create", "note.update", "dictionary.add",
		"annotation.propose_fixes",
		"dictionary.add_batch",
		"dictionary.update", "dictionary.set_active", "assistant_guide.propose_create",
		"assistant_guide.propose_update", "project_memory.add", "project_memory.add_batch",
		"project_memory.update",
		"project_memory.set_active", "project_memory.set_always_relevant",
		"version.create_snapshot":
		return true
	default:
		return false
	}
}

func AllowedToolNames() []string {
	return []string{
		"assistant.begin_turn", "assistant.get_recent_change_set",
		"project.get_overview", "project.get_structure", "project.list_recent",
		"project.search", "project.get_activity",
		"tree.list", "tree.get_node", "tree.create_node", "tree.create_nodes_batch",
		"tree.rename_node", "tree.move_node",
		"tree.set_status", "tree.duplicate_node", "tree.prepare_archive",
		"document.get", "document.find_exact", "document.find_exact_batch", "document.get_range",
		"document.list_references", "document.insert",
		"document.replace_range", "document.append", "document.append_chunk", "document.set_title", "document.apply_marks",
		"document.create_draft", "note.create", "note.update", "document.prepare_transfer",
		"annotation.list", "annotation.get", "annotation.propose_fixes",
		"draft.prepare_transfer",
		"document.prepare_replace_all",
		"dictionary.list", "dictionary.find", "dictionary.add", "dictionary.add_batch",
		"dictionary.update",
		"dictionary.set_active", "mentions.find", "assistant_guide.get",
		"assistant_guide.propose_create", "assistant_guide.propose_update",
		"project_memory.list", "project_memory.search", "project_memory.get",
		"project_memory.add", "project_memory.add_batch", "project_memory.update",
		"project_memory.set_active",
		"project_memory.set_always_relevant",
		"version.list", "version.compare", "version.create_snapshot", "version.prepare_restore",
		"dictation.list_sessions", "transcript.get", "transcript.get_uncertain",
		"audio.get_source_range", "transcript.prepare_transfer", "transcript.prepare_retranscribe",
		"export.preview", "export.prepare", "recovery.get_status",
		"settings.get_writer_preferences", "assistant.get_capabilities",
	}
}
