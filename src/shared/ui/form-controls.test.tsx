import { describe, expect, test } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { Button } from "./button";
import { Field, Input } from "./field";

describe("shared form controls", () => {
  test("associates a field label and error with its input", () => {
    render(
      <Field label="Рабочее название" error="Укажите название">
        <Input />
      </Field>,
    );
    const input = screen.getByRole("textbox", { name: "Рабочее название" });
    expect(input.getAttribute("aria-describedby")).toBeTruthy();
    expect(screen.getByText("Укажите название").id).toBe(
      input.getAttribute("aria-describedby") ?? "",
    );
    cleanup();
  });

  test("supports keyboard activation without a pointer", async () => {
    let activations = 0;
    render(<Button onClick={() => (activations += 1)}>Сохранить</Button>);
    const button = screen.getByRole("button", { name: "Сохранить" });
    button.focus();
    await userEvent.keyboard("{Enter}");
    expect(activations).toBe(1);
    cleanup();
  });
});
