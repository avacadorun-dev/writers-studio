package stt

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"writers.studio/internal/domain"
)

const deepgramStreamingURL = "wss://api.deepgram.com/v1/listen"

type Deepgram struct {
	apiKey   string
	model    string
	endpoint string
}

func NewDeepgram(apiKey, model string) *Deepgram {
	return &Deepgram{
		apiKey:   apiKey,
		model:    model,
		endpoint: deepgramStreamingURL,
	}
}

func (d *Deepgram) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Available:         true,
		Modes:             []string{"realtime"},
		Deployment:        "cloud",
		StreamingProtocol: "pcm16",
		Languages:         []string{"ru", "en"},
		PartialResults:    true,
		SegmentTimestamps: true,
		WordTimestamps:    true,
		Confidence:        "word",
		VocabularyHints:   "keyterm",
		SupportsLongAudio: true,
	}
}

func (d *Deepgram) Open(ctx context.Context, config LiveConfig) (LiveSession, error) {
	streamURL, err := deepgramStreamURL(d.endpoint, d.model, config)
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("Authorization", "Token "+d.apiKey)
	dialContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(dialContext, streamURL, &websocket.DialOptions{
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("connect Deepgram streaming STT (status %d): %w", status, err)
	}
	session := &deepgramSession{
		connection: connection,
		closeCh:    make(chan struct{}),
		lastWrite:  time.Now(),
	}
	go session.keepAlive()
	return session, nil
}

func deepgramStreamURL(endpoint, model string, config LiveConfig) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse Deepgram endpoint: %w", err)
	}
	if config.SampleRate < 8_000 || config.SampleRate > 48_000 {
		return "", fmt.Errorf("unsupported Deepgram sample rate %d", config.SampleRate)
	}
	language := strings.ToLower(strings.TrimSpace(config.Language))
	if language == "" {
		language = "ru"
	}
	query := parsed.Query()
	query.Set("model", model)
	query.Set("language", language)
	query.Set("encoding", "linear16")
	query.Set("sample_rate", strconv.Itoa(config.SampleRate))
	query.Set("channels", "1")
	query.Set("interim_results", "true")
	query.Set("punctuate", "true")
	query.Set("smart_format", "true")
	query.Set("vad_events", "true")
	query.Set("endpointing", "500")
	query.Set("utterance_end_ms", "1500")
	for _, term := range limitedDeepgramKeyterms(config.DictionaryTerms) {
		query.Add("keyterm", term)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func limitedDeepgramKeyterms(terms []string) []string {
	result := make([]string, 0, min(len(terms), 50))
	seen := make(map[string]struct{})
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" || len([]rune(term)) > 100 {
			continue
		}
		key := strings.ToLower(term)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, term)
		if len(result) == 50 {
			break
		}
	}
	return result
}

type deepgramSession struct {
	connection     *websocket.Conn
	writeMu        sync.Mutex
	stateMu        sync.Mutex
	lastWrite      time.Time
	finalizeWaiter chan struct{}
	closeOnce      sync.Once
	closeCh        chan struct{}
	eventSequence  atomic.Uint64
}

func (s *deepgramSession) SendAudio(ctx context.Context, audio []byte) error {
	if len(audio) == 0 {
		return nil
	}
	return s.write(ctx, websocket.MessageBinary, audio)
}

func (s *deepgramSession) Finalize(ctx context.Context) error {
	waiter := make(chan struct{})
	s.stateMu.Lock()
	s.finalizeWaiter = waiter
	s.stateMu.Unlock()
	if err := s.writeJSON(ctx, map[string]string{"type": "Finalize"}); err != nil {
		s.clearFinalizeWaiter(waiter)
		return err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-waiter:
		settleTimer := time.NewTimer(150 * time.Millisecond)
		defer settleTimer.Stop()
		select {
		case <-settleTimer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-timer.C:
		s.clearFinalizeWaiter(waiter)
		return fmt.Errorf("Deepgram did not confirm stream finalization")
	case <-ctx.Done():
		s.clearFinalizeWaiter(waiter)
		return ctx.Err()
	}
}

func (s *deepgramSession) ReadEvent(ctx context.Context) (LiveEvent, error) {
	for {
		messageType, data, err := s.connection.Read(ctx)
		if err != nil {
			return LiveEvent{}, err
		}
		if messageType != websocket.MessageText {
			continue
		}
		var message deepgramMessage
		if err := json.Unmarshal(data, &message); err != nil {
			continue
		}
		if message.FromFinalize {
			s.signalFinalized()
		}
		event, ok := normalizeDeepgramEvent(message, s.eventSequence.Add(1))
		if ok {
			return event, nil
		}
		if message.FromFinalize {
			return LiveEvent{Kind: "finalized", FromFinalize: true}, nil
		}
	}
}

func (s *deepgramSession) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
		closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.writeJSON(closeContext, map[string]string{"type": "CloseStream"})
		_ = s.connection.Close(websocket.StatusNormalClosure, "")
		s.signalFinalized()
	})
}

