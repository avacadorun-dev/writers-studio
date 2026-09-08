import { expect, type Locator, type Page, test } from "@playwright/test";

test("release 0.7 editor interactions survive autosave and reload", async ({ page }, testInfo) => {
  const suffix = `release-070-editor-${testInfo.project.name}-${Date.now()}`;
  const { nodeId } = await registerAndCreateProject(page, suffix);
  const viewportWidth = page.viewportSize()?.width ?? 0;
  const toolbar = page.getByRole("toolbar", { name: "Форматирование текста" });

  await expect(toolbar).toHaveAttribute(
    "data-toolbar-presentation",
    viewportWidth >= 1280 ? "expanded" : "compact",
  );
  if (viewportWidth >= 1280) {
    for (const action of [
      "Курсив",
      "Полужирный",
      "Подчёркнутый",
      "Зачёркнутый",
      "Строчный код",
      "Цитата",
      "Маркированный список",
      "Нумерованный список",
    ]) {
      await expect(toolbar.getByRole("button", { name: action })).toBeVisible();
    }
  } else {
    await toolbar.getByRole("button", { name: "Дополнительное форматирование" }).click();
    await expect(page.getByRole("menuitem", { name: "Подчёркнутый" })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: "Нумерованный список" })).toBeVisible();
    await page.keyboard.press("Escape");
  }

  const manuscriptTitle = page
    .getByRole("region", { name: "Редактор документа" })
    .getByTitle("Дважды нажмите, чтобы переименовать");
  await manuscriptTitle.dblclick();
  const renamedTitle = `Сцена с формулой ${testInfo.project.name}`;
  const titleInput = page.getByRole("textbox", { name: /Новое название/ }).last();
  await titleInput.fill(renamedTitle);
  await titleInput.press("Enter");
  await expect(manuscriptTitle).toContainText(renamedTitle);

  if (viewportWidth >= 1280) {
    const selectedTreeItem = page.getByRole("treeitem", { selected: true });
    await selectedTreeItem.focus();
    await selectedTreeItem.press("F2");
    const treeTitleInput = selectedTreeItem.getByRole("textbox", { name: /Новое название/ });
    await expect(treeTitleInput).toBeFocused();
    await treeTitleInput.fill(`${renamedTitle} F2`);
    await treeTitleInput.press("Enter");
    await expect(selectedTreeItem).toContainText(`${renamedTitle} F2`);
  }

  const editor = page.locator(".tiptap");
  await editor.click();
  await page.keyboard.type("код`");
  await editor.evaluate((element) => {
    const text = element.querySelector("p")?.firstChild;
    if (!text) throw new Error("Editor paragraph has no text node");
    const range = document.createRange();
    range.setStart(text, 0);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
  await page.keyboard.type("`");
  await expect(editor.locator("code")).toHaveText("код");
  await page.keyboard.press("ControlOrMeta+Z");
  await expect(editor).toContainText("код`");
  await page.keyboard.press("ControlOrMeta+Shift+Z");
  await expect(editor.locator("code")).toHaveText("код");

  await toolbar.getByRole("button", { name: "Типографика" }).click();
  await page.locator("#editor-toolbar-paragraph-spacing").selectOption("1.1");
  await page.locator("#editor-toolbar-list-spacing").selectOption("0.5");
  await page.keyboard.press("Escape");
  await expect
    .poll(() =>
      page.evaluate(() => ({
        paragraph: document.documentElement.style.getPropertyValue("--editor-paragraph-spacing"),
        list: document.documentElement.style.getPropertyValue("--editor-list-item-spacing"),
      })),
    )
    .toEqual({ paragraph: "1.1em", list: "0.5em" });

  await toolbar.getByRole("button", { name: "Вставить блок" }).click();
  await page.getByRole("menuitem", { name: "Формула", exact: true }).click();
  const formulaDialog = page.getByRole("alertdialog", { name: "Вставить формулу" });
  await formulaDialog.getByRole("textbox", { name: "Исходник LaTeX" }).fill("\\frac{a}{b}");
  await formulaDialog.getByRole("button", { name: "Вставить" }).last().click();
  const formula = page.locator("figure[data-math-block]");
  await expect(formula).toBeVisible();
  await expect(formula.locator(".katex")).toBeVisible();
  const formulaSource = formula.getByRole("textbox", { name: "Исходник блочной формулы" });
  await formulaSource.fill("\\frac{");
  await formulaSource.blur();
  await expect(formula).toContainText("Не удалось отобразить формулу");
  await formulaSource.fill("E = mc^2");
  await formulaSource.blur();
  await expect(formula.locator(".katex")).toBeVisible();

  const imageCount = await page.locator("figure[data-image-block]").count();
  await dropFile(page.locator(".editor-content"), {
    name: "notes.txt",
    type: "text/plain",
    bytes: [1, 2, 3],
  });
  await expect(page.getByText("Изображение не добавлено", { exact: true })).toBeVisible();
  await expect(page.locator("figure[data-image-block]")).toHaveCount(imageCount);

  await dropFile(page.locator(".editor-content"), {
    name: "pixel.png",
    type: "image/png",
    base64:
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WlqZVQAAAAASUVORK5CYII=",
  });
  await expect(page.locator("figure[data-image-block]")).toHaveCount(imageCount + 1, {
    timeout: 15_000,
  });
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({ timeout: 15_000 });

  const statusBar = page.getByRole("region", { name: "Редактор документа" }).locator("footer");
  await expect(statusBar).toBeVisible();
  expect(await statusBar.evaluate((element) => getComputedStyle(element).height)).toBe("40px");
  expect(await hasDocumentOverflow(page)).toBe(false);

  await page.reload();
  await expect(page.locator("figure[data-math-block]")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Исходник блочной формулы" })).toHaveValue(
    "E = mc^2",
  );
  await expect(page.locator("figure[data-image-block]")).toHaveCount(imageCount + 1);
  await expect(editor.locator("code")).toHaveText("код");
  expect(await hasDocumentOverflow(page)).toBe(false);

  const storedNode = await page.request.get(`/api/nodes/${nodeId}`);
  expect(storedNode.ok()).toBe(true);
});

