package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

const openRouterTranscriptionsURL = "https://openrouter.ai/api/v1/audio/transcriptions"

type OpenRouter struct {
	apiKey string
	model  string
	client *http.Client
}

func NewOpenRouter(apiKey, model string) *OpenRouter {
	return &OpenRouter{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (o *OpenRouter) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Available:         true,
		Modes:             []string{"chunked_live", "file"},
		Deployment:        "cloud",
		StreamingProtocol: "wav_chunks",
		Languages:         []string{"ru", "en"},
		PartialResults:    false,
		SegmentTimestamps: true,
		WordTimestamps:    true,
		Confidence:        "none",
		VocabularyHints:   "prompt",
		SupportsLongAudio: true,
	}
}

type openRouterRequest struct {
	Model                  string               `json:"model"`
	InputAudio             openRouterInputAudio `json:"input_audio"`
	Language               string               `json:"language,omitempty"`
	Prompt                 string               `json:"prompt,omitempty"`
	Temperature            float64              `json:"temperature"`
	ResponseFormat         string               `json:"response_format,omitempty"`
	TimestampGranularities []string             `json:"timestamp_granularities,omitempty"`
}

type openRouterInputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type openRouterResponse struct {
	Text     string                  `json:"text"`
	Duration float64                 `json:"duration"`
	Segments []openRouterSegment     `json:"segments"`
	Words    []openRouterWord        `json:"words"`
	Usage    openRouterResponseUsage `json:"usage"`
}

type openRouterSegment struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type openRouterWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type openRouterResponseUsage struct {
	Seconds float64 `json:"seconds"`
}

func (o *OpenRouter) Transcribe(
	ctx context.Context,
	audio []byte,
	config FinalConfig,
) (FinalResult, error) {
	result, status, err := o.requestTranscription(ctx, audio, config, true)
	if err != nil && status == http.StatusBadRequest {
		result, _, err = o.requestTranscription(ctx, audio, config, false)
	}
	if err != nil {
		return FinalResult{}, err
	}
	return o.normalizeResult(result, config)
}

func (o *OpenRouter) requestTranscription(
	ctx context.Context,
	audio []byte,
	config FinalConfig,
	verbose bool,
) (openRouterResponse, int, error) {
	responseFormat := ""
	var timestampGranularities []string
	if verbose {
		responseFormat = "verbose_json"
		timestampGranularities = []string{"segment", "word"}
	}
	requestBody, err := json.Marshal(openRouterRequest{
		Model: o.model,
		InputAudio: openRouterInputAudio{
			Data:   base64.StdEncoding.EncodeToString(audio),
			Format: config.Format,
		},
		Language:               config.Language,
		Prompt:                 dictionaryPrompt(config.DictionaryTerm),
		Temperature:            0,
		ResponseFormat:         responseFormat,
		TimestampGranularities: timestampGranularities,
	})
	if err != nil {
		return openRouterResponse{}, 0, fmt.Errorf("encode OpenRouter request: %w", err)
	}

	for attempt := 0; attempt < 3; attempt++ {
		request, requestErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			openRouterTranscriptionsURL,
			bytes.NewReader(requestBody),
		)
		if requestErr != nil {
			return openRouterResponse{}, 0, fmt.Errorf(
				"create OpenRouter request: %w",
				requestErr,
			)
		}
		request.Header.Set("Authorization", "Bearer "+o.apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("HTTP-Referer", "https://writers.studio")
		request.Header.Set("X-Title", "Writer's Studio")

		response, requestErr := o.client.Do(request)
		if requestErr != nil {
			if attempt < 2 && waitForOpenRouterRetry(ctx, time.Duration(attempt+1)*250*time.Millisecond) {
				continue
			}
			return openRouterResponse{}, 0, fmt.Errorf(
				"call OpenRouter transcription: %w",
				requestErr,
			)
		}
		status := response.StatusCode
		if status < 200 || status >= 300 {
			_ = response.Body.Close()
			if attempt < 2 &&
				isRetryableOpenRouterStatus(status) &&
				waitForOpenRouterRetry(ctx, time.Duration(attempt+1)*250*time.Millisecond) {
				continue
			}
			return openRouterResponse{}, status, fmt.Errorf(
				"OpenRouter transcription status %d",
				status,
			)
		}

		var result openRouterResponse
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result)
		_ = response.Body.Close()
		if decodeErr != nil {
			return openRouterResponse{}, status, fmt.Errorf(
				"decode OpenRouter transcription: %w",
				decodeErr,
			)
		}
		return result, status, nil
	}
	return openRouterResponse{}, 0, fmt.Errorf("OpenRouter transcription retries exhausted")
}

func (o *OpenRouter) normalizeResult(
	result openRouterResponse,
	config FinalConfig,
) (FinalResult, error) {
	result.Text = strings.TrimSpace(result.Text)
	if result.Text == "" {
		return FinalResult{}, fmt.Errorf("OpenRouter transcription returned empty text")
	}

	durationMS := config.DurationMS
	if result.Duration > 0 {
		durationMS = secondsToMilliseconds(result.Duration)
	} else if result.Usage.Seconds > 0 {
		durationMS = secondsToMilliseconds(result.Usage.Seconds)
	}
	words := make([]domain.TranscriptWord, 0, len(result.Words))
	for _, word := range result.Words {
		text := strings.TrimSpace(word.Word)
		if text == "" {
			continue
		}
		words = append(words, domain.TranscriptWord{
			Text:    text,
			StartMS: secondsToMilliseconds(word.Start),
			EndMS:   secondsToMilliseconds(word.End),
		})
	}
	segments := make([]domain.TranscriptSegment, 0, max(1, len(result.Segments)))
	for _, providerSegment := range result.Segments {
		text := strings.TrimSpace(providerSegment.Text)
		if text == "" {
			continue
		}
		startMS := secondsToMilliseconds(providerSegment.Start)
		endMS := secondsToMilliseconds(providerSegment.End)
		segmentWords := make([]domain.TranscriptWord, 0)
		for _, word := range words {
			if word.StartMS >= startMS && word.EndMS <= endMS {
				segmentWords = append(segmentWords, word)
			}
		}
		segments = append(segments, domain.TranscriptSegment{
			ID:      uuid.NewString(),
			Text:    text,
			StartMS: startMS,
			EndMS:   endMS,
			Words:   segmentWords,
		})
	}
	if len(segments) == 0 {
		segments = append(segments, domain.TranscriptSegment{
			ID:      uuid.NewString(),
			Text:    result.Text,
			StartMS: 0,
			EndMS:   max(0, durationMS),
			Words:   words,
		})
	}
	return FinalResult{
		PipelineVersion: "openrouter:" + o.model + ":v2",
		Provider:        "openrouter",
		Model:           o.model,
		Language:        config.Language,
		Text:            result.Text,
		Segments:        segments,
	}, nil
}

func secondsToMilliseconds(seconds float64) int64 {
	return max(0, int64(math.Round(seconds*1_000)))
}

func isRetryableOpenRouterStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func waitForOpenRouterRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func dictionaryPrompt(terms []string) string {
	selected := make([]string, 0, min(len(terms), 50))
	length := 0
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if len([]rune(term)) > 80 {
			term = string([]rune(term)[:80])
		}
		if length+len([]rune(term)) > 1_000 {
			break
		}
		selected = append(selected, term)
		length += len([]rune(term))
		if len(selected) == 50 {
			break
		}
	}
	if len(selected) == 0 {
		return ""
	}
	return "Expected project vocabulary: " + strings.Join(selected, ", ")
}
