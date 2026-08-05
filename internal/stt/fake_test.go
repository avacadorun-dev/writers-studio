package stt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFakeTranscriberIsDeterministicAndTimestamped(t *testing.T) {
	t.Parallel()

	adapter := NewFake()
	first, err := adapter.Transcribe(context.Background(), []byte("audio"), FinalConfig{Language: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Transcribe(context.Background(), []byte("audio"), FinalConfig{Language: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != second.Text {
		t.Fatalf("fake text differs: %q vs %q", first.Text, second.Text)
	}
	if len(first.Segments) < 2 || first.Segments[0].EndMS <= first.Segments[0].StartMS {
		t.Fatalf("fake adapter must provide useful timestamps: %#v", first.Segments)
	}
}

func TestFakeTranscriberExposesDeterministicFailure(t *testing.T) {
	t.Parallel()

	_, err := NewFake().Transcribe(
		context.Background(),
		[]byte("FAIL_STT fixture"),
		FinalConfig{Language: "ru"},
	)
	if !errors.Is(err, ErrFixtureFailure) {
		t.Fatalf("expected fixture failure, got %v", err)
	}
}

func TestFakeTranscriberCanFailOncePerSessionThenRecover(t *testing.T) {
	t.Parallel()

	adapter := NewFake()
	config := FinalConfig{Language: "ru", SessionID: "recoverable-session"}
	if _, err := adapter.Transcribe(
		context.Background(),
		[]byte("FAIL_STT_ONCE fixture"),
		config,
	); !errors.Is(err, ErrFixtureFailure) {
		t.Fatalf("expected first attempt to fail, got %v", err)
	}
	result, err := adapter.Transcribe(
		context.Background(),
		[]byte("FAIL_STT_ONCE fixture"),
		config,
	)
	if err != nil {
		t.Fatalf("expected retry to recover, got %v", err)
	}
	if result.Text == "" {
		t.Fatal("expected recovered transcript")
	}

	if _, err := adapter.Transcribe(
		context.Background(),
		[]byte("FAIL_STT_ONCE fixture"),
		FinalConfig{Language: "ru", SessionID: "another-session"},
	); !errors.Is(err, ErrFixtureFailure) {
		t.Fatalf("expected first attempt in a different session to fail, got %v", err)
	}
}

func TestFakeTranscriberAppliesDictionaryHintDeterministically(t *testing.T) {
	t.Parallel()

	result, err := NewFake().Transcribe(
		context.Background(),
		[]byte("audio"),
		FinalConfig{
			Language:       "ru",
			DictionaryTerm: []string{"Домоводье"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "Домоводье остановилась") {
		t.Fatalf("dictionary hint was not reflected in fixture output: %q", result.Text)
	}
}
