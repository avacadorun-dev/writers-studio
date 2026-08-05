import { createHash } from "node:crypto";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { inflateRawSync } from "node:zlib";

import { expect, type Page, test } from "@playwright/test";

test("imports a real directory through the Markdown source mode", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "chromium-1440", "Directory acceptance runs once");
  const fixture = await mkdtemp(join(tmpdir(), "writers-vault-"));
  try {
    await mkdir(join(fixture, "nested"));
    await writeFile(join(fixture, "nested", "note.md"), "---\nkind: test\n---\n# Directory note\n");
    await register(page, `directory-${Date.now()}`);
    await page.getByRole("button", { name: "Markdown / Obsidian" }).click();
    await page.getByLabel("Источник").selectOption("directory");
    await page.locator("input[webkitdirectory]").setInputFiles(fixture);
    await page.getByRole("button", { name: "Создать проект" }).click();
    await expect(markdownImportReport(page)).toContainText("1 документов", { timeout: 30_000 });
  } finally {
    await rm(fixture, { recursive: true, force: true });
  }
});

test("round-trips a 5,000-document deeply nested Obsidian vault", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "chromium-1440", "Large acceptance runs once");
  test.setTimeout(360_000);
  await register(page, `large-vault-${Date.now()}`);

  const source = buildLargeVault();
  await importZIPThroughUI(page, source.archive, "large-vault.zip", "Large Vault");
  await expect(markdownImportReport(page)).toContainText(
    /5000 документов.*100 изображений.*10000 ссылок/,
    {
      timeout: 120_000,
    },
  );
  await page.getByRole("button", { name: "Открыть проект" }).click();
  await expect(page).toHaveURL(/\/projects\/[^/]+\/studio\/[^/]+$/);
  const projectID = projectIDFromURL(page);

  await page.goto(
    `/projects/${projectID}/search?q=${encodeURIComponent("deep marker 4999")}&scope=all`,
  );
  await page.getByText("deep marker 4999", { exact: false }).click();
  await expect(page.locator(".tiptap")).toContainText("deep marker 4999", { timeout: 30_000 });
  await expect(page.locator('.tiptap a[href*="/projects/"]')).toHaveCount(2);
  await expect(page.locator('.tiptap img[alt*="img-099"]')).toBeVisible();

  const originalOutline = await fetchOutline(page, projectID);
  await page.goto(`/projects/${projectID}/export`);
  await page.getByRole("button", { name: "Экспортировать дерево" }).click();
  const readyDownload = page.getByRole("link", { name: "Скачать ZIP" });
  await expect(readyDownload).toBeVisible({ timeout: 120_000 });
  const downloadPromise = page.waitForEvent("download");
  await readyDownload.click();
  const download = await downloadPromise;
  const downloaded = Buffer.from(await download.createReadStream().then(readStream));
  const exported = readZIP(downloaded);

  expect(exported.has(`Large Vault/${source.deepPath}`)).toBe(true);
  for (let index = 0; index < 10; index += 1) {
    expect(exported.has(`Large Vault/empty-${String(index).padStart(2, "0")}/`)).toBe(true);
  }
  const exportedDeep = exported.get(`Large Vault/${source.deepPath}`)?.toString("utf8") ?? "";
  expect(exportedDeep.startsWith("---\nmarker: 4999\nraw-spacing:  value\n---\n")).toBe(true);
  expect(exportedDeep).toContain("deep marker 4999");
  expect(exported.get("Large Vault/assets/img-099.png")).toEqual(
    source.images.get("assets/img-099.png"),
  );
  expect(
    createHash("sha256")
      .update(exported.get("Large Vault/assets/img-099.png") ?? Buffer.alloc(0))
      .digest("hex"),
  ).toBe(
    createHash("sha256")
      .update(source.images.get("assets/img-099.png") ?? Buffer.alloc(0))
      .digest("hex"),
  );

  await page.goto("/projects");
  await importZIPThroughUI(page, downloaded, "large-vault-export.zip", "Large Vault Reimport");
  await expect(markdownImportReport(page)).toContainText(/5000 документов.*100 изображений/, {
    timeout: 120_000,
  });
  await page.getByRole("button", { name: "Открыть проект" }).click();
  const reimportedProjectID = projectIDFromURL(page);
  const reimportedOutline = await fetchOutline(page, reimportedProjectID);
  expect(semanticTree(reimportedOutline.nodes)).toEqual(semanticTree(originalOutline.nodes));

  const deepNode = reimportedOutline.nodes.find((node) => node.title === "Deep");
  expect(deepNode).toBeTruthy();
  const deepDocument = await page.evaluate(async (nodeID) => {
    const response = await fetch(`/api/nodes/${nodeID}`, { credentials: "include" });
    return response.json();
  }, deepNode?.id ?? "");
  expect(deepDocument.node.plainText).toContain("deep marker 4999");
});

