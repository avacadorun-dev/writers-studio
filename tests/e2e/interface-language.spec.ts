import { expect, test } from "@playwright/test";

test("uses the browser language and keeps an explicit choice across pages and reloads", async ({
  browser,
  baseURL,
}) => {
  const context = await browser.newContext({ baseURL, locale: "zh-CN" });
  try {
    const page = await context.newPage();
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("lang", "zh-Hans");
    await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();

    await page.getByLabel("界面语言").selectOption("en");
    await page.getByRole("button", { name: "应用语言", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();

    await page.getByLabel("Interface language").selectOption("ru");
    await page.getByRole("button", { name: "Apply language", exact: true }).click();
    await expect(page.getByRole("button", { name: "Войти", exact: true })).toBeVisible();
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("lang", "ru");

    await page.goto("/docs");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("lang", "ru");
  } finally {
    await context.close();
  }
});

test("localizes a known sign-in error without changing the server contract", async ({ page }) => {
  await page.route("**/api/auth/login", (route) =>
    route.fulfill({
      status: 401,
      json: { error: { code: "unauthorized", message: "Неверная почта или пароль" } },
    }),
  );
  await page.goto("/login");
  await page.getByLabel("Язык интерфейса").selectOption("en");
  await page.getByRole("button", { name: "Применить язык", exact: true }).click();
  await page.getByLabel("Email", { exact: true }).fill("author@example.com");
  await page.getByLabel("Password", { exact: true }).fill("incorrect-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByText("Invalid email or password", { exact: true })).toBeVisible();
});

test("changes the interface from Settings without changing the dictation default", async ({
  page,
}) => {
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ json: { user: { email: "author@example.com" } } }),
  );
  await page.route("**/api/preferences", (route) =>
    route.fulfill({ json: { preferences: { audioRetentionDays: 365 } } }),
  );

  await page.goto("/settings");
  await expect(page.getByLabel("Язык по умолчанию")).toHaveValue("ru");
  await page.getByLabel("Язык интерфейса").selectOption("zh-Hans");
  await page.getByRole("button", { name: "Применить язык", exact: true }).click();
  await expect(page.getByRole("heading", { name: "设置", exact: true })).toBeVisible();
  await expect(page.getByLabel("默认语言")).toHaveValue("ru");
});
