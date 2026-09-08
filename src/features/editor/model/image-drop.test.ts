import { describe, expect, test } from "bun:test";

import { isExternalFileDrag, maximumEditorImageBytes, validateEditorImageFile } from "./image-drop";

describe("editor image drop", () => {
  test("accepts supported images up to and including 12 MiB", () => {
    for (const type of ["image/png", "image/jpeg", "image/gif"]) {
      expect(validateEditorImageFile({ type, size: maximumEditorImageBytes })).toEqual({
        valid: true,
      });
    }
  });

  test("rejects unsupported, empty and oversized files with an author-facing reason", () => {
    expect(validateEditorImageFile({ type: "image/webp", size: 100 })).toEqual({
      valid: false,
      message: "Поддерживаются только PNG, JPEG и GIF.",
    });
    expect(validateEditorImageFile({ type: "image/png", size: 0 }).valid).toBeFalse();
    expect(
      validateEditorImageFile({ type: "image/png", size: maximumEditorImageBytes + 1 }),
    ).toEqual({
      valid: false,
      message: "Изображение должно быть не больше 12 МиБ.",
    });
  });

  test("distinguishes external files from text and internal editor drags", () => {
    expect(isExternalFileDrag({ types: ["Files", "text/plain"] })).toBeTrue();
    expect(
      isExternalFileDrag({ types: ["text/plain", "application/x-prosemirror-slice"] }),
    ).toBeFalse();
  });
});
