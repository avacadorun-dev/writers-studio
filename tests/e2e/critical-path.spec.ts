import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

test("P-01 creates, dictates, reviews, transfers, and durably saves a manuscript", async ({
  page,
}, testInfo) => {
  const diagnostics = watchDiagnostics(page);
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  const projectTitle = `Дом у воды ${suffix}`;

  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`writer-${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await expect(page.getByRole("heading", { name: "Ваши проекты" })).toBeVisible();

  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill(projectTitle);
  await page.getByText("Рассказ", { exact: true }).click();
  await page.getByRole("button", { name: "Создать и открыть" }).click();
  await expect(page.getByRole("heading", { name: "Сцена 1" })).toBeVisible();

  const studioURL = new URL(page.url());
  const match = studioURL.pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  expect(match).toBeTruthy();
  const projectId = match?.[1] ?? "";
  const nodeId = match?.[2] ?? "";

  const initialViewport = page.viewportSize();
  await page.setViewportSize({ width: 2_560, height: 1_440 });
  await expect
    .poll(() =>
      page.evaluate(() => ({
        documentHeight: document.documentElement.scrollHeight,
        documentWidth: document.documentElement.scrollWidth,
        viewportHeight: document.documentElement.clientHeight,
        viewportWidth: document.documentElement.clientWidth,
        sidebarHasHorizontalOverflow: Array.from(
          document.querySelectorAll('[aria-label="Структура проекта"], [role="tree"]'),
        ).some((element) => element.scrollWidth > element.clientWidth),
      })),
    )
    .toEqual({
      documentHeight: 1_440,
      documentWidth: 2_560,
      viewportHeight: 1_440,
      viewportWidth: 2_560,
      sidebarHasHorizontalOverflow: false,
    });
  if (initialViewport) await page.setViewportSize(initialViewport);

  const existingSceneText = "Существующее начало сцены остаётся на месте.";
  await page.locator(".tiptap").click();
  await page.keyboard.type(existingSceneText);
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({
    timeout: 10_000,
  });

  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await expect(page.getByRole("dialog", { name: "Подготовка диктовки" })).toBeVisible();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
    await expect(page.getByText("Доступ разрешён, сигнал поступает")).toBeVisible();
  }
  await page.evaluate(() => {
    performance.clearMarks("writers-studio-record-click");
    performance.clearMarks("writers-studio-recording-ready");
    const observer = new MutationObserver(() => {
      if (
        !performance.getEntriesByName("writers-studio-record-click").length ||
        !window.location.pathname.includes("/dictate/")
      ) {
        return;
      }
      const pause = Array.from(document.querySelectorAll("button")).find(
        (button) => button.textContent?.trim() === "Пауза",
      );
      if (
        pause &&
        pause.getClientRects().length > 0 &&
        getComputedStyle(pause).visibility !== "hidden"
      ) {
        performance.mark("writers-studio-recording-ready");
        observer.disconnect();
      }
    });
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["class", "hidden", "style"],
    });
    const startButton = Array.from(document.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "Начать запись",
    );
    if (!startButton) {
      throw new Error("recording start button not found");
    }
    startButton.addEventListener("click", () => performance.mark("writers-studio-record-click"), {
      capture: true,
      once: true,
    });
  });
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page).toHaveURL(/\/dictate\//);
  await expect(page.getByRole("button", { name: "Пауза" })).toBeVisible();
  const localStartLatencyMs = await page.evaluate(() => {
    const mark = performance.getEntriesByName("writers-studio-record-click").at(-1);
    const ready = performance.getEntriesByName("writers-studio-recording-ready").at(-1);
    if (!mark || !ready) {
      throw new Error("recording start or ready mark is missing");
    }
    // Both timestamps are captured in the page, excluding Playwright round trips.
    return ready.startTime - mark.startTime;
  });
  expect(localStartLatencyMs).toBeLessThanOrEqual(300);
  await expect(page.getByText(/Дом стоял у самой воды/)).toBeVisible({ timeout: 20_000 });

  await page.getByRole("button", { name: "Пауза" }).click();
  await expect(
    page.getByRole("paragraph").filter({ hasText: "Запись приостановлена" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Исправить текст" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Прослушать последние 10 секунд" })).toBeVisible();
  await page.getByRole("button", { name: "Продолжить" }).click();
  await page.getByRole("button", { name: /Метка/ }).click();
  await expect(page.getByText(/Метка ·/)).toBeVisible();
  const marker = page.locator('[data-command-kind="marker"]').last();
  await marker.getByRole("button", { name: "Отменить" }).click();
  await expect(marker).toHaveAttribute("data-command-executed", "false");
  await marker.getByRole("button", { name: "Выполнить" }).click();
  await expect(marker).toHaveAttribute("data-command-executed", "true");
  await page.getByRole("button", { name: "Завершить" }).click();

  await expect(page).toHaveURL(/\/dictations\/[^/]+\/(finalizing|review)/, {
    timeout: 20_000,
  });
  if (page.url().endsWith("/finalizing")) {
    await expect(page).toHaveURL(/\/review$/, { timeout: 25_000 });
  }
  await expect(page.getByText(/Голосовая сессия ·/)).toBeVisible();
  await expect(page.locator(".tiptap")).toContainText("Дом стоял у самой воды");
  const sourcedParagraph = page.locator(".tiptap [data-source-refs]").first();
  await sourcedParagraph.click();
  await expect(sourcedParagraph).toHaveAttribute("data-active-source", "true");
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type(" Продолжение после прослушивания.");
  await expect(page.locator(".tiptap")).toContainText("Продолжение после прослушивания.");

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);

  await page.getByRole("button", { name: /Вставить в рукопись/ }).click();
  await page.getByLabel("Документ", { exact: true }).selectOption(nodeId);
  await page.getByRole("button", { name: "Подтвердить вставку" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}`));
  await expect(page.locator(".tiptap")).toContainText(existingSceneText);
  await expect(page.locator(".tiptap")).toContainText("Дом стоял у самой воды");

  await page.locator(".tiptap").click();
  await page.keyboard.press("ControlOrMeta+End");
  await page.keyboard.type(" Авторская правка.");
  await expect(page.locator('[data-save-state="saving"]:visible')).toBeVisible();
  const editorStateBeforeSave = await readEditorInteractionState(page);
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({
    timeout: 10_000,
  });
  expect(await readEditorInteractionState(page)).toEqual(editorStateBeforeSave);
  await page.reload();
  await expect(page.locator(".tiptap")).toContainText("Авторская правка.");
  await expect(page.locator(".tiptap [data-source-refs]").first()).toHaveAttribute(
    "data-source-refs",
    /.+/,
  );

  await page.screenshot({
    path: `artifacts/${testInfo.project.name}-p01-studio.png`,
    fullPage: true,
  });
  expect(diagnostics.consoleErrors).toEqual([]);
  expect(diagnostics.failedRequests).toEqual([]);
});

