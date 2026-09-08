import { mkdir, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const minimum = 90;
const goCache = process.env.GOCACHE ?? join(tmpdir(), "writers-studio-go-cache");
const frontendSources = [
  "src/features/editor/model/code-block.tsx",
  "src/features/editor/model/code-languages.ts",
];

const lcov = await readFile("coverage/frontend/lcov.info", "utf8");
const frontendCoverage = new Map<string, number>();
for (const record of lcov.split("end_of_record")) {
  const source = record.match(/^SF:(.+)$/m)?.[1];
  if (!source || !frontendSources.includes(source)) continue;
  const lines = Number(record.match(/^LF:(\d+)$/m)?.[1] ?? 0);
  const hit = Number(record.match(/^LH:(\d+)$/m)?.[1] ?? 0);
  if (lines > 0) frontendCoverage.set(source, (hit / lines) * 100);
}

for (const source of frontendSources) {
  const coverage = frontendCoverage.get(source);
  if (!Number.isFinite(coverage)) {
    throw new Error(`${source} is missing from LCOV`);
  }
  console.log(`${source}: ${coverage?.toFixed(2)}% line coverage`);
  if ((coverage ?? 0) < minimum) {
    throw new Error(`${source} coverage must be at least ${minimum}%`);
  }
}

const backendTargets = [
  {
    package: "./internal/editor",
    profile: "coverage/backend/native-code-editor.out",
    functions: [
      "ParseFencedCodeBlock",
      "NormalizeFencedCodeBlocks",
      "ParseInlineCodeNodes",
      "NormalizeInlineCodeMarks",
      "markdownCodeFence",
      "markdownInlineCode",
    ],
  },
  {
    package: "./internal/assistantapp",
    profile: "coverage/backend/native-code-assistant.out",
    functions: [
      "replaceDocumentRanges",
      "normalizeAssistantContent",
      "assistantBlocksFromDocument",
      "assistantBlockText",
      "assistantInlineCode",
    ],
  },
  {
    package: "./internal/markdowntree",
    profile: "coverage/backend/native-code-markdown.out",
    functions: ["trimMarkdownFenceLineEnding"],
  },
  {
    package: "./internal/projectmcp",
    profile: "coverage/backend/native-code-mcp.out",
    functions: ["assistantContentSchema"],
  },
  {
    package: "./internal/export",
    profile: "coverage/backend/native-code-export.out",
    functions: [
      "normalizeLegacyCodeFormatting",
      "renderWordNodes",
      "renderPrintHTML",
      "inlineCodePublicationCSS",
    ],
  },
] as const;

await mkdir("coverage/backend", { recursive: true });
for (const target of backendTargets) {
  const environment = { ...process.env, GOCACHE: goCache };
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
