import { expect, type Locator, type Page, test } from "@playwright/test";

test("Canvas board creates, renders, autosaves, and stays read-only on mobile", async ({
  page,
}, testInfo) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  const suffix = `canvas-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const desktop = (page.viewportSize()?.width ?? 0) >= 1024;
  const canvasId = desktop
    ? await createCanvasFromProjectTree(page)
    : await createCanvasThroughAPI(page, projectId);

  const seeded = await page.request.put(`/api/canvases/${canvasId}/revisions`, {
    data: {
      baseRevision: 1,
      source: "e2e_seed",
      content: {
        nodes: [
          {
            id: "group-story",
            type: "group",
            x: -40,
            y: -50,
            width: 720,
            height: 340,
            label: "Сюжет",
            color: "5",
          },
          {
            id: "note-premise",
            type: "text",
            x: 0,
            y: 0,
            width: 260,
            height: 180,
            text: "# Завязка\n\nГерой получает письмо.",
          },
          {
            id: "document-opening",
            type: "file",
            x: 340,
            y: 0,
            width: 280,
            height: 180,
            file: `writers-studio://documents/${nodeId}`,
            "x-writers-studio": { resource: { kind: "document", id: nodeId } },
          },
        ],
        edges: [
          {
            id: "edge-premise-opening",
            fromNode: "note-premise",
            fromSide: "right",
            fromEnd: "none",
            toNode: "document-opening",
            toSide: "left",
            toEnd: "arrow",
            label: "ведёт к сцене",
            color: "5",
          },
        ],
        "x-writers-studio": { schemaVersion: 1 },
      },
    },
  });
  expect(seeded.status()).toBe(201);

  await page.goto(`/projects/${projectId}/canvases/${canvasId}`);
  await expect(page.getByTestId("canvas-board")).toBeVisible();
  await expect(page.getByTestId("canvas-text-note-premise")).toContainText("Герой получает письмо");
  await expect(page.getByTestId("canvas-document-document-opening")).toBeVisible();
  await expect(page.getByTestId("canvas-group-group-story")).toContainText("Сюжет");
  await expect(page.locator(".react-flow__edge")).toHaveCount(1);

  if (!desktop) {
    await expect(page.getByText("На мобильном доступен просмотр доски")).toBeVisible();
    await expect(page.getByRole("button", { name: "Заметка" })).toHaveCount(0);
    await page.getByTestId("canvas-document-document-opening").click();
    await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}$`));
    expect(pageErrors).toEqual([]);
    return;
  }

  await expect(page.getByRole("group", { name: "Действия с доской" })).toBeVisible();
  const legacyNote = page.getByTestId("canvas-text-note-premise");
  const canvasDocument = page.getByTestId("canvas-document-document-opening");
  const legacyNoteBox = await legacyNote.boundingBox();
  const initialDocumentBox = await canvasDocument.boundingBox();
  if (!legacyNoteBox || !initialDocumentBox) {
    throw new Error("Seeded Canvas cards are not measurable");
  }
  const canvasScale = initialDocumentBox.width / 280;
  const centerDeltaX =
    initialDocumentBox.x +
    initialDocumentBox.width / 2 -
    (legacyNoteBox.x + legacyNoteBox.width / 2);
  await dragBy(page, legacyNoteBox, centerDeltaX, 200 * canvasScale);
  await alignHorizontalCenters(page, legacyNote, canvasDocument);

  const documentTreeItem = page
    .getByRole("treeitem")
    .filter({ has: page.locator(`a[href$="/studio/${nodeId}"]`) });
  await expect(documentTreeItem).toHaveAttribute("aria-selected", "false");
  const statusMark = page.locator('[data-document-status="draft"]');
  await expect(statusMark).toHaveAttribute("aria-label", "Статус: Черновик");
  await statusMark.hover();
  await expect(page.getByRole("tooltip")).toHaveText("Статус: Черновик");

  const canvasIcon = page.locator(`a[href$="/canvases/${canvasId}"] svg`);
  const documentIcon = page.locator(`a[href$="/studio/${nodeId}"] svg`);
  const canvasIconBox = await canvasIcon.boundingBox();
  const documentIconBox = await documentIcon.boundingBox();
  if (!canvasIconBox || !documentIconBox) throw new Error("Sidebar icons are not visible");
  expect(Math.abs(canvasIconBox.x - documentIconBox.x)).toBeLessThan(1);

  await documentTreeItem.hover();
  const dragHandle = documentTreeItem.getByRole("button", { name: /^Переместить/ });
  const addChildButton = documentTreeItem.getByRole("button", { name: /^Добавить внутрь/ });
  await expect(dragHandle.locator("svg")).toHaveCSS("opacity", "1");
  await expect(addChildButton).toHaveCSS("opacity", "1");
  const dragHandleBox = await dragHandle.boundingBox();
  const addChildBox = await addChildButton.boundingBox();
  const statusMarkBox = await statusMark.boundingBox();
  if (!dragHandleBox || !addChildBox || !statusMarkBox) {
    throw new Error("Sidebar controls are not measurable");
  }
  expect(
    Math.abs(
      dragHandleBox.x + dragHandleBox.width / 2 - (documentIconBox.x + documentIconBox.width / 2),
    ),
  ).toBeLessThan(1);
  expect(addChildBox.x).toBeLessThan(statusMarkBox.x);

  const canvasSectionIcon = page
    .getByText("Доски", { exact: true })
    .locator("..")
    .locator("svg")
    .first();
  expect(await canvasSectionIcon.getAttribute("class")).not.toBe(
    await canvasIcon.getAttribute("class"),
  );

  const toastViewportBox = await page.getByTestId("toast-viewport").boundingBox();
  const viewportWidth = page.viewportSize()?.width;
  if (!toastViewportBox || !viewportWidth) throw new Error("Toast viewport is not measurable");
  expect(
    Math.abs(toastViewportBox.x + toastViewportBox.width / 2 - viewportWidth / 2),
  ).toBeLessThan(1);

  await page.getByRole("button", { name: "Заметка" }).click();
  const notes = page.locator('[data-testid^="canvas-text-"]');
  await expect(notes).toHaveCount(2);
  const newNote = notes.filter({ hasText: "Новая заметка" });
  const newNoteTestId = await newNote.getAttribute("data-testid");
  if (!newNoteTestId) throw new Error("New Canvas note has no stable test id");
  const editableNote = page.getByTestId(newNoteTestId);
  const newNoteBox = await editableNote.boundingBox();
  const canvasDocumentBox = await page
    .getByTestId("canvas-document-document-opening")
    .boundingBox();
  if (!newNoteBox || !canvasDocumentBox) throw new Error("Canvas cards are not measurable");
  expect(Math.abs(newNoteBox.width - canvasDocumentBox.width)).toBeLessThan(1);
  await editableNote.dblclick();
  const editor = page.getByRole("textbox", { name: "Markdown заметки" });
  await editor.fill("# Развязка\n\nУникальный финал Canvas.");
  await editor.press(process.platform === "darwin" ? "Meta+Enter" : "Control+Enter");
  await expect(page.getByText("Уникальный финал Canvas.")).toBeVisible();

  await connectHandles(
    page,
    editableNote.locator('.react-flow__handle[data-handlepos="right"]'),
    page
      .getByTestId("canvas-document-document-opening")
      .locator('.react-flow__handle[data-handlepos="left"]'),
  );
  await expect(page.locator(".react-flow__edge")).toHaveCount(2);
  await expect(page.getByText("Something went wrong!", { exact: true })).toHaveCount(0);

  await expect
    .poll(
      async () => {
        const response = await page.request.get(`/api/canvases/${canvasId}`);
        const workspace = (await response.json()) as { revision: { revision: number } };
        return workspace.revision.revision;
      },
      { timeout: 20_000 },
    )
    .toBeGreaterThan(2);

  await page.reload();
  await expect(page.getByText("Уникальный финал Canvas.")).toBeVisible();
  await page.getByTestId("canvas-document-document-opening").dblclick();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}$`));
  expect(pageErrors).toEqual([]);
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
  const [, , projectId = "", , nodeId = ""] = new URL(page.url()).pathname.split("/");
  return { projectId, nodeId };
}