test("S-11/S-12/S-15 dictionary, versions, search, exports, and refresh are functional", async ({
  page,
}, testInfo) => {
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  await registerAndCreateProject(page, suffix);
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  expect(match).toBeTruthy();
  const projectId = match?.[1] ?? "";
  const nodeId = match?.[2] ?? "";

  await page.locator(".tiptap").click();
  await page.keyboard.type("Анна вернулась к старому дому.");
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({ timeout: 10_000 });
  await page.reload();
  await expect(page.locator(".tiptap")).toContainText("Анна вернулась");

  await page.goto(`/projects/${projectId}/dictionary`);
  await page.getByRole("button", { name: "Добавить термин", exact: true }).click();
  await page.getByRole("textbox", { name: "Правильное написание" }).fill("Анна");
  await page.getByRole("textbox", { name: /Варианты/ }).fill("Аня, Анна Сергеевна");
  await page.getByRole("button", { name: "Сохранить термин" }).click();
  await expect(page.getByRole("heading", { name: "Анна" })).toBeVisible();

  await page.goto(`/projects/${projectId}/search?q=${encodeURIComponent("Анна")}&scope=all`);
  await expect(page.getByText(/Анна вернулась/)).toBeVisible();

  await page.goto(`/nodes/${nodeId}/versions`);
  await page.getByRole("button", { name: "Сохранить версию" }).click();
  await page.getByRole("textbox", { name: "Название версии" }).fill("После первой правки");
  await page.getByRole("button", { name: "Сохранить снимок" }).click();
  await expect(page.getByRole("heading", { name: "После первой правки" })).toBeVisible();

  const exportFormats = [
    ["txt", "text/plain"],
    ["markdown", "text/markdown"],
    ["docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"],
    ["epub", "application/epub+zip"],
    ["pdf", "application/pdf"],
  ] as const;
  for (const [format, expectedContentType] of exportFormats) {
    const response = await page.evaluate(
      async ({ projectId: id, format: targetFormat }) => {
        const workspace = await fetch(`/api/projects/${id}`, { credentials: "include" }).then(
          (result) => result.json(),
        );
        const created = await fetch(`/api/projects/${id}/exports`, {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            format: targetFormat,
            nodeIds: workspace.nodes.map((node: { id: string }) => node.id),
            titlePage: true,
            chapterTitles: true,
            includeNotes: false,
          }),
        }).then((result) => result.json());
        return created.export.id as string;
      },
      { projectId, format },
    );
    const downloadResponse = await expect
      .poll(
        async () => {
          const status = await page.evaluate(async (exportId) => {
            const result = await fetch(`/api/exports/${exportId}`, {
              credentials: "include",
            });
            return result.json();
          }, response);
          if (status.export?.status === "failed") {
            throw new Error(
              `Export ${format} failed: ${status.export.errorCode ?? "unknown_error"}`,
            );
          }
          return status.export;
        },
        { timeout: 20_000 },
      )
      .toMatchObject({ status: "ready" });
    void downloadResponse;
    const body = await page.request.get(`/api/exports/${response}/download`, {
      headers: {
        Cookie: (await page.context().cookies())
          .map((cookie) => `${cookie.name}=${cookie.value}`)
          .join("; "),
      },
    });
    expect(body.ok()).toBe(true);
    expect(body.headers()["content-type"]).toContain(expectedContentType);
    expect(body.headers()["content-disposition"]).toContain(
      `.${format === "markdown" ? "md" : format}`,
    );
    expect((await body.body()).byteLength).toBeGreaterThan(20);
  }

  await page.goto(`/settings?projectId=${projectId}&nodeId=${nodeId}`);
  await expect(page.getByRole("link", { name: "Назад" })).toHaveAttribute(
    "href",
    `/projects/${projectId}/studio/${nodeId}`,
  );
  await expect(page.getByLabel("Шрифт рукописи")).toHaveValue("sofia");
  await page.getByLabel("Пауза перед предложением выполнить").selectOption("8000");
  await page.getByLabel("Максимум шагов на поручение").fill("40");
  await page.getByLabel("Шрифт рукописи").selectOption("inter");
  await page.getByLabel("Срок хранения исходного аудио на сервере").selectOption("90");
  await page.getByLabel("Кегль").selectOption("21");
  await page.getByLabel("Межстрочный интервал").selectOption("2");
  await page.getByLabel("Ширина рукописи").selectOption("80");
  await page.getByLabel("Тема", { exact: true }).selectOption("light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  const lightTheme = await readThemeColors(page);
  const lightThemeAccessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(lightThemeAccessibility.violations).toEqual([]);
  await page.getByLabel("Тема", { exact: true }).selectOption("dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  const darkTheme = await readThemeColors(page);
  expect(darkTheme.surfaceApp).not.toBe(lightTheme.surfaceApp);
  expect(darkTheme.inkPrimary).not.toBe(lightTheme.inkPrimary);
  expect(darkTheme.browserThemeColor).toBe(darkTheme.surfaceApp);
  await page.getByLabel("Цель по словам").fill("500");
  await page.getByRole("switch", { name: "Проверять орфографию средствами браузера" }).click();
  await page.getByRole("button", { name: "Сохранить настройки" }).click();
  await expect(page.getByText("Настройки сохранены", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.getByLabel("Шрифт рукописи")).toHaveValue("inter");
  await expect(page.getByLabel("Пауза перед предложением выполнить")).toHaveValue("8000");
  await expect(page.getByLabel("Максимум шагов на поручение")).toHaveValue("40");
  await expect(page.getByLabel("Кегль")).toHaveValue("21");
  await expect(page.getByLabel("Цель по словам")).toHaveValue("500");
  const settingsAccessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(settingsAccessibility.violations).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await expect(page.locator(".tiptap")).toHaveAttribute("spellcheck", "false");
  const editorPreferences = await page.locator(".tiptap").evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      fontFamily: style.fontFamily,
      fontFeatureSettings: style.fontFeatureSettings,
      fontSize: style.fontSize,
      lineHeight: style.lineHeight,
      maxWidth: style.maxWidth,
      configuredWidth: getComputedStyle(document.documentElement)
        .getPropertyValue("--editor-max-width")
        .trim(),
    };
  });
  expect(editorPreferences.fontFamily).toContain("Inter");
  expect(editorPreferences.fontFeatureSettings).not.toContain('"locl" 0');
  expect(editorPreferences.fontSize).toBe("21px");
  expect(editorPreferences.lineHeight).toBe("42px");
  expect(editorPreferences.configuredWidth).toBe("80ch");
  expect(Number.parseFloat(editorPreferences.maxWidth)).toBeGreaterThan(900);
  if ((page.viewportSize()?.width ?? 0) >= 1_024) {
    await page.getByRole("button", { name: "Статистика документа" }).click();
    await expect(page.getByText(/5\s*\/\s*500/)).toBeVisible();
  }
  const storedPreferences = await page.evaluate(async () => {
    const response = await fetch("/api/preferences", { credentials: "include" });
    return response.json();
  });
  expect(storedPreferences.preferences.audioRetentionDays).toBe(90);
  const backup = await page.request.get("/api/account/backup");
  expect(backup.ok()).toBe(true);
  expect(backup.headers()["content-type"]).toContain("application/zip");
  const backupBytes = await backup.body();
  expect(backupBytes.subarray(0, 2).toString()).toBe("PK");
});

