package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabaseURL            string
	StaticDir              string
	PublicBaseURL          string
	ObjectStorageEndpoint  string
	ObjectStorageAccessKey string
	ObjectStorageSecretKey string
	ObjectStorageBucket    string
	ObjectStorageUseSSL    bool
	AllowTestAdapters      bool
	STTAdapter             string
	LiveSTTAdapter         string
	OpenRouterAPIKey       string
	OpenRouterSTTModel     string
	DeepgramAPIKey         string
	DeepgramSTTModel       string
	LLMAdapter             string
	LLMBaseURL             string
	LLMAPIKey              string
	LLMModel               string
	LLMDialect             string
	LLMCredentialKey       string
	LLMAllowPrivate        bool
	LLMRequestTimeout      time.Duration
	SessionTTL             time.Duration
	MCPEnabled             bool
	MCPOAuthSecret         string
}

func Load() (Config, error) {
	useSSL, err := strconv.ParseBool(envOr("OBJECT_STORAGE_USE_SSL", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse OBJECT_STORAGE_USE_SSL: %w", err)
	}
	allowPrivateLLM, err := strconv.ParseBool(envOr("LLM_ALLOW_PRIVATE_ENDPOINTS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse LLM_ALLOW_PRIVATE_ENDPOINTS: %w", err)
	}
	allowTestAdapters, err := strconv.ParseBool(envOr("ALLOW_TEST_ADAPTERS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse ALLOW_TEST_ADAPTERS: %w", err)
	}
	mcpEnabled, err := strconv.ParseBool(envOr("MCP_ENABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse MCP_ENABLED: %w", err)
	}
	llmTimeout, err := time.ParseDuration(envOr("LLM_REQUEST_TIMEOUT", "10m"))
	if err != nil || llmTimeout < time.Second || llmTimeout > 10*time.Minute {
		return Config{}, fmt.Errorf("LLM_REQUEST_TIMEOUT must be between 1s and 10m")
	}

	cfg := Config{
		AppEnv:                 envOr("APP_ENV", "development"),
		HTTPAddr:               envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:            envOr("DATABASE_URL", "postgres://writers:writers@localhost:5432/writers_studio?sslmode=disable"),
		StaticDir:              envOr("STATIC_DIR", "dist"),
		PublicBaseURL:          strings.TrimRight(envOr("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		ObjectStorageEndpoint:  envOr("OBJECT_STORAGE_ENDPOINT", "localhost:9000"),
		ObjectStorageAccessKey: envOr("OBJECT_STORAGE_ACCESS_KEY", "writers"),
		ObjectStorageSecretKey: envOr("OBJECT_STORAGE_SECRET_KEY", "writers-secret"),
		ObjectStorageBucket:    envOr("OBJECT_STORAGE_BUCKET", "writers-studio"),
		ObjectStorageUseSSL:    useSSL,
		AllowTestAdapters:      allowTestAdapters,
		STTAdapter:             envOr("STT_ADAPTER", "auto"),
		LiveSTTAdapter:         envOr("LIVE_STT_ADAPTER", "auto"),
		OpenRouterAPIKey:       os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterSTTModel:     envOr("OPENROUTER_STT_MODEL", "openai/whisper-large-v3"),
		DeepgramAPIKey:         os.Getenv("DEEPGRAM_API_KEY"),
		DeepgramSTTModel:       envOr("DEEPGRAM_STT_MODEL", "nova-3"),
		LLMAdapter:             envOr("LLM_ADAPTER", "auto"),
		LLMBaseURL:             strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/"),
		LLMAPIKey:              os.Getenv("LLM_API_KEY"),
		LLMModel:               strings.TrimSpace(os.Getenv("LLM_MODEL")),
		LLMDialect:             envOr("LLM_DIALECT", "chat_completions"),
		LLMCredentialKey:       os.Getenv("LLM_CREDENTIAL_ENCRYPTION_KEY"),
		LLMAllowPrivate:        allowPrivateLLM,
		LLMRequestTimeout:      llmTimeout,
		SessionTTL:             30 * 24 * time.Hour,
		MCPEnabled:             mcpEnabled,
		MCPOAuthSecret:         strings.TrimSpace(os.Getenv("MCP_OAUTH_SECRET")),
	}

	cfg.STTAdapter, err = resolveSTTAdapter(cfg.STTAdapter, cfg.OpenRouterAPIKey)
	if err != nil {
		return Config{}, err
	}
	cfg.LiveSTTAdapter, err = resolveLiveSTTAdapter(
		cfg.LiveSTTAdapter,
		cfg.DeepgramAPIKey,
		cfg.STTAdapter,
	)
	if err != nil {
		return Config{}, err
	}
	cfg.LLMAdapter, err = resolveLLMAdapter(cfg.LLMAdapter, cfg.LLMBaseURL, cfg.LLMAPIKey)
	if err != nil {
		return Config{}, err
	}
	if err := validateTestAdapters(cfg); err != nil {
		return Config{}, err
	}
	if cfg.LLMAdapter == "fixture" {
		if cfg.LLMModel == "" {
			cfg.LLMModel = "fixture"
		}
	}
	if cfg.LLMAdapter == "openai_compatible" && cfg.LLMModel == "" {
		return Config{}, fmt.Errorf(
			"LLM_MODEL is required when LLM_ADAPTER=openai_compatible",
		)
	}
	cfg.LLMDialect, err = resolveLLMDialect(cfg.LLMDialect)
	if err != nil {
		return Config{}, err
	}
	if cfg.MCPEnabled && len(cfg.MCPOAuthSecret) < 32 {
		return Config{}, fmt.Errorf("MCP_OAUTH_SECRET must contain at least 32 characters when MCP_ENABLED=true")
	}
	return cfg, nil
}

func resolveSTTAdapter(requested, openRouterAPIKey string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "auto":
		if strings.TrimSpace(openRouterAPIKey) != "" {
			return "openrouter", nil
		}
		return "disabled", nil
	case "disabled":
		return "disabled", nil
	case "fake":
		return "fake", nil
	case "openrouter":
		if strings.TrimSpace(openRouterAPIKey) == "" {
			return "", fmt.Errorf("OPENROUTER_API_KEY is required when STT_ADAPTER=openrouter")
		}
		return "openrouter", nil
	default:
		return "", fmt.Errorf("unsupported STT_ADAPTER %q", requested)
	}
}

func resolveLiveSTTAdapter(requested, deepgramAPIKey, finalAdapter string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "auto":
		if strings.TrimSpace(deepgramAPIKey) != "" {
			return "deepgram", nil
		}
		if finalAdapter == "openrouter" {
			return "openrouter", nil
		}
		return "disabled", nil
	case "disabled":
		return "disabled", nil
	case "fake":
		return "fake", nil
	case "openrouter":
		if finalAdapter != "openrouter" {
			return "", fmt.Errorf(
				"STT_ADAPTER=openrouter is required when LIVE_STT_ADAPTER=openrouter",
			)
		}
		return "openrouter", nil
	case "deepgram":
		if strings.TrimSpace(deepgramAPIKey) == "" {
			return "", fmt.Errorf(
				"DEEPGRAM_API_KEY is required when LIVE_STT_ADAPTER=deepgram",
			)
		}
		return "deepgram", nil
	default:
		return "", fmt.Errorf("unsupported LIVE_STT_ADAPTER %q", requested)
	}
}

func validateTestAdapters(cfg Config) error {
	if isProductionEnvironment(cfg.AppEnv) && cfg.AllowTestAdapters {
		return fmt.Errorf("ALLOW_TEST_ADAPTERS=true is forbidden in production")
	}
	testAdapters := make([]string, 0, 3)
	if cfg.STTAdapter == "fake" {
		testAdapters = append(testAdapters, "STT_ADAPTER=fake")
	}
	if cfg.LiveSTTAdapter == "fake" {
		testAdapters = append(testAdapters, "LIVE_STT_ADAPTER=fake")
	}
	if cfg.LLMAdapter == "fixture" {
		testAdapters = append(testAdapters, "LLM_ADAPTER=fixture")
	}
	if len(testAdapters) == 0 {
		return nil
	}
	if isProductionEnvironment(cfg.AppEnv) {
		return fmt.Errorf(
			"test adapters are forbidden in production: %s",
			strings.Join(testAdapters, ", "),
		)
	}
	if !cfg.AllowTestAdapters {
		return fmt.Errorf(
			"test adapters require ALLOW_TEST_ADAPTERS=true in a non-production environment: %s",
			strings.Join(testAdapters, ", "),
		)
	}
	return nil
}

func isProductionEnvironment(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

func resolveLLMAdapter(requested, baseURL, apiKey string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	configured := strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != ""
	switch requested {
	case "auto":
		if configured {
			return "openai_compatible", nil
		}
		return "disabled", nil
	case "disabled":
		return "disabled", nil
	case "fixture":
		return "fixture", nil
	case "openai_compatible":
		if !configured {
			return "", fmt.Errorf(
				"LLM_BASE_URL and LLM_API_KEY are required when LLM_ADAPTER=openai_compatible",
			)
		}
		return "openai_compatible", nil
	default:
		return "", fmt.Errorf("unsupported LLM_ADAPTER %q", requested)
	}
}

func resolveLLMDialect(requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "responses", "chat_completions":
		return requested, nil
	case "auto":
		return "chat_completions", nil
	default:
		return "", fmt.Errorf("unsupported LLM_DIALECT %q", requested)
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
