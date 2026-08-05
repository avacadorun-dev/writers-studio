package config

import "testing"

func TestResolveSTTAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		key       string
		want      string
		wantError bool
	}{
		{name: "auto with key", requested: "auto", key: "secret", want: "openrouter"},
		{name: "auto without key", requested: "auto", want: "disabled"},
		{name: "explicit disabled", requested: "disabled", want: "disabled"},
		{name: "explicit fake with key", requested: "fake", key: "secret", want: "fake"},
		{name: "explicit OpenRouter", requested: "openrouter", key: "secret", want: "openrouter"},
		{name: "OpenRouter requires key", requested: "openrouter", wantError: true},
		{name: "unsupported adapter", requested: "unknown", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveSTTAdapter(test.requested, test.key)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, got adapter %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveLiveSTTAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		requested    string
		deepgramKey  string
		finalAdapter string
		want         string
		wantError    bool
	}{
		{
			name:         "auto prefers Deepgram",
			requested:    "auto",
			deepgramKey:  "secret",
			finalAdapter: "openrouter",
			want:         "deepgram",
		},
		{
			name:         "auto keeps OpenRouter chunked fallback",
			requested:    "auto",
			finalAdapter: "openrouter",
			want:         "openrouter",
		},
		{name: "auto without provider is disabled", requested: "auto", want: "disabled"},
		{name: "explicit disabled", requested: "disabled", want: "disabled"},
		{name: "explicit fake", requested: "fake", want: "fake"},
		{
			name:        "explicit Deepgram",
			requested:   "deepgram",
			deepgramKey: "secret",
			want:        "deepgram",
		},
		{
			name:      "Deepgram requires key",
			requested: "deepgram",
			wantError: true,
		},
		{
			name:         "OpenRouter live requires OpenRouter finalizer",
			requested:    "openrouter",
			finalAdapter: "fake",
			wantError:    true,
		},
		{
			name:      "unsupported live adapter",
			requested: "unknown",
			wantError: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveLiveSTTAdapter(
				test.requested,
				test.deepgramKey,
				test.finalAdapter,
			)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, got adapter %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveLLMAdapter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested string
		baseURL   string
		key       string
		want      string
		wantError bool
	}{
		{
			name: "auto configured", requested: "auto", baseURL: "https://llm.example/v1",
			key: "secret", want: "openai_compatible",
		},
		{name: "auto disabled", requested: "auto", want: "disabled"},
		{name: "explicit disabled", requested: "disabled", want: "disabled"},
		{name: "explicit fixture", requested: "fixture", want: "fixture"},
		{
			name: "explicit compatible", requested: "openai_compatible",
			baseURL: "https://llm.example/v1", key: "secret", want: "openai_compatible",
		},
		{
			name:      "explicit compatible requires configuration",
			requested: "openai_compatible", wantError: true,
		},
		{name: "unknown adapter", requested: "other", wantError: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveLLMAdapter(test.requested, test.baseURL, test.key)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoadRequiresExplicitTestAdapterGate(t *testing.T) {
	t.Setenv("STT_ADAPTER", "fake")
	t.Setenv("LIVE_STT_ADAPTER", "fake")
	t.Setenv("LLM_ADAPTER", "fixture")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("APP_ENV", "docker")
	t.Setenv("ALLOW_TEST_ADAPTERS", "false")

	if _, err := Load(); err == nil {
		t.Fatal("test adapters were accepted without the explicit gate")
	}

	t.Setenv("ALLOW_TEST_ADAPTERS", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMAdapter != "fixture" || cfg.LLMModel != "fixture" {
		t.Fatalf("unexpected fixture config: %+v", cfg)
	}

	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted the test-adapter gate")
	}
}

func TestLoadRejectsProductionTestGateWithoutActiveFixtures(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("ALLOW_TEST_ADAPTERS", "true")
	t.Setenv("STT_ADAPTER", "disabled")
	t.Setenv("LIVE_STT_ADAPTER", "disabled")
	t.Setenv("LLM_ADAPTER", "disabled")

	if _, err := Load(); err == nil {
		t.Fatal("production accepted ALLOW_TEST_ADAPTERS=true")
	}
}

func TestLoadAutoNeverFallsBackToCannedAdapters(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("ALLOW_TEST_ADAPTERS", "false")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("DEEPGRAM_API_KEY", "")
	t.Setenv("STT_ADAPTER", "auto")
	t.Setenv("LIVE_STT_ADAPTER", "auto")
	t.Setenv("LLM_ADAPTER", "auto")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.STTAdapter != "disabled" || cfg.LiveSTTAdapter != "disabled" ||
		cfg.LLMAdapter != "disabled" {
		t.Fatalf("auto resolved to a non-disabled adapter without credentials: %+v", cfg)
	}
}

func TestLoadRejectsInvalidTestAdapterGate(t *testing.T) {
	t.Setenv("ALLOW_TEST_ADAPTERS", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("invalid ALLOW_TEST_ADAPTERS value was accepted")
	}
}

func TestResolveLLMDialect(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"responses", "chat_completions"} {
		got, err := resolveLLMDialect(dialect)
		if err != nil || got != dialect {
			t.Fatalf("resolve %q: %q, %v", dialect, got, err)
		}
	}
	got, err := resolveLLMDialect("auto")
	if err != nil || got != "chat_completions" {
		t.Fatalf("resolve auto: %q, %v", got, err)
	}
	if _, err := resolveLLMDialect("other"); err == nil {
		t.Fatal("unknown dialect was accepted")
	}
}

func TestLoadRequiresExactProviderModel(t *testing.T) {
	t.Setenv("STT_ADAPTER", "disabled")
	t.Setenv("LIVE_STT_ADAPTER", "disabled")
	t.Setenv("LLM_ADAPTER", "openai_compatible")
	t.Setenv("LLM_BASE_URL", "https://llm.example/v1")
	t.Setenv("LLM_API_KEY", "secret")
	t.Setenv("LLM_MODEL", "")

	if _, err := Load(); err == nil {
		t.Fatal("configured provider without an exact model ID was accepted")
	}

	t.Setenv("LLM_MODEL", "provider-model-v3")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMModel != "provider-model-v3" {
		t.Fatalf("provider model was replaced: %q", cfg.LLMModel)
	}
}