test("Studio and Canvas share one isolated responsive Assistant dock", async ({
  page,
}, testInfo) => {
  const suffix = `release-070-dock-${testInfo.project.name}-${Date.now()}`;
  const { projectId } = await registerAndCreateProject(page, suffix);
  const viewportWidth = page.viewportSize()?.width ?? 0;
  const studioTrigger = page.getByRole("banner").getByRole("button", { name: "Ассистент" });
  await studioTrigger.click();
  const studioDock = page.getByTestId("assistant-dock");
  await assertDockGeometry(studioDock, viewportWidth);
  const beforeWheel = await page.evaluate(() => ({ x: scrollX, y: scrollY }));
  await studioDock.dispatchEvent("wheel", { deltaY: 500 });
  expect(await page.evaluate(() => ({ x: scrollX, y: scrollY }))).toEqual(beforeWheel);
  await studioDock.getByRole("button", { name: "Скрыть панель" }).click();
  await expect(studioTrigger).toBeFocused();

  const canvasResponse = await page.request.post(`/api/projects/${projectId}/canvases`, {
    data: { title: "Доска релиза 0.7" },
  });
  expect(canvasResponse.status()).toBe(201);
  const canvasId = ((await canvasResponse.json()) as { canvas: { id: string } }).canvas.id;
  await page.goto(`/projects/${projectId}/canvases/${canvasId}`);
  await expect(page.getByTestId("canvas-board")).toBeVisible();
  const canvasTrigger = page.getByRole("banner").getByRole("button", { name: "Ассистент" });
  await canvasTrigger.click();
  const canvasDock = page.getByTestId("canvas-assistant-panel");
  await assertDockGeometry(canvasDock, viewportWidth);
  await expect(canvasDock.getByText("Ассистент · новый чат")).toBeVisible();
  if (viewportWidth >= 1280) {
    const board = page.getByTestId("canvas-board");
    const [boardBox, dockBox] = await Promise.all([board.boundingBox(), canvasDock.boundingBox()]);
    expect(Math.abs((boardBox?.x ?? 0) + (boardBox?.width ?? 0) - (dockBox?.x ?? 0))).toBeLessThan(
      2,
    );
  }
  expect(await hasDocumentOverflow(page)).toBe(false);
  await canvasDock.getByRole("button", { name: "Скрыть панель" }).click();
  await expect(canvasTrigger).toBeFocused();
});

async function assertDockGeometry(dock: Locator, viewportWidth: number) {
  await expect(dock).toBeVisible();
  const geometry = await dock.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    return {
      width: Math.round(bounds.width),
      right: Math.round(window.innerWidth - bounds.right),
      radius: Number.parseFloat(getComputedStyle(element).borderRadius),
    };
  });
  expect(geometry.radius).toBe(0);
  expect(geometry.right).toBe(0);
  expect(geometry.width).toBe(viewportWidth < 768 ? viewportWidth : 380);
}

async function dropFile(
  target: Locator,
  file: { name: string; type: string; bytes?: number[]; base64?: string },
) {
  const box = await target.boundingBox();
  if (!box) throw new Error("Editor drop target is not measurable");
  await target.evaluate(
    (element, input) => {
      const binary = input.file.base64
        ? Uint8Array.from(atob(input.file.base64), (character) => character.charCodeAt(0))
        : new Uint8Array(input.file.bytes ?? []);
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(
        new File([binary], input.file.name, { type: input.file.type, lastModified: Date.now() }),
      );
      for (const type of ["dragenter", "dragover", "drop"]) {
        element.dispatchEvent(
          new DragEvent(type, {
            bubbles: true,
            cancelable: true,
            clientX: input.x,
            clientY: input.y,
            dataTransfer,
          }),
        );
      }
    },
    { file, x: box.x + box.width / 2, y: box.y + Math.min(180, box.height / 2) },
  );
}

async function hasDocumentOverflow(page: Page): Promise<boolean> {
  return page.evaluate(
    () =>
      document.documentElement.scrollWidth > document.documentElement.clientWidth ||
      document.documentElement.scrollHeight > document.documentElement.clientHeight,
  );
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
