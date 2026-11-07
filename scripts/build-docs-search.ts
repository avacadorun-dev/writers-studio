import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { structure } from "fumadocs-core/mdx-plugins";
import { createSearchAPI } from "fumadocs-core/search/server";

const contentRoot = join(import.meta.dir, "..", "content", "docs");
const outputPath = join(import.meta.dir, "..", "public", "docs", "search.json");

type Frontmatter = { title: string; description: string };

async function listMdxFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) return listMdxFiles(path);
      return entry.name.endsWith(".mdx") || entry.name.endsWith(".md") ? [path] : [];
    }),
  );
  return nested.flat();
}

function readFrontmatter(source: string, path: string): { data: Frontmatter; body: string } {
  const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?/);
  if (!match) throw new Error(`${path}: missing frontmatter`);
  const values = Object.fromEntries(
    match[1]
      .split(/\r?\n/)
      .map((line) => line.match(/^([A-Za-z][\w-]*):\s*(.+)$/))
      .filter((item): item is RegExpMatchArray => Boolean(item))
      .map((item) => [item[1], item[2].trim().replace(/^['"]|['"]$/g, "")]),
  );
  if (!values.title || !values.description) {
    throw new Error(`${path}: title and description are required`);
  }
  return { data: values as Frontmatter, body: source.slice(match[0].length) };
}

function pageUrl(path: string): string {
  const slug = relative(contentRoot, path)
    .replace(/\\/g, "/")
    .replace(/\.(md|mdx)$/, "")
    .replace(/(^|\/)index$/, "");
  return slug ? `/docs/${slug}` : "/docs";
}

const files = await listMdxFiles(contentRoot);
const indexes = await Promise.all(
  files.map(async (path) => {
    const { data, body } = readFrontmatter(await readFile(path, "utf8"), path);
    return {
      id: pageUrl(path),
      url: pageUrl(path),
      title: data.title,
      description: data.description,
      structuredData: structure(body),
    };
  }),
);
const api = createSearchAPI("advanced", { indexes, language: "english" });
const response = await api.staticGET();
await mkdir(dirname(outputPath), { recursive: true });
await writeFile(outputPath, `${await response.text()}\n`);
console.log(`Documentation index: ${indexes.length} pages`);
