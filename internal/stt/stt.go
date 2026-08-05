package stt

import (
	"context"

	"writers.studio/internal/domain"
)

type Capabilities struct {
	Available         bool     `json:"available"`
	ReasonCode        string   `json:"reasonCode,omitempty"`
	Modes             []string `json:"modes"`
	Deployment        string   `json:"deployment"`
	StreamingProtocol string   `json:"streamingProtocol"`
	Languages         []string `json:"languages"`
	PartialResults    bool     `json:"partialResults"`
	SegmentTimestamps bool     `json:"segmentTimestamps"`
	WordTimestamps    bool     `json:"wordTimestamps"`
	Confidence        string   `json:"confidence"`
	VocabularyHints   string   `json:"vocabularyHints"`
	SupportsLongAudio bool     `json:"supportsLongAudio"`
}

type FinalConfig struct {
	SessionID      string
	Language       string
	Format         string
	DurationMS     int64
	DictionaryTerm []string
}

type FinalResult struct {
	PipelineVersion string
	Provider        string
	Model           string
	Language        string
	Text            string
	Segments        []domain.TranscriptSegment
}

type FinalTranscriber interface {
	Capabilities(context.Context) Capabilities
	Transcribe(context.Context, []byte, FinalConfig) (FinalResult, error)
}

type LiveConfig struct {
	Language        string
	SampleRate      int
	DictionaryTerms []string
}

type LiveEvent struct {
	Kind           string
	ProviderTurnID string
	Text           string
	StartMS        int64
	EndMS          int64
	Words          []domain.TranscriptWord
	Confidence     *float64
	FromFinalize   bool
}

type LiveSession interface {
	SendAudio(context.Context, []byte) error
	Finalize(context.Context) error
	ReadEvent(context.Context) (LiveEvent, error)
	Close()
}

type LiveTranscriber interface {
	Capabilities(context.Context) Capabilities
	Open(context.Context, LiveConfig) (LiveSession, error)
}
