import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";

import type { Project } from "~entities/project";

import { DictationPreflight } from "./dictation-preflight";

afterEach(() => {
  cleanup();
  mock.restore();
});

describe("DictationPreflight STT gate", () => {
  test("blocks recording instead of accepting an unavailable canned fallback", async () => {
    mockCapabilities({
      available: false,
      reasonCode: "provider_not_configured",
      modes: [],
      deployment: "unavailable",
      streamingProtocol: "unavailable",
      languages: [],
      partialResults: false,
      segmentTimestamps: false,
      wordTimestamps: false,
      confidence: "none",
      vocabularyHints: "none",
      supportsLongAudio: false,
    });

    renderPreflight();

    expect(await screen.findByText("Распознавание речи не настроено")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Начать запись" }).hasAttribute("disabled"),
    ).toBeTrue();
  });

  test("explains honest file-only transcription without claiming live text", async () => {
    mockCapabilities({
      available: true,
      modes: ["file"],
      deployment: "cloud",
      streamingProtocol: "unavailable",
      languages: ["ru", "en"],
      partialResults: false,
      segmentTimestamps: true,
      wordTimestamps: true,
      confidence: "none",
      vocabularyHints: "prompt",
      supportsLongAudio: true,
    });

    renderPreflight();

    expect(await screen.findByText("Без live-текста")).toBeTruthy();
    expect(screen.getByText(/текст после завершения/)).toBeTruthy();
  });

  test("blocks recording when the capability check itself fails", async () => {
    globalThis.fetch = mock(() =>
      Promise.reject(new Error("network unavailable")),
    ) as unknown as typeof fetch;

    renderPreflight();

    expect(await screen.findByText("Не удалось проверить распознавание речи")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Начать запись" }).hasAttribute("disabled"),
    ).toBeTrue();
  });
});

function renderPreflight() {
  render(
    <DictationPreflight
      project={project}
      nodes={[]}
      onClose={() => undefined}
      onStarted={() => undefined}
    />,
  );
}

function mockCapabilities(capabilities: Record<string, unknown>) {
  globalThis.fetch = mock(() =>
    Promise.resolve(
      new Response(JSON.stringify(capabilities), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  ) as unknown as typeof fetch;
}

const project: Project = {
  id: "project-1",
  title: "Проверка диктовки",
  kind: "story",
  language: "ru",
  synopsis: "",
  status: "active",
  structureRevision: 1,
  structure: {
    schemaVersion: 1,
    template: "story",
    rootLabel: "Рассказ",
    nodeTypes: [],
  },
  createdAt: "2026-08-01T00:00:00Z",
  updatedAt: "2026-08-01T00:00:00Z",
};
