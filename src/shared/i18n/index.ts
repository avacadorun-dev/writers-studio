import { messages } from "./messages";

export const interfaceLanguages = ["en", "ru", "zh-Hans"] as const;
export type InterfaceLanguage = (typeof interfaceLanguages)[number];

const storageKey = "writers-studio.interface-language";

export function isInterfaceLanguage(value: unknown): value is InterfaceLanguage {
  return interfaceLanguages.some((language) => language === value);
}

export function resolveInterfaceLanguage(
  preference: unknown,
  browserLanguages: readonly string[],
): InterfaceLanguage {
  if (isInterfaceLanguage(preference)) return preference;
  for (const language of browserLanguages) {
    switch (language.toLowerCase().split("-")[0]) {
      case "en":
        return "en";
      case "ru":
        return "ru";
      case "zh":
        return "zh-Hans";
    }
  }
  return "en";
}

function readInterfaceLanguage(): InterfaceLanguage {
  if (typeof window === "undefined") return "en";
  let preference: string | null = null;
  try {
    preference = window.localStorage.getItem(storageKey);
  } catch {
    // The browser's language remains available when storage is disabled.
  }
  return resolveInterfaceLanguage(
    preference,
    navigator.languages?.length ? navigator.languages : [navigator.language],
  );
}

// Static labels and editor extensions use the same language for the lifetime of this page.
export const interfaceLanguage = readInterfaceLanguage();

export function saveInterfaceLanguage(language: InterfaceLanguage) {
  window.localStorage.setItem(storageKey, language);
}

export function localizeMessage(message: string): string {
  if (interfaceLanguage === "ru" || !Object.hasOwn(messages, message)) return message;
  return messages[message as keyof typeof messages][interfaceLanguage === "en" ? 0 : 1];
}

export function t(message: keyof typeof messages, ...values: unknown[]): string {
  const translated = localizeMessage(message);
  return values.length
    ? translated.replace(/\{(\d+)\}/g, (_, index) => String(values[Number(index)]))
    : translated;
}
