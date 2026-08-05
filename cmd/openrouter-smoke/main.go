package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"writers.studio/internal/stt"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: openrouter-smoke <audio-file>")
		os.Exit(2)
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is required")
		os.Exit(2)
	}
	model := os.Getenv("OPENROUTER_STT_MODEL")
	if model == "" {
		model = "openai/whisper-large-v3"
	}
	audio, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read smoke audio:", err)
		os.Exit(1)
	}
	format := filepath.Ext(os.Args[1])
	if len(format) > 1 {
		format = format[1:]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := stt.NewOpenRouter(apiKey, model).Transcribe(ctx, audio, stt.FinalConfig{
		SessionID: "manual-openrouter-smoke",
		Language:  "ru",
		Format:    format,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "OpenRouter STT smoke failed:", err)
		os.Exit(1)
	}
	timedSegments := 0
	words := 0
	for _, segment := range result.Segments {
		if segment.EndMS > segment.StartMS {
			timedSegments++
		}
		words += len(segment.Words)
	}
	fmt.Printf(
		"OpenRouter STT smoke passed: provider=%s model=%s characters=%d segments=%d timed_segments=%d words=%d\n",
		result.Provider,
		result.Model,
		len([]rune(result.Text)),
		len(result.Segments),
		timedSegments,
		words,
	)
}