func (s *deepgramSession) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.stateMu.Lock()
			idle := time.Since(s.lastWrite)
			s.stateMu.Unlock()
			if idle >= 5*time.Second {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = s.writeJSON(ctx, map[string]string{"type": "KeepAlive"})
				cancel()
			}
		case <-s.closeCh:
			return
		}
	}
}

func (s *deepgramSession) writeJSON(ctx context.Context, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.write(ctx, websocket.MessageText, data)
}

func (s *deepgramSession) write(
	ctx context.Context,
	messageType websocket.MessageType,
	data []byte,
) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.connection.Write(ctx, messageType, data); err != nil {
		return err
	}
	s.stateMu.Lock()
	s.lastWrite = time.Now()
	s.stateMu.Unlock()
	return nil
}

func (s *deepgramSession) signalFinalized() {
	s.stateMu.Lock()
	waiter := s.finalizeWaiter
	s.finalizeWaiter = nil
	s.stateMu.Unlock()
	if waiter != nil {
		close(waiter)
	}
}

func (s *deepgramSession) clearFinalizeWaiter(waiter chan struct{}) {
	s.stateMu.Lock()
	if s.finalizeWaiter == waiter {
		s.finalizeWaiter = nil
	}
	s.stateMu.Unlock()
}

type deepgramMessage struct {
	Type         string          `json:"type"`
	Start        float64         `json:"start"`
	Duration     float64         `json:"duration"`
	IsFinal      bool            `json:"is_final"`
	SpeechFinal  bool            `json:"speech_final"`
	FromFinalize bool            `json:"from_finalize"`
	Timestamp    float64         `json:"timestamp"`
	LastWordEnd  float64         `json:"last_word_end"`
	Channel      deepgramChannel `json:"channel"`
	Metadata     struct {
		RequestID string `json:"request_id"`
	} `json:"metadata"`
}

type deepgramChannel struct {
	Alternatives []deepgramAlternative `json:"alternatives"`
}

type deepgramAlternative struct {
	Transcript string         `json:"transcript"`
	Confidence float64        `json:"confidence"`
	Words      []deepgramWord `json:"words"`
}

type deepgramWord struct {
	Word           string  `json:"word"`
	PunctuatedWord string  `json:"punctuated_word"`
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Confidence     float64 `json:"confidence"`
}

func normalizeDeepgramEvent(message deepgramMessage, sequence uint64) (LiveEvent, bool) {
	switch message.Type {
	case "SpeechStarted":
		return LiveEvent{
			Kind:         "speech_started",
			StartMS:      deepgramSecondsToMilliseconds(message.Timestamp),
			FromFinalize: message.FromFinalize,
		}, true
	case "UtteranceEnd":
		return LiveEvent{
			Kind:         "speech_stopped",
			EndMS:        deepgramSecondsToMilliseconds(message.LastWordEnd),
			FromFinalize: message.FromFinalize,
		}, true
	case "Results":
	default:
		return LiveEvent{}, false
	}
	if len(message.Channel.Alternatives) == 0 {
		return LiveEvent{}, false
	}
	alternative := message.Channel.Alternatives[0]
	text := strings.TrimSpace(alternative.Transcript)
	if text == "" {
		return LiveEvent{}, false
	}
	startMS := deepgramSecondsToMilliseconds(message.Start)
	endMS := deepgramSecondsToMilliseconds(message.Start + message.Duration)
	words := make([]domain.TranscriptWord, 0, len(alternative.Words))
	for _, word := range alternative.Words {
		wordText := strings.TrimSpace(word.PunctuatedWord)
		if wordText == "" {
			wordText = strings.TrimSpace(word.Word)
		}
		confidence := word.Confidence
		words = append(words, domain.TranscriptWord{
			Text:       wordText,
			StartMS:    deepgramSecondsToMilliseconds(word.Start),
			EndMS:      deepgramSecondsToMilliseconds(word.End),
			Confidence: &confidence,
		})
		if word.End > 0 {
			endMS = max(endMS, deepgramSecondsToMilliseconds(word.End))
		}
	}
	confidence := alternative.Confidence
	kind := "partial"
	if message.IsFinal {
		kind = "final"
	}
	requestID := strings.TrimSpace(message.Metadata.RequestID)
	if requestID == "" {
		requestID = "deepgram"
	}
	return LiveEvent{
		Kind:           kind,
		ProviderTurnID: fmt.Sprintf("%s-%d-%d", requestID, startMS, sequence),
		Text:           text,
		StartMS:        startMS,
		EndMS:          endMS,
		Words:          words,
		Confidence:     &confidence,
		FromFinalize:   message.FromFinalize,
	}, true
}

func deepgramSecondsToMilliseconds(value float64) int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	return int64(math.Round(value * 1_000))
}

var _ LiveTranscriber = (*Deepgram)(nil)
var _ LiveSession = (*deepgramSession)(nil)
