import { describe, expect, test } from "bun:test";

const tokenFile = "src/shared/styles/globals.css";
const auditFile = "src/shared/styles/theme-tokens.test.ts";
const extraThemeFiles = ["index.html", "public/icon.svg", "public/theme-init.js", "vite.config.ts"];
const selectableStyles = [
  "quiet",
  "cedar-and-copper",
  "dried-rose-and-wine",
  "olive-ink",
  "fog-graphite",
  "linen-clay",
  "dusty-lavender",
  "porcelain-ink",
] as const;

describe("theme tokens", () => {
  test("keeps every theme-dependent color paired across light and dark palettes", async () => {
    const css = await Bun.file(tokenFile).text();
    const light = readStyleBlocks(css, ":root").join("\n");
    const dark = readStyleBlocks(css, ':root[data-theme="dark"]').join("\n");
    const lightColors = readDeclarations(light).filter(([, value]) => isColor(value));
    const darkTokens = new Set(readDeclarations(dark).map(([name]) => name));

    expect(lightColors.length).toBeGreaterThan(20);
    for (const [name] of lightColors) {
      expect(darkTokens.has(name), `Missing dark value for ${name}`).toBe(true);
    }
  });

  test("declares every custom property used by the interface", async () => {
    const files = await sourceFiles();
    const contents = await Promise.all(files.map(async (file) => Bun.file(file).text()));
    const declarations = new Set(
      contents
        .flatMap((source) => [...source.matchAll(/(--[a-z0-9-]+)\s*:/g)])
        .map((match) => match[1]),
    );
    const usages = new Set(
      contents
        .flatMap((source) => [...source.matchAll(/var\((--[a-z0-9-]+)/g)])
        .map((match) => match[1]),
    );

    expect([...usages].filter((token) => !declarations.has(token))).toEqual([]);
  });

  test("keeps semantic text pairs at WCAG AA contrast in both themes", async () => {
    const css = await Bun.file(tokenFile).text();
    const palettes = readAllPalettes(css);
    const pairs = [
      ["--ink-primary", "--surface-app"],
      ["--ink-secondary", "--surface-paper"],
      ["--ink-tertiary", "--surface-paper"],
      ["--accent", "--surface-paper"],
      ["--ink-on-solid", "--accent-surface"],
      ["--ink-on-creative", "--creative-surface"],
      ["--accent-ink", "--accent-soft"],
      ["--creative-ink", "--creative-soft"],
      ["--ink-on-solid", "--record-surface"],
      ["--ink-on-solid", "--danger-surface"],
      ["--warning", "--warning-soft"],
      ["--success", "--success-soft"],
      ["--info", "--info-soft"],
      ["--danger", "--danger-soft"],
      ["--ink-on-inverse", "--surface-inverse"],
      ["--ink-on-inverse-muted", "--surface-inverse"],
    ] as const;

    for (const [theme, palette] of Object.entries(palettes)) {
      for (const [foreground, background] of pairs) {
        const ratio = contrastRatio(palette[foreground], palette[background]);
        expect(
          ratio,
          `${theme}: ${foreground} on ${background} has ${ratio.toFixed(2)}:1 contrast`,
        ).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  test("defines a complete light and dark identity for every selectable style", async () => {
    const css = await Bun.file(tokenFile).text();
    const requiredTokens = [
      "--surface-app",
      "--surface-panel",
      "--surface-paper",
      "--surface-raised",
      "--surface-subtle",
      "--surface-selected",
      "--ink-primary",
      "--ink-secondary",
      "--ink-tertiary",
      "--border-subtle",
      "--border-strong",
      "--accent",
      "--accent-hover",
      "--accent-surface",
      "--accent-surface-hover",
      "--accent-soft",
      "--accent-ink",
      "--creative",
      "--creative-hover",
      "--creative-surface",
      "--creative-surface-hover",
      "--creative-soft",
      "--creative-ink",
      "--record",
      "--record-surface",
      "--record-soft",
      "--warning",
      "--warning-soft",
      "--success",
      "--success-soft",
      "--info",
      "--info-soft",
      "--danger",
      "--danger-surface",
      "--danger-soft",
      "--focus",
      "--diff-insert",
      "--diff-delete",
    ];

    for (const style of selectableStyles) {
      for (const theme of ["light", "dark"] as const) {
        const declarations = new Set(
          readDeclarations(readStyleBlock(css, style, theme)).map(([name]) => name),
        );
        for (const token of requiredTokens) {
          expect(declarations.has(token), `${style}/${theme} is missing ${token}`).toBe(true);
        }
      }
    }
  });

  test("does not bypass tokens with raw interface colors", async () => {
    const files = [
      ...(await sourceFiles()).filter((file) => file !== tokenFile && file !== auditFile),
      ...extraThemeFiles,
    ];
    const rawColor =
      /#[\da-f]{3,8}\b|\b(?:rgb|hsl)a?\s*\(|\b(?:bg|text|border|ring|outline|shadow|fill|stroke)-(?:white|black|gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)(?:[-/]|(?=["'`\s]))/i;
    const violations: string[] = [];

    for (const file of files) {
      const lines = (await Bun.file(file).text()).split("\n");
      lines.forEach((line, index) => {
        if (rawColor.test(line)) {
          violations.push(`${file}:${index + 1}`);
        }
      });
    }

    expect(violations).toEqual([]);
  });
});

async function sourceFiles(): Promise<string[]> {
  return [...new Bun.Glob("src/**/*.{ts,tsx,css}").scanSync({ cwd: "." })].sort();
}

function readDeclarations(block: string): Array<[string, string]> {
  return [...block.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)].map((match) => [
    match[1],
    match[2].trim(),
  ]);
}

function readPalette(block: string): Record<string, string> {
  return Object.fromEntries(readDeclarations(block));
}

function readAllPalettes(css: string): Record<string, Record<string, string>> {
  const baseLight = readStylePalette(css, ":root");
  const baseDark = readStylePalette(css, ':root[data-theme="dark"]');
  const palettes: Record<string, Record<string, string>> = {
    "quiet/light": baseLight,
    "quiet/dark": baseDark,
  };
  for (const style of selectableStyles) {
    if (style === "quiet") {
      continue;
    }
    const lightOverrides = readStylePalette(css, `:root[data-style="${style}"]`);
    const darkOverrides = readStylePalette(css, `:root[data-style="${style}"][data-theme="dark"]`);
    palettes[`${style}/light`] = { ...baseLight, ...lightOverrides };
    palettes[`${style}/dark`] = { ...baseDark, ...lightOverrides, ...darkOverrides };
  }
  return palettes;
}

function readStyleBlock(css: string, style: string, theme: "light" | "dark"): string {
  if (style === "quiet") {
    return readStyleBlocks(css, theme === "light" ? ":root" : ':root[data-theme="dark"]').join(
      "\n",
    );
  }
  const selector =
    theme === "light"
      ? `:root[data-style="${style}"]`
      : `:root[data-style="${style}"][data-theme="dark"]`;
  const blocks = readStyleBlocks(css, selector);
  expect(blocks.length, `Missing palette for ${style}/${theme}`).toBeGreaterThan(0);
  return blocks.join("\n");
}

function readStylePalette(css: string, selector: string): Record<string, string> {
  return Object.assign({}, ...readStyleBlocks(css, selector).map(readPalette));
}

function readStyleBlocks(css: string, selector: string): string[] {
  const pattern = new RegExp(`${escapeRegExp(selector)}\\s*\\{([\\s\\S]*?)\\}`, "g");
  return [...css.matchAll(pattern)].map((match) => match[1]);
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function isColor(value: string): boolean {
  return /^(?:#[\da-f]{3,8}|(?:rgb|hsl)a?\()/i.test(value);
}

function contrastRatio(foreground: string, background: string): number {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

function relativeLuminance(hex: string): number {
  const channels = hex
    .slice(1)
    .match(/../g)
    ?.map((channel) => Number.parseInt(channel, 16) / 255)
    .map((channel) => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4));
  if (!channels || channels.length < 3) {
    throw new Error(`Expected an opaque hex color, received ${hex}`);
  }
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}
