import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { AssistantMessage, classifyAssistantMessage } from "./assistant-message";

afterEach(cleanup);

describe("AssistantMessage", () => {
  test("formats valid JSON, collapses long payloads and copies the exact source", async () => {
    const source = JSON.stringify({ items: Array.from({ length: 40 }, (_, index) => ({ index })) });
    let copied = "";
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: async (value: string) => (copied = value) },
    });
    render(<AssistantMessage>{source}</AssistantMessage>);

    const surface = screen.getByRole("region", { name: "Структурированные данные" });
    expect(surface.textContent).toContain('"items"');
    expect(screen.getByRole("button", { name: "Показать полностью" })).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Скопировать исходный JSON" }));
    expect(copied).toBe(source);
    await userEvent.click(screen.getByRole("button", { name: "Показать полностью" }));
    expect(screen.getByRole("button", { name: "Свернуть" }).getAttribute("aria-expanded")).toBe(
      "true",
    );
  });

  test("preserves plain line breaks, wraps long tokens and does not interpret HTML", () => {
    const source = `<img src=x onerror=alert(1)>\n${"a".repeat(240)}`;
    const { container } = render(<AssistantMessage>{source}</AssistantMessage>);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/<img src=x/u).className).toContain("whitespace-pre-wrap");
    expect(screen.getByText(/<img src=x/u).className).toContain("[overflow-wrap:anywhere]");
  });

  test("routes fenced code through the keyboard-scrollable Markdown surface", () => {
    const { container } = render(
      <AssistantMessage>{'```json\n{"ok":true}\n```'}</AssistantMessage>,
    );
    expect(container.querySelector("pre")?.closest("section")?.getAttribute("tabindex")).toBe("0");
  });

  test("keeps malformed JSON as inert plain text", () => {
    expect(classifyAssistantMessage('{"broken":}').kind).toBe("plain");
  });
});
