import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StrictMode } from "react";

import type { Project } from "~entities/project";

import { DictationPreflight } from "./dictation-preflight";

const restoreProperties: Array<() => void> = [];

afterEach(() => {
  cleanup();
  for (const restore of restoreProperties.splice(0).reverse()) restore();
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

describe("DictationPreflight granted microphone", () => {
  test("warms one stream in StrictMode and disables Start until the meter is ready", async () => {
    let resolveMedia!: (stream: MediaStream) => void;
    const getUserMedia = mock(
      () =>
        new Promise<MediaStream>((resolve) => {
          resolveMedia = resolve;
        }),
    );
    const { media, stop, createMediaStreamSource } = mockMicrophone("granted", getUserMedia);
    mockReadyCapabilities();
    const view = renderPreflight(true);

    await waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(1));
    expect(
      screen.getByRole("button", { name: "Начать запись" }).hasAttribute("disabled"),
    ).toBeTrue();
    resolveMedia(media);
    await waitFor(() => expect(createMediaStreamSource).toHaveBeenCalledWith(media));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Начать запись" }).hasAttribute("disabled"),
      ).toBeFalse(),
    );
    expect(getUserMedia).toHaveBeenCalledTimes(1);
    view.unmount();
    expect(stop).toHaveBeenCalledTimes(1);
  });

  test("stops a device that resolves after the preflight has closed", async () => {
    let resolveMedia!: (stream: MediaStream) => void;
    const getUserMedia = mock(
      () =>
        new Promise<MediaStream>((resolve) => {
          resolveMedia = resolve;
        }),
    );
    const { media, stop, createMediaStreamSource } = mockMicrophone("granted", getUserMedia);
    mockReadyCapabilities();
    const view = renderPreflight(true);
    await waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(1));
    view.unmount();
    resolveMedia(media);
    await waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
    expect(createMediaStreamSource).not.toHaveBeenCalled();
  });

  test.each(["prompt", "denied"] as const)(
    "does not automatically request %s permission",
    async (state) => {
      const getUserMedia = mock(() => Promise.reject(new Error("unexpected permission request")));
      mockMicrophone(state, getUserMedia);
      mockReadyCapabilities();
      renderPreflight(true);
      await screen.findByText(state === "prompt" ? "Нужно проверить доступ" : "Доступ запрещён");
      expect(getUserMedia).not.toHaveBeenCalled();
    },
  );
});

function renderPreflight(strict = false) {
  const content = (
    <DictationPreflight
      project={project}
      nodes={[]}
      onClose={() => undefined}
      onStarted={() => undefined}
    />
  );
  return render(strict ? <StrictMode>{content}</StrictMode> : content);
}

function mockReadyCapabilities() {
  mockCapabilities({ available: true, modes: ["realtime"], languages: ["ru", "en"] });
}

function replaceProperty(target: object, key: string, value: unknown) {
  const original = Object.getOwnPropertyDescriptor(target, key);
  Object.defineProperty(target, key, { configurable: true, writable: true, value });
  restoreProperties.push(() => {
    if (original) Object.defineProperty(target, key, original);
    else Reflect.deleteProperty(target, key);
  });
}

function mockMicrophone(state: PermissionState, getUserMedia: () => Promise<MediaStream>) {
  const stop = mock(() => undefined);
  const track = {
    stop,
    muted: false,
    enabled: true,
    readyState: "live",
    addEventListener: mock(() => undefined),
  };
  const media = {
    getTracks: () => [track],
    getAudioTracks: () => [track],
  } as unknown as MediaStream;
  const createMediaStreamSource = mock(() => ({
    connect: mock(() => undefined),
    disconnect: mock(() => undefined),
  }));
  const context = {
    state: "running",
    close: mock(() => Promise.resolve()),
    createMediaStreamSource,
    createAnalyser: () => ({
      fftSize: 1024,
      smoothingTimeConstant: 0,
      getFloatTimeDomainData: (samples: Float32Array) => samples.fill(0.1),
      connect: mock(() => undefined),
      disconnect: mock(() => undefined),
    }),
    createMediaStreamDestination: () => ({ disconnect: mock(() => undefined) }),
  };
  replaceProperty(window, "isSecureContext", true);
  replaceProperty(globalThis, "MediaRecorder", class {});
  replaceProperty(
    globalThis,
    "AudioContext",
    mock(() => context),
  );
  replaceProperty(navigator, "permissions", {
    query: mock(() =>
      Promise.resolve({
        state,
        addEventListener: mock(() => undefined),
        removeEventListener: mock(() => undefined),
      }),
    ),
  });
  replaceProperty(navigator, "mediaDevices", {
    getUserMedia,
    enumerateDevices: mock(() => Promise.resolve([])),
  });
  return { media, stop, createMediaStreamSource };
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
