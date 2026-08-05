import { expect, type Page, test } from "@playwright/test";

test("project-wide replace previews, applies one operation, highlights, and undoes", async ({
  page,
}, testInfo) => {
  const { projectId, nodeId } = await registerAndCreateProject(
    page,
    `replace-${testInfo.project.name}-${Date.now()}`,
  );
  await page.locator(".tiptap").click();
  await page.keyboard.type("Анна вошла в дом. Анна закрыла дверь.");
  await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({ timeout: 10_000 });

  await page.goto(`/projects/${projectId}/search?q=${encodeURIComponent("Анна")}&scope=all`);
  await expect(page.getByText(/Анна вошла в дом/)).toBeVisible();
  await page.getByText("Заменить по проекту", { exact: true }).click();
  await page.getByLabel("Найти", { exact: true }).fill("Анна");
  await page.getByLabel("Заменить на").fill("Елена");
  await page.getByRole("button", { name: "Предпросмотр" }).click();
  await expect(page.getByText("2 замен в 1 документах")).toBeVisible();
  await expect(page.getByText(/→.*Елена/)).toBeVisible();
  await page.getByRole("button", { name: "Применить показанные изменения" }).click();
  await expect.poll(async () => (await getNode(page, nodeId)).plainText).toContain("Елена");
  const appliedHistory = await getHistory(page, nodeId);
  expect(appliedHistory.revisions[0].source).toBe("project_replace");
  expect(appliedHistory.revisions[0].operationId).toBeTruthy();

  await page.locator('[data-radix-toast-announce-alt="Отменить"]').click();
  await expect.poll(async () => (await getNode(page, nodeId)).plainText).toContain("Анна");
  const undoHistory = await getHistory(page, nodeId);
  expect(undoHistory.revisions[0].source).toBe("project_replace_undo");
  expect(undoHistory.revisions[0].revision).toBeGreaterThan(appliedHistory.revisions[0].revision);

  await page.goto(`/projects/${projectId}/search?q=${encodeURIComponent("Анна")}&scope=all`);
  await page.getByRole("link").filter({ hasText: "Анна вошла в дом" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/studio/${nodeId}`));
  await expect.poll(() => page.evaluate(() => window.getSelection()?.toString())).toBe("Анна");
});

test("document and project lifecycle actions are reversible until confirmed purge", async ({
  page,
}, testInfo) => {
  const { projectId, nodeId, projectTitle } = await registerAndCreateProject(
    page,
    `lifecycle-${testInfo.project.name}-${Date.now()}`,
  );
  const parent = await page.evaluate(async (project) => {
    const response = await fetch(`/api/projects/${project}/nodes`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind: "part", title: "Часть структуры" }),
    });
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }, projectId);
  if ((page.viewportSize()?.width ?? 0) >= 1_024) {
    if ((page.viewportSize()?.width ?? 0) < 1_280) {
      const expandTree = page.getByRole("button", { name: /Развернуть структуру/ });
      await expect(expandTree).toBeVisible();
      await expandTree.click();
    }
    await page.locator(".tiptap").click();
    await page.keyboard.type("Начало конец");
    await expect(page.locator('[data-save-state="saved"]:visible')).toBeVisible({
      timeout: 10_000,
    });
    await page.reload();
    await expect(page.getByRole("link", { name: "Часть структуры" })).toBeVisible();
    const draft = await page.evaluate(async (project) => {
      const response = await fetch(`/api/projects/${project}/nodes`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind: "draft", title: "Черновик для переноса" }),
      });
      if (!response.ok) throw new Error(await response.text());
      return response.json();
    }, projectId);
    await page.reload();
    const draftRow = page.getByRole("treeitem").filter({
      has: page.getByRole("link", { name: "Черновик для переноса", exact: true }),
    });
    const chapterRow = page.getByRole("treeitem").filter({
      has: page.getByRole("link", { name: "Глава 1", exact: true }),
    });
    await expect(draftRow).toBeVisible();
    await draftRow.dragTo(chapterRow);
    await expect(page.getByText("Материал перемещён", { exact: true })).toBeVisible();
    await expect.poll(async () => (await getNode(page, draft.node.id)).parentId).toBe(nodeId);
    await page.reload();
    await expect(draftRow).toHaveAttribute("aria-level", "2");

    const selectionBeforeMove = await page.evaluate(() => {
      const text = document.querySelector(".tiptap p")?.firstChild;
      if (!text) {
        throw new Error("editor text node not found");
      }
      const range = document.createRange();
      range.setStart(text, 6);
      range.collapse(true);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
      return { offset: selection?.anchorOffset, text: selection?.anchorNode?.textContent };
    });

    await page.getByRole("button", { name: "Действия с материалом" }).click();
    await page.getByRole("menuitem", { name: "Переместить" }).click();
    const moveDialog = page.getByRole("alertdialog", { name: /Переместить/ });
    await moveDialog.getByLabel("Новое расположение").selectOption(parent.node.id);
    await moveDialog.getByRole("button", { name: "Переместить" }).click();
    await expect(page.getByText("Материал перемещён", { exact: true })).toBeVisible();
    await expect
      .poll(() =>
        page.evaluate(() => ({
          offset: window.getSelection()?.anchorOffset,
          text: window.getSelection()?.anchorNode?.textContent,
        })),
      )
      .toEqual(selectionBeforeMove);
    expect((await getNode(page, nodeId)).parentId).toBe(parent.node.id);

    await page.locator('[data-radix-toast-announce-alt="Отменить"]').click();
    await expect.poll(async () => (await getNode(page, nodeId)).parentId).toBeFalsy();
  }
  const lifecycle = await page.evaluate(
    async ({ project, node, parent }) => {
      const patch = await fetch(`/api/nodes/${node}`, {
        method: "PATCH",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: "Глава переименована", status: "editing" }),
      }).then((response) => response.json());
      const duplicate = await fetch(`/api/nodes/${node}/duplicate`, {
        method: "POST",
        credentials: "include",
      }).then((response) => response.json());
      const duplicateId = duplicate.node.id as string;
      const moved = await fetch(`/api/nodes/${duplicateId}/move`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ parentId: parent, position: 500 }),
      });
      const trashed = await fetch(`/api/nodes/${duplicateId}`, {
        method: "DELETE",
        credentials: "include",
      });
      const afterTrash = await fetch(`/api/projects/${project}`, {
        credentials: "include",
      }).then((response) => response.json());
      const restored = await fetch(`/api/nodes/${duplicateId}/restore`, {
        method: "POST",
        credentials: "include",
      });
      const afterRestore = await fetch(`/api/projects/${project}`, {
        credentials: "include",
      }).then((response) => response.json());
      return {
        patched: patch.node,
        duplicateId,
        movedStatus: moved.status,
        trashedStatus: trashed.status,
        trashedNode: afterTrash.nodes.find(
          (candidate: { id: string }) => candidate.id === duplicateId,
        ),
        restoredStatus: restored.status,
        restoredNode: afterRestore.nodes.find(
          (candidate: { id: string }) => candidate.id === duplicateId,
        ),
      };
    },
    { project: projectId, node: nodeId, parent: parent.node.id },
  );
  expect(lifecycle.patched.title).toBe("Глава переименована");
  expect(lifecycle.patched.status).toBe("editing");
  expect(lifecycle.movedStatus).toBe(200);
  expect(lifecycle.trashedStatus).toBe(200);
  expect(lifecycle.trashedNode.trashedAt).toBeTruthy();
  expect(lifecycle.restoredStatus).toBe(200);
  expect(lifecycle.restoredNode.trashedAt).toBeFalsy();
  expect(lifecycle.restoredNode.parentId).toBe(parent.node.id);

  await page.goto("/projects");
  await page.getByRole("button", { name: `Действия: ${projectTitle}` }).click();
  await page.getByRole("menuitem", { name: "В архив" }).click();
  await page.getByRole("button", { name: "Архив" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();
  await page.getByRole("button", { name: `Действия: ${projectTitle}` }).click();
  await page.getByRole("menuitem", { name: "Восстановить" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeHidden();

  await page.getByRole("button", { name: "Недавние" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();
  await page.getByRole("button", { name: `Действия: ${projectTitle}` }).click();
  await page.getByRole("menuitem", { name: "В корзину" }).click();
  await page.getByRole("button", { name: "Корзина" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();

  const rejected = await page.evaluate(
    async ({ project }) => {
      const response = await fetch(`/api/projects/${project}/purge`, {
        method: "DELETE",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirmation: "неверное название" }),
      });
      return response.status;
    },
    { project: projectId },
  );
  expect(rejected).toBe(422);
  await expect(page.getByRole("heading", { name: projectTitle })).toBeVisible();

  await page.getByRole("button", { name: `Действия: ${projectTitle}` }).click();
  await page.getByRole("menuitem", { name: "Удалить навсегда" }).click();
  const purgeDialog = page.getByRole("alertdialog", { name: "Удалить проект безвозвратно?" });
  await purgeDialog.getByRole("textbox", { name: "Название проекта" }).fill(projectTitle);
  await purgeDialog.getByRole("button", { name: "Удалить безвозвратно" }).click();
  await expect(page.getByRole("heading", { name: projectTitle })).toBeHidden();
  const afterPurge = await page.request.get(`/api/projects/${projectId}`);
  expect(afterPurge.status()).toBe(404);

  const session = await page.request.get("/api/auth/me");
  const email = ((await session.json()) as { user: { email: string } }).user.email;
  await page.goto("/settings");
  await page.getByRole("button", { name: "Удалить аккаунт" }).click();
  const accountDialog = page.getByRole("alertdialog", { name: "Удалить аккаунт безвозвратно?" });
  await accountDialog.getByRole("textbox", { name: "Адрес электронной почты" }).fill(email);
  await accountDialog.getByRole("button", { name: "Удалить аккаунт" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect.poll(async () => (await page.request.get("/api/auth/me")).status()).toBe(401);
  expect(await page.evaluate(() => localStorage.length)).toBe(0);
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

async function getNode(page: Page, nodeId: string) {
  return page.evaluate(async (id) => {
    const response = await fetch(`/api/nodes/${id}`, { credentials: "include" });
    return (await response.json()).node;
  }, nodeId);
}

async function getHistory(page: Page, nodeId: string) {
  return page.evaluate(async (id) => {
    const response = await fetch(`/api/nodes/${id}/versions`, { credentials: "include" });
    return response.json();
  }, nodeId);
}
