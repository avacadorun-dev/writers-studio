import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ToastProvider, useToast } from "./toast-provider";

afterEach(cleanup);

describe("ToastProvider", () => {
  test("centers notifications above the bottom edge", async () => {
    function Harness() {
      const { showToast } = useToast();
      return (
        <button type="button" onClick={() => showToast({ title: "Доска сохранена" })}>
          Показать
        </button>
      );
    }

    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Показать" }));

    expect(screen.getByText("Доска сохранена")).toBeTruthy();
    const viewport = screen.getByTestId("toast-viewport");
    expect(viewport.className).toContain("left-1/2");
    expect(viewport.className).toContain("-translate-x-1/2");
    expect(viewport.className).not.toContain("right-4");
  });
});
