import { expect, type Page, test } from "@playwright/test";

test("chat-started assistant keeps the voice threshold and a visible draft across mode switches", async ({
  page,
}, testInfo) => {
  await captureAssistantVoiceSockets(page);
  const suffix = `assistant-live-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  await page.evaluate(() => {
    const key = "writers-studio-preferences";
    const preferences = JSON.parse(localStorage.getItem(key) ?? "{}");
    localStorage.setItem(
      key,
      JSON.stringify({ ...preferences, assistantSilenceCheckpointMs: 3000 }),
    );
  });
  const runId = await launchAssistant(page, projectId, nodeId, "chat");
  const runResponse = await page.request.get(`/api/assistant/runs/${runId}`);
  const runBody = (await runResponse.json()) as {
    run: {
      silenceCheckpointMs: number;
      readContext: { writerPreferences?: Record<string, unknown> };
    };
  };
  expect(runBody.run.silenceCheckpointMs).toBe(3000);
  expect(runBody.run.readContext.writerPreferences).toMatchObject({
    language: "ru",
    assistantSilenceCheckpointMs: 3000,
  });
  expect(runBody.run.readContext.writerPreferences).not.toHaveProperty("microphoneId");

  const assistantPanel = page.getByRole("complementary", { name: "Панель ассистента" });
  await assistantPanel.getByRole("button", { name: "Голосом", exact: true }).click();
  await assistantPanel.getByRole("button", { name: "Начать", exact: true }).click();
  await sendFixtureToCapturedVoiceSocket(page, "Напиши сцену встречи Лиора у старой башни.");
  const transcript = page.getByRole("textbox", { name: "Стенограмма поручения" });
  await expect(transcript).toHaveValue(/Напиши сцену встречи Лиора у старой башни/);

  await assistantPanel.getByRole("button", { name: "Пауза", exact: true }).click();
  await assistantPanel.getByRole("button", { name: "Чат", exact: true }).click();
  await expect(page.getByText("Голосовой черновик сохранён")).toBeVisible();
  await page.getByRole("button", { name: "Перенести в чат" }).click();
  await expect(page.getByRole("textbox", { name: "Поручение ассистенту" })).toHaveValue(
    /Напиши сцену встречи Лиора у старой башни/,
  );

  await expect
    .poll(async () => {
      const response = await page.request.get(`/api/assistant/runs/${runId}/turns`);
      const body = (await response.json()) as { turns: unknown[] };
      return body.turns.length;
    })
    .toBe(0);
});

test("assistant voice distinguishes permission setup and cancels a late microphone result", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: () =>
        new Promise<MediaStream>((resolve) => {
          (
            window as typeof window & {
              __resolvePendingAssistantMicrophone?: (stream: MediaStream) => void;
            }
          ).__resolvePendingAssistantMicrophone = resolve;
        }),
    });
  });
  const suffix = `assistant-pending-mic-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  await launchAssistant(page, projectId, nodeId);

  const assistantPanel = page.getByRole("complementary", { name: "Панель ассистента" });
  await assistantPanel.getByRole("button", { name: "Начать", exact: true }).click();
  await expect(page.getByText("Подключаю голос")).toBeVisible();
  await page.getByRole("button", { name: "Отмена" }).click();
  await expect(page.getByText("Голосовой live-режим")).toBeVisible();
  await expect(assistantPanel.getByRole("button", { name: "Начать", exact: true })).toBeVisible();

  await page.evaluate(() => {
    const scope = window as typeof window & {
      __resolvePendingAssistantMicrophone?: (stream: MediaStream) => void;
    };
    scope.__resolvePendingAssistantMicrophone?.(new MediaStream());
  });
  await expect(assistantPanel.getByRole("button", { name: "Начать", exact: true })).toBeVisible();
  await expect(assistantPanel.getByRole("button", { name: "Пауза", exact: true })).toBeHidden();
});

test("durable silence checkpoint restores, edits and continues the same voice turn", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-checkpoint-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const runId = await launchAssistant(page, projectId, nodeId);
  await createVoiceCheckpoint(page, runId, "Сделай диалог менее прямолинейным.");

  const assistantPanel = page.getByRole("complementary", { name: "Панель ассистента" });
  await assistantPanel.getByRole("button", { name: "Начать", exact: true }).click();
  await expect(page.getByText("Проверьте стенограмму")).toBeVisible();
  const transcript = page.getByRole("textbox", { name: "Стенограмма поручения" });
  await expect(transcript).toHaveValue("Сделай диалог менее прямолинейным.");
  await transcript.fill("Сделай диалог менее прямолинейным, но сохрани последнюю реплику.");

  await expect
    .poll(async () => {
      const response = await page.request.get(`/api/assistant/runs/${runId}/turns`);
      const body = (await response.json()) as {
        turns: Array<{ state: string; transcript: string }>;
      };
      return body.turns[0]?.state;
    })
    .toBe("awaiting_confirmation");

  await page.getByRole("button", { name: "Продолжить" }).click();
  await expect(assistantPanel.getByRole("button", { name: "Пауза", exact: true })).toBeVisible();
  await assistantPanel.getByRole("button", { name: "Пауза", exact: true }).click();
  await expect
    .poll(async () => {
      const response = await page.request.get(`/api/assistant/runs/${runId}/turns`);
      const body = (await response.json()) as {
        turns: Array<{ state: string; transcript: string }>;
      };
      return body.turns[0]?.state;
    })
    .toBe("collecting");
});

