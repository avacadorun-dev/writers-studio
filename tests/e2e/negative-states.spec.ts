import { expect, type Page, test } from "@playwright/test";

test("microphone check displays a live non-zero input level", async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        const context = new AudioContext();
        const oscillator = context.createOscillator();
        const quietInput = context.createGain();
        const destination = context.createMediaStreamDestination();
        quietInput.gain.value = 0.001;
        oscillator.connect(quietInput);
        quietInput.connect(destination);
        oscillator.start();
        await context.resume();
        (
          window as typeof window & {
            writersStudioMeterTestSource?: { context: AudioContext; oscillator: OscillatorNode };
          }
        ).writersStudioMeterTestSource = { context, oscillator };
        return destination.stream;
      },
    });
  });
  await registerAndCreateProject(page, `meter-${testInfo.project.name}-${Date.now()}`);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
  }

  const meter = page.getByLabel("Уровень микрофона");
  await expect(meter).toBeAttached();
  await expect
    .poll(() => meter.evaluate((element: HTMLMeterElement) => element.value))
    .toBeGreaterThan(0);
  await expect
    .poll(() => meter.evaluate((element: HTMLMeterElement) => element.value))
    .toBeLessThan(30);
});

test("microphone check explains a pending browser permission instead of looking frozen", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.permissions, "query", {
      configurable: true,
      value: async () => ({
        state: "prompt",
        addEventListener: () => undefined,
      }),
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: () => new Promise<MediaStream>(() => undefined),
    });
  });
  await registerAndCreateProject(page, `pending-mic-${testInfo.project.name}-${Date.now()}`);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await page.getByRole("button", { name: "Проверить" }).click();

  await expect(page.getByText("Ждём разрешения браузера…")).toBeVisible();
  await expect(page.getByText("Разрешите доступ во всплывающем окне браузера")).toBeVisible();
  await expect(page.getByRole("button", { name: "Ждём…" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Начать запись" })).toBeDisabled();
});

test("insecure HTTP origin explains why microphone capture is unavailable", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: false,
    });
  });
  await registerAndCreateProject(page, `insecure-mic-${testInfo.project.name}-${Date.now()}`);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();

  await expect(page.getByText("Нужен HTTPS или localhost")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Браузер блокирует микрофон на HTTP-адресе" }),
  ).toBeVisible();
  await expect(page.getByText(/Откройте приложение по HTTPS/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Начать запись" })).toBeDisabled();
  await expect(page.getByRole("link", { name: "Открыть localhost" })).toHaveAttribute(
    "href",
    /^http:\/\/localhost(?::\d+)?\/projects\//,
  );
});

test("offline start keeps a local recording and deletion requires the exact confirmation", async ({
  page,
  context,
}, testInfo) => {
  const suffix = `offline-${testInfo.project.name}-${Date.now()}`;
  const { projectTitle } = await registerAndCreateProject(page, suffix);
  const dictateButton = page.getByRole("banner").getByRole("button", { name: "Диктовать" });
  await expect(dictateButton).toBeVisible();
  await context.setOffline(true);
  await dictateButton.click();
  await expect(page.getByText("Можно записывать без сети")).toBeVisible();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page.getByText("Нет соединения")).toBeVisible();
  await expect.poll(() => localChunkCount(page), { timeout: 10_000 }).toBeGreaterThan(0);
  await page.getByRole("button", { name: "Завершить" }).click();
  await expect(page).toHaveURL(/\/recovery$/);
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();

  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Скачать аудио" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toMatch(/^writers-studio-.+\.(webm|ogg)$/);

  await page.getByRole("button", { name: "Удалить с устройства" }).click();
  const deleteDialog = page.getByRole("alertdialog", {
    name: "Удалить запись с этого устройства?",
  });
  await deleteDialog.getByRole("textbox", { name: "Подтверждение" }).fill("НЕВЕРНО");
  await expect(deleteDialog.getByRole("button", { name: "Удалить безвозвратно" })).toBeDisabled();
  await deleteDialog.getByRole("button", { name: "Отмена" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();

  await page.getByRole("button", { name: "Удалить с устройства" }).click();
  await deleteDialog.getByRole("textbox", { name: "Подтверждение" }).fill("УДАЛИТЬ");
  await deleteDialog.getByRole("button", { name: "Удалить безвозвратно" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeHidden();
  await expect(page.getByText("Несинхронизированных локальных записей нет.")).toBeVisible();
  await context.setOffline(false);
});

test("crash and tab close leave chunks recoverable and synchronizable", async ({
  page,
  context,
}, testInfo) => {
  const suffix = `crash-${testInfo.project.name}-${Date.now()}`;
  const { projectTitle } = await registerAndCreateProject(page, suffix);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect.poll(() => localChunkCount(page), { timeout: 10_000 }).toBeGreaterThan(0);

  const recoveredPage = await context.newPage();
  await recoveredPage.goto("/recovery");
  await expect(recoveredPage.getByRole("heading", { name: projectTitle }).first()).toBeVisible();
  await page.close({ runBeforeUnload: false });
  await recoveredPage.getByRole("button", { name: "Синхронизировать" }).click();
  await expect(recoveredPage).toHaveURL(/\/dictations\/[^/]+\/(?:finalizing|review)$/);
  await expect(
    recoveredPage.getByText(
      /Голосовая сессия ·|Запись уже в проекте|Готовим черновик|Аудио сохранено/,
    ),
  ).toBeVisible();
});

test("denied microphone permission blocks capture with actionable guidance", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.permissions, "query", {
      configurable: true,
      value: async () => ({
        state: "denied",
        addEventListener: () => undefined,
      }),
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        throw new DOMException("denied", "NotAllowedError");
      },
    });
  });
  await registerAndCreateProject(page, `denied-${testInfo.project.name}-${Date.now()}`);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await expect(page.getByText("Доступ запрещён")).toBeVisible();
  await expect(page.getByRole("button", { name: "Начать запись" })).toBeDisabled();
  await page.getByRole("button", { name: "Проверить" }).click();
  await expect(page.getByText(/Разрешите доступ к микрофону/)).toBeVisible();
});

