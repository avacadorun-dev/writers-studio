import { expect, type Page, test } from "@playwright/test";

test("assistant stages an exact diff, applies it, and undoes the grouped change", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-change-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const original = [
    "Коридор был пуст.",
    "Во втором абзаце слабое голубое сияние дрожало на каменной стене.",
    "Последняя реплика: — Я вернусь до рассвета.",
  ].join("\n\n");
  const seeded = await page.request.put(`/api/nodes/${nodeId}/revisions`, {
    data: {
      baseRevision: 1,
      source: "manual",
      content: {
        type: "doc",
        content: [
          paragraph("first", "Коридор был пуст."),
          paragraph("second", "Во втором абзаце слабое голубое сияние дрожало на каменной стене."),
          paragraph("last", "Последняя реплика: — Я вернусь до рассвета."),
        ],
      },
    },
  });
  expect(seeded.status()).toBe(201);
  await page.reload();

  const floatingManuscript = page.locator('[data-workspace-layout="floating"]');
  await expect(floatingManuscript).toBeVisible();
  const floatingStyle = await floatingManuscript.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      marginTop: Number.parseFloat(style.marginTop),
      borderRadius: Number.parseFloat(style.borderRadius),
    };
  });
  expect(floatingStyle.marginTop).toBeGreaterThan(0);
  expect(floatingStyle.borderRadius).toBeGreaterThan(0);

  const runId = await launchChatAssistant(page, projectId, nodeId);
  await expect(page.getByRole("region", { name: "Редактор документа" })).toBeVisible();
  const assistantPanel = page.getByTestId("assistant-docked-panel");
  await expect(assistantPanel).toBeVisible();
  await expect(page.getByTestId("assistant-transcript-viewport")).toBeVisible();
  await expect(page.locator('[data-workspace-layout="integrated"]')).toBeVisible();

  const sequentialLayout = (page.viewportSize()?.width ?? 0) < 1_280;
  if (!sequentialLayout) {
    const headerGeometry = await page.evaluate(() => {
      const tree = document.querySelector('nav[aria-label="Структура проекта"] > div:first-child');
      const editor = document.querySelector('[role="toolbar"][aria-label="Форматирование текста"]');
      const assistant = document.querySelector('[data-testid="assistant-docked-panel"] header');
      return [tree, editor, assistant].map((element) => {
        if (!element) return undefined;
        const bounds = element.getBoundingClientRect();
        return { top: bounds.top, bottom: bounds.bottom, height: bounds.height };
      });
    });
    expect(headerGeometry).toEqual([
      { top: 56, bottom: 112, height: 56 },
      { top: 56, bottom: 112, height: 56 },
      { top: 56, bottom: 112, height: 56 },
    ]);
  }
  if (sequentialLayout) {
    await assistantPanel.getByRole("button", { name: "Закрыть панель ассистента" }).click();
    await expect(assistantPanel).toBeHidden();
    await expect(page.locator('[data-workspace-layout="floating"]')).toBeVisible();
  }
  const manuscript = page.locator(".ProseMirror");
  await manuscript.click();
  await page.keyboard.press("ControlOrMeta+A");
  await manuscript.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Прикрепить к ассистенту" }).click();
  await expect(page.getByText("Выделение прикреплено")).toBeVisible();
  if (sequentialLayout) {
    await page.getByRole("button", { name: "Открыть ассистента" }).click();
    await expect(assistantPanel).toBeVisible();
    await expect(page.locator('[data-workspace-layout="integrated"]')).toBeVisible();
  }
  await expect(assistantPanel.getByText("Фрагмент прикреплён", { exact: true })).toBeVisible();

  const instruction = JSON.stringify({
    fixture: "exact_replace",
    nodeId,
    revision: 2,
    expected: "слабое голубое сияние",
    replacement: "ровное янтарное свечение",
  });
  await page.getByRole("textbox", { name: "Поручение ассистенту" }).fill(instruction);
  await page.getByRole("textbox", { name: "Поручение ассистенту" }).press("ControlOrMeta+Enter");

  await expect(page.getByText("Изменение готово к проверке")).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByTestId("assistant-live-preview")).toContainText(
    "Предложение · ещё не применено",
  );
  const reviewLink = page.getByRole("button", { name: "Проверить изменения" });
  await expect(reviewLink).toBeVisible();
  const studioURL = page.url();
  await reviewLink.click();
  await expect.poll(() => new URL(page.url()).searchParams.get("assistantRunId")).toBe(runId);
  await expect.poll(() => page.url()).toBe(studioURL);
  expect(new URL(page.url()).searchParams.get("assistantReviewId")).toBeNull();

  const review = page.getByTestId("assistant-review-overlay");
  await expect(review).toHaveAttribute("role", "dialog");
  const reviewBounds = await review.boundingBox();
  expect(reviewBounds?.x).toBeGreaterThan(0);
  expect(reviewBounds?.y).toBeGreaterThan(0);
  expect(reviewBounds?.width).toBeLessThan(page.viewportSize()?.width ?? Number.POSITIVE_INFINITY);
  expect(reviewBounds?.height).toBeLessThan(
    page.viewportSize()?.height ?? Number.POSITIVE_INFINITY,
  );
  await expect(review.getByText("Пока ничего не изменено")).toBeVisible();
  await expect(review.getByText("слабое голубое сияние", { exact: true })).toBeVisible();
  await expect(review.getByText("ровное янтарное свечение", { exact: true })).toBeVisible();
  expect(await nodePlainText(page, nodeId)).toBe(original);

  await review.getByRole("button", { name: "Попросить исправить" }).click();
  await expect(
    review.getByRole("textbox", { name: "Что нужно исправить в предложении?" }),
  ).toBeVisible();
  await expect(review.getByRole("button", { name: "Отправить", exact: true })).toBeVisible();
  await expect(review.getByRole("button", { name: "Применить изменения" })).toBeHidden();
  await review.getByRole("button", { name: "Отмена" }).click();
  await expect(review.getByRole("button", { name: "Применить изменения" })).toBeVisible();

  await review.getByRole("button", { name: "Применить изменения" }).click();
  await expect(review.getByText("Применено", { exact: true })).toBeVisible();
  await expect
    .poll(() => nodePlainText(page, nodeId))
    .toBe(original.replace("слабое голубое сияние", "ровное янтарное свечение"));
  await expect(manuscript).toContainText("ровное янтарное свечение");
  await expect(manuscript).not.toContainText("слабое голубое сияние");

  await review.getByRole("button", { name: "Мгновенно отменить весь набор" }).click();
  await expect(review.getByText("Отменено", { exact: true })).toBeVisible();
  await expect.poll(() => nodePlainText(page, nodeId)).toBe(original);
  await expect(manuscript).toContainText("слабое голубое сияние");

  await review.getByRole("button", { name: "Вернуться в Studio" }).click();
  await expect(review).toBeHidden();
  const currentRevision = await nodeRevision(page, nodeId);
  await page.getByRole("textbox", { name: "Поручение ассистенту" }).fill(
    JSON.stringify({
      fixture: "exact_replace",
      nodeId,
      revision: currentRevision,
      expected: "слабое голубое сияние",
      replacement: "ровное янтарное свечение",
    }),
  );
  await page.getByRole("textbox", { name: "Поручение ассистенту" }).press("ControlOrMeta+Enter");
  await expect(page.getByText("Изменение готово к проверке")).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "Применить сразу" }).click();
  await expect(page.getByText("Изменение применено", { exact: true })).toBeVisible();
  await expect
    .poll(() => nodePlainText(page, nodeId))
    .toBe(original.replace("слабое голубое сияние", "ровное янтарное свечение"));
  await expect(manuscript).toContainText("ровное янтарное свечение");
  await page
    .getByRole("region", { name: "Notifications (F8)" })
    .getByRole("button", { name: "Отменить", exact: true })
    .click();
  await expect.poll(() => nodePlainText(page, nodeId)).toBe(original);
});

