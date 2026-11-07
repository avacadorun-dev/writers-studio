import { access, readdir, readFile } from "node:fs/promises";
import { basename, join } from "node:path";

const root = join(import.meta.dir, "..", "content", "docs");
const publicRoot = join(import.meta.dir, "..", "public");
const files = (await readdir(root)).filter((name) => name.endsWith(".mdx")).sort();
const slugs = new Set(files.map((name) => (name === "index.mdx" ? "" : basename(name, ".mdx"))));
const meta = JSON.parse(await readFile(join(root, "meta.json"), "utf8")) as { pages?: string[] };
const errors: string[] = [];

if (!Array.isArray(meta.pages)) errors.push("meta.json: pages must be an array");
const listed = new Set(meta.pages ?? []);
for (const slug of slugs) {
  const name = slug || "index";
  if (!listed.has(name)) errors.push(`meta.json: page is not listed: ${name}`);
}
for (const name of listed) {
  if (!slugs.has(name === "index" ? "" : name)) errors.push(`meta.json: missing file ${name}.mdx`);
}

for (const name of files) {
  const source = await readFile(join(root, name), "utf8");
  const frontmatter = source.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!frontmatter) {
    errors.push(`${name}: missing frontmatter`);
    continue;
  }
  for (const field of ["title", "description"]) {
    if (!new RegExp(`^${field}:\\s*\\S`, "m").test(frontmatter[1])) {
      errors.push(`${name}: missing field ${field}`);
    }
  }

  for (const match of source.matchAll(/\]\((\/docs(?:\/[^)#\s]*)?)(?:#[^)]+)?\)/g)) {
    if (match[1].startsWith("/docs/examples/")) {
      try {
        await access(join(publicRoot, match[1].replace(/^\//, "")));
      } catch {
        errors.push(`${name}: missing file ${match[1]}`);
      }
      continue;
    }
    const slug = match[1].replace(/^\/docs\/?/, "");
    if (slug && !slugs.has(slug)) errors.push(`${name}: broken link ${match[1]}`);
  }

  for (const match of source.matchAll(/href=["'](\/docs\/examples\/[^"']+)["']/g)) {
    try {
      await access(join(publicRoot, match[1].replace(/^\//, "")));
    } catch {
      errors.push(`${name}: missing file ${match[1]}`);
    }
  }

  for (const match of source.matchAll(/(?:src=|!\[[^\]]*\]\()["']?(\/docs\/images\/[^)"'\s]+)/g)) {
    try {
      await access(join(publicRoot, match[1].replace(/^\//, "")));
    } catch {
      errors.push(`${name}: missing image ${match[1]}`);
    }
  }

  if (/(?:api[_-]?key|token|password)\s*[:=]\s*["'][^"']{8,}/i.test(source)) {
    errors.push(`${name}: possible exposed secret`);
  }
}

if (errors.length > 0) throw new Error(`Documentation check failed:\n- ${errors.join("\n- ")}`);
console.log(`Documentation checked: ${files.length} pages; internal links valid`);
