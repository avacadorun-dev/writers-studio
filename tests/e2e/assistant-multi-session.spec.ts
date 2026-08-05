import { expect, type Page, test } from "@playwright/test";

test("project assistant supports independent chats, draft restoration and responsive switching", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-threads-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);

  const firstRunId = await createTextThread(page, "Составь краткое резюме этой главы.");
  await expect
    .poll(async () => {
      const response = await page.request.get(`/api/assistant/runs/${firstRunId}/turns`);
      const body = (await response.json()) as { turns: unknown[] };
      return body.turns.length;
    })
    .toBe(1);

  await page
    .getByRole("complementary", { name: "Панель ассистента" })
    .getByRole("button", { name: "Новый чат" })
    .click();
  const secondRunId = await createTextThread(page, "Проверь темп сцены и ритм абзацев.", false);
  expect(secondRunId).not.toBe(firstRunId);

  const session = page.getByRole("complementary", { name: "Панель ассистента" });
  await session.getByRole("button", { name: "Открыть список чатов проекта" }).click();
  const switcher = page.getByTestId("assistant-thread-switcher");
  await expect(switcher).toBeVisible();
  await expect(switcher.getByText("Составь краткое резюме этой главы.")).toBeVisible();
  await expect(switcher.getByText("Проверь темп сцены и ритм абзацев.")).toBeVisible();

  const bounds = await switcher.boundingBox();
  const viewport = page.viewportSize();
  expect(bounds).not.toBeNull();
  if (bounds && viewport) {
    if (viewport.width < 768) {
      expect(Math.round(bounds.x)).toBe(0);
      expect(Math.round(bounds.width)).toBe(viewport.width);
    } else {
      expect(Math.round(bounds.width)).toBe(360);
      expect(Math.round(bounds.x + bounds.width)).toBe(viewport.width);
    }
  }

  await page.getByTestId(`assistant-thread-${firstRunId}`).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("assistantRunId")).toBe(firstRunId);
  const firstComposer = page.getByRole("textbox", { name: "Поручение ассистенту" });
  await expect(firstComposer).toBeEnabled({ timeout: 30_000 });
  await firstComposer.fill("Черновик первого чата");

  await page
    .getByRole("complementary", { name: "Панель ассистента" })
    .getByRole("button", { name: "Открыть список чатов проекта" })
    .click();
  await page.getByTestId(`assistant-thread-${secondRunId}`).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("assistantRunId")).toBe(secondRunId);
  const secondComposer = page.getByRole("textbox", { name: "Поручение ассистенту" });
  await expect(secondComposer).toBeEnabled({ timeout: 30_000 });
  await expect(secondComposer).toHaveValue("");
  await secondComposer.fill("Черновик второго чата");

  await page
    .getByRole("complementary", { name: "Панель ассистента" })
    .getByRole("button", { name: "Открыть список чатов проекта" })
    .click();
  await page.getByTestId(`assistant-thread-${firstRunId}`).click();
  await expect(page.getByRole("textbox", { name: "Поручение ассистенту" })).toHaveValue(
    "Черновик первого чата",
  );

  const listResponse = await page.request.get(`/api/projects/${projectId}/assistant/runs`);
  expect(listResponse.ok()).toBe(true);
  const listBody = (await listResponse.json()) as { runs: Array<{ id: string; title: string }> };
  expect(listBody.runs.map((run) => run.id)).toEqual(
    expect.arrayContaining([firstRunId, secondRunId]),
  );
  expect(listBody.runs.find((run) => run.id === firstRunId)?.title).toBe(
    "Составь краткое резюме этой главы.",
  );

  await expect(page.getByRole("region", { name: "Редактор документа" })).toBeAttached();
  expect(nodeId).not.toBe("");
});

