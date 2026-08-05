import { expect, type Page, test } from "@playwright/test";

test("voice command grammar, undo, command palette, and keyboard map stay functional", async ({
  page,
}, testInfo) => {
  const suffix = `commands-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);

  // The URL changes before the lazy Studio chunk has mounted its global
  // shortcut listener. Wait for an editor control that is installed with it.
  await expect(page.getByRole("button", { name: "Режим фокуса" }).first()).toBeVisible();
  await page.keyboard.press("ControlOrMeta+k");
  const palette = page.getByRole("dialog", { name: "Палитра команд" });
  await expect(palette).toBeVisible();
  await palette.getByRole("button", { name: /Режим фокуса/ }).click();
  await expect(page.getByRole("button", { name: "Выйти из фокуса" })).toBeVisible();
  await page.keyboard.press("ControlOrMeta+Shift+Enter");
  await expect(page.getByRole("button", { name: "Выйти из фокуса" })).toBeHidden();

  const collapseTree = page.getByRole("button", { name: "Свернуть структуру проекта" });
  if (await collapseTree.isVisible()) {
    await collapseTree.click();
    await expect(page.getByRole("button", { name: "Развернуть структуру проекта" })).toBeVisible();
    await page.getByRole("button", { name: "Развернуть структуру проекта" }).click();
  } else {
    await expect(collapseTree).toBeHidden();
  }

  await page.keyboard.press("ControlOrMeta+Shift+s");
  const versionDialog = page.getByRole("alertdialog", { name: "Сохранить именованную версию" });
  await versionDialog.getByRole("textbox", { name: "Название версии" }).fill("Версия с клавиатуры");
  await versionDialog.getByRole("button", { name: "Сохранить версию" }).click();
  await expect(page.getByText("Именованная версия создана", { exact: true })).toBeVisible();

  const commandResult = await exerciseVoiceCommands(page, projectId, nodeId);
  expect(commandResult.disabledCommand).toBeNull();
  expect(commandResult.executedKind).toBe("new_paragraph");
  expect(commandResult.executed).toBe(true);
  expect(commandResult.undoneSegmentId).toBe(commandResult.executedSegmentId);
  expect(commandResult.undoKind).toBe("undo_command");
  expect(commandResult.dialogueKind).toBe("dialogue_start");
  expect(commandResult.dialogueExecuted).toBe(true);
  expect(commandResult.dialogueText).toBe("Дорогой мистер Беннет, сказала она.");

  await page.keyboard.press("ControlOrMeta+Shift+f");
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/search`));
});

test("voice navigation opens the spoken sentence for editing and returns to live dictation", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    const browserWindow = window as typeof window & {
      __writersLiveSockets?: WebSocket[];
      __writersLiveMessages?: unknown[];
    };
    browserWindow.__writersLiveSockets = [];
    browserWindow.__writersLiveMessages = [];
    const NativeWebSocket = window.WebSocket;
    window.WebSocket = new Proxy(NativeWebSocket, {
      construct(Target, argumentsList) {
        const socket = Reflect.construct(Target, argumentsList) as WebSocket;
        if (String(argumentsList[0]).includes("/live")) {
          browserWindow.__writersLiveSockets?.push(socket);
          socket.addEventListener("message", (event) => {
            if (typeof event.data !== "string") {
              return;
            }
            try {
              browserWindow.__writersLiveMessages?.push(JSON.parse(event.data));
            } catch {
              // Ignore non-JSON diagnostics.
            }
          });
        }
        return socket;
      },
    });
  });

  const suffix = `voice-edit-${testInfo.project.name}-${Date.now()}`;
  await registerAndCreateProject(page, suffix);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page).toHaveURL(/\/dictate\//);
  await expect(page.getByText(/Дом стоял у самой воды/)).toBeVisible({ timeout: 20_000 });

  const oldSentence = "Дракон не чистил зубы по утрам, и зубы у него заболели.";
  await sendTrackedLiveFixture(page, oldSentence, 20_000, 24_000);
  await expect(page.getByText(oldSentence, { exact: true })).toBeVisible();
  await sendTrackedLiveFixture(
    page,
    "Команда, вернись к предложению зубы у него заболели",
    24_000,
    25_000,
  );

  const editor = page.getByRole("textbox", { name: /Правка абзаца/ });
  await expect(editor).toBeVisible();
  await expect(page.getByRole("button", { name: "Продолжить", exact: true })).toBeVisible();
  await expect
    .poll(() =>
      editor.evaluate((element) => {
        const textarea = element as HTMLTextAreaElement;
        return textarea.value.slice(textarea.selectionStart, textarea.selectionEnd);
      }),
    )
    .toBe(oldSentence);

  const correctedSentence = "Дракон ел много сладкого, и поэтому зубы у него заболели.";
  await page.keyboard.insertText(correctedSentence);
  await expect(editor).toHaveValue(new RegExp(correctedSentence));
  await page.getByRole("button", { name: "К концу и продолжить" }).click();
  await expect(editor).toBeHidden();
  await expect(page.getByRole("button", { name: "Пауза" })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const messages = (
          window as typeof window & { __writersLiveMessages?: Array<{ type?: string }> }
        ).__writersLiveMessages;
        return messages?.some((message) => message.type === "segments_replaced") ?? false;
      }),
    )
    .toBe(true);
  await expect(page.getByText(correctedSentence, { exact: false })).toBeVisible();
  await expect(page.getByText(oldSentence, { exact: true })).toBeHidden();

  const continuation = "И вот решил дракон полететь к стоматологу.";
  await sendTrackedLiveFixture(page, continuation, 25_000, 28_000);
  await expect(page.getByText(continuation, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Завершить" }).click();
  await expect(page).toHaveURL(/\/dictations\/[^/]+\/(finalizing|review)/, {
    timeout: 20_000,
  });
});

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
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  return { projectId: match?.[1] ?? "", nodeId: match?.[2] ?? "" };
}

