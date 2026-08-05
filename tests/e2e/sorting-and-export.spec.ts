import { expect, type Locator, type Page, test } from "@playwright/test";

test("scene order persists while the export queue stays local to the build", async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name.includes("mobile"),
    "The project tree is intentionally hidden on phones",
  );
  if (testInfo.project.name.includes("tablet")) {
    await page.setViewportSize({ width: 1100, height: 900 });
  }

  const { projectId, initialNodeId } = await registerStory(
    page,
    `sorting-${testInfo.project.name}-${Date.now()}`,
  );
  await saveText(page, initialNodeId, "Первая сцена.");
  const second = await createScene(page, projectId, "Сцена 2", "Вторая сцена.");
  const third = await createScene(page, projectId, "Сцена 3", "Третья сцена.");
  await page.reload();
  if (testInfo.project.name.includes("tablet")) {
    const expandTree = page.getByRole("button", { name: "Развернуть структуру" });
    await expect(expandTree).toBeVisible();
    await expandTree.click();
  }

  // KeyboardSensor: move the third scene one position up.
  const keyboardHandle = page.getByRole("button", { name: "Переместить «Сцена 3»" });
  await keyboardMove(page, keyboardHandle, [second.id]);
  await expect(page.getByText("Материал перемещён", { exact: true })).toBeVisible();
  await expect.poll(() => rootOrder(page, projectId)).toEqual([initialNodeId, third.id, second.id]);

  // PointerSensor on desktop and TouchSensor in the touch project: put scene 2 first.
  const pointerHandle = page.getByRole("button", { name: "Переместить «Сцена 2»" });
  const firstRow = treeRow(page, "Сцена 1");
  if (testInfo.project.name.includes("tablet")) {
    await touchDrag(page, pointerHandle, firstRow);
  } else {
    await mouseDrag(page, pointerHandle, firstRow);
  }
  await expect.poll(() => rootOrder(page, projectId)).toEqual([second.id, initialNodeId, third.id]);
  await page.reload();
  await expect.poll(() => rootOrder(page, projectId)).toEqual([second.id, initialNodeId, third.id]);

  const persistedProjectOrder = await rootOrder(page, projectId);
  await page.goto(`/projects/${projectId}/export`);
  const sceneThreeExportHandle = page.getByRole("button", {
    name: "Изменить порядок «Сцена 3»",
  });
  await keyboardMove(page, sceneThreeExportHandle, [initialNodeId, second.id]);
  await expect(exportTitles(page)).toHaveText(["Сцена 3", "Сцена 2", "Сцена 1"]);
  expect(await rootOrder(page, projectId)).toEqual(persistedProjectOrder);

  // The export order belongs to this screen only and resets from the persisted tree.
  await page.reload();
  await expect(exportTitles(page)).toHaveText(["Сцена 2", "Сцена 1", "Сцена 3"]);
  expect(await rootOrder(page, projectId)).toEqual(persistedProjectOrder);
});

function treeRow(page: Page, title: string): Locator {
  return page.getByRole("treeitem").filter({
    has: page.getByRole("link", { name: title, exact: true }),
  });
}

function exportTitles(page: Page): Locator {
  return page.locator("[data-export-queue-row] p:first-child");
}

async function keyboardMove(page: Page, handle: Locator, overIds: string[]) {
  await handle.focus();
  await page.keyboard.press("Space");
  await expect(handle).toHaveAttribute("aria-pressed", "true");
  for (const overId of overIds) {
    await page.keyboard.press("ArrowUp");
    await expect(page.getByRole("status").filter({ hasText: overId })).toBeVisible();
  }
  await page.keyboard.press("Space");
  await expect(handle).not.toHaveAttribute("aria-pressed", "true");
}

async function touchDrag(page: Page, source: Locator, target: Locator) {
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  if (!sourceBox || !targetBox) throw new Error("drag target is not visible");
  const from = { x: sourceBox.x + sourceBox.width / 2, y: sourceBox.y + sourceBox.height / 2 };
  const to = { x: targetBox.x + 32, y: targetBox.y + 4 };
  const session = await page.context().newCDPSession(page);
  await session.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [from] });
  await page.waitForTimeout(220);
  for (let step = 1; step <= 6; step += 1) {
    const progress = step / 6;
    await session.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        {
          x: from.x + (to.x - from.x) * progress,
          y: from.y + (to.y - from.y) * progress,
        },
      ],
    });
  }
  await session.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await session.detach();
}

async function mouseDrag(page: Page, source: Locator, target: Locator) {
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  if (!sourceBox || !targetBox) throw new Error("drag target is not visible");
  const from = { x: sourceBox.x + sourceBox.width / 2, y: sourceBox.y + sourceBox.height / 2 };
  const to = { x: targetBox.x + 32, y: targetBox.y + 4 };
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 8, from.y, { steps: 2 });
  await page.mouse.move(to.x, to.y, { steps: 8 });
  await page.mouse.up();
}

async function registerStory(page: Page, suffix: string) {
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill(`Порядок ${suffix}`);
  await page.getByText("Рассказ", { exact: true }).click();
  await page.getByRole("button", { name: "Создать и открыть" }).click();
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/);
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  if (!match) throw new Error("project route was not opened");
  return { projectId: match[1] ?? "", initialNodeId: match[2] ?? "" };
}

async function createScene(page: Page, projectId: string, title: string, text: string) {
  const result = await page.evaluate(
    async ({ id, sceneTitle, manuscript }) => {
      const created = await fetch(`/api/projects/${id}/nodes`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind: "scene", title: sceneTitle }),
      }).then((response) => response.json());
      const node = created.node as { id: string; currentRevision: number };
      const revision = await fetch(`/api/nodes/${node.id}/revisions`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          baseRevision: node.currentRevision,
          source: "e2e_fixture",
          content: {
            type: "doc",
            content: [{ type: "paragraph", content: [{ type: "text", text: manuscript }] }],
          },
        }),
      });
      if (!revision.ok) throw new Error(await revision.text());
      return node;
    },
    { id: projectId, sceneTitle: title, manuscript: text },
  );
  return result;
}

async function saveText(page: Page, nodeId: string, text: string) {
  await page.evaluate(
    async ({ id, manuscript }) => {
      const current = await fetch(`/api/nodes/${id}`, { credentials: "include" }).then((response) =>
        response.json(),
      );
      const response = await fetch(`/api/nodes/${id}/revisions`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          baseRevision: current.node.currentRevision,
          source: "e2e_fixture",
          content: {
            type: "doc",
            content: [{ type: "paragraph", content: [{ type: "text", text: manuscript }] }],
          },
        }),
      });
      if (!response.ok) throw new Error(await response.text());
    },
    { id: nodeId, manuscript: text },
  );
}

async function rootOrder(page: Page, projectId: string): Promise<string[]> {
  return page.evaluate(async (id) => {
    const workspace = await fetch(`/api/projects/${id}`, { credentials: "include" }).then(
      (response) => response.json(),
    );
    return (workspace.nodes as Array<{ id: string; parentId?: string; trashedAt?: string }>)
      .filter((node) => !node.parentId && !node.trashedAt)
      .map((node) => node.id);
  }, projectId);
}
