package stt

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"

	"writers.studio/internal/domain"
)

var ErrFixtureFailure = errors.New("deterministic fixture STT failure")

type Fake struct {
	mu         sync.Mutex
	failedOnce map[string]bool
}

func NewFake() *Fake {
	return &Fake{failedOnce: make(map[string]bool)}
}

func (f *Fake) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Available:         true,
		Modes:             []string{"realtime", "file"},
		Deployment:        "self_hosted",
		StreamingProtocol: "fixture",
		Languages:         []string{"ru", "en"},
		PartialResults:    true,
		SegmentTimestamps: true,
		WordTimestamps:    true,
		Confidence:        "word",
		VocabularyHints:   "keywords",
		SupportsLongAudio: true,
	}
}

func (f *Fake) Transcribe(
	_ context.Context,
	audio []byte,
	config FinalConfig,
) (FinalResult, error) {
	if strings.HasPrefix(string(audio), "FAIL_STT_ONCE") {
		f.mu.Lock()
		alreadyFailed := f.failedOnce[config.SessionID]
		f.failedOnce[config.SessionID] = true
		f.mu.Unlock()
		if !alreadyFailed {
			return FinalResult{}, ErrFixtureFailure
		}
	}
	if strings.HasPrefix(string(audio), "FAIL_STT") &&
		!strings.HasPrefix(string(audio), "FAIL_STT_ONCE") {
		return FinalResult{}, ErrFixtureFailure
	}

	if config.Language == "en" {
		return fakeEnglish(config.SessionID, config.DictionaryTerm), nil
	}
	return fakeRussian(config.SessionID, config.DictionaryTerm), nil
}

func fakeRussian(sessionID string, dictionaryTerms []string) FinalResult {
	firstConfidence := 0.98
	uncertainConfidence := 0.68
	segments := []domain.TranscriptSegment{
		{
			ID:         uuid.NewString(),
			Text:       "Дом стоял у самой воды, чуть в стороне от старой дороги.",
			StartMS:    0,
			EndMS:      4200,
			Confidence: &firstConfidence,
			Words: []domain.TranscriptWord{
				{Text: "Дом", StartMS: 0, EndMS: 480, Confidence: &firstConfidence},
				{Text: "стоял", StartMS: 500, EndMS: 940, Confidence: &firstConfidence},
				{Text: "у самой воды", StartMS: 960, EndMS: 2100, Confidence: &firstConfidence},
			},
		},
		{
			ID:         uuid.NewString(),
			Text:       "Анна остановилась у калитки. За десять лет здесь почти ничего не изменилось.",
			StartMS:    4300,
			EndMS:      9100,
			Confidence: &uncertainConfidence,
			Words: []domain.TranscriptWord{
				{Text: "Анна", StartMS: 4300, EndMS: 4950, Confidence: &uncertainConfidence},
				{Text: "остановилась", StartMS: 4980, EndMS: 5800, Confidence: &firstConfidence},
			},
		},
		{
			ID:         uuid.NewString(),
			Text:       "Она положила ладонь на холодное дерево и прислушалась.",
			StartMS:    9200,
			EndMS:      12800,
			Confidence: &firstConfidence,
		},
	}
	if term := firstFixtureTerm(dictionaryTerms); term != "" {
		segments[1].Text = term + " остановилась у калитки. За десять лет здесь почти ничего не изменилось."
		segments[1].Words[0].Text = term
	}
	return FinalResult{
		PipelineVersion: "fake-v1",
		Provider:        "fixture",
		Model:           "deterministic-writers-studio",
		Language:        "ru",
		Text:            joinSegments(segments),
		Segments:        segments,
	}
}

func fakeEnglish(sessionID string, dictionaryTerms []string) FinalResult {
	confidence := 0.97
	segments := []domain.TranscriptSegment{
		{
			ID:         uuid.NewString(),
			Text:       "The house stood beside the water, just beyond the old road.",
			StartMS:    0,
			EndMS:      4100,
			Confidence: &confidence,
		},
		{
			ID:         uuid.NewString(),
			Text:       "Anna stopped at the gate and listened to the quiet rooms.",
			StartMS:    4200,
			EndMS:      8300,
			Confidence: &confidence,
		},
	}
	if term := firstFixtureTerm(dictionaryTerms); term != "" {
		segments[1].Text = term + " stopped at the gate and listened to the quiet rooms."
	}
	return FinalResult{
		PipelineVersion: "fake-v1",
		Provider:        "fixture",
		Model:           "deterministic-writers-studio",
		Language:        "en",
		Text:            joinSegments(segments),
		Segments:        segments,
	}
}

func firstFixtureTerm(terms []string) string {
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term != "" {
			return term
		}
	}
	return ""
}

func joinSegments(segments []domain.TranscriptSegment) string {
	paragraphs := make([]string, 0, len(segments))
	for _, segment := range segments {
		paragraphs = append(paragraphs, segment.Text)
	}
	return strings.Join(paragraphs, "\n\n")
}
