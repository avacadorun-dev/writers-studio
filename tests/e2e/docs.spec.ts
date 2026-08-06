import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test.describe("public documentation", () => {
  test("opens without a session, navigates, searches and returns to the app", async ({ page }) => {
    await page.goto("/docs");
    await expect(
      page.getByRole("heading", { name: "Добро пожаловать в Студию автора" }),
    ).toBeVisible();
    const isMobile = (page.viewportSize()?.width ?? 1440) < 768;
    if (isMobile) await page.locator('button[aria-label="Открыть оглавление"]:visible').click();
    const projectsLink = page.getByRole("link", { name: "Проекты и материалы", exact: true });
    await expect(projectsLink).toBeVisible();

    await projectsLink.click();
    await expect(page).toHaveURL(/\/docs\/projects$/);
    await expect(
      page.getByRole("heading", { name: "Проекты и материалы", level: 1 }),
    ).toBeVisible();
    await page.goBack();
    await expect(page).toHaveURL(/\/docs$/);

    await page.locator(isMobile ? "button[data-search]" : "button[data-search-full]").click();
    await page.getByRole("textbox", { name: "Поиск по документации" }).fill("микрофон");
    await expect(page.getByRole("dialog")).toContainText("Диктовка");

    await page.keyboard.press("Escape");
    await page.goto("/docs/example-project");
    await expect(
      page.getByRole("heading", { name: "Демонстрационный проект «Эхо Предела»" }),
    ).toBeVisible();
    const demoDownload = page.getByRole("link", { name: /Скачать «Эхо Предела»/ });
    await expect(demoDownload).toHaveAttribute(
      "href",
      "/docs/examples/echo-frontier-canvas-demo.wstudio",
    );
    const archive = await page.request.get("/docs/examples/echo-frontier-canvas-demo.wstudio");
    expect(archive.status()).toBe(200);
    expect((await archive.body()).byteLength).toBeGreaterThan(1_000_000);

    if (isMobile) await page.locator('button[aria-label="Открыть оглавление"]:visible').click();
    const returnToStudio = page.getByRole("link", { name: "Вернуться в Студию" });
    await expect(returnToStudio).toHaveAttribute("href", "/projects");
    await returnToStudio.click();
    await expect(page.locator('link[data-writers-studio-docs-styles="true"]')).toHaveCount(0);
    await expect(page).not.toHaveURL(/\/docs(?:\/|$)/);
  });

  test("has a friendly not-found page and no serious accessibility violations", async ({
    page,
  }) => {
    await page.goto("/docs/missing-material");
    await expect(
      page.getByRole("heading", { name: "Похоже, эта страница переехала" }),
    ).toBeVisible();
    await page.goto("/docs/dictation");
    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter((item) => item.impact === "critical" || item.impact === "serious"),
    ).toEqual([]);
  });
});
