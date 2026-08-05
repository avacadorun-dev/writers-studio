package assistant

import (
	"encoding/json"
	"time"
)

// EventType follows AG-UI's lifecycle and activity vocabulary while keeping
// Writers Studio's durable project-change events explicit.
type EventType string

const (
	EventRunStarted          EventType = "RUN_STARTED"
	EventRunFinished         EventType = "RUN_FINISHED"
	EventRunError            EventType = "RUN_ERROR"
	EventStepStarted         EventType = "STEP_STARTED"
	EventStepFinished        EventType = "STEP_FINISHED"
	EventTextMessageStart    EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent  EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd      EventType = "TEXT_MESSAGE_END"
	EventToolCallStart       EventType = "TOOL_CALL_START"
	EventToolCallArgs        EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd         EventType = "TOOL_CALL_END"
	EventToolCallResult      EventType = "TOOL_CALL_RESULT"
	EventStateSnapshot       EventType = "STATE_SNAPSHOT"
	EventStateDelta          EventType = "STATE_DELTA"
	EventActivitySnapshot    EventType = "ACTIVITY_SNAPSHOT"
	EventActivityDelta       EventType = "ACTIVITY_DELTA"
	EventMessagesSnapshot    EventType = "MESSAGES_SNAPSHOT"
	EventRaw                 EventType = "RAW"
	EventTurnCheckpoint      EventType = "writers.turn.checkpoint"
	EventTurnModeEscalated   EventType = "writers.turn.mode_escalated"
	EventChangeSetStaged     EventType = "writers.change_set.staged"
	EventChangeSetApplied    EventType = "writers.change_set.applied"
	EventChangeSetRejected   EventType = "writers.change_set.rejected"
	EventChangeSetStale      EventType = "writers.change_set.stale"
	EventChangeSetUndone     EventType = "writers.change_set.undone"
	EventProposalUpdated     EventType = "writers.proposal.updated"
	EventVoiceTranscriptPart EventType = "writers.voice.transcript"
	EventModeChanged         EventType = "writers.input_mode.changed"
)

type Event struct {
	ID        string          `json:"id"`
	RunID     string          `json:"runId"`
	TurnID    *string         `json:"turnId,omitempty"`
	Sequence  int64           `json:"sequence"`
	Type      EventType       `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
}

func NewEventData(value any) (json.RawMessage, error) {
	return json.Marshal(value)
}

func IsLifecycleEvent(eventType EventType) bool {
	switch eventType {
	case EventRunStarted, EventRunFinished, EventRunError, EventStepStarted, EventStepFinished:
		return true
	default:
		return false
	}
}
