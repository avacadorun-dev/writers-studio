import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

test("P-04 quick action completes the compact capture and basic review flow", async ({
  page,
}, testInfo) => {
  const suffix = `p04-${testInfo.project.name}-${Date.now()}`;
  const { projectId, projectTitle } = await registerAndCreateProject(page, suffix);
  await page.goto("/projects");
  if ((page.viewportSize()?.width ?? 1_000) < 640) {
    await page.getByRole("link", { name: `Диктовать в проект ${projectTitle}` }).click();
  } else {
    await page.getByRole("heading", { name: projectTitle }).click();
    await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  }

  await expect(page.getByRole("dialog", { name: "Подготовка диктовки" })).toBeVisible();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/dictate/`));
  await expect(page.getByText(/Дом стоял у самой воды/)).toBeVisible({ timeout: 20_000 });
  await expect(page.getByTestId("dictation-command-panel")).toContainText(
    "голосом с префиксом «Команда» или кнопкой",
  );
  await page.getByRole("button", { name: "Новый абзац" }).click();
  await expect(page.getByText("Выполнено: новый абзац", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Отменить команду" }).click();
  await expect(page.getByText("Предыдущая команда отменена", { exact: true })).toBeVisible();
  for (const name of ["Команды", "Метка", "Пауза", "Завершить"]) {
    const bounds = await page.getByRole("button", { name }).boundingBox();
    expect(bounds).toBeTruthy();
    expect((bounds?.x ?? 0) + (bounds?.width ?? 0)).toBeLessThanOrEqual(
      page.viewportSize()?.width ?? 10_000,
    );
  }
  await page.getByRole("button", { name: "Метка" }).click();
  await page.getByRole("button", { name: "Завершить" }).click();
  await expect(page).toHaveURL(/\/dictations\/[^/]+\/review$/, { timeout: 30_000 });
  await expect(page.getByText(/Голосовая сессия ·/)).toBeVisible();
  await expect(page.locator(".tiptap")).toContainText("Дом стоял у самой воды");

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({
    path: `artifacts/${testInfo.project.name}-p04-review.png`,
    fullPage: true,
  });
});

async function registerAndCreateProject(
  page: Page,
  suffix: string,
): Promise<{ projectId: string; projectTitle: string }> {
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
  const projectId = new URL(page.url()).pathname.split("/")[2] ?? "";
  return { projectId, projectTitle };
}
