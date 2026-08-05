package stt

import (
	"context"
	"errors"
	"testing"
)

func TestUnavailableReportsNoCapabilities(t *testing.T) {
	adapter := NewUnavailable()
	capabilities := adapter.Capabilities(context.Background())
	if capabilities.Available || len(capabilities.Modes) != 0 {
		t.Fatalf("unavailable adapter exposed modes: %+v", capabilities)
	}
	if capabilities.ReasonCode != "provider_not_configured" ||
		capabilities.StreamingProtocol != "unavailable" {
		t.Fatalf("unavailable reason is not actionable: %+v", capabilities)
	}

	_, err := adapter.Transcribe(context.Background(), []byte("audio"), FinalConfig{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}
