package httpapi

import (
	"context"
	"reflect"
	"testing"

	"writers.studio/internal/stt"
)

func TestCombineSTTCapabilitiesNeverInventsAProvider(t *testing.T) {
	ctx := context.Background()
	unavailable := stt.NewUnavailable().Capabilities(ctx)
	openRouter := stt.NewOpenRouter("key", "model").Capabilities(ctx)
	deepgram := stt.NewDeepgram("key", "model").Capabilities(ctx)
	fake := stt.NewFake().Capabilities(ctx)

	tests := []struct {
		name        string
		final       stt.Capabilities
		live        *stt.Capabilities
		liveAdapter string
		available   bool
		modes       []string
		protocol    string
		reasonCode  string
	}{
		{
			name: "nothing configured", final: unavailable, liveAdapter: "disabled",
			modes: []string{}, protocol: "unavailable", reasonCode: "provider_not_configured",
		},
		{
			name: "file only", final: openRouter, liveAdapter: "disabled", available: true,
			modes: []string{"file"}, protocol: "unavailable",
		},
		{
			name: "OpenRouter chunked live", final: openRouter, liveAdapter: "openrouter",
			available: true, modes: []string{"chunked_live", "file"}, protocol: "wav_chunks",
		},
		{
			name: "Deepgram only", final: unavailable, live: &deepgram, liveAdapter: "deepgram",
			available: true, modes: []string{"realtime"}, protocol: "pcm16",
		},
		{
			name: "Deepgram plus file finalizer", final: openRouter, live: &deepgram,
			liveAdapter: "deepgram", available: true, modes: []string{"realtime", "file"},
			protocol: "pcm16",
		},
		{
			name: "missing Deepgram does not claim realtime", final: openRouter,
			liveAdapter: "deepgram", available: true, modes: []string{"file"},
			protocol: "unavailable",
		},
		{
			name: "explicit gated fixture", final: fake, liveAdapter: "fake", available: true,
			modes: []string{"realtime", "file"}, protocol: "fixture",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := combineSTTCapabilities(test.final, test.live, test.liveAdapter)
			if got.Available != test.available || !reflect.DeepEqual(got.Modes, test.modes) ||
				got.StreamingProtocol != test.protocol || got.ReasonCode != test.reasonCode {
				t.Fatalf("unexpected capabilities: %+v", got)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	if !containsString([]string{"file", "realtime"}, "realtime") {
		t.Fatal("existing mode was not found")
	}
	if containsString([]string{"file"}, "fixture") {
		t.Fatal("missing mode was reported as present")
	}
}
