import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";

import { AssistantMarkdown, sanitizeAssistantMarkdown } from "./assistant-markdown";

afterEach(cleanup);

describe("AssistantMarkdown", () => {
  test("renders useful Markdown and removes hidden provider reasoning", () => {
    render(
      <AssistantMarkdown>
        {`<think>private chain of thought</think>
# Результат

- Первый шаг
- Второй шаг

| Поле | Значение |
| --- | --- |
| Статус | Готово |

\`inline\`

\`\`\`ts
const ready = true;
\`\`\`

[Документация](https://example.com)

<script>window.compromised = true</script>`}
      </AssistantMarkdown>,
    );

    expect(screen.queryByText("private chain of thought")).toBeNull();
    expect(screen.getByRole("heading", { name: "Результат" })).toBeTruthy();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByRole("table")).toBeTruthy();
    expect(screen.getByText("const ready = true;")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Документация" }).getAttribute("href")).toBe(
      "https://example.com",
    );
    expect(screen.queryByText("window.compromised = true")).toBeNull();
  });

  test("drops complete, unclosed and orphan reasoning tags", () => {
    expect(
      sanitizeAssistantMarkdown(
        "Ответ.\n\n<analysis>секрет</analysis>\n\nДальше.\n<think>незакрытый секрет",
      ),
    ).toBe("Ответ.\n\nДальше.");
    expect(sanitizeAssistantMarkdown("</think>\nГотово")).toBe("Готово");
  });

  test("does not expose an executable Markdown URL", () => {
    const { container } = render(
      <AssistantMarkdown>{"[опасная ссылка](javascript:alert(1))"}</AssistantMarkdown>,
    );
    const href = container.querySelector("a")?.getAttribute("href") ?? "";
    expect(href.startsWith("javascript:")).toBeFalse();
  });

  test("renders the complete author-facing typography set", () => {
    const { container } = render(
      <AssistantMarkdown>
        {`## Раздел

### Деталь

1. Первый
2. Второй

> **Важно:** проверьте предложение.

---`}
      </AssistantMarkdown>,
    );
    expect(screen.getByRole("heading", { level: 2, name: "Раздел" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 3, name: "Деталь" })).toBeTruthy();
    expect(screen.getByRole("list")).toBeTruthy();
    expect(container.querySelector("blockquote")).toBeTruthy();
    expect(container.querySelector("strong")?.textContent).toBe("Важно:");
    expect(container.querySelector("hr")).toBeTruthy();
  });
});
