import { afterEach, describe, expect, mock, test } from "bun:test";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { EditorContent, useEditor } from "@tiptap/react";
import { useEffect } from "react";

import { editorExtensions } from "./extensions";

afterEach(() => {
  cleanup();
  mock.restore();
});

function CodeEditor({ onJSON }: { onJSON?: (value: unknown) => void }) {
  const editor = useEditor({
    extensions: editorExtensions,
    content: {
      type: "doc",
      attrs: { schemaVersion: 2 },
      content: [
        {
          type: "codeBlock",
          attrs: { language: "go" },
          content: [{ type: "text", text: 'package main\n\tprintln("hello")' }],
        },
      ],
    },
    immediatelyRender: false,
    onUpdate: ({ editor: current }) => onJSON?.(current.getJSON()),
  });
  useEffect(() => () => editor?.destroy(), [editor]);
  return <EditorContent editor={editor} />;
}

describe("code block editor UI", () => {
  test("highlights Go and exposes language selection plus lossless copy", async () => {
    const writeText = mock(async () => undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    let documentJSON: unknown;
    render(<CodeEditor onJSON={(value) => (documentJSON = value)} />);

    const language = await screen.findByLabelText("Язык блока кода");
    expect((language as HTMLSelectElement).value).toBe("go");
    expect(screen.getByRole("option", { name: "Go / Golang" })).toBeTruthy();
    expect(document.querySelector(".token.keyword")?.textContent).toBe("package");

    fireEvent.click(screen.getByRole("button", { name: "Скопировать код" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('package main\n\tprintln("hello")'));
    expect(screen.getByRole("button", { name: "Код скопирован" })).toBeTruthy();

    fireEvent.change(language, { target: { value: "python" } });
    await waitFor(() => expect((language as HTMLSelectElement).value).toBe("python"));
    expect(JSON.stringify(documentJSON)).toContain('"language":"python"');
  });

  test("pastes a complete fenced block as native code", async () => {
    let documentJSON: unknown;
    render(<CodeEditor onJSON={(value) => (documentJSON = value)} />);
    const editor = await waitFor(() => {
      const element = document.querySelector(".ProseMirror");
      expect(element).toBeTruthy();
      return element as HTMLElement;
    });
    fireEvent.paste(editor, {
      clipboardData: {
        getData: (format: string) =>
          format === "text/plain"
            ? '```go\npackage main  \n\nfunc main() {\n\tprintln("hello")\n}\n```'
            : "",
      },
    });
    await waitFor(() => {
      const encoded = JSON.stringify(documentJSON);
      expect(encoded).toContain('"type":"codeBlock"');
      expect(encoded).toContain('"language":"go"');
      expect(encoded).toContain("package main  ");
      expect(encoded).toContain("\\tprintln");
    });
  });

  test("keeps the copy control usable when clipboard access fails", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: mock(async () => Promise.reject(new Error("denied"))) },
    });
    render(<CodeEditor />);
    const copy = await screen.findByRole("button", { name: "Скопировать код" });
    fireEvent.click(copy);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Скопировать код" })).toBeTruthy(),
    );
  });
});