async function register(page: Page, suffix: string) {
  await page.goto("/login");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByRole("textbox", { name: "Почта" }).fill(`${suffix}@example.test`);
  await page.getByLabel("Пароль").fill("writers-studio");
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await expect(page.getByRole("heading", { name: "Ваши проекты" })).toBeVisible();
}

function markdownImportReport(page: Page) {
  return page.getByRole("status").filter({ hasText: "Markdown-хранилище импортировано" });
}

async function importZIPThroughUI(page: Page, archive: Buffer, filename: string, title: string) {
  await page.getByRole("button", { name: "Markdown / Obsidian" }).click();
  await page.locator('input[accept*=".zip"]').setInputFiles({
    name: filename,
    mimeType: "application/zip",
    buffer: archive,
  });
  await page.getByLabel("Название проекта").fill(title);
  await page.getByRole("button", { name: "Создать проект" }).click();
}

function projectIDFromURL(page: Page): string {
  const match = new URL(page.url()).pathname.match(/^\/projects\/([^/]+)\/studio\/[^/]+$/);
  if (!match?.[1]) throw new Error("studio project route is missing");
  return match[1];
}

interface OutlineNode {
  id: string;
  parentId?: string;
  kind: string;
  title: string;
  position: number;
}

async function fetchOutline(page: Page, projectID: string): Promise<{ nodes: OutlineNode[] }> {
  return page.evaluate(async (id) => {
    const response = await fetch(`/api/projects/${id}/outline`, { credentials: "include" });
    return response.json();
  }, projectID);
}

function semanticTree(nodes: OutlineNode[]): string[] {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const cache = new Map<string, string>();
  const nodePath = (node: OutlineNode): string => {
    const cached = cache.get(node.id);
    if (cached) return cached;
    const result =
      node.parentId && byID.has(node.parentId)
        ? `${nodePath(byID.get(node.parentId) as OutlineNode)}/${node.title}`
        : node.title;
    cache.set(node.id, result);
    return result;
  };
  return nodes.map((node) => `${node.kind}:${nodePath(node)}`).sort();
}

function buildLargeVault(): { archive: Buffer; deepPath: string; images: Map<string, Buffer> } {
  const entries: Array<{ name: string; data: Buffer }> = [
    { name: "Large Vault/", data: Buffer.alloc(0) },
  ];
  const links = "[[bulk/group-000/note-0000]] [[bulk/group-000/note-0001#heading]]";
  for (let index = 0; index < 4_999; index += 1) {
    const group = String(index % 50).padStart(3, "0");
    const number = String(index).padStart(4, "0");
    entries.push({
      name: `Large Vault/bulk/group-${group}/note-${number}.md`,
      data: Buffer.from(
        `---\nmarker: ${index}\nraw-spacing:  value\n---\n# Note ${index}\n\nmarker ${index}\n\n${links}\n`,
      ),
    });
  }
  const deepDirectory = `deep/${Array.from({ length: 128 }, (_, index) => `level-${String(index).padStart(3, "0")}`).join("/")}`;
  const deepPath = `${deepDirectory}/Deep.md`;
  entries.push({
    name: `Large Vault/${deepPath}`,
    data: Buffer.from(
      `---\nmarker: 4999\nraw-spacing:  value\n---\n###### Deep\n\ndeep marker 4999\n\n${links}\n\n![[assets/img-099.png]]\n`,
    ),
  });
  for (let index = 0; index < 10; index += 1) {
    entries.push({
      name: `Large Vault/empty-${String(index).padStart(2, "0")}/`,
      data: Buffer.alloc(0),
    });
  }
  const images = new Map<string, Buffer>();
  const png = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
    "base64",
  );
  for (let index = 0; index < 100; index += 1) {
    const name = `assets/img-${String(index).padStart(3, "0")}.png`;
    images.set(name, png);
    entries.push({ name: `Large Vault/${name}`, data: png });
  }
  return { archive: writeStoredZIP(entries), deepPath, images };
}