function paragraph(blockId: string, text: string) {
  return {
    type: "paragraph",
    attrs: { blockId, sourceRefs: [] },
    content: [{ type: "text", text }],
  };
}

async function nodePlainText(page: Page, nodeId: string): Promise<string> {
  const response = await page.request.get(`/api/nodes/${nodeId}`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { node: { plainText: string } };
  return body.node.plainText;
}

async function nodeRevision(page: Page, nodeId: string): Promise<number> {
  const response = await page.request.get(`/api/nodes/${nodeId}`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { node: { currentRevision: number } };
  return body.node.currentRevision;
}

async function launchChatAssistant(page: Page, projectId: string, nodeId: string): Promise<string> {
  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await page.getByRole("button", { name: "Ассистент" }).click();
  await expect(page.getByRole("heading", { name: "Что поручить?" })).toBeVisible();
  await page.getByRole("button", { name: "Голосом", exact: true }).click();
  const start = page.getByRole("button", { name: "Открыть голосовой чат" });
  await expect(start).toBeEnabled({ timeout: 30_000 });
  await start.click();
  await expect.poll(() => new URL(page.url()).searchParams.get("assistantRunId")).not.toBeNull();
  await page
    .getByRole("complementary", { name: "Панель ассистента" })
    .getByRole("button", { name: "Чат", exact: true })
    .click();
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
