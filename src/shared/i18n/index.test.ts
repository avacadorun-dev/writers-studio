import { describe, expect, test } from "bun:test";

import { resolveInterfaceLanguage, t } from "./index";
import { messages } from "./messages";

describe("interface language", () => {
  test("prefers a saved selection to browser preferences", () => {
    expect(resolveInterfaceLanguage("ru", ["zh-CN", "en-US"])).toBe("ru");
  });

  test("normalizes regional browser languages and defaults to English", () => {
    expect(resolveInterfaceLanguage(null, ["de-DE", "zh-TW", "ru-RU"])).toBe("zh-Hans");
    expect(resolveInterfaceLanguage("unsupported", ["ru-UA"])).toBe("ru");
    expect(resolveInterfaceLanguage(null, ["fr-FR"])).toBe("en");
  });

  test("interpolates content without expanding placeholders inside it", () => {
    expect(t("Пишет в «{0}»", "Draft {0}")).toBe("Пишет в «Draft {0}»");
  });

  test("keeps interpolation markers in both translations", () => {
    const markers = (text: string) => [...text.matchAll(/\{\d+\}/g)].map(([match]) => match).sort();
    const mismatches = Object.entries(messages).flatMap(([source, translations]) =>
      translations.filter((translation) => markers(translation).join() !== markers(source).join()),
    );
    expect(mismatches).toEqual([]);
  });
});
