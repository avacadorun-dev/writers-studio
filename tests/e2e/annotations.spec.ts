import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

test("dictionary links and project notes stay anchored and deep-link to the exact manuscript range", async ({
  page,
}, testInfo) => {
  const suffix = `annotations-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);
  const fixture = await page.evaluate(
    async ({ projectId, nodeId }) => {
      const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
        const response = await fetch(path, {
          credentials: "include",
          headers: { "Content-Type": "application/json", ...init?.headers },
          ...init,
        });
        if (!response.ok) throw new Error(`${init?.method ?? "GET"} ${path}: ${response.status}`);
        return (await response.json()) as T;
      };
      const content = {
        type: "doc",
        attrs: { schemaVersion: 1 },
        content: [
          {
            type: "paragraph",
            attrs: { blockId: "annotation-block", sourceRefs: [] },
            content: [
              {
                type: "text",
                text: "Лиса вошла в Тёмный лес и слишком громко закрыла дверь.",
              },
            ],
          },
        ],
      };
      await request(`/api/nodes/${nodeId}/revisions`, {
        method: "PUT",
        body: JSON.stringify({ baseRevision: 1, content, source: "e2e_fixture" }),
      });
      const term = await request<{ term: { id: string } }>(
        `/api/projects/${projectId}/dictionary`,
        {
          method: "POST",
          body: JSON.stringify({
            canonical: "Тёмный лес",
            variants: ["лес"],
            kind: "place",
            language: "ru",
          }),
        },
      );
      const link = await request<{ annotation: { id: string } }>(
        `/api/nodes/${nodeId}/annotations`,
        {
          method: "POST",
          body: JSON.stringify({
            kind: "dictionary_link",
            dictionaryTermId: term.term.id,
            baseRevision: 2,
            anchor: {
              blockId: "annotation-block",
              start: 13,
              end: 23,
              exactText: "Тёмный лес",
              prefix: "Лиса вошла в ",
              suffix: " и слишком громко закрыла дверь.",
            },
          }),
        },
      );
      const note = await request<{ annotation: { id: string } }>(
        `/api/nodes/${nodeId}/annotations`,
        {
          method: "POST",
          body: JSON.stringify({
            kind: "note",
            note: "Сделать действие тише и правдоподобнее",
            baseRevision: 2,
            anchor: {
              blockId: "annotation-block",
              start: 34,
              end: 40,
              exactText: "громко",
              prefix: " вошла в Тёмный лес и слишком ",
              suffix: " закрыла дверь.",
            },
          }),
        },
      );
      return { linkId: link.annotation.id, noteId: note.annotation.id };
    },
    { projectId, nodeId },
  );

  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await expect(
    page.locator(`.ws-dictionary-link[data-annotation-id="${fixture.linkId}"]`),
  ).toHaveText("Тёмный лес");
  await expect(
    page.locator(`.ws-note-highlight[data-annotation-id="${fixture.noteId}"]`),
  ).toHaveText("громко");

  await page.locator(`.ws-note-highlight[data-annotation-id="${fixture.noteId}"]`).click();
  await expect(page.getByRole("heading", { name: "Заметки и ссылки" })).toBeVisible();
  await expectWorkspaceHeadersToAlign(page);
  await expect(page.getByRole("textbox", { name: "Заметка" })).toHaveValue(
    "Сделать действие тише и правдоподобнее",
  );

  await page.goto(`/projects/${projectId}/annotations`);
  await expect(page.getByRole("heading", { name: "Правки в тексте" })).toBeVisible();
  await expect(page.getByText("2 открытых пометок")).toBeVisible();
  const noteCard = page.getByRole("article").filter({ hasText: "Сделать действие тише" });
  await expect(noteCard).toContainText("громко");
  await noteCard.getByRole("link", { name: "Перейти" }).click();
  await expect(page).toHaveURL(
    new RegExp(`/projects/${projectId}/studio/${nodeId}\\?annotationId=${fixture.noteId}`),
  );
  await expect(page.getByRole("textbox", { name: "Заметка" })).toHaveValue(
    "Сделать действие тише и правдоподобнее",
  );
  await expect.poll(() => page.evaluate(() => window.getSelection()?.toString())).toBe("громко");

  await page.getByRole("button", { name: "Исправлено" }).click();
  await expect(page.getByRole("button", { name: "Вернуть" })).toBeVisible();
  await page.getByRole("link", { name: "Все правки проекта" }).click();
  await page.getByRole("button", { name: /Исправлено · 1/ }).click();
  await expect(
    page.getByRole("article").filter({ hasText: "Сделать действие тише" }),
  ).toBeVisible();

  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await selectEditorText(page, "дверь");
  await page.locator(".tiptap").click({ button: "right" });
  await page.getByRole("menuitem", { name: "Добавить заметку" }).click();
  await expect(page.getByRole("dialog", { name: "Заметка на полях" })).toBeVisible();
  await page.getByPlaceholder(/уточнить мотивацию/).fill("Проверить финальный жест сцены");
  await page.getByRole("button", { name: "Добавить заметку" }).last().click();
  await expect(page.getByRole("textbox", { name: "Заметка" })).toHaveValue(
    "Проверить финальный жест сцены",
  );

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
});

async function expectWorkspaceHeadersToAlign(page: Page) {
  if ((page.viewportSize()?.width ?? 0) < 1024) return;
  const headers = [
    page.getByTestId("project-tree-header"),
    page.getByRole("toolbar", { name: "Форматирование текста" }),
    page.getByTestId("annotations-panel-header"),
  ];
  const boxes = await Promise.all(headers.map((header) => header.boundingBox()));
  expect(boxes.every(Boolean)).toBe(true);
  const [tree, editor, annotations] = boxes as NonNullable<(typeof boxes)[number]>[];
  expect(tree.height).toBe(56);
  expect(editor.height).toBe(56);
  expect(annotations.height).toBe(56);
  expect(Math.abs(tree.y - editor.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(annotations.y - editor.y)).toBeLessThanOrEqual(1);
}

async function registerAndCreateProject(page: Page, suffix: string) {
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
  await expect(page).toHaveURL(/\/projects\/([^/]+)\/studio\/([^/]+)/);
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/([^/]+)$/);
  if (!match) throw new Error("studio URL did not contain project and node IDs");
  return { projectId: match[1], nodeId: match[2] };
}

async function selectEditorText(page: Page, needle: string) {
  await page.locator(".tiptap").evaluate((root, selectedText) => {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let textNode: Text | null = null;
    let offset = -1;
    while (walker.nextNode()) {
      const candidate = walker.currentNode as Text;
      const candidateOffset = candidate.data.indexOf(selectedText);
      if (candidateOffset >= 0) {
        textNode = candidate;
        offset = candidateOffset;
        break;
      }
    }
    if (!textNode || offset < 0) throw new Error(`text not found: ${selectedText}`);
    const range = document.createRange();
    range.setStart(textNode, offset);
    range.setEnd(textNode, offset + selectedText.length);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    document.dispatchEvent(new Event("selectionchange", { bubbles: true }));
    root.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  }, needle);
}
