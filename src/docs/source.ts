import type { StructuredData } from "fumadocs-core/mdx-plugins";
import { loader, source } from "fumadocs-core/source";
import type { TOCItemType } from "fumadocs-core/toc";
import type { MDXContent } from "mdx/types";

import navigation from "../../content/docs/meta.json";

type DocsMdxModule = {
  default: MDXContent;
  frontmatter: {
    title: string;
    description: string;
  };
  toc: TOCItemType[];
  structuredData: StructuredData;
};

const modules = import.meta.glob<DocsMdxModule>("/content/docs/**/*.mdx", { eager: true });

const pages = Object.entries(modules).map(([file, module]) => ({
  type: "page" as const,
  path: file.replace(/^\/content\/docs\//, ""),
  data: {
    ...module.frontmatter,
    body: module.default,
    toc: module.toc,
    structuredData: module.structuredData,
  },
}));

export const docsSource = loader({
  baseUrl: "/docs",
  source: source({
    pages,
    metas: [{ type: "meta", path: "meta.json", data: navigation }],
  }),
});
