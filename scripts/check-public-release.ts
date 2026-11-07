import { lstat, readFile } from "node:fs/promises";
import { basename, dirname, resolve } from "node:path";

// Inspect publishable files only. Git history needs an additional full-history scanner.
const root = resolve(process.argv[2] ?? new URL("..", import.meta.url).pathname);
const files = Bun.spawnSync(
  ["git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
  { stdout: "pipe", stderr: "pipe" },
);
if (files.exitCode !== 0) throw new Error("Cannot enumerate release files with Git");
const rules = [
  { name: "private key", pattern: /-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/g },
  {
    name: "provider credential",
    pattern:
      /\b(?:sk-(?:proj-|or-v1-)?[A-Za-z0-9_-]{25,}|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[A-Z0-9]{16})\b/g,
  },
  { name: "personal machine path", pattern: /\/(?:Users|home)\/[A-Za-z0-9_.-]+\//g },
  {
    name: "private workspace link",
    pattern:
      /https:\/\/(?:www\.)?(?:figma\.com\/(?:file|design)|docs\.google\.com|drive\.google\.com|notion\.so)\//g,
  },
  { name: "design service token", pattern: /["']fileToken["']\s*:/g },
];
const privateAddress =
  /\b(?:192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})\b/g;
// These tests deliberately exercise private-network rejection with synthetic addresses.
const networkTests = new Set([
  "internal/httpapi/link_preview_test.go",
  "internal/llm/credentials_test.go",
]);
const forbiddenDirectory =
  /(?:^|\/)(?:\.soba|\.codex|\.agents|desktop|artifacts|backups|data|coverage|tmp|test-results|playwright-report(?:-real-stt)?)(?:\/|$)/;
const privateMarkers = (process.env.PUBLICATION_PRIVATE_MARKERS ?? "")
  .split(",")
  .map((value) => value.trim())
  .filter(Boolean);
const findings: string[] = [];
let checked = 0;

for (const path of new Set(files.stdout.toString().split("\0").filter(Boolean))) {
  const absolute = resolve(root, path);
  const info = await lstat(absolute).catch(() => undefined);
  if (!info) continue; // Deletions in the working tree are absent from the publication snapshot.
  const name = basename(path);
  if (
    forbiddenDirectory.test(path) ||
    (/^\.env(?:\.|$)/.test(name) && name !== ".env.example") ||
    /\.(?:key|pem|p12|pfx|pen|log|pid|sql\.gz)$/i.test(name) ||
    ["goal.md", "writers-studio-local-ca.crt", "Archive.zip", "build-macos.sh"].includes(name)
  ) {
    findings.push(`${path}: excluded release material`);
    continue;
  }
  if (info.isSymbolicLink()) {
    findings.push(`${path}: symbolic link requires explicit review`);
    continue;
  }
  if (!info.isFile()) continue;
  checked += 1;
  const bytes = await readFile(absolute);
  if (bytes.includes(0)) continue;
  const source = bytes.toString("utf8");
  for (const { name: label, pattern } of rules) {
    for (const match of source.matchAll(pattern)) {
      findings.push(`${path}:${source.slice(0, match.index).split("\n").length}: ${label}`);
    }
  }
  if (!networkTests.has(path) && privateAddress.test(source)) {
    findings.push(`${path}: private network address`);
  }
  privateAddress.lastIndex = 0;
  if (privateMarkers.some((marker) => source.toLowerCase().includes(marker.toLowerCase()))) {
    findings.push(`${path}: operator-supplied private marker`);
  }
  if (/\.(?:md|mdx)$/i.test(path)) {
    if (/[А-Яа-яЁё]/.test(source)) findings.push(`${path}: documentation must be English`);
    for (const match of source.matchAll(/\]\(([^\s)]+)\)/g)) {
      const target = match[1].split("#")[0];
      if (!target || /^(?:[a-z][a-z0-9+.-]*:|\/)/i.test(target)) continue;
      if (!(await lstat(resolve(dirname(absolute), decodeURI(target))).catch(() => undefined))) {
        findings.push(`${path}: broken relative documentation link`);
      }
    }
  }
}

if (findings.length) {
  console.error(
    `Public release check failed (${findings.length} findings):\n${findings.join("\n")}`,
  );
  process.exitCode = 1;
} else {
  console.log(
    `Public release check passed: ${checked} files; known private-material patterns absent`,
  );
}
