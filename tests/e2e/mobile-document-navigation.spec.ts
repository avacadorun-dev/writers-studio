import { expect, type Page, test } from "@playwright/test";

test("compact Studio navigation switches documents, Canvas and project tools", async ({
  page,
}, testInfo) => {
  test.skip((page.viewportSize()?.width ?? 0) >= 1024, "Compact navigation below 1024px");
  const { projectId, nodeId } = await registerAndCreateProject(
    page,
    `mobile-documents-${testInfo.project.name}-${Date.now()}`,
  );
  const initialResponse = await page.request.get(`/api/nodes/${nodeId}`);
  const initial = (await initialResponse.json()) as { node: { title: string } };
  const chapter = await createNode(page, projectId, { kind: "chapter", title: "Вторая глава" });
  const nested = await createNode(page, projectId, {
    kind: "draft",
    title: "Заметка внутри главы",
    parentId: chapter.id,
  });
  const canvasResponse = await page.request.post(`/api/projects/${projectId}/canvases`, {
    data: { title: "Карта сюжета" },
  });
  expect(canvasResponse.status()).toBe(201);
  const { canvas } = (await canvasResponse.json()) as { canvas: { id: string } };

  // A desktop collapse preference must never make phone navigation inaccessible.
  await page.evaluate(() => localStorage.setItem("writers-studio-tree-collapsed", "true"));
  await page.reload();
  const trigger = page.getByRole("button", { name: "Документы", exact: true });
  const drawer = page.getByRole("dialog", { name: "Документы проекта" });
  await expect(trigger).toBeVisible();
  await trigger.click();
  await expect(drawer).toBeVisible();
  await expect(drawer.getByRole("link", { name: initial.node.title, exact: true })).toBeVisible();
  await drawer.getByRole("button", { name: "Развернуть «Вторая глава»" }).click();
  await drawer.getByRole("link", { name: "Заметка внутри главы", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/studio/${nested.id}$`));
  await expect(drawer).not.toBeVisible();
  await expect(page.getByRole("region", { name: "Редактор документа" })).toBeVisible();

  await trigger.click();
  await expect(drawer.getByRole("treeitem", { name: /Заметка внутри главы/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Escape");
  await expect(drawer).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await drawer.getByRole("button", { name: "Действия с «Заметка внутри главы»" }).click();
  await page.getByRole("menuitem", { name: "Переименовать", exact: true }).click();
  const rename = drawer.getByRole("textbox", { name: "Новое название для «Заметка внутри главы»" });
  await rename.fill("Заметка о финале");
  await rename.press("Enter");
  await expect(drawer.getByRole("link", { name: "Заметка о финале", exact: true })).toBeVisible();
  await drawer.getByRole("button", { name: "Закрыть список документов" }).click();
  await expect(drawer).not.toBeVisible();
  await expect(trigger).toBeFocused();

  await trigger.click();
  await drawer.getByRole("link", { name: "Словарь", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/dictionary$`));
  await expect(drawer).not.toBeVisible();
  await trigger.click();
  await drawer.getByRole("link", { name: "Карта сюжета", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/canvases/${canvas.id}$`));
  await expect(drawer).not.toBeVisible();
  await trigger.click();
  await drawer.getByRole("link", { name: initial.node.title, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/studio/${nodeId}$`));

  await page.setViewportSize({ width: 320, height: 740 });
  await expect(trigger).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(
    false,
  );
  await trigger.click();
  const drawerBounds = await drawer.boundingBox();
  expect(drawerBounds).not.toBeNull();
  expect((drawerBounds?.x ?? 0) + (drawerBounds?.width ?? 0)).toBeLessThanOrEqual(320);
  // The visible strip outside the drawer remains a dismiss target.
  await page.mouse.click(315, 370);
  await expect(drawer).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

async function createNode(
  page: Page,
  projectId: string,
  data: { kind: string; title: string; parentId?: string },
): Promise<{ id: string }> {
  const response = await page.request.post(`/api/projects/${projectId}/nodes`, { data });
  expect(response.status()).toBe(201);
  const result = (await response.json()) as { node: { id: string } };
  return result.node;
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