test("missing microphone reports no-device state without losing the destination", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.permissions, "query", {
      configurable: true,
      value: async () => ({
        state: "prompt",
        addEventListener: () => undefined,
      }),
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        throw new DOMException("missing", "NotFoundError");
      },
    });
  });
  const { nodeId } = await registerAndCreateProject(
    page,
    `missing-${testInfo.project.name}-${Date.now()}`,
  );
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await page.getByRole("button", { name: "Проверить" }).click();
  await expect(page.getByText(/Микрофон не найден/)).toBeVisible();
  await expect(page.getByLabel("Куда вставить после проверки")).toHaveValue(nodeId);
});

test("low storage is visible and prevents capture", async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator.storage, "estimate", {
      configurable: true,
      value: async () => ({
        quota: 500 * 1024 * 1024,
        usage: 460 * 1024 * 1024,
      }),
    });
  });
  await registerAndCreateProject(page, `storage-${testInfo.project.name}-${Date.now()}`);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await expect(page.getByText("Недостаточно свободного места")).toBeVisible();
  await expect(page.getByRole("button", { name: "Начать запись" })).toBeDisabled();
});

test("stale autosave creates a durable conflict copy with both texts", async ({
  page,
}, testInfo) => {
  const { projectId, nodeId } = await registerAndCreateProject(
    page,
    `conflict-${testInfo.project.name}-${Date.now()}`,
  );
  const responses = await page.evaluate(
    async ({ project, node }) => {
      const content = (text: string) => ({
        type: "doc",
        content: [
          {
            type: "paragraph",
            attrs: { blockId: crypto.randomUUID(), sourceRefs: [] },
            content: [{ type: "text", text }],
          },
        ],
      });
      const first = await fetch(`/api/nodes/${node}/revisions`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ baseRevision: 1, content: content("Серверная правка") }),
      });
      const stale = await fetch(`/api/nodes/${node}/revisions`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ baseRevision: 1, content: content("Локальная правка") }),
      });
      const workspace = await fetch(`/api/projects/${project}`, {
        credentials: "include",
      }).then((response) => response.json());
      return {
        firstStatus: first.status,
        staleStatus: stale.status,
        staleBody: await stale.json(),
        nodes: workspace.nodes,
      };
    },
    { project: projectId, node: nodeId },
  );
  expect(responses.firstStatus).toBe(201);
  expect(responses.staleStatus).toBe(409);
  expect(responses.staleBody.error.code).toBe("revision_conflict");
  expect(
    responses.nodes.some(
      (node: { title: string; plainText: string }) =>
        node.title.includes("конфликтующая копия") && node.plainText === "Локальная правка",
    ),
  ).toBe(true);
  expect(
    responses.nodes.some(
      (node: { id: string; plainText: string }) =>
        node.id === nodeId && node.plainText === "Серверная правка",
    ),
  ).toBe(true);
});

async function registerAndCreateProject(
  page: Page,
  suffix: string,
): Promise<{ projectId: string; nodeId: string; projectTitle: string }> {
  const projectTitle = `Проект ${suffix}`;
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill(projectTitle);
  await page.getByRole("button", { name: "Создать и открыть" }).click();
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/);
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  expect(match).toBeTruthy();
  return {
    projectId: match?.[1] ?? "",
    nodeId: match?.[2] ?? "",
    projectTitle,
  };
}

function localChunkCount(page: Page): Promise<number> {
  return page.evaluate(() => {
    return new Promise<number>((resolve, reject) => {
      const open = indexedDB.open("writers-studio-recovery");
      open.onerror = () => reject(open.error);
      open.onsuccess = () => {
        const request = open.result.transaction("audioChunks").objectStore("audioChunks").count();
        request.onerror = () => reject(request.error);
        request.onsuccess = () => resolve(request.result);
      };
    });
  });
}
