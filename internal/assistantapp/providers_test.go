package assistantapp

import (
	"context"
	"testing"
	"time"

	"writers.studio/internal/assistant"
	"writers.studio/internal/config"
	"writers.studio/internal/llm"
)

func TestProviderRegistryUsesOneGlobalRequestTimeout(t *testing.T) {
	t.Parallel()
	registry, err := NewProviderRegistry(config.Config{
		LLMRequestTimeout: 7 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.RequestTimeoutMS(); got != 420_000 {
		t.Fatalf("unexpected global provider timeout: %d", got)
	}

	defaultRegistry, err := NewProviderRegistry(config.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := defaultRegistry.RequestTimeoutMS(); got != 600_000 {
		t.Fatalf("unexpected safe default provider timeout: %d", got)
	}
}

func TestFixtureSystemProviderUsesTheNormalCapabilityAndRunPath(t *testing.T) {
	t.Parallel()
	registry, err := NewProviderRegistry(config.Config{
		LLMAdapter: "fixture",
		LLMModel:   "fixture",
		LLMDialect: "chat_completions",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reference, report, configured := registry.SystemProviderReport(context.Background(), true)
	if !configured || report.Status != "limited" || !reference.Capabilities.CanRunAssistant() {
		t.Fatalf("fixture system provider is not usable: %+v, %+v", reference, report)
	}
	provider, err := registry.ProviderForRun(context.Background(), "owner", assistant.Run{
		Provider: reference,
	})
	if err != nil {
		t.Fatal(err)
	}
	completion, err := provider.Complete(context.Background(), llm.Request{})
	if err != nil || completion.Text != "Готово." {
		t.Fatalf("fixture run provider failed: %+v, %v", completion, err)
	}
}
