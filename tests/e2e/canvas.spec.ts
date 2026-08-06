import { expect, type Locator, type Page, test } from "@playwright/test";

test("Canvas board creates, renders, autosaves, and stays read-only on mobile", async ({
  page,
}, testInfo) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  const suffix = `canvas-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const viewportWidth = page.viewportSize()?.width ?? 0;
  const desktop = viewportWidth >= 1024;
  const editable = viewportWidth >= 768;
  const localDocsURL = new URL("/docs", page.url()).toString();
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
          {
            id: "link-docs",
            type: "link",
            x: 700,
            y: 0,
            width: 280,
            height: 180,
            url: localDocsURL,
            "x-writers-studio": {
              preview: {
                url: localDocsURL,
                title: "Документация Writers Studio",
                description: "Справочник по возможностям продукта.",
                siteName: "Writers Studio",
              },
            },
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
  await expect(page.getByTestId("canvas-link-link-docs")).toContainText(
    "Документация Writers Studio",
  );
  await expect(page.getByTestId("canvas-group-group-story")).toContainText("Сюжет");
  await expect(page.locator(".react-flow__edge")).toHaveCount(1);

  if (!editable) {
    await expect(page.getByText("На телефоне доступен просмотр доски")).toBeAttached();
    await expect(page.getByRole("button", { name: "Добавить" })).toHaveCount(0);
    await page.getByTestId("canvas-document-document-opening").click();
    await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}$`));
    expect(pageErrors).toEqual([]);
    return;
  }

  await expect(page.getByRole("group", { name: "Действия с доской" })).toBeVisible();
  const toolRail = page.getByRole("toolbar", { name: "Инструменты Canvas" });
  const selectTool = toolRail.getByRole("button", { name: "Выбор (V)" });
  await expect(selectTool).toHaveAttribute("data-ui-tooltip", "Выбор (V)");
  if (desktop) {
    const initialViewport = page.viewportSize();
    await page.setViewportSize({ width: 2_560, height: 1_440 });
    await expect(selectTool.locator("[data-tool-label]")).toBeHidden();
    await expect
      .poll(async () => (await toolRail.boundingBox())?.width ?? Number.POSITIVE_INFINITY)
      .toBeLessThanOrEqual(44);
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth === document.documentElement.clientWidth &&
          document.documentElement.scrollHeight === document.documentElement.clientHeight,
      ),
    ).toBe(true);
    if (initialViewport) await page.setViewportSize(initialViewport);
  }
  await selectTool.hover();
  await expect(page.getByRole("tooltip")).toContainText("Выбор (V)");
  if (desktop) {
    await toolRail.getByRole("button", { name: "Рисование" }).click();
    const drawingMenu = page.getByRole("group", { name: "Рисование и фигуры" });
    await expect(drawingMenu).toBeVisible();
    expect(
      await drawingMenu.evaluate((element) => element.scrollWidth === element.clientWidth),
    ).toBe(true);
    await toolRail.getByRole("button", { name: "Рисование" }).click();
  }
  if (!desktop) {
    await createNoteFromToolRail(page, toolRail);
    await expect(page.locator('[data-testid^="canvas-text-"]')).toHaveCount(2);
    expect(pageErrors).toEqual([]);
    return;
  }

  const legacyNote = page.getByTestId("canvas-text-note-premise");
  const canvasDocument = page.getByTestId("canvas-document-document-opening");
  const canvasLink = page.getByTestId("canvas-link-link-docs");
  const canvasURL = page.url();
  const pagesBeforeSingleClick = page.context().pages().length;
  await canvasDocument.click();
  await expect(page).toHaveURL(canvasURL);
  await canvasLink.click();
  await expect(page).toHaveURL(canvasURL);
  expect(page.context().pages()).toHaveLength(pagesBeforeSingleClick);

  await canvasDocument.click({ button: "right" });
  const nodeMenu = page.getByRole("menu", { name: "Действия с карточкой" });
  await expect(nodeMenu).toBeVisible();
  await nodeMenu.getByRole("menuitem", { name: "На передний план" }).click();
  await expect(nodeMenu).toBeHidden();
  await expect
    .poll(() =>
      page
        .locator(".react-flow__node")
        .evaluateAll((items) => items.at(-1)?.getAttribute("data-id")),
    )
    .toBe("document-opening");

  const popupPromise = page.waitForEvent("popup");
  await canvasLink.dblclick();
  const docsPopup = await popupPromise;
  await docsPopup.waitForLoadState("domcontentloaded");
  await expect(docsPopup).toHaveURL(localDocsURL);
  await docsPopup.close();

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
  const toastViewportWidth = page.viewportSize()?.width;
  if (!toastViewportBox || !toastViewportWidth) {
    throw new Error("Toast viewport is not measurable");
  }
  expect(
    Math.abs(toastViewportBox.x + toastViewportBox.width / 2 - toastViewportWidth / 2),
  ).toBeLessThan(1);

  await createNoteFromToolRail(page, toolRail);
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

