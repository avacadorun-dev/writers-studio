import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

test("assistant context, guide, memory and AI settings are separate visible destinations", async ({
  page,
}, testInfo) => {
  const suffix = `assistant-nav-${testInfo.project.name}-${Date.now()}`;
  const { projectId, nodeId } = await registerAndCreateProject(page, suffix);

  const contextLink = page.getByRole("link", {
    name: "Контекст, память и руководство ассистента",
  });
  await expect(contextLink).toBeVisible();
  await contextLink.click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/assistant-context$`));
  await expect(page.getByRole("heading", { name: "ASSISTANT.md" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Память проекта" })).toBeVisible();

  await page.getByRole("link", { name: "Открыть руководство" }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/assistant-guide$`));
  const guide = page.getByRole("textbox", { name: /Руководство проекта/ });
  await page.getByRole("button", { name: "Вставить структурированный шаблон" }).click();
  await expect(guide).toHaveValue(/# Голос и тон[\s\S]+# Канон и приоритет источников/);
  await guide.fill("# Локальный черновик\n\nОн должен пережить перезагрузку.");
  await expect(page.getByText("Несохранённый черновик защищён локально")).toBeVisible();
  await page.reload();
  await expect(guide).toHaveValue("# Локальный черновик\n\nОн должен пережить перезагрузку.");
  await expect(page.getByText("Несохранённый черновик защищён локально")).toBeVisible();
  await guide.fill("# Голос\n\nПиши сдержанно и сверяй имена со словарём.");
  await page.getByRole("button", { name: "Сохранить revision" }).click();
  await expect(page.getByText("Руководство сохранено")).toBeVisible();
  await expect(page.getByText("Несохранённый черновик защищён локально")).toBeHidden();

  await page.getByRole("link", { name: "Память проекта", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/memory$`));
  await page.getByRole("combobox", { name: "Тип", exact: true }).selectOption("character");
  await page
    .getByRole("textbox", { name: "Что нужно помнить" })
    .fill("Главная героиня боится глубокой воды.");
  await page.getByRole("combobox", { name: "Область действия" }).selectOption("node");
  await page.getByRole("combobox", { name: "Файл области" }).selectOption(nodeId);
  await page.getByRole("textbox", { name: "Теги" }).fill("героиня, канон, вода");
  await page.getByRole("group", { name: "Файлы-источники" }).getByRole("checkbox").first().check();
  await page.getByRole("checkbox", { name: /Всегда включать/ }).check();
  await page.getByRole("button", { name: "Добавить в память" }).click();
  await expect(page.getByText("Главная героиня боится глубокой воды.")).toBeVisible();
  await expect(page.getByText("канон", { exact: true })).toBeVisible();
  await expect(page.getByText(/Файлы-источники:/)).toBeVisible();

  const memorySearch = page.getByRole("textbox", { name: "Поиск по памяти проекта" });
  await memorySearch.fill("несуществующий факт");
  await expect(page.getByText("Ничего не найдено")).toBeVisible();
  await page.getByRole("button", { name: "Сбросить фильтры" }).click();
  const memoryCard = page
    .getByRole("article")
    .filter({ hasText: "Главная героиня боится глубокой воды." });
  await expect(memoryCard.locator(`a[href="/projects/${projectId}/studio/${nodeId}"]`)).toHaveCount(
    2,
  );
  await memoryCard.getByRole("button", { name: "Изменить" }).click();
  await page
    .getByRole("textbox", { name: "Что нужно помнить" })
    .fill("Главная героиня боится открытой глубокой воды.");
  await page.getByRole("button", { name: "Сохранить новую revision" }).click();
  const editedMemoryCard = page
    .getByRole("article")
    .filter({ hasText: "Главная героиня боится открытой глубокой воды." });
  await expect(editedMemoryCard).toBeVisible();
  await editedMemoryCard.getByRole("button", { name: "История" }).click();
  await expect(editedMemoryCard.getByText("r2", { exact: true })).toBeVisible();
  await expect(editedMemoryCard.getByText("r1", { exact: true })).toBeVisible();
  await editedMemoryCard.getByRole("button", { name: "Восстановить r1" }).click();
  await expect(page.getByText("Версия r1 восстановлена новой revision")).toBeVisible();
  const restoredMemoryCard = page
    .getByRole("article")
    .filter({ hasText: "Главная героиня боится глубокой воды." });
  await expect(restoredMemoryCard.getByText("r3", { exact: true }).first()).toBeVisible();
  await restoredMemoryCard.getByRole("button", { name: "Не использовать" }).click();
  await page.getByRole("combobox", { name: "Фильтр по состоянию памяти" }).selectOption("inactive");
  await expect(restoredMemoryCard.getByText("не используется")).toBeVisible();
  const memoryAccessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(memoryAccessibility.violations).toEqual([]);

  await page.goto(`/projects/${projectId}/studio/${nodeId}`);
  await page.getByRole("button", { name: "Ещё" }).click();
  await page.getByRole("menuitem", { name: "AI и провайдеры" }).click();
  await expect(page).toHaveURL(/\/settings\/assistant-providers$/);
  await expect(page.getByRole("heading", { name: "OpenAI-compatible подключения" })).toBeVisible();
  await expect(page.getByText(/последовательных строгих вызова инструментов работают/)).toBeVisible(
    {
      timeout: 30_000,
    },
  );
  await expect(page.getByRole("button", { name: "Добавить" })).toBeEnabled();
  await page.getByRole("button", { name: "Добавить" }).click();
  const providerForm = page.locator('form[autocomplete="off"]');
  const modelId = page.getByRole("textbox", { name: "Model ID" });
  const apiKey = page.getByLabel("API-ключ");
  await expect(providerForm).toHaveCount(1);
  await expect(modelId).toHaveAttribute("name", "provider-model-id");
  await expect(modelId).toHaveAttribute("autocomplete", "off");
  await expect(modelId).toHaveAttribute("data-1p-ignore", "true");
  await expect(apiKey).toHaveAttribute("name", "provider-api-key");
  await expect(apiKey).toHaveAttribute("autocomplete", "new-password");
  await expect(apiKey).toHaveAttribute("data-lpignore", "true");
  await expect(modelId).toHaveValue("");
  await expect(apiKey).toHaveValue("");

  const accessibility = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(accessibility.violations).toEqual([]);
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
