package llm

import (
	"context"
	"encoding/json"
	"strings"

	"writers.studio/internal/assistant"
)

const ProbeProtocolVersion = "writers-provider-probe-v1"

type ProbeReport struct {
	Status       string                       `json:"status"`
	Capabilities assistant.CapabilitySnapshot `json:"capabilities"`
	Detail       string                       `json:"detail"`
}

func Probe(
	ctx context.Context,
	provider Provider,
	dialect assistant.ProviderDialect,
	model string,
) ProbeReport {
	capabilities := assistant.CapabilitySnapshot{
		SupportedDialects:    []string{string(dialect)},
		CheckedWithModel:     model,
		ProbeProtocolVersion: ProbeProtocolVersion,
	}
	if provider == nil {
		return failedProbe(capabilities, "Провайдер не настроен")
	}
	tools := probeTools()
	input := []json.RawMessage{UserInput(
		"Call probe_echo exactly once with value \"alpha\". Do not answer with text yet.",
	)}
	first, err := provider.Complete(ctx, Request{
		Instructions: probeInstructions(), Input: input, Tools: tools,
		ToolChoice: "probe_echo",
	})
	if err != nil {
		return failedProbe(capabilities, "Не удалось получить текстовый ответ и вызов инструмента")
	}
	capabilities.Text = first.Text != "" || first.ResponseID != ""
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "probe_echo" {
		return failedProbe(capabilities, "Модель не выполнила строгий вызов инструмента")
	}
	var echo struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(first.ToolCalls[0].Arguments, &echo); err != nil ||
		echo.Value != "alpha" {
		return failedProbe(capabilities, "Модель изменила аргументы строгого инструмента")
	}
	capabilities.StrictTools = true
	output, err := ToolOutput(dialect, first.ToolCalls[0].ID, map[string]string{
		"echo": echo.Value,
	})
	if err != nil {
		return failedProbe(capabilities, "Диалект провайдера не поддерживается")
	}
	input = append(input, first.Continuation...)
	input = append(input, output)
	second, err := provider.Complete(ctx, Request{
		Instructions: probeInstructions(), Input: input, Tools: tools,
		ToolChoice: "probe_finish",
	})
	if err != nil {
		return failedProbe(capabilities, "Провайдер не принял результат инструмента")
	}
	capabilities.ToolResults = true
	if len(second.ToolCalls) != 1 || second.ToolCalls[0].Name != "probe_finish" {
		return failedProbe(capabilities, "Модель не выполнила второй последовательный вызов")
	}
	var finish struct {
		Previous string `json:"previous"`
	}
	if err := json.Unmarshal(second.ToolCalls[0].Arguments, &finish); err != nil ||
		finish.Previous != "alpha" {
		return failedProbe(capabilities, "Модель потеряла результат предыдущего инструмента")
	}
	capabilities.SequentialToolCalls = true
	secondOutput, err := ToolOutput(dialect, second.ToolCalls[0].ID, map[string]bool{
		"complete": true,
	})
	if err != nil {
		return failedProbe(capabilities, "Не удалось вернуть второй результат инструмента")
	}
	input = append(input, second.Continuation...)
	input = append(input, secondOutput)
	third, err := provider.Complete(ctx, Request{
		Instructions: probeInstructions(), Input: input, Tools: tools,
	})
	if err != nil || strings.TrimSpace(third.Text) == "" || len(third.ToolCalls) != 0 {
		return failedProbe(capabilities, "Модель не завершила проверку текстовым ответом")
	}
	capabilities.Text = true
	status := "compatible"
	detail := "Текст и два последовательных строгих вызова инструментов работают"
	if !capabilities.Streaming {
		status = "limited"
		capabilities.Limitations = append(
			capabilities.Limitations,
			"Потоковая выдача модели не подтверждена; активность будет показываться по шагам",
		)
		detail += "; потоковая выдача не подтверждена"
	}
	return ProbeReport{Status: status, Capabilities: capabilities, Detail: detail}
}

func failedProbe(
	capabilities assistant.CapabilitySnapshot,
	detail string,
) ProbeReport {
	if strings.TrimSpace(detail) == "" {
		detail = "Проверка совместимости не завершена"
	}
	return ProbeReport{Status: "unavailable", Capabilities: capabilities, Detail: detail}
}

func probeInstructions() string {
	return "You are a provider compatibility probe. Follow the requested tool-call sequence " +
		"exactly. Never call more than one tool at a time. After probe_finish returns, answer OK."
}

func probeTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name: "probe_echo", Description: "Echo a required value.",
			Parameters: strictObjectSchema(map[string]any{
				"value": map[string]any{"type": "string"},
			}, "value"),
		},
		{
			Name: "probe_finish", Description: "Finish after echo returned.",
			Parameters: strictObjectSchema(map[string]any{
				"previous": map[string]any{"type": "string"},
			}, "previous"),
		},
	}
}

func strictObjectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}