function writeStoredZIP(entries: Array<{ name: string; data: Buffer }>): Buffer {
  const local: Buffer[] = [];
  const central: Buffer[] = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.name);
    const checksum = crc32(entry.data);
    const header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50, 0);
    header.writeUInt16LE(20, 4);
    header.writeUInt16LE(0x0800, 6);
    header.writeUInt32LE(checksum, 14);
    header.writeUInt32LE(entry.data.length, 18);
    header.writeUInt32LE(entry.data.length, 22);
    header.writeUInt16LE(name.length, 26);
    local.push(header, name, entry.data);
    const directory = Buffer.alloc(46);
    directory.writeUInt32LE(0x02014b50, 0);
    directory.writeUInt16LE(20, 4);
    directory.writeUInt16LE(20, 6);
    directory.writeUInt16LE(0x0800, 8);
    directory.writeUInt32LE(checksum, 16);
    directory.writeUInt32LE(entry.data.length, 20);
    directory.writeUInt32LE(entry.data.length, 24);
    directory.writeUInt16LE(name.length, 28);
    directory.writeUInt32LE(entry.name.endsWith("/") ? 0x10 : 0, 38);
    directory.writeUInt32LE(offset, 42);
    central.push(directory, name);
    offset += header.length + name.length + entry.data.length;
  }
  const centralData = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(centralData.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...local, centralData, end]);
}

function readZIP(data: Buffer): Map<string, Buffer> {
  const endOffset = data.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  if (endOffset < 0) throw new Error("ZIP end record is missing");
  const count = data.readUInt16LE(endOffset + 10);
  let offset = data.readUInt32LE(endOffset + 16);
  const result = new Map<string, Buffer>();
  for (let index = 0; index < count; index += 1) {
    if (data.readUInt32LE(offset) !== 0x02014b50) throw new Error("invalid ZIP directory");
    const method = data.readUInt16LE(offset + 10);
    const size = data.readUInt32LE(offset + 20);
    const nameLength = data.readUInt16LE(offset + 28);
    const extraLength = data.readUInt16LE(offset + 30);
    const commentLength = data.readUInt16LE(offset + 32);
    const localOffset = data.readUInt32LE(offset + 42);
    const name = data.subarray(offset + 46, offset + 46 + nameLength).toString("utf8");
    const localNameLength = data.readUInt16LE(localOffset + 26);
    const localExtraLength = data.readUInt16LE(localOffset + 28);
    const start = localOffset + 30 + localNameLength + localExtraLength;
    const compressed = data.subarray(start, start + size);
    result.set(name, method === 8 ? inflateRawSync(compressed) : Buffer.from(compressed));
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return result;
}

async function readStream(stream: NodeJS.ReadableStream): Promise<Buffer> {
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(Buffer.from(chunk));
  return Buffer.concat(chunks);
}

const crcTable = Array.from({ length: 256 }, (_, value) => {
  let current = value;
  for (let bit = 0; bit < 8; bit += 1)
    current = current & 1 ? 0xedb88320 ^ (current >>> 1) : current >>> 1;
  return current >>> 0;
});

function crc32(data: Buffer): number {
  let crc = 0xffffffff;
  for (const value of data) crc = (crc >>> 8) ^ (crcTable[(crc ^ value) & 0xff] ?? 0);
  return (crc ^ 0xffffffff) >>> 0;
}
