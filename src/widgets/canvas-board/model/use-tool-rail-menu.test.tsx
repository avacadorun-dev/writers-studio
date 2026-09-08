import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { useToolRailMenu } from "./use-tool-rail-menu";

afterEach(cleanup);

function Harness() {
  const menu = useToolRailMenu<"create" | "draw">();
  return (
    <>
      <div ref={menu.railRef}>
        <button type="button" onClick={() => menu.toggleMenu("create")}>
          Создание
        </button>
        <button type="button" onClick={() => menu.toggleMenu("draw")}>
          Рисование
        </button>
        {menu.openMenu ? <div>{menu.openMenu}</div> : null}
      </div>
      <button type="button">Холст</button>
    </>
  );
}

describe("useToolRailMenu", () => {
  test("keeps only one group open and toggles the active group", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Создание" }));
    expect(screen.getByText("create")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Рисование" }));
    expect(screen.queryByText("create")).toBeNull();
    expect(screen.getByText("draw")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Рисование" }));
    expect(screen.queryByText("draw")).toBeNull();
  });

  test("dismisses the flyout outside and with Escape", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Создание" }));
    fireEvent.pointerDown(screen.getByRole("button", { name: "Холст" }));
    expect(screen.queryByText("create")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Создание" }));
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByText("create")).toBeNull();
  });
});
