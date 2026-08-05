import { expect, type Page, test } from "@playwright/test";

const failureAudio = Buffer.from("FAIL_STT_ONCE fixture");
const failureSHA = "3314e11921f4e0b8c1c0494e01dea17ae9aad53dc964d9cd7b224f692ad8a10b";

test("P-02 keeps local recording through live loss and recovers from batch failure", async ({
  page,
  context,
}, testInfo) => {
  const suffix = `p02-${testInfo.project.name}-${Date.now()}`;
  const { projectId } = await registerAndCreateProject(page, suffix);
  await installRecoverableFailureInterception(page);

  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
    await expect(page.getByText("Доступ разрешён, сигнал поступает")).toBeVisible();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page.getByText(/Дом стоял у самой воды/)).toBeVisible({ timeout: 20_000 });

  const chunksBeforeLoss = await localChunkCount(page);
  await context.setOffline(true);
  await expect(page.getByText("Нет соединения")).toBeVisible();
  await expect
    .poll(() => localChunkCount(page), { timeout: 10_000 })
    .toBeGreaterThan(chunksBeforeLoss);
  await expect(page.getByText(/Запись продолжается и сохраняется/)).toBeVisible();

  await context.setOffline(false);
  await expect(page.getByText("Нет соединения")).toBeHidden({ timeout: 15_000 });
  await expect.poll(() => uploadedLocalChunkCount(page), { timeout: 15_000 }).toBeGreaterThan(0);

  await page.getByRole("button", { name: "Завершить" }).click();
  await expect(page).toHaveURL(/\/dictations\/([^/]+)\/finalizing/);
  const sessionId = page.url().match(/\/dictations\/([^/]+)\//)?.[1] ?? "";
  await expect(page.getByRole("heading", { name: "Аудио сохранено" })).toBeVisible({
    timeout: 25_000,
  });
  await expect(page.getByRole("link", { name: "Проверить живой черновик" })).toBeVisible();

  const audio = await page.request.get(`/api/dictations/${sessionId}/audio/content`);
  expect(audio.ok()).toBe(true);
  expect((await audio.body()).byteLength).toBeGreaterThan(failureAudio.byteLength);

  await page.getByRole("link", { name: "Проверить живой черновик" }).click();
  await expect(page.locator(".tiptap")).toContainText("Дом стоял у самой воды");
  await page.goto(`/dictations/${sessionId}/finalizing`);
  await page.getByRole("button", { name: "Повторить обработку" }).click();
  await expect(page).toHaveURL(new RegExp(`/dictations/${sessionId}/review$`), {
    timeout: 30_000,
  });
  await expect(page.locator(".tiptap")).toContainText("Дом стоял у самой воды");

  const finalState = await getDictation(page, sessionId);
  expect(finalState.status).toBe("review_ready");
  expect(finalState.batchArtifact?.provider).toBe("fixture");
  expect(finalState.liveArtifact?.text).toContain("Дом стоял у самой воды");
  expect(finalState.draft?.plainText).toBeTruthy();
  await page.screenshot({
    path: `artifacts/${testInfo.project.name}-p02-recovered.png`,
    fullPage: true,
  });
  expect(projectId).toBeTruthy();
});

