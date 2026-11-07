import { mkdir, readdir, rename } from "node:fs/promises";
import { resolve } from "node:path";
import { chromium } from "@playwright/test";

import { defaultWriterPreferences } from "../src/shared/lib/preferences";

// Browser-only synthetic fixtures: no database, credentials, recording or provider calls.
const origin = "http://127.0.0.1:4173";
const output = resolve("public/docs/images");
const timestamp = "2026-09-08T12:00:00Z";
const projectId = "00000000-0000-4000-8000-000000000001";
const nodeId = "00000000-0000-4000-8000-000000000002";
const paragraphs = [
  "Chapter One. A light in the bay",
  "By evening the sea was as still as dark glass. Lina climbed the lighthouse stairs and found an unmarked letter on the windowsill.",
  "Light the beacon seven minutes early, it said in a familiar hand.",
  "She checked the logbook, marked the time in the margin, and made a note to find out who had watched the station last autumn.",
];
const content = {
  type: "doc",
  content: paragraphs.map((text, index) => ({
    type: "paragraph",
    attrs: { blockId: `demo-block-${index}` },
    content: [{ type: "text", text }],
  })),
};
const project = {
  id: projectId,
  title: "The North Lighthouse",
  kind: "novel",
  language: "en",
  synopsis:
    "A fictional keeper discovers letters from the future. Synthetic documentation example.",
  status: "active",
  structureRevision: 1,
  structure: {
    schemaVersion: 1,
    template: "novel",
    rootLabel: "Manuscript",
    nodeTypes: [
      {
        key: "chapter",
        label: "Chapter",
        pluralLabel: "Chapters",
        icon: "BookOpen",
        contentBearing: true,
        allowedChildren: ["chapter"],
        canBeRoot: true,
      },
    ],
  },
  createdAt: timestamp,
  updatedAt: timestamp,
};
const node = {
  id: nodeId,
  projectId,
  kind: "chapter",
  title: "Chapter One",
  status: "draft",
  position: 1000,
  currentRevision: 2,
  content,
  plainText: paragraphs.join("\n"),
  createdAt: timestamp,
  updatedAt: timestamp,
};
const profile = {
  schemaVersion: 1,
  metadata: { title: project.title, subtitle: "", author: "", annotation: project.synopsis },
  style: {
    preset: "book",
    fontFamily: "literata",
    fontSizePt: 11,
    lineHeight: 1.5,
    paragraphMode: "indent",
    firstLineIndentEm: 1.5,
    paragraphSpaceEm: 0,
    textAlign: "justify",
    pageSize: "a5",
    marginMm: 20,
    sceneSeparator: "asterisks",
  },
  titlePage: true,
  tableOfContents: true,
  chapterTitles: true,
  chapterPageBreak: true,
  includeNotes: false,
  lastFormat: "docx",
};
const revision = {
  id: "demo-revision-2",
  nodeId,
  revision: 2,
  baseRevision: 1,
  schemaVersion: 1,
  content,
  plainText: node.plainText,
  source: "manual",
  createdAt: timestamp,
};
const responses: Record<string, unknown> = {
  "/api/auth/me": {
    user: {
      id: "demo-user",
      email: "demo@example.test",
      displayName: "Demo",
      createdAt: timestamp,
    },
  },
  "/api/projects": { projects: [project] },
  [`/api/projects/${projectId}/outline`]: { project, nodes: [node] },
  [`/api/nodes/${nodeId}`]: { node },
  [`/api/nodes/${nodeId}/annotations`]: { annotations: [] },
  [`/api/nodes/${nodeId}/versions`]: {
    versions: [],
    revisions: [
      revision,
      {
        ...revision,
        id: "demo-revision-1",
        revision: 1,
        baseRevision: 0,
        source: "project_create",
        content: { type: "doc", content: [{ type: "paragraph" }] },
        plainText: "",
      },
    ],
  },
  [`/api/projects/${projectId}/dictionary`]: {
    terms: [
      {
        id: "demo-term",
        projectId,
        canonical: "Lina Vale",
        variants: ["Lina"],
        kind: "character",
        pronunciation: "",
        language: "en",
        active: true,
        createdAt: timestamp,
        updatedAt: timestamp,
      },
    ],
  },
  [`/api/projects/${projectId}/dictations`]: { sessions: [] },
  [`/api/projects/${projectId}/assistant/runs`]: { runs: [] },
  [`/api/projects/${projectId}/canvases`]: { canvases: [] },
  [`/api/projects/${projectId}/media`]: { media: [] },
  [`/api/projects/${projectId}/export-profile`]: { profile, revision: 1 },
  [`/api/projects/${projectId}/assistant-settings`]: { settings: {} },
  [`/api/projects/${projectId}/search`]: { results: [] },
  "/api/preferences": { preferences: { audioRetentionDays: 365 } },
  "/api/recovery": { items: [] },
  "/api/assistant/providers": { connections: [] },
  "/api/assistant/capabilities": {
    available: true,
    voiceAvailable: true,
    systemProviderConfigured: true,
    systemProviderAvailable: true,
    systemProvider: {
      name: "Documentation fixture",
      model: "synthetic-demo",
      dialect: "chat_completions",
      capabilities: { text: true, toolCalling: true, streaming: true },
    },
  },
  "/api/stt/capabilities": {
    available: true,
    adapter: "fixture",
    languages: ["ru", "en"],
    modes: ["realtime"],
    audioFormats: ["pcm16"],
    timestamps: true,
    confidence: true,
  },
};

