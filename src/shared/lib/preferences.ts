export type EditorFontFamily = "sofia" | "inter" | "source-serif";
export type EditorTheme = "light" | "dark" | "system";
export const editorStyleOptions = [
  { id: "quiet", label: "Тихая редакция" },
  { id: "cedar-and-copper", label: "Кедр и медь" },
  { id: "dried-rose-and-wine", label: "Сухая роза и вино" },
  { id: "olive-ink", label: "Олива и чернила" },
  { id: "fog-graphite", label: "Туман и графит" },
  { id: "linen-clay", label: "Лён и глина" },
  { id: "dusty-lavender", label: "Пыльная лаванда" },
  { id: "porcelain-ink", label: "Фарфор и чернила" },
] as const;
export type EditorStyle = (typeof editorStyleOptions)[number]["id"];
export type EditorFontSize = 17 | 19 | 21;
export type EditorLineHeight = 1.5 | 1.75 | 2;
export type EditorManuscriptWidth = 60 | 70 | 80;
export type AssistantSilenceCheckpointMs = 0 | 3000 | 5000 | 8000;

export interface WriterPreferences {
  language: "ru" | "en";
  commandPrefix: string;
  microphoneId: string;
  commandsEnabled: boolean;
  liveTranscript: boolean;
  autoPlaySource: boolean;
  assistantSilenceCheckpointMs: AssistantSilenceCheckpointMs;
  fontFamily: EditorFontFamily;
  fontSize: EditorFontSize;
  lineHeight: EditorLineHeight;
  manuscriptWidth: EditorManuscriptWidth;
  theme: EditorTheme;
  style: EditorStyle;
  spellcheck: boolean;
  wordGoal: number;
}

export const writerPreferencesStorageKey = "writers-studio-preferences";
export const writerPreferencesChangedEvent = "writers-studio-preferences-changed";

const editorFontVariables: Record<EditorFontFamily, string> = {
  sofia: "var(--font-manuscript-sofia)",
  inter: "var(--font-manuscript-inter)",
  "source-serif": "var(--font-manuscript-source-serif)",
};

export const defaultWriterPreferences: WriterPreferences = {
  language: "ru",
  commandPrefix: "Команда",
  microphoneId: "",
  commandsEnabled: true,
  liveTranscript: true,
  autoPlaySource: false,
  assistantSilenceCheckpointMs: 5_000,
  fontFamily: "sofia",
  fontSize: 19,
  lineHeight: 1.75,
  manuscriptWidth: 70,
  theme: "system",
  style: "quiet",
  spellcheck: true,
  wordGoal: 0,
};

export function loadWriterPreferences(): WriterPreferences {
  try {
    const stored = JSON.parse(localStorage.getItem(writerPreferencesStorageKey) ?? "{}");
    return normalizeWriterPreferences(stored);
  } catch {
    return { ...defaultWriterPreferences };
  }
}

export function saveWriterPreferences(preferences: WriterPreferences): WriterPreferences {
  const normalized = normalizeWriterPreferences(preferences);
  localStorage.setItem(writerPreferencesStorageKey, JSON.stringify(normalized));
  applyWriterPreferences(normalized);
  window.dispatchEvent(
    new CustomEvent<WriterPreferences>(writerPreferencesChangedEvent, { detail: normalized }),
  );
  return normalized;
}

export function applyWriterPreferences(preferences = loadWriterPreferences()): void {
  const root = document.documentElement;
  const dark =
    preferences.theme === "dark" ||
    (preferences.theme === "system" &&
      window.matchMedia?.("(prefers-color-scheme: dark)").matches === true);
  root.dataset.theme = dark ? "dark" : "light";
  root.dataset.style = preferences.style;
  root.style.setProperty("--editor-font-family", editorFontVariables[preferences.fontFamily]);
  root.style.setProperty(
    "--editor-font-features",
    preferences.fontFamily === "sofia" ? '"locl" 0, "liga" 0' : '"liga" 0',
  );
  root.style.setProperty("--editor-font-size", `${preferences.fontSize}px`);
  root.style.setProperty("--editor-line-height", String(preferences.lineHeight));
  root.style.setProperty("--editor-max-width", `${preferences.manuscriptWidth}ch`);
  syncBrowserThemeColor();
}

export function syncBrowserThemeColor(): void {
  const themeColor = getComputedStyle(document.documentElement)
    .getPropertyValue("--surface-app")
    .trim();
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  if (meta && themeColor) {
    meta.content = themeColor;
  }
}

export function normalizeWriterPreferences(value: unknown): WriterPreferences {
  if (!isRecord(value)) {
    return { ...defaultWriterPreferences };
  }
  const commandPrefix =
    typeof value.commandPrefix === "string" && value.commandPrefix.trim()
      ? value.commandPrefix.trim().slice(0, 40)
      : defaultWriterPreferences.commandPrefix;
  return {
    language: isChoice(value.language, ["ru", "en"]) ? value.language : "ru",
    commandPrefix,
    microphoneId:
      typeof value.microphoneId === "string" ? value.microphoneId.trim().slice(0, 512) : "",
    commandsEnabled: readBoolean(value.commandsEnabled, true),
    liveTranscript: readBoolean(value.liveTranscript, true),
    autoPlaySource: readBoolean(value.autoPlaySource, false),
    assistantSilenceCheckpointMs: isChoice(
      value.assistantSilenceCheckpointMs,
      [0, 3000, 5000, 8000],
    )
      ? value.assistantSilenceCheckpointMs
      : 5_000,
    fontFamily: normalizeEditorFontFamily(value.fontFamily),
    fontSize: isChoice(value.fontSize, [17, 19, 21]) ? value.fontSize : 19,
    lineHeight: isChoice(value.lineHeight, [1.5, 1.75, 2]) ? value.lineHeight : 1.75,
    manuscriptWidth: isChoice(value.manuscriptWidth, [60, 70, 80]) ? value.manuscriptWidth : 70,
    theme: isChoice(value.theme, ["light", "dark", "system"]) ? value.theme : "system",
    style: normalizeEditorStyle(value.style),
    spellcheck: readBoolean(value.spellcheck, true),
    wordGoal: normalizeWordGoal(value.wordGoal),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isChoice<const T>(value: unknown, choices: readonly T[]): value is T {
  return choices.includes(value as T);
}

function isEditorStyle(value: unknown): value is EditorStyle {
  return editorStyleOptions.some((option) => option.id === value);
}

function normalizeEditorStyle(value: unknown): EditorStyle {
  if (value === "pine-copper") {
    return "cedar-and-copper";
  }
  if (value === "moss-wine") {
    return "dried-rose-and-wine";
  }
  return isEditorStyle(value) ? value : defaultWriterPreferences.style;
}

function readBoolean(value: unknown, fallback: boolean): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function normalizeEditorFontFamily(value: unknown): EditorFontFamily {
  if (value === "inter" || value === "sans") {
    return "inter";
  }
  if (value === "source-serif" || value === "serif" || value === "literata") {
    return "source-serif";
  }
  return "sofia";
}

function normalizeWordGoal(value: unknown): number {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return 0;
  }
  return Math.min(1_000_000, Math.max(0, Math.round(value)));
}
