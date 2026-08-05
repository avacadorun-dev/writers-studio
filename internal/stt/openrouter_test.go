package stt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestOpenRouterDoesNotExposeProviderBodyOrCredential(t *testing.T) {
	t.Parallel()

	const credential = "private-test-credential"
	const providerSecret = "provider echoed sensitive payload"
	adapter := NewOpenRouter(credential, "test/model")
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") != "Bearer "+credential {
				return nil, errors.New("missing authorization")
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(providerSecret)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err := adapter.Transcribe(context.Background(), []byte("audio"), FinalConfig{
		Language: "ru",
		Format:   "wav",
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), providerSecret) {
		t.Fatalf("sensitive value escaped sanitization: %v", err)
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected actionable status code, got %v", err)
	}
}

func TestOpenRouterDecodesSuccessfulTranscription(t *testing.T) {
	t.Parallel()

	adapter := NewOpenRouter("test", "test/model")
	var prompt string
	var responseFormat string
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			var input openRouterRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				return nil, err
			}
			prompt = input.Prompt
			responseFormat = input.ResponseFormat
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"text":" Дом стоял у самой воды. "}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
	}

	result, err := adapter.Transcribe(context.Background(), []byte("audio"), FinalConfig{
		Language:       "ru",
		Format:         "wav",
		DurationMS:     2_300,
		DictionaryTerm: []string{"Домоводье", "Анна Сергеевна"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "openrouter" || result.Text != "Дом стоял у самой воды." {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Segments) != 1 || result.Segments[0].Text != result.Text {
		t.Fatalf("expected one normalized segment: %#v", result.Segments)
	}
	if result.Segments[0].EndMS != 2_300 {
		t.Fatalf("expected configured duration fallback: %#v", result.Segments[0])
	}
	if responseFormat != "verbose_json" {
		t.Fatalf("expected timestamp-capable response format, got %q", responseFormat)
	}
	if !strings.Contains(prompt, "Домоводье") || !strings.Contains(prompt, "Анна Сергеевна") {
		t.Fatalf("dictionary terms were not sent as provider hints: %q", prompt)
	}
}

func TestOpenRouterNormalizesVerboseTimestamps(t *testing.T) {
	t.Parallel()

	adapter := NewOpenRouter("test", "test/model")
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"text":"Первая фраза. Вторая фраза.",
					"duration":2.5,
					"segments":[
						{"text":" Первая фраза.","start":0.1,"end":1.2},
						{"text":" Вторая фраза.","start":1.2,"end":2.4}
					],
					"words":[
						{"word":"Первая","start":0.1,"end":0.6},
						{"word":"фраза","start":0.7,"end":1.1},
						{"word":"Вторая","start":1.2,"end":1.7},
						{"word":"фраза","start":1.8,"end":2.3}
					]
				}`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	result, err := adapter.Transcribe(
		context.Background(),
		[]byte("audio"),
		FinalConfig{Language: "ru", Format: "wav"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("expected two segments: %#v", result.Segments)
	}
	if result.Segments[0].StartMS != 100 ||
		result.Segments[0].EndMS != 1_200 ||
		len(result.Segments[0].Words) != 2 {
		t.Fatalf("unexpected first segment timestamps: %#v", result.Segments[0])
	}
	if result.Segments[1].StartMS != 1_200 ||
		result.Segments[1].EndMS != 2_400 ||
		len(result.Segments[1].Words) != 2 {
		t.Fatalf("unexpected second segment timestamps: %#v", result.Segments[1])
	}
}

func TestOpenRouterRetriesTransientProviderFailure(t *testing.T) {
	t.Parallel()

	adapter := NewOpenRouter("test", "test/model")
	attempts := 0
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("temporary provider failure")),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"text":"Распознано.","duration":1}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	result, err := adapter.Transcribe(
		context.Background(),
		[]byte("audio"),
		FinalConfig{Language: "ru", Format: "wav"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || result.Text != "Распознано." {
		t.Fatalf("unexpected retry result: attempts=%d result=%#v", attempts, result)
	}
}

func TestOpenRouterAdvertisesChunkedLiveMode(t *testing.T) {
	t.Parallel()

	capabilities := NewOpenRouter("test", "test/model").Capabilities(context.Background())
	if len(capabilities.Modes) != 2 ||
		capabilities.Modes[0] != "chunked_live" ||
		capabilities.Modes[1] != "file" {
		t.Fatalf("unexpected modes: %#v", capabilities.Modes)
	}
	if !capabilities.SegmentTimestamps || !capabilities.WordTimestamps {
		t.Fatalf("expected timestamp capabilities: %#v", capabilities)
	}
}