test("global assistant persists project scope without a hidden node target", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-global-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const runId = await launchAssistant(page, projectId, nodeId, "chat", "project");

  const assistantPanel = page.getByRole("complementary", { name: "Панель ассистента" });
  await expect(assistantPanel.getByText("Весь проект · fixture", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Редактор документа" })).toBeVisible();

  const response = await page.request.get(`/api/assistant/runs/${runId}`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as {
    run: { targetKind: string; targetNodeId?: string | null };
  };
  expect(body.run.targetKind).toBe("project");
  expect(body.run.targetNodeId ?? null).toBeNull();
});

async function captureAssistantVoiceSockets(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;
    const CapturedWebSocket = new Proxy(NativeWebSocket, {
      construct(target, argumentsList) {
        const socket = Reflect.construct(target, argumentsList) as WebSocket;
        if (
          String(argumentsList[0]).includes("/api/assistant/runs/") &&
          String(argumentsList[0]).endsWith("/voice")
        ) {
          const scope = window as typeof window & { __assistantVoiceSockets?: WebSocket[] };
          scope.__assistantVoiceSockets = [...(scope.__assistantVoiceSockets ?? []), socket];
        }
        return socket;
      },
    });
    Object.defineProperty(window, "WebSocket", { value: CapturedWebSocket });
  });
}

async function sendFixtureToCapturedVoiceSocket(page: Page, text: string): Promise<void> {
  await page.evaluate(async (fixtureText) => {
    const scope = window as typeof window & { __assistantVoiceSockets?: WebSocket[] };
    const deadline = Date.now() + 10_000;
    while (Date.now() < deadline) {
      const socket = scope.__assistantVoiceSockets?.at(-1);
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(
          JSON.stringify({
            type: "fixture_segment",
            text: fixtureText,
            startMs: 0,
            endMs: 1800,
            confidence: 0.99,
          }),
        );
        return;
      }
      await new Promise((resolve) => window.setTimeout(resolve, 50));
    }
    throw new Error("assistant voice socket did not open");
  }, text);
}

async function createVoiceCheckpoint(page: Page, runId: string, text: string): Promise<void> {
  await page.evaluate(
    ({ assistantRunId, fixtureText }) =>
      new Promise<void>((resolve, reject) => {
        const socket = new WebSocket(
          `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/api/assistant/runs/${assistantRunId}/voice`,
        );
        const timer = window.setTimeout(() => {
          socket.close();
          reject(new Error("assistant checkpoint timeout"));
        }, 15_000);
        socket.onopen = () => {
          socket.send(
            JSON.stringify({
              type: "fixture_segment",
              text: fixtureText,
              startMs: 0,
              endMs: 1600,
              confidence: 0.99,
            }),
          );
        };
        socket.onmessage = (event) => {
          const message = JSON.parse(String(event.data)) as { type?: string };
          if (message.type !== "confirmation_required") return;
          window.clearTimeout(timer);
          socket.close();
          resolve();
        };
        socket.onerror = () => {
          window.clearTimeout(timer);
          reject(new Error("assistant checkpoint socket failed"));
        };
      }),
    { assistantRunId: runId, fixtureText: text },
  );
}

async function launchAssistant(
  page: Page,
  projectId: string,
  nodeId: string,
  inputMode: "voice" | "chat" = "voice",
  targetKind: "node" | "project" = "node",
): Promise<string> {
  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await page.getByRole("button", { name: "Ассистент" }).click();
  await expect(page.getByRole("heading", { name: "Что поручить?" })).toBeVisible();
  if (targetKind === "project") {
    await page.getByRole("button", { name: "Весь проект", exact: true }).click();
  }
  await page.getByRole("button", { name: "Голосом", exact: true }).click();
  const start = page.getByRole("button", { name: "Открыть голосовой чат" });
  await expect(start).toBeEnabled({ timeout: 30_000 });
  await start.click();
  await expect.poll(() => new URL(page.url()).searchParams.get("assistantRunId")).not.toBeNull();
  if (inputMode === "chat") {
    await page
      .getByRole("complementary", { name: "Панель ассистента" })
      .getByRole("button", { name: "Чат", exact: true })
      .click();
  }
  return new URL(page.url()).searchParams.get("assistantRunId") ?? "";
}

async function registerAndCreateProject(
  page: Page,
  suffix: string,
): Promise<{ projectId: string; nodeId: string }> {
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill(`Проект ${suffix}`);
  await page.getByRole("button", { name: "Создать и открыть" }).click();
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/);
  const [, , projectId = "", , nodeId = ""] = new URL(page.url()).pathname.split("/");
  return { projectId, nodeId };
}
