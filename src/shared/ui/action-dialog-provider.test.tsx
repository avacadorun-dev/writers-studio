import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ActionDialogProvider, useActionDialog } from "./action-dialog-provider";
import { ModalDialog } from "./dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./dropdown-menu";

afterEach(cleanup);

const options = {
  title: "Новая доска",
  description: "Введите название доски.",
  label: "Название",
};

describe("ActionDialogProvider focus", () => {
  test("returns focus to the invoking control inside a parent modal", async () => {
    function Harness() {
      const { requestInput } = useActionDialog();
      return (
        <ModalDialog open onOpenChange={() => {}} title="Документы" description="Структура проекта">
          <button type="button" onClick={() => void requestInput(options)}>
            Создать доску
          </button>
        </ModalDialog>
      );
    }
    render(
      <ActionDialogProvider>
        <Harness />
      </ActionDialogProvider>,
    );
    const trigger = screen.getByRole("button", { name: "Создать доску" });
    await userEvent.click(trigger);
    await userEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", { name: "Отмена" }),
    );
    await waitFor(() => expect(document.activeElement === trigger).toBe(true));
  });

  test("returns focus to a menu trigger after its selected item has unmounted", async () => {
    function Harness() {
      const { requestInput } = useActionDialog();
      return (
        <DropdownMenu>
          <DropdownMenuTrigger>Действия</DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onSelect={() => void requestInput(options)}>
              Переименовать
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      );
    }
    render(
      <ActionDialogProvider>
        <Harness />
      </ActionDialogProvider>,
    );
    const trigger = screen.getByRole("button", { name: "Действия" });
    await userEvent.click(trigger);
    await userEvent.click(screen.getByRole("menuitem", { name: "Переименовать" }));
    await userEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", { name: "Отмена" }),
    );
    await waitFor(() => expect(document.activeElement === trigger).toBe(true));
  });

  test("keeps focus in the next dialog of a sequence and then returns to its original control", async () => {
    function Harness() {
      const { requestInput, requestSelect } = useActionDialog();
      return (
        <button
          type="button"
          onClick={() =>
            void requestSelect({
              title: "Выбор типа",
              description: "Выберите тип материала.",
              label: "Тип",
              options: [{ value: "chapter", label: "Глава" }],
              confirmLabel: "Продолжить",
            }).then((kind) => {
              if (kind) void requestInput(options);
            })
          }
        >
          Добавить материал
        </button>
      );
    }
    render(
      <ActionDialogProvider>
        <Harness />
      </ActionDialogProvider>,
    );
    const trigger = screen.getByRole("button", { name: "Добавить материал" });
    await userEvent.click(trigger);
    await userEvent.click(screen.getByRole("button", { name: "Продолжить" }));
    const nextDialog = screen.getByRole("alertdialog", { name: "Новая доска" });
    await waitFor(() => expect(nextDialog.contains(document.activeElement)).toBe(true));
    await userEvent.click(within(nextDialog).getByRole("button", { name: "Отмена" }));
    await waitFor(() => expect(document.activeElement === trigger).toBe(true));
  });
});
