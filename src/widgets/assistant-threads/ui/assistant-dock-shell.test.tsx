import { describe, expect, test } from "bun:test";
import { fireEvent, render } from "@testing-library/react";

import { AssistantDockShell } from "./assistant-dock-shell";

describe("AssistantDockShell", () => {
  test("uses one responsive dock surface and isolates wheel events", () => {
    let parentWheels = 0;
    const view = render(
      <div onWheel={() => (parentWheels += 1)}>
        <AssistantDockShell>
          <p>Лента</p>
        </AssistantDockShell>
      </div>,
    );
    const dock = view.getByTestId("assistant-dock");
    expect(dock.className).toContain("md:w-[380px]");
    expect(dock.className).toContain("xl:static");
    expect(dock.className).not.toContain("rounded");
    fireEvent.wheel(dock);
    expect(parentWheels).toBe(0);
  });
});