function exerciseVoiceCommands(page: Page, projectId: string, nodeId: string) {
  return page.evaluate(
    async ({ projectId: project, nodeId: node }) => {
      const created = await fetch("/api/dictations", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          projectId: project,
          destinationNodeId: node,
          clientSessionId: crypto.randomUUID(),
          language: "ru",
        }),
      }).then((response) => response.json());
      const sessionId = created.session.id as string;
      return new Promise<{
        disabledCommand: unknown;
        executedKind?: string;
        executed: boolean;
        executedSegmentId?: string;
        undoneSegmentId?: string;
        undoKind?: string;
        dialogueKind?: string;
        dialogueExecuted: boolean;
        dialogueText?: string;
      }>((resolve, reject) => {
        const socket = new WebSocket(
          `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/dictations/${sessionId}/live`,
        );
        const timeout = window.setTimeout(
          () => reject(new Error("command fixture timeout")),
          10_000,
        );
        let stage = 0;
        let executedSegmentId = "";
        let undoneSegmentId = "";
        let disabledCommand: unknown;
        let executedKind: string | undefined;
        let executed = false;
        let undoKind: string | undefined;
        let dialogueKind: string | undefined;
        let dialogueExecuted = false;
        socket.addEventListener("open", () => {
          socket.send(
            JSON.stringify({
              type: "configure",
              commandsEnabled: false,
              commandPrefix: "Команда",
            }),
          );
          socket.send(
            JSON.stringify({
              type: "fixture_segment",
              text: "Команда, новый абзац",
              startMs: 0,
              endMs: 1_000,
            }),
          );
        });
        socket.addEventListener("message", (event) => {
          const message = JSON.parse(String(event.data));
          if (message.type === "command_updated") {
            undoneSegmentId = message.segmentId;
            return;
          }
          if (message.type !== "segment_final") {
            return;
          }
          if (stage === 0) {
            disabledCommand = message.command ?? null;
            stage = 1;
            socket.send(
              JSON.stringify({
                type: "configure",
                commandsEnabled: true,
                commandPrefix: "Команда",
              }),
            );
            socket.send(
              JSON.stringify({
                type: "fixture_segment",
                text: "Команда, новый абзац",
                startMs: 1_000,
                endMs: 2_000,
              }),
            );
            return;
          }
          if (stage === 1) {
            executedSegmentId = message.segmentId;
            executedKind = message.command?.kind;
            executed = message.command?.executed === true;
            stage = 2;
            socket.send(
              JSON.stringify({
                type: "fixture_segment",
                text: "Команда, отменить команду",
                startMs: 2_000,
                endMs: 3_000,
              }),
            );
            return;
          }
          if (stage === 2) {
            undoKind = message.command?.kind;
            stage = 3;
            socket.send(
              JSON.stringify({
                type: "fixture_segment",
                text: "Команда, начало диалога",
                startMs: 3_000,
                endMs: 4_000,
              }),
            );
            return;
          }
          if (stage === 3) {
            dialogueKind = message.command?.kind;
            dialogueExecuted = message.command?.executed === true;
            stage = 4;
            socket.send(
              JSON.stringify({
                type: "fixture_segment",
                text: "Дорогой мистер Беннет, сказала она.",
                startMs: 4_000,
                endMs: 6_000,
              }),
            );
            return;
          }
          window.clearTimeout(timeout);
          socket.close();
          resolve({
            disabledCommand,
            executedKind,
            executed,
            executedSegmentId,
            undoneSegmentId,
            undoKind,
            dialogueKind,
            dialogueExecuted,
            dialogueText: message.text,
          });
        });
      });
    },
    { projectId, nodeId },
  );
}

async function sendTrackedLiveFixture(
  page: Page,
  text: string,
  startMs: number,
  endMs: number,
): Promise<void> {
  await page.waitForFunction(() => {
    const sockets = (window as typeof window & { __writersLiveSockets?: WebSocket[] })
      .__writersLiveSockets;
    return sockets?.some((socket) => socket.readyState === WebSocket.OPEN) ?? false;
  });
  await page.evaluate(
    ({ fixtureText, fixtureStartMs, fixtureEndMs }) => {
      const sockets = (window as typeof window & { __writersLiveSockets?: WebSocket[] })
        .__writersLiveSockets;
      const socket = sockets?.findLast((candidate) => candidate.readyState === WebSocket.OPEN);
      if (!socket) {
        throw new Error("live dictation socket is not open");
      }
      socket.send(
        JSON.stringify({
          type: "fixture_segment",
          text: fixtureText,
          startMs: fixtureStartMs,
          endMs: fixtureEndMs,
          confidence: 0.99,
        }),
      );
    },
    { fixtureText: text, fixtureStartMs: startMs, fixtureEndMs: endMs },
  );
}
