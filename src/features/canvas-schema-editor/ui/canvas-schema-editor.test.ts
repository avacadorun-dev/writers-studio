import { expect, test } from "bun:test";

import { nextTypeId } from "./canvas-schema-editor";

test("nextTypeId keeps stable unique schema ids", () => {
  expect(nextTypeId("custom-card", new Set())).toBe("custom-card");
  expect(nextTypeId("custom-card", new Set(["custom-card", "custom-card-2"]))).toBe(
    "custom-card-3",
  );
});
