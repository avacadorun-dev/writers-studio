import { GlobalRegistrator } from "@happy-dom/global-registrator";
import "fake-indexeddb/auto";
import { existsSync } from "node:fs";
import { resolve } from "node:path";

GlobalRegistrator.register();

const aliases: Record<string, string> = {
  app: "app",
  pages: "pages",
  widgets: "widgets",
  features: "features",
  entities: "entities",
  shared: "shared",
};

Bun.plugin({
  name: "writers-studio-test-aliases",
  setup(build) {
    build.onResolve({ filter: /^~[^/]+\// }, (args) => {
      const [alias, ...segments] = args.path.slice(1).split("/");
      const directory = aliases[alias];
      if (!directory) {
        return undefined;
      }
      const base = resolve(process.cwd(), "src", directory, ...segments);
      for (const candidate of [
        base,
        `${base}.ts`,
        `${base}.tsx`,
        resolve(base, "index.ts"),
        resolve(base, "index.tsx"),
      ]) {
        if (existsSync(candidate)) {
          return { path: candidate };
        }
      }
      return undefined;
    });
  },
});