test("P-03 protects author edits, supports diff undo, compare, and restore", async ({
  page,
}, testInfo) => {
  const suffix = `p03-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const sessionId = await createRecoverableDictationViaAPI(page, projectId, nodeId);

  await expect
    .poll(async () => (await getDictation(page, sessionId)).status, { timeout: 25_000 })
    .toBe("finalization_failed");
  await page.goto(`/dictations/${sessionId}/finalizing`);
  await page.getByRole("link", { name: "Проверить живой черновик" }).click();
  await page.locator(".tiptap").click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type(" Авторская строка не должна исчезнуть.");
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({ timeout: 10_000 });

  const beforeRetry = await getDictation(page, sessionId);
  const protectedContent = beforeRetry.draft?.content;
  const protectedText = beforeRetry.draft?.plainText;
  expect(protectedText).toContain("Авторская строка не должна исчезнуть.");

  await page.goto(`/dictations/${sessionId}/finalizing`);
  await page.getByRole("button", { name: "Повторить обработку" }).click();
  await expect(page).toHaveURL(new RegExp(`/dictations/${sessionId}/review$`), {
    timeout: 30_000,
  });
  const afterRetry = await getDictation(page, sessionId);
  expect(afterRetry.draft?.plainText).toBe(protectedText);
  expect(afterRetry.batchArtifact?.text).not.toBe(afterRetry.draft?.baseSourceText);
  await expect(page.getByText("Готова уточнённая расшифровка")).toBeVisible();

  await page.getByRole("link", { name: "Сравнить", exact: true }).first().click();
  await page.getByRole("checkbox").first().check();
  await page.getByRole("button", { name: "Применить выбранное" }).click();
  await expect(page).toHaveURL(new RegExp(`/dictations/${sessionId}/review$`));
  await expect(page.getByText("Применена финальная расшифровка")).toBeVisible();
  await page.locator('[data-radix-toast-announce-alt="Отменить"]').click();
  await expect
    .poll(async () => JSON.stringify((await getDictation(page, sessionId)).draft?.content))
    .toBe(JSON.stringify(protectedContent));

  const originalBatchID = (await getDictation(page, sessionId)).batchArtifact?.id;
  await page.evaluate(async (id) => {
    const response = await fetch(`/api/projects/${id}/dictionary`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        canonical: "Домоводье",
        variants: ["Домоводье"],
        kind: "place",
        language: "ru",
      }),
    });
    if (!response.ok) {
      throw new Error(`dictionary fixture failed: ${response.status}`);
    }
  }, projectId);
  await page.getByRole("button", { name: "Повторить расшифровку со словарём" }).click();
  await expect(page.getByText("Повторно расшифровываем со словарём")).toBeVisible();
  await expect
    .poll(async () => (await getDictation(page, sessionId)).suggestionArtifact?.id, {
      timeout: 25_000,
    })
    .not.toBeFalsy();
  const afterRetranscription = await getDictation(page, sessionId);
  expect(afterRetranscription.batchArtifact?.id).toBe(originalBatchID);
  expect(afterRetranscription.suggestionArtifact?.text).toContain("Домоводье");
  expect(afterRetranscription.draft?.plainText).toBe(protectedText);

  const beforeTransfer = await getNode(page, nodeId);
  await page.getByRole("button", { name: "Вставить в рукопись" }).click();
  await page.getByLabel("Документ", { exact: true }).selectOption(nodeId);
  await page.getByRole("button", { name: "Подтвердить вставку" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}`));
  await expect(page.locator(".tiptap")).toContainText("Авторская строка не должна исчезнуть");
  await page.locator('[data-radix-toast-announce-alt="Отменить"]').click();
  await expect
    .poll(async () => (await getNode(page, nodeId)).plainText)
    .toBe(beforeTransfer.plainText);

  await page.goto(`/dictations/${sessionId}/review`);
  await page.getByRole("button", { name: "Вставить в рукопись" }).click();
  await page.getByLabel("Документ", { exact: true }).selectOption(nodeId);
  await page.getByRole("button", { name: "Подтвердить вставку" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}`));
  await expect(page.locator(".tiptap")).toContainText("Авторская строка не должна исчезнуть");

  await page.goto(`/nodes/${nodeId}/versions`);
  await page.getByRole("button", { name: "Сохранить версию" }).click();
  await page.getByRole("textbox", { name: "Название версии" }).fill("Контрольная версия");
  await page.getByRole("button", { name: "Сохранить снимок" }).click();
  await expect(page.getByRole("heading", { name: "Контрольная версия" })).toBeVisible();

  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await page.locator(".tiptap").click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type(" Позднее изменение.");
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({ timeout: 10_000 });
  const beforeCompare = await getNode(page, nodeId);

  await page.goto(`/nodes/${nodeId}/versions`);
  await expect(page.getByText("Сравнение работает только для чтения")).toBeVisible();
  await expect(page.getByRole("region", { name: "Сравнение редакций" })).toContainText(
    "Позднее изменение",
  );
  const afterCompare = await getNode(page, nodeId);
  expect(afterCompare.currentRevision).toBe(beforeCompare.currentRevision);
  expect(afterCompare.plainText).toBe(beforeCompare.plainText);

  await page
    .getByRole("article")
    .filter({ has: page.getByRole("heading", { name: "Контрольная версия" }) })
    .getByRole("button", { name: "Восстановить" })
    .click();
  await page
    .getByRole("alertdialog", { name: "Восстановить «Контрольная версия»?" })
    .getByRole("button", { name: "Восстановить" })
    .click();
  await expect
    .poll(async () => (await getNode(page, nodeId)).currentRevision)
    .toBeGreaterThan(beforeCompare.currentRevision);
  const restored = await getNode(page, nodeId);
  expect(restored.plainText).not.toContain("Позднее изменение");
  expect(restored.plainText).toContain("Авторская строка не должна исчезнуть");

  const history = await page.evaluate(async (id) => {
    const response = await fetch(`/api/nodes/${id}/versions`, { credentials: "include" });
    return response.json();
  }, nodeId);
  expect(history.versions).toHaveLength(1);
  expect(
    history.revisions.some(
      (revision: { revision: number }) => revision.revision === beforeCompare.currentRevision,
    ),
  ).toBe(true);
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
  expect(match).toBeTruthy();
  return { projectId: match?.[1] ?? "", nodeId: match?.[2] ?? "" };
}

async function installRecoverableFailureInterception(page: Page): Promise<void> {
  await page.route(/\/api\/dictations\/[^/]+\/chunks\/0$/, async (route) => {
    const headers: Record<string, string> = {
      ...route.request().headers(),
      "x-content-sha256": failureSHA,
    };
    delete headers["content-length"];
    const response = await route.fetch({ headers, postData: failureAudio });
    await route.fulfill({ response });
  });
  await page.route(/\/api\/dictations\/[^/]+\/stop$/, async (route) => {
    const body = route.request().postDataJSON() as {
      manifest: { chunks: Array<{ sequence: number; sha256: string }> };
    };
    const first = body.manifest.chunks.find((chunk) => chunk.sequence === 0);
    if (first) {
      first.sha256 = failureSHA;
    }
    const response = await route.fetch({ postData: JSON.stringify(body) });
    await route.fulfill({ response });
  });
}

async function createRecoverableDictationViaAPI(
  page: Page,
  projectId: string,
  nodeId: string,
): Promise<string> {
  return page.evaluate(
    async ({ projectId: project, nodeId: node, sha }) => {
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
      await new Promise<void>((resolve, reject) => {
        const socket = new WebSocket(
          `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/dictations/${sessionId}/live`,
        );
        const timer = window.setTimeout(() => reject(new Error("live fixture timeout")), 10_000);
        socket.addEventListener("open", () => {
          socket.send(
            JSON.stringify({
              type: "fixture_segment",
              text: "Живой авторский черновик.",
              startMs: 0,
              endMs: 2_000,
            }),
          );
        });
        socket.addEventListener("message", (event) => {
          const message = JSON.parse(String(event.data));
          if (message.type === "segment_final") {
            window.clearTimeout(timer);
            socket.close();
            resolve();
          }
        });
      });
      const bytes = new TextEncoder().encode("FAIL_STT_ONCE fixture");
      const upload = await fetch(`/api/dictations/${sessionId}/chunks/0`, {
        method: "PUT",
        credentials: "include",
        headers: {
          "Content-Type": "audio/webm",
          "X-Content-SHA256": sha,
          "X-Audio-Duration-Ms": "2000",
        },
        body: bytes,
      });
      if (!upload.ok) {
        throw new Error(`fixture upload failed: ${upload.status}`);
      }
      const stopped = await fetch(`/api/dictations/${sessionId}/stop`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          durationMs: 2_000,
          recordedDurationMs: 2_000,
          manifest: { format: "webm", chunks: [{ sequence: 0, sha256: sha }] },
        }),
      });
      if (!stopped.ok) {
        throw new Error(`fixture stop failed: ${stopped.status}`);
      }
      return sessionId;
    },
    { projectId, nodeId, sha: failureSHA },
  );
}

async function getDictation(page: Page, sessionId: string) {
  return page.evaluate(async (id) => {
    const response = await fetch(`/api/dictations/${id}`, { credentials: "include" });
    return (await response.json()).session;
  }, sessionId);
}

async function getNode(page: Page, nodeId: string) {
  return page.evaluate(async (id) => {
    const response = await fetch(`/api/nodes/${id}`, { credentials: "include" });
    return (await response.json()).node;
  }, nodeId);
}

async function localChunkCount(page: Page): Promise<number> {
  return countLocalChunks(page, false);
}

async function uploadedLocalChunkCount(page: Page): Promise<number> {
  return countLocalChunks(page, true);
}

function countLocalChunks(page: Page, uploadedOnly: boolean): Promise<number> {
  return page.evaluate((onlyUploaded) => {
    return new Promise<number>((resolve, reject) => {
      const open = indexedDB.open("writers-studio-recovery", 1);
      open.onerror = () => reject(open.error);
      open.onsuccess = () => {
        const request = open.result.transaction("audioChunks").objectStore("audioChunks").getAll();
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const chunks = request.result as Array<{ uploaded: boolean }>;
          resolve(onlyUploaded ? chunks.filter((chunk) => chunk.uploaded).length : chunks.length);
        };
      };
    });
  }, uploadedOnly);
}
