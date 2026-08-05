import { expect, test } from "@playwright/test";

test("real Deepgram live audio becomes the final draft without an automatic batch pass", async ({
  page,
}, testInfo) => {
  const consoleErrors: string[] = [];
  const failedRequests: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });
  page.on("requestfailed", (request) => {
    failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText}`);
  });

  const suffix = `real-stt-${Date.now()}`;
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await expect(page).toHaveURL(/\/projects$/);
  const capabilities = await page.evaluate(async () => {
    const response = await fetch("/api/stt/capabilities", { credentials: "include" });
    if (!response.ok) {
      throw new Error(`load STT capabilities: ${response.status}`);
    }
    return response.json();
  });
  expect(capabilities.modes).toEqual(expect.arrayContaining(["realtime", "file"]));
  expect(capabilities.streamingProtocol).toBe("pcm16");
  expect(capabilities.partialResults).toBe(true);
  expect(capabilities.vocabularyHints).toBe("keyterm");

  await page
    .getByRole("link", { name: /Новый проект|Создать проект/ })
    .first()
    .click();
  await page.getByRole("textbox", { name: "Рабочее название" }).fill("Проверка реального STT");
  await page.getByRole("button", { name: "Создать и открыть" }).click();

  await page.getByRole("banner").getByRole("button", { name: "Диктовать" }).click();
  await expect(page.getByRole("dialog", { name: "Всё готово к диктовке?" })).toBeVisible();
  const checkMicrophone = page.getByRole("button", { name: "Проверить" });
  if (await checkMicrophone.isVisible()) {
    await checkMicrophone.click();
    await expect(page.getByText("Доступ разрешён, сигнал поступает")).toBeVisible();
  }
  await page.getByRole("button", { name: "Начать запись" }).click();
  await expect(page).toHaveURL(/\/dictate\//);
  await expect(page.getByRole("button", { name: "Пауза" })).toBeVisible();

  await expect
    .poll(() => page.locator("body").innerText(), {
      message: "real recognized words should appear in the live transcript",
      timeout: 45_000,
    })
    .toMatch(/настоящ\S*\s+распознаван/i);
  await expect(page.locator("body")).not.toContainText("Дом стоял у самой воды");
  await expect(page.locator('[data-transcript-run="prose"]')).toHaveCount(1);
  await page.screenshot({
    path: `artifacts/${testInfo.project.name}-real-deepgram-live.png`,
    fullPage: true,
  });

  await page.getByRole("button", { name: "Завершить" }).click();
  await expect(page).toHaveURL(/\/dictations\/[^/]+\/(finalizing|review)/, {
    timeout: 45_000,
  });
  const pendingSessionID = new URL(page.url()).pathname.split("/")[2];
  for (let attempt = 0; attempt < 3 && !page.url().endsWith("/review"); attempt += 1) {
    let status = "";
    await expect
      .poll(
        async () => {
          status = await page.evaluate(async (id) => {
            const response = await fetch(`/api/dictations/${id}`, {
              credentials: "include",
            });
            const body = await response.json();
            return body.session.status as string;
          }, pendingSessionID);
          return status;
        },
        { timeout: 60_000 },
      )
      .toMatch(/review_ready|finalization_failed|missing_chunks/);
    if (status === "review_ready") {
      break;
    }
    await page.getByRole("button", { name: "Повторить обработку" }).click();
    await expect
      .poll(
        () =>
          page.evaluate(async (id) => {
            const response = await fetch(`/api/dictations/${id}`, {
              credentials: "include",
            });
            const body = await response.json();
            return body.session.status as string;
          }, pendingSessionID),
        { timeout: 15_000 },
      )
      .toMatch(/^(finalizing|review_ready)$/);
  }
  await expect(page).toHaveURL(/\/review$/, { timeout: 60_000 });
  const sessionID = new URL(page.url()).pathname.split("/")[2];
  expect(sessionID).toBeTruthy();

  const session = await page.evaluate(async (id) => {
    const response = await fetch(`/api/dictations/${id}`, { credentials: "include" });
    if (!response.ok) {
      throw new Error(`load dictation: ${response.status}`);
    }
    return response.json();
  }, sessionID);
  expect(session.session.liveArtifact.provider).toBe("deepgram");
  expect(session.session.liveArtifact.text).toMatch(/настоящ\S*\s+распознаван/i);
  expect(session.session.liveArtifact.text).not.toContain("Дом стоял у самой воды");
  expect(
    session.session.liveArtifact.segments.some(
      (segment: { startMs: number; endMs: number }) => segment.endMs > segment.startMs,
    ),
  ).toBe(true);
  expect(session.session.batchArtifact).toBeUndefined();
  expect(session.session.draftRevisions).toHaveLength(1);
  expect(session.session.draftRevisions[0].source).toBe("live_source");

  await page.screenshot({
    path: `artifacts/${testInfo.project.name}-real-deepgram-review.png`,
    fullPage: true,
  });
  expect(consoleErrors).toEqual([]);
  expect(failedRequests).toEqual([]);
});
