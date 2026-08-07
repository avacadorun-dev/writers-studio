import { existsSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { isAbsolute, join, resolve } from "node:path";

const minimum = 90;
const coverageDirectory = "coverage/backend";
const goCache = process.env.GOCACHE ?? join(tmpdir(), "writers-studio-go-cache");

const chromiumCandidates = [
  process.env.WRITERS_TEST_CHROMIUM,
  process.env.CHROMIUM_PATH,
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  "/usr/bin/chromium",
  "/usr/bin/chromium-browser",
  "/usr/bin/google-chrome",
].filter((value): value is string => Boolean(value));
const chromium = chromiumCandidates.find(existsSync);
const mermaidCandidates = [
  process.env.MERMAID_JS_PATH,
  "node_modules/mermaid/dist/mermaid.min.js",
  "/app/mermaid.min.js",
].filter((value): value is string => Boolean(value));
const mermaidCandidate = mermaidCandidates.find(existsSync);
const mermaid = mermaidCandidate
  ? isAbsolute(mermaidCandidate)
    ? mermaidCandidate
    : resolve(mermaidCandidate)
  : undefined;
if (!chromium || !mermaid) {
  throw new Error("rich-block coverage requires local Chromium and mermaid.min.js");
}

const targets = [
  {
    package: "./internal/projectmedia",
    profile: `${coverageDirectory}/rich-project-media.out`,
    functions: ["InspectImage", "imageFormat", "SafeFileName"],
  },
  {
    package: "./internal/assistantapp",
    profile: `${coverageDirectory}/rich-assistant.out`,
    functions: [
      "callDirect",
      "mutationAcceptsCanonicalContent",
      "validateContentImageAssets",
      "normalizeAssistantContent",
      "assistantBlocksFromDocument",
    ],
  },
  {
    package: "./internal/projectmcp",
    profile: `${coverageDirectory}/rich-mcp.out`,
    functions: ["assistantContentSchema"],
  },
  {
    package: "./internal/export",
    profile: `${coverageDirectory}/rich-export.out`,
    functions: [
      "renderCallout",
      "renderCodeBlock",
      "renderTable",
      "tableColumnWidths",
      "renderPublicationCodeListing",
      "calloutLabel",
      "normalizedCalloutKind",
      "renderPublicationTable",
      "publicationHeaderRow",
      "highlightCodeWithLexer",
      "prepareMermaidAssets",
      "countMermaidBlocks",
      "replaceMermaidNodes",
      "renderMermaidPNG",
      "validatedMermaidPNG",
    ],
  },
] as const;

await mkdir(coverageDirectory, { recursive: true });
for (const target of targets) {
  const environment = {
    ...process.env,
    GOCACHE: goCache,
    WRITERS_TEST_CHROMIUM: chromium,
    MERMAID_JS_PATH: mermaid,
  };
  const goTest = Bun.spawnSync(["go", "test", `-coverprofile=${target.profile}`, target.package], {
    stdout: "inherit",
    stderr: "inherit",
    env: environment,
  });
  if (goTest.exitCode !== 0) process.exit(goTest.exitCode);

  const goCover = Bun.spawnSync(["go", "tool", "cover", `-func=${target.profile}`], {
    env: environment,
  });
  if (goCover.exitCode !== 0) {
    process.stderr.write(goCover.stderr.toString());
    process.exit(goCover.exitCode);
  }
  const functionCoverage = new Map<string, number>();
  for (const line of goCover.stdout.toString().split("\n")) {
    const match = line.match(/:\d+:\s+(\S+)\s+([\d.]+)%$/u);
    if (match) functionCoverage.set(match[1], Number(match[2]));
  }
  for (const name of target.functions) {
    const coverage = functionCoverage.get(name);
    if (!Number.isFinite(coverage)) {
      throw new Error(`${target.package}.${name} is missing from Go coverage`);
    }
    console.log(`${target.package}.${name}: ${coverage?.toFixed(1)}% statement coverage`);
    if ((coverage ?? 0) < minimum) {
      throw new Error(`${target.package}.${name} coverage must be at least ${minimum}%`);
    }
  }
}

console.log(`Rich-block coverage gate passed at ${minimum}% or better.`);
