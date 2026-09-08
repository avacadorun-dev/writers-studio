import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { useDismissiblePanel } from "./use-dismissible-panel";

afterEach(cleanup);

function Harness() {
  const panel = useDismissiblePanel<string>();
  return (
    <div>
      <span ref={panel.triggerRef}>
        <button type="button" onClick={() => (panel.isOpen ? panel.close() : panel.open("open"))}>
          Диагностика
        </button>
      </span>
      <button type="button">Снаружи</button>
      {panel.value ? (
        <div ref={panel.panelRef} role="dialog">
          <button type="button">Внутри</button>
        </div>
      ) : null}
    </div>
  );
}

describe("useDismissiblePanel", () => {
  test("toggles a panel from its trigger", () => {
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Диагностика" });

    fireEvent.click(trigger);
    expect(screen.queryByRole("dialog")).toBeTruthy();

    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("dismisses on an outside pointer event but stays open for panel interactions", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Диагностика" }));

    fireEvent.pointerDown(screen.getByRole("button", { name: "Внутри" }));
    expect(screen.queryByRole("dialog")).toBeTruthy();

    fireEvent.pointerDown(screen.getByRole("button", { name: "Снаружи" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("dismisses on Escape and removes listeners after closing", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Диагностика" }));
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