const browser = await chromium.launch({
  headless: true,
  channel: process.env.PLAYWRIGHT_CHANNEL,
  args: ["--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"],
});
const preview = Bun.spawn(
  [
    "bun",
    "./node_modules/vite/bin/vite.js",
    "preview",
    "--host",
    "127.0.0.1",
    "--port",
    "4173",
    "--strictPort",
  ],
  { stdout: "ignore", stderr: "pipe" },
);
const errors: string[] = [];
try {
  for (let attempt = 0; ; attempt++) {
    if (
      await fetch(origin)
        .then((r) => r.ok)
        .catch(() => false)
    )
      break;
    if (attempt === 100) throw new Error("Local preview did not become ready");
    await Bun.sleep(100);
  }
  await mkdir(output, { recursive: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 960 },
    locale: "en-US",
    colorScheme: "light",
    reducedMotion: "reduce",
  });
  await context.addInitScript(
    (preferences) => {
      localStorage.setItem("writers-studio-preferences", JSON.stringify(preferences));
    },
    {
      ...defaultWriterPreferences,
      theme: "light",
      style: "fog-graphite",
      language: "en",
      commandPrefix: "Command",
    },
  );
  const unhandled = new Set<string>();
  await context.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const response = responses[path];
    if (response === undefined) {
      unhandled.add(path);
      await route.fulfill({
        status: 404,
        json: { error: { code: "fixture_missing", message: "Synthetic fixture not configured" } },
      });
      return;
    }
    await route.fulfill({ json: response });
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  const capture = async (name: string) => {
    await page.evaluate(() => document.fonts.ready);
    await page.locator('[data-theme="light"][data-style="fog-graphite"]').waitFor();
    await page.screenshot({ path: resolve(output, `${name}.png`), animations: "disabled" });
  };
  const open = async (path: string) => {
    await page.goto(origin + path);
    await page.waitForLoadState("networkidle");
  };
  await open("/projects");
  await page.getByRole("heading", { name: "Ваши проекты" }).waitFor();
  await capture("projects");
  const studio = `/projects/${projectId}/studio/${nodeId}`;
  await open(studio);
  await page.locator(".tiptap").waitFor();
  await capture("studio-overview");
  await page.getByRole("button", { name: "Вставить блок", exact: true }).click();
  await capture("rich-blocks");
  await page.keyboard.press("Escape");
  await page
    .getByRole("banner")
    .getByRole("button", { name: "Открыть ассистента", exact: true })
    .click();
  await page.getByRole("heading", { name: "Что поручить?" }).waitFor();
  await capture("assistant");
  await open(studio);
  await page.getByRole("banner").getByRole("button", { name: "Диктовать", exact: true }).click();
  await page.getByRole("dialog", { name: "Подготовка диктовки" }).waitFor();
  await capture("dictation-preparation");
  await page.setViewportSize({ width: 390, height: 844 });
  await capture("dictation-mobile");
  await page.setViewportSize({ width: 1440, height: 960 });
  for (const [name, path] of [
    ["dictionary", `/projects/${projectId}/dictionary`],
    ["export", `/projects/${projectId}/export`],
    ["project-search", `/projects/${projectId}/search`],
    ["versions", `/nodes/${nodeId}/versions`],
    ["settings", "/settings"],
    ["recovery", "/recovery"],
  ]) {
    await open(path);
    if (name === "versions") await page.getByRole("heading", { name: "Версии" }).waitFor();
    await capture(name);
  }
  if (errors.length) throw new Error(`Browser errors: ${errors.join("; ")}`);
  if (unhandled.size) throw new Error(`Missing fixtures: ${[...unhandled].join(", ")}`);
  const archived = resolve("tmp/previous-docs-images");
  await mkdir(archived, { recursive: true });
  for (const name of await readdir(output))
    if (name.endsWith(".webp")) await rename(resolve(output, name), resolve(archived, name));
  console.log("Captured 12 synthetic screenshots: light fog-graphite theme");
} finally {
  await browser.close();
  preview.kill();
}