test("Canvas presentation focuses frames and the board keeps floating panels in place", async ({
  page,
}, testInfo) => {
  test.skip((page.viewportSize()?.width ?? 0) < 1_024, "Desktop Canvas presentation");
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  const suffix = `canvas-presentation-${testInfo.project.name}-${Date.now()}`;
  const { projectId } = await registerAndCreateProject(page, suffix);
  const canvasId = await createCanvasThroughAPI(page, projectId);
  const seeded = await page.request.put(`/api/canvases/${canvasId}/revisions`, {
    data: {
      baseRevision: 1,
      source: "e2e_presentation_seed",
      content: {
        nodes: [
          {
            id: "frame-opening",
            type: "group",
            x: 0,
            y: 0,
            width: 800,
            height: 500,
            "x-writers-studio": {
              element: { kind: "frame", order: 1, label: "Завязка" },
            },
          },
          {
            id: "note-opening",
            type: "text",
            x: 80,
            y: 100,
            width: 280,
            height: 180,
            text: "# Завязка\n\nГерой получает сигнал.",
          },
          {
            id: "frame-finale",
            type: "group",
            x: 1_000,
            y: 0,
            width: 800,
            height: 500,
            "x-writers-studio": {
              element: { kind: "frame", order: 2, label: "Финал" },
            },
          },
          {
            id: "note-finale",
            type: "text",
            x: 1_080,
            y: 100,
            width: 280,
            height: 180,
            text: "# Финал\n\nСигнал получает ответ.",
          },
          {
            id: "note-outside",
            type: "text",
            x: 2_000,
            y: 700,
            width: 280,
            height: 180,
            text: "# Черновик за рамками",
          },
        ],
        edges: [],
        "x-writers-studio": { schemaVersion: 2 },
      },
    },
  });
  expect(seeded.status()).toBe(201);

  await page.goto(`/projects/${projectId}/canvases/${canvasId}`);
  const board = page.getByTestId("canvas-board");
  const toolRail = page.getByRole("toolbar", { name: "Инструменты Canvas" });
  await expect(board).toBeVisible();
  await expect(toolRail).toBeVisible();

  await toolRail.getByRole("button", { name: "Режимы и анализ" }).click();
  await page.getByRole("button", { name: "Объекты и поиск" }).click();
  const navigator = page.getByRole("dialog", { name: "Объекты доски" });
  await expect(navigator).toBeVisible();
  await expect(page.getByRole("group", { name: "Режимы, поиск и анализ" })).toBeHidden();
  const railBox = await toolRail.boundingBox();
  const navigatorBox = await navigator.boundingBox();
  if (!railBox || !navigatorBox) throw new Error("Canvas navigation panels are not measurable");
  expect(navigatorBox.x).toBeGreaterThanOrEqual(railBox.x + railBox.width + 4);
  await navigator.getByRole("button", { name: "Закрыть" }).click();

  await toolRail.getByRole("button", { name: "Режимы и анализ" }).click();
  await page.getByRole("button", { name: "Презентация" }).click();
  await expect(toolRail).toBeHidden();
  await expect(page.getByText(/1\/2 · Завязка/)).toBeVisible();
  await expect(page.getByTestId("canvas-frame-frame-opening")).toBeVisible();
  await expect(page.getByTestId("canvas-text-note-opening")).toBeVisible();
  await expect(page.getByTestId("canvas-frame-frame-finale")).toBeHidden();
  await expect(page.getByTestId("canvas-text-note-finale")).toBeHidden();
  await expect(page.getByTestId("canvas-text-note-outside")).toBeHidden();
  await expect(page.locator(".react-flow__minimap")).toHaveCount(0);
  await expect(page.locator(".react-flow__controls")).toHaveCount(0);
  const boardBox = await board.boundingBox();
  if (!boardBox) throw new Error("Presentation board is not measurable");
  await expect
    .poll(
      async () => (await page.getByTestId("canvas-frame-frame-opening").boundingBox())?.width ?? 0,
    )
    .toBeGreaterThan(boardBox.width * 0.55);

  await page.getByRole("button", { name: "Следующий фрейм" }).click();
  await expect(page.getByText(/2\/2 · Финал/)).toBeVisible();
  await expect(page.getByTestId("canvas-frame-frame-opening")).toBeHidden();
  await expect(page.getByTestId("canvas-frame-frame-finale")).toBeVisible();
  await page.getByRole("button", { name: "Завершить презентацию" }).click();
  await expect(toolRail).toBeVisible();
  await expect(page.getByTestId("canvas-text-note-outside")).toBeVisible();

  const boardBeforeAssistant = await board.boundingBox();
  await page
    .getByRole("group", { name: "Действия с доской" })
    .getByRole("button", { name: "Открыть Ассистента для этой доски" })
    .click();
  const assistantPanel = page.getByTestId("canvas-assistant-panel");
  await expect(assistantPanel).toBeVisible();
  await expect(page.getByTestId("assistant-new-chat-panel")).toBeVisible();
  await expect(page.getByRole("button", { name: /^Сюжетная доска/ })).toBeVisible();
  await expect(board).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/canvases/${canvasId}$`));
  const boardAfterAssistant = await board.boundingBox();
  if (!boardBeforeAssistant || !boardAfterAssistant) {
    throw new Error("Canvas board is not measurable with Assistant open");
  }
  expect(Math.abs(boardBeforeAssistant.width - boardAfterAssistant.width)).toBeLessThan(1);
  expect(Math.abs(boardBeforeAssistant.height - boardAfterAssistant.height)).toBeLessThan(1);
  await assistantPanel.getByRole("button", { name: "Скрыть панель" }).click();
  await expect(assistantPanel).toBeHidden();
  expect(pageErrors).toEqual([]);
});

async function createNoteFromToolRail(page: Page, toolRail: Locator) {
  await toolRail.getByRole("button", { name: "Добавить" }).click();
  const createFlyout = page.getByRole("group", { name: "Добавить на доску" });
  await expect(createFlyout).toBeVisible();
  const noteTool = createFlyout.getByRole("button", { name: "Заметка (N)" });
  await expect(noteTool).toHaveAttribute("data-ui-tooltip", "Заметка (N)");
  await noteTool.click();
  const pane = page.locator(".react-flow__pane");
  const box = await pane.boundingBox();
  if (!box) throw new Error("Canvas pane is not measurable");
  await pane.dispatchEvent("pointerdown", {
    bubbles: true,
    button: 0,
    buttons: 1,
    clientX: box.x + box.width * 0.72,
    clientY: box.y + box.height * 0.75,
    isPrimary: true,
    pointerId: 1,
    pointerType: "mouse",
  });
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
