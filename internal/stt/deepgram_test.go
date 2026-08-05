package stt

import (
	"net/url"
	"testing"
)

func TestDeepgramStreamURL(t *testing.T) {
	t.Parallel()

	streamURL, err := deepgramStreamURL(
		"wss://api.deepgram.com/v1/listen",
		"nova-3",
		LiveConfig{
			Language:        "ru",
			SampleRate:      48_000,
			DictionaryTerms: []string{"Анна Каренина", "Лев Толстой", "анна каренина"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("model") != "nova-3" ||
		query.Get("language") != "ru" ||
		query.Get("encoding") != "linear16" ||
		query.Get("sample_rate") != "48000" ||
		query.Get("interim_results") != "true" {
		t.Fatalf("unexpected Deepgram query: %s", parsed.RawQuery)
	}
	keyterms := query["keyterm"]
	if len(keyterms) != 2 ||
		keyterms[0] != "Анна Каренина" ||
		keyterms[1] != "Лев Толстой" {
		t.Fatalf("unexpected keyterms: %#v", keyterms)
	}
}

func TestNormalizeDeepgramResult(t *testing.T) {
	t.Parallel()

	message := deepgramMessage{
		Type:         "Results",
		Start:        1.25,
		Duration:     2.5,
		IsFinal:      true,
		FromFinalize: true,
		Channel: deepgramChannel{Alternatives: []deepgramAlternative{{
			Transcript: "Проверка настоящего распознавания.",
			Confidence: 0.91,
			Words: []deepgramWord{{
				Word:           "проверка",
				PunctuatedWord: "Проверка",
				Start:          1.25,
				End:            1.8,
				Confidence:     0.95,
			}},
		}}},
	}
	message.Metadata.RequestID = "request-1"

	event, ok := normalizeDeepgramEvent(message, 7)
	if !ok {
		t.Fatal("expected normalized event")
	}
	if event.Kind != "final" ||
		event.Text != "Проверка настоящего распознавания." ||
		event.StartMS != 1_250 ||
		event.EndMS != 3_750 ||
		event.ProviderTurnID != "request-1-1250-7" ||
		!event.FromFinalize {
		t.Fatalf("unexpected event: %#v", event)
	}
	if len(event.Words) != 1 ||
		event.Words[0].Text != "Проверка" ||
		event.Words[0].StartMS != 1_250 ||
		event.Words[0].EndMS != 1_800 {
		t.Fatalf("unexpected words: %#v", event.Words)
	}
}

func TestNormalizeDeepgramInterimAndSpeechEvents(t *testing.T) {
	t.Parallel()

	partial, ok := normalizeDeepgramEvent(deepgramMessage{
		Type: "Results",
		Channel: deepgramChannel{Alternatives: []deepgramAlternative{{
			Transcript: "Черновая гипотеза",
		}}},
	}, 1)
	if !ok || partial.Kind != "partial" {
		t.Fatalf("unexpected partial: %#v", partial)
	}
	started, ok := normalizeDeepgramEvent(deepgramMessage{
		Type:      "SpeechStarted",
		Timestamp: 2.4,
	}, 2)
	if !ok || started.Kind != "speech_started" || started.StartMS != 2_400 {
		t.Fatalf("unexpected speech event: %#v", started)
	}
}
