package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"writers.studio/internal/assistant"
	"writers.studio/internal/llm"
	"writers.studio/internal/projectmcp"
)

func main() {
	baseURL, err := url.Parse(strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/"))
	if err != nil || baseURL.Scheme == "" {
		fmt.Fprintln(os.Stderr, "assistant provider smoke: LLM_BASE_URL is invalid")
		os.Exit(1)
	}
	requestTimeout := envDuration("ASSISTANT_SMOKE_TIMEOUT", envDuration("LLM_REQUEST_TIMEOUT", 10*time.Minute))
	dialect := assistant.ProviderDialect(strings.TrimSpace(os.Getenv("LLM_DIALECT")))
	provider, err := llm.NewOpenAICompatible(llm.OpenAICompatibleConfig{
		BaseURL: baseURL, APIKey: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL"),
		Dialect: dialect, HTTPClient: &http.Client{Timeout: requestTimeout},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "assistant provider smoke:", err)
		os.Exit(1)
	}

	catalog := projectmcp.PublicCatalog()
	declaration := make([]projectmcp.ToolSpec, 0, 1)
	project := make([]projectmcp.ToolSpec, 0, len(catalog)-1)
	for _, tool := range catalog {
		if tool.Name == "assistant.begin_turn" {
			declaration = append(declaration, tool)
		} else {
			project = append(project, tool)
		}
	}
	start := envInt("ASSISTANT_SMOKE_TOOL_START", 0)
	limit := envInt("ASSISTANT_SMOKE_TOOL_LIMIT", len(project)-start)
	if start < 0 || start >= len(project) || limit < 1 || start+limit > len(project) {
		fmt.Fprintln(os.Stderr, "assistant provider smoke: invalid tool slice")
		os.Exit(1)
	}
	project = project[start : start+limit]
	if os.Getenv("ASSISTANT_SMOKE_KEEP_DECLARATION") == "true" {
		project = append(declaration, project...)
	}
	instruction := "Read project memory and answer one short question without changing the project."
	if path := strings.TrimSpace(os.Getenv("ASSISTANT_SMOKE_INSTRUCTION_FILE")); path != "" {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			fail("read instruction fixture", readErr)
		}
		instruction = strings.TrimSpace(string(content))
		if instruction == "" || len([]rune(instruction)) > 200_000 {
			fail("read instruction fixture", errors.New("instruction fixture must contain 1 to 200000 characters"))
		}
	}
	input := []json.RawMessage{llm.UserInput(instruction)}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*3+time.Minute)
	defer cancel()
	first, err := provider.Complete(ctx, llm.Request{
		Instructions: "Declare the turn mode with assistant.begin_turn.", Input: input,
		Tools: projectmcp.Definitions(declaration), ToolChoice: "assistant.begin_turn",
	})
	if err != nil {
		fail("declaration request", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "assistant.begin_turn" {
		fmt.Fprintln(os.Stderr, "assistant provider smoke: declaration tool was not called")
		os.Exit(1)
	}
	output, err := llm.ToolOutput(dialect, first.ToolCalls[0].ID, map[string]any{
		"accepted": true, "mode": "answer",
	})
	if err != nil {
		fail("encode declaration result", err)
	}
	input = append(input, first.Continuation...)
	input = append(input, output)
	toolChoice := strings.TrimSpace(os.Getenv("ASSISTANT_SMOKE_FORCE_TOOL_NAME"))
	if toolChoice == "" && os.Getenv("ASSISTANT_SMOKE_FORCE_TOOL") == "true" {
		toolChoice = project[len(project)-1].Name
	}
	if toolChoice != "" && !containsTool(project, toolChoice) {
		fail("select forced tool", fmt.Errorf("tool %q is outside the selected catalog", toolChoice))
	}
	second, err := provider.Complete(ctx, llm.Request{
		Instructions: "Use the supplied project tools to inspect the project. Do not invent IDs.",
		Input:        input, Tools: projectmcp.Definitions(project), ToolChoice: toolChoice,
	})
	if err != nil {
		fail(fmt.Sprintf("full MCP request (%d tools)", len(project)), err)
	}
	steps := 2
	last := second
	if os.Getenv("ASSISTANT_SMOKE_CONTINUE") == "true" {
		if len(second.ToolCalls) == 0 {
			fail("continue tool loop", errors.New("provider returned no tool call to continue"))
		}
		input = append(input, second.Continuation...)
		for _, call := range second.ToolCalls {
			output, outputErr := llm.ToolOutput(dialect, call.ID, map[string]any{
				"ok": true, "synthetic": true,
			})
			if outputErr != nil {
				fail("encode synthetic tool result", outputErr)
			}
			input = append(input, output)
		}
		continueChoice := strings.TrimSpace(os.Getenv("ASSISTANT_SMOKE_CONTINUE_TOOL_NAME"))
		if continueChoice == "" {
			continueChoice = toolChoice
		}
		if continueChoice != "" && !containsTool(project, continueChoice) {
			fail("select continuation tool", fmt.Errorf("tool %q is outside the selected catalog", continueChoice))
		}
		last, err = provider.Complete(ctx, llm.Request{
			Instructions: "Continue the same bounded tool loop after the synthetic result.",
			Input:        input, Tools: projectmcp.Definitions(project), ToolChoice: continueChoice,
		})
		if err != nil {
			fail(fmt.Sprintf("sequential MCP request (%d tools)", len(project)), err)
		}
		steps = 3
	}
	fmt.Printf(
		"assistant provider smoke passed: model=%s dialect=%s tools=%d steps=%d instruction_chars=%d returned_tool_calls=%d text=%t\n",
		os.Getenv("LLM_MODEL"), dialect, len(project), steps, len([]rune(instruction)),
		len(last.ToolCalls), strings.TrimSpace(last.Text) != "",
	)
}

func fail(stage string, err error) {
	var providerError *llm.ProviderHTTPError
	if errors.As(err, &providerError) {
		fmt.Fprintf(
			os.Stderr,
			"assistant provider smoke failed at %s: HTTP %d code=%s message=%s\n",
			stage,
			providerError.StatusCode,
			providerError.ProviderCode,
			providerError.Message+formatDiagnostic("type", providerError.ErrorType)+
				formatDiagnostic("provider", providerError.ProviderName),
		)
	} else {
		fmt.Fprintf(os.Stderr, "assistant provider smoke failed at %s: %v\n", stage, err)
	}
	os.Exit(1)
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return -1
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < time.Second || parsed > 10*time.Minute {
		fail("parse "+name, errors.New("duration must be between 1s and 10m"))
	}
	return parsed
}

func containsTool(catalog []projectmcp.ToolSpec, name string) bool {
	for _, tool := range catalog {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func formatDiagnostic(name, value string) string {
	if value == "" {
		return ""
	}
	return " " + name + "=" + value
}
