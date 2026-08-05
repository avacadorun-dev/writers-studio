import { describe, expect, test } from "bun:test";

import { createClientId } from "./client-id";

describe("client ID generation", () => {
  test("uses native randomUUID when it is available", () => {
    const expected = "11111111-1111-4111-8111-111111111111";
    expect(createClientId({ randomUUID: () => expected })).toBe(expected);
  });

  test("creates an RFC 4122 v4 UUID on non-secure HTTP origins", () => {
    const id = createClientId({
      getRandomValues: (values) => {
        values.fill(0xab);
        return values;
      },
    });
    expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  test("keeps a last-resort UUID path for restricted webviews", () => {
    const id = createClientId({});
    expect(id).toHaveLength(36);
    expect(id[14]).toBe("4");
  });
});