test("project navigation and native .wstudio transfer work end to end", async ({
  page,
}, testInfo) => {
  const suffix = `transfer-${testInfo.project.name}-${Date.now()}`;
  await registerAndCreateProject(page, suffix);
  const sourceURL = new URL(page.url());
  const sourceMatch = sourceURL.pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  expect(sourceMatch).toBeTruthy();
  const sourceProjectId = sourceMatch?.[1] ?? "";

  await page.locator(".tiptap").click();
  await page.keyboard.type("Текст, который должен пережить перенос проекта.");
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({
    timeout: 10_000,
  });

  await page.goto(`/projects/${sourceProjectId}/search`);
  await expect(page.getByRole("link", { name: "Назад в проект" })).toBeVisible();
  await page.goto("/recovery");
  await expect(page.getByRole("link", { name: "Назад к проектам" })).toBeVisible();

  await page.goto(`/projects/${sourceProjectId}/export`);
  await expect(page.getByRole("link", { name: "Скачать .wstudio" })).toBeVisible();
  const download = await page.request.get(`/api/projects/${sourceProjectId}/transfer`);
  expect(download.ok()).toBe(true);
  expect(download.headers()["content-type"]).toContain(
    "application/vnd.writers-studio.project+zip",
  );
  const projectFile = await download.body();
  expect(projectFile.subarray(0, 2).toString()).toBe("PK");

  await page.goto("/projects");
  await page.getByLabel("Выбрать файл проекта Writers Studio").setInputFiles({
    name: "portable-project.wstudio",
    mimeType: "application/vnd.writers-studio.project+zip",
    buffer: projectFile,
  });
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/, { timeout: 20_000 });
  const importedMatch = new URL(page.url()).pathname.match(
    /^\/projects\/([^/]+)\/studio\/([^/]+)$/,
  );
  expect(importedMatch?.[1]).not.toBe(sourceProjectId);
  await expect(page.locator(".tiptap")).toContainText("Текст, который должен пережить перенос");
});