async function createCanvasFromProjectTree(page: Page): Promise<string> {
  await page.waitForLoadState("networkidle");
  const expand = page.getByRole("button", { name: "Развернуть структуру" });
  const create = page.getByRole("button", { name: "Создать доску" });
  if (!(await create.isVisible())) {
    await expect(expand).toBeVisible();
    await expand.click();
  }
  await create.click();
  const dialog = page.getByRole("alertdialog", { name: "Новая доска" });
  await dialog.getByRole("textbox", { name: "Название" }).fill("Сюжетная доска");
  await dialog.getByRole("button", { name: "Создать" }).click();
  await expect(page).toHaveURL(/\/canvases\/[^/]+$/);
  return new URL(page.url()).pathname.split("/").at(-1) ?? "";
}

async function connectHandles(page: Page, source: Locator, target: Locator) {
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  if (!sourceBox || !targetBox) throw new Error("Canvas connection handles are not visible");
  await page.mouse.move(sourceBox.x + sourceBox.width / 2, sourceBox.y + sourceBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(targetBox.x + targetBox.width / 2, targetBox.y + targetBox.height / 2, {
    steps: 8,
  });
  await page.mouse.up();
}

async function dragBy(
  page: Page,
  box: { x: number; y: number; width: number; height: number },
  deltaX: number,
  deltaY: number,
) {
  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
  await page.mouse.up();
}

async function alignHorizontalCenters(page: Page, moving: Locator, peer: Locator) {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const movingBox = await moving.boundingBox();
    const peerBox = await peer.boundingBox();
    if (!movingBox || !peerBox) throw new Error("Canvas cards are not measurable while aligning");
    const delta = peerBox.x + peerBox.width / 2 - (movingBox.x + movingBox.width / 2);
    if (Math.abs(delta) < 1) return;
    await dragBy(page, movingBox, delta, 0);
  }
  const movingBox = await moving.boundingBox();
  const peerBox = await peer.boundingBox();
  if (!movingBox || !peerBox) throw new Error("Aligned Canvas cards are not measurable");
  expect(
    Math.abs(movingBox.x + movingBox.width / 2 - (peerBox.x + peerBox.width / 2)),
  ).toBeLessThan(1);
}

async function createCanvasThroughAPI(page: Page, projectId: string): Promise<string> {
  const response = await page.request.post(`/api/projects/${projectId}/canvases`, {
    data: { title: "Сюжетная доска" },
  });
  expect(response.status()).toBe(201);
  const payload = (await response.json()) as { canvas: { id: string } };
  return payload.canvas.id;
}
