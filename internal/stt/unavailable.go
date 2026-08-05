package stt

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("speech recognition provider is not configured")

type Unavailable struct{}

func NewUnavailable() *Unavailable {
	return &Unavailable{}
}

func (*Unavailable) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Available:         false,
		ReasonCode:        "provider_not_configured",
		Modes:             []string{},
		Deployment:        "unavailable",
		StreamingProtocol: "unavailable",
		Languages:         []string{},
		Confidence:        "none",
		VocabularyHints:   "none",
	}
}

func (*Unavailable) Transcribe(
	context.Context,
	[]byte,
	FinalConfig,
) (FinalResult, error) {
	return FinalResult{}, ErrUnavailable
}
