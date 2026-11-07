import { expect, type Page, test } from "@playwright/test";

test("library keeps creation above the scrolling projects and preserves filters", async ({
  page,
}, testInfo) => {
  const suffix = `library-layout-${testInfo.project.name}-${Date.now()}`;
  const registration = await page.request.post("/api/auth/register", {
    data: { email: `${suffix}@example.test`, password: "writers-studio" },
  });
  expect(registration.ok()).toBe(true);

  for (let index = 0; index < 24; index += 1) {
    await createProject(
      page,
      index === 0
        ? "Полярная экспедиция — история возвращения к самому длинному дню"
        : `Рукопись ${index + 1}`,
      index === 0 ? "Письма и заметки о северном путешествии." : "Рабочие заметки автора.",
    );
  }
  const archived = await createProject(page, "Сохранённая история");
  const trashed = await createProject(page, "Удалённая идея");
  for (const [projectId, status] of [
    [archived, "archived"],
    [trashed, "trash"],
  ]) {
    const response = await page.request.patch(`/api/projects/${projectId}/status`, {
      data: { status },
    });
    expect(response.ok()).toBe(true);
  }

  await page.goto("/projects");
  await expect(page.getByRole("heading", { name: "Ваши проекты" })).toBeVisible();
  await page.getByRole("button", { name: "Все", exact: true }).click();
  const projects = page.getByRole("main", { name: "Проекты", exact: true });
  const list = page.getByRole("region", { name: "Список проектов", exact: true });
  const cards = list.locator("article");
  await expect(cards).toHaveCount(24);

  const newProject = page.getByRole("link", { name: "Новый проект", exact: true }).first();
  const createCard = list.getByRole("link", { name: /Новый проект/ });
  await expect(newProject).toBeInViewport();
  await expect(createCard).toBeInViewport();
  const createBounds = await createCard.boundingBox();
  const firstBounds = await cards.first().boundingBox();
  expect(createBounds).not.toBeNull();
  expect(firstBounds).not.toBeNull();
  const firstByRow =
    (createBounds?.y ?? 0) + (createBounds?.height ?? 0) <= (firstBounds?.y ?? 0) + 1;
  const firstInSameRow =
    Math.abs((createBounds?.y ?? 0) - (firstBounds?.y ?? 0)) <= 1 &&
    (createBounds?.x ?? 0) < (firstBounds?.x ?? 0);
  expect(firstByRow || firstInSameRow).toBe(true);

  const buttonBeforeScroll = await newProject.boundingBox();
  await projects.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect.poll(() => projects.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(newProject).toBeInViewport();
  expect(await newProject.boundingBox()).toEqual(buttonBeforeScroll);
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
  expect(
    await page.evaluate(() => document.documentElement.scrollHeight > window.innerHeight),
  ).toBe(false);

  // The creation action remains usable after reaching the bottom of a long library.
  await newProject.click();
  await expect(page).toHaveURL(/\/projects\/new$/);
  const setup = page.getByRole("main", { name: "Создание проекта", exact: true });
  const submit = page.getByRole("button", { name: "Создать и открыть", exact: true });
  await expect(submit).toBeInViewport();
  const submitBeforeScroll = await submit.boundingBox();
  await setup.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect(submit).toBeInViewport();
  expect(await submit.boundingBox()).toEqual(submitBeforeScroll);
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
  expect(
    await page.evaluate(() => document.documentElement.scrollHeight > window.innerHeight),
  ).toBe(false);
  await page.getByRole("textbox", { name: "Рабочее название", exact: true }).fill("Новая история");
  await submit.click();
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/);
  await expect(page.getByRole("region", { name: "Редактор документа" })).toBeVisible();
  await page.goto("/projects");
  await expect(page.getByRole("heading", { name: "Ваши проекты" })).toBeVisible();

  await page.setViewportSize({ width: 320, height: 740 });
  await expect(newProject).toBeInViewport();
  await expect(
    page.getByRole("button", { name: "Проект Writers Studio", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Markdown / Obsidian", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(
    false,
  );

  await page.getByRole("button", { name: "Все", exact: true }).click();
  const search = page.getByRole("searchbox", { name: "Поиск проектов", exact: true });
  await search.fill("полярная");
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("Полярная экспедиция");
  await search.fill("");
  await page.getByRole("button", { name: "Архив", exact: true }).click();
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("Сохранённая история");
  await page.getByRole("button", { name: "Корзина", exact: true }).click();
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("Удалённая идея");
  await expect(newProject).toBeInViewport();
});

async function createProject(page: Page, title: string, synopsis = ""): Promise<string> {
  const response = await page.request.post("/api/projects", {
    data: { title, synopsis, kind: "free", language: "ru" },
  });
  expect(response.ok()).toBe(true);
  const result = (await response.json()) as { project: { id: string } };
  return result.project.id;
}