async function readThemeColors(page: Page) {
  return page.evaluate(() => {
    const rootStyle = getComputedStyle(document.documentElement);
    return {
      surfaceApp: rootStyle.getPropertyValue("--surface-app").trim(),
      inkPrimary: rootStyle.getPropertyValue("--ink-primary").trim(),
      browserThemeColor:
        document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content ?? "",
    };
  });
}

async function registerAndCreateProject(page: Page, suffix: string) {
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`tools-${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill(`Проект ${suffix}`);
  await page.getByRole("button", { name: "Создать и открыть" }).click();
  await expect(page).toHaveURL(/\/studio\//);
}

function watchDiagnostics(page: Page) {
  const consoleErrors: string[] = [];
  const failedRequests: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });
  page.on("requestfailed", (request) => {
    failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText}`);
  });
  return { consoleErrors, failedRequests };
}

function readEditorInteractionState(page: Page) {
  return page.evaluate(() => {
    const selection = window.getSelection();
    const scroller = document
      .querySelector('[aria-label="Редактор рукописи"] .tiptap')
      ?.closest(".overflow-y-auto");
    return {
      anchorOffset: selection?.anchorOffset,
      focusOffset: selection?.focusOffset,
      anchorText: selection?.anchorNode?.textContent,
      scrollTop: scroller?.scrollTop,
    };
  });
}