test("a change applied from one chat makes another chat's proposal safely stale", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-conflict-${testInfo.project.name}-${Date.now()}`;
  const { nodeId } = await registerAndCreateProject(page, suffix);
  const seeded = await page.request.put(`/api/nodes/${nodeId}/revisions`, {
    data: {
      baseRevision: 1,
      source: "manual",
      content: {
        type: "doc",
        content: [paragraph("conflict", "Слабое голубое сияние дрожало на стене.")],
      },
    },
  });
  expect(seeded.status()).toBe(201);
  await page.reload();

  const firstInstruction = JSON.stringify({
    fixture: "exact_replace",
    nodeId,
    revision: 2,
    expected: "Слабое голубое сияние",
    replacement: "Ровное янтарное свечение",
  });
  const firstRunId = await createTextThread(page, firstInstruction);
  const firstChangeSetId = await waitForStagedChangeSet(page, firstRunId);

  await page
    .getByRole("complementary", { name: "Панель ассистента" })
    .getByRole("button", { name: "Новый чат" })
    .click();
  const secondInstruction = JSON.stringify({
    fixture: "exact_replace",
    nodeId,
    revision: 2,
    expected: "Слабое голубое сияние",
    replacement: "Холодная серебряная полоса",
  });
  const secondRunId = await createTextThread(page, secondInstruction, false);
  const secondChangeSetId = await waitForStagedChangeSet(page, secondRunId);

  const firstApply = await page.request.post(
    `/api/assistant/runs/${firstRunId}/change-sets/${firstChangeSetId}/apply`,
    { data: { selectedGroups: ["document"], annotationResolutions: {} } },
  );
  expect(firstApply.ok()).toBe(true);
  const staleApply = await page.request.post(
    `/api/assistant/runs/${secondRunId}/change-sets/${secondChangeSetId}/apply`,
    { data: { selectedGroups: ["document"], annotationResolutions: {} } },
  );
  expect(staleApply.status()).toBe(409);
  await expect(staleApply.json()).resolves.toMatchObject({
    error: { code: "stale_change_set" },
  });

  await page.reload();
  const staleCard = page.getByTestId("assistant-stale-change-set");
  await expect(staleCard).toBeVisible();
  await expect(staleCard.getByText("Конфликт версий", { exact: true })).toBeVisible();
  await expect(staleCard).toContainText("Основа r2 · сейчас r3");
  await expect(staleCard.getByRole("button", { name: "Обновить предложение" })).toBeEnabled();
  await expect(
    staleCard.getByRole("button", { name: "Отложить и открыть другие чаты" }),
  ).toBeVisible();
  await expect
    .poll(() => nodePlainText(page, nodeId))
    .toBe("Ровное янтарное свечение дрожало на стене.");

  await staleCard.getByRole("button", { name: "Обновить предложение" }).click();
  await expect(page.getByText("Предложение обновляется", { exact: true })).toBeVisible();
  await expect
    .poll(async () => {
      const response = await page.request.get(`/api/assistant/runs/${secondRunId}/turns`);
      const body = (await response.json()) as { turns: unknown[] };
      return body.turns.length;
    })
    .toBe(2);
});

async function createTextThread(
  page: Page,
  instruction: string,
  openPanel = true,
): Promise<string> {
  if (openPanel) {
    await page.getByRole("button", { name: "Ассистент" }).click();
  }
  const newChat = page.getByRole("complementary", { name: "Новый чат ассистента" });
  await expect(newChat).toBeVisible();
  await newChat.getByRole("textbox", { name: "Первое поручение" }).fill(instruction);
  const start = newChat.getByRole("button", { name: "Начать чат" });
  await expect(start).toBeEnabled({ timeout: 30_000 });
  const previousRunId = new URL(page.url()).searchParams.get("assistantRunId");
  await start.click();
  await expect
    .poll(() => new URL(page.url()).searchParams.get("assistantRunId"))
    .not.toBe(previousRunId);
  return new URL(page.url()).searchParams.get("assistantRunId") ?? "";
}

async function waitForStagedChangeSet(page: Page, runId: string): Promise<string> {
  let changeSetId = "";
  await expect
    .poll(
      async () => {
        const response = await page.request.get(`/api/assistant/runs/${runId}`);
        expect(response.ok()).toBe(true);
        const body = (await response.json()) as {
          run: { pendingChangeSet?: { id: string; state: string } };
        };
        changeSetId = body.run.pendingChangeSet?.id ?? "";
        return body.run.pendingChangeSet?.state;
      },
      { timeout: 30_000 },
    )
    .toBe("staged");
  return changeSetId;
}

async function nodePlainText(page: Page, nodeId: string): Promise<string> {
  const response = await page.request.get(`/api/nodes/${nodeId}`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as { node: { plainText: string } };
  return body.node.plainText;
}

function paragraph(blockId: string, text: string) {
  return {
    type: "paragraph",
    attrs: { blockId, sourceRefs: [] },
    content: [{ type: "text", text }],
  };
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
