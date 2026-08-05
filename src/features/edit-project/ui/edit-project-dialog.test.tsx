import { afterEach, describe, expect, mock, test } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import type { Project } from "~entities/project";
import { ToastProvider } from "~shared/ui";

import { EditProjectDialog } from "./edit-project-dialog";

afterEach(() => {
  cleanup();
  mock.restore();
});

describe("EditProjectDialog", () => {
  test("saves renamed project metadata and closes after success", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const onOpenChange = mock(() => undefined);
    let requestBody: Record<string, unknown> | undefined;
    globalThis.fetch = mock(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requestBody = JSON.parse(String(init?.body)) as Record<string, unknown>;
      return Response.json({
        project: {
          ...project,
          ...requestBody,
          updatedAt: "2026-08-05T00:00:00Z",
        },
      });
    }) as unknown as typeof fetch;

    render(
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <EditProjectDialog project={project} open onOpenChange={onOpenChange} />
        </ToastProvider>
      </QueryClientProvider>,
    );

    fireEvent.change(screen.getByLabelText("Название проекта"), {
      target: { value: "Новое название" },
    });
    fireEvent.change(screen.getByLabelText("Коротко о замысле"), {
      target: { value: "Обновлённый замысел" },
    });
    fireEvent.change(screen.getByLabelText("Язык диктовки"), {
      target: { value: "en" },
    });
    fireEvent.change(screen.getByLabelText("Цель по словам"), {
      target: { value: "90000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить изменения" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(requestBody).toEqual({
      title: "Новое название",
      language: "en",
      synopsis: "Обновлённый замысел",
      wordGoal: 90_000,
    });
    expect(screen.getByText("Проект обновлён")).toBeTruthy();
  });
});

const project: Project = {
  id: "project-1",
  title: "Старое название",
  kind: "novel",
  language: "ru",
  synopsis: "Старый замысел",
  wordGoal: 80_000,
  status: "active",
  structureRevision: 1,
  structure: {
    schemaVersion: 1,
    template: "novel",
    rootLabel: "Рукопись",
    nodeTypes: [],
  },
  createdAt: "2026-08-01T00:00:00Z",
  updatedAt: "2026-08-01T00:00:00Z",
};
