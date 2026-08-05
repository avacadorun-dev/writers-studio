import { copyFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath, URL } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import mdx from "fumadocs-mdx/vite";
import { defineConfig, type Plugin } from "vite";
import { VitePWA } from "vite-plugin-pwa";

const swaggerUIVersion = "5.32.12";

function swaggerUIAssets(): Plugin {
  const sourceDirectory = fileURLToPath(new URL("./node_modules/swagger-ui-dist", import.meta.url));
  const files = ["swagger-ui.css", "swagger-ui-bundle.js", "swagger-ui-standalone-preset.js"];
  return {
    name: "swagger-ui-assets",
    apply: "build",
    async writeBundle(options) {
      const destinationDirectory = resolve(
        options.dir ?? "dist",
        "swagger-assets",
        swaggerUIVersion,
      );
      await mkdir(destinationDirectory, { recursive: true });
      await Promise.all(
        files.map((file) =>
          copyFile(resolve(sourceDirectory, file), resolve(destinationDirectory, file)),
        ),
      );
    },
  };
}

export default defineConfig({
  resolve: {
    alias: {
      "~app": fileURLToPath(new URL("./src/app", import.meta.url)),
      "~pages": fileURLToPath(new URL("./src/pages", import.meta.url)),
      "~widgets": fileURLToPath(new URL("./src/widgets", import.meta.url)),
      "~features": fileURLToPath(new URL("./src/features", import.meta.url)),
      "~entities": fileURLToPath(new URL("./src/entities", import.meta.url)),
      "~shared": fileURLToPath(new URL("./src/shared", import.meta.url)),
    },
  },
  plugins: [
    mdx({}),
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
      routesDirectory: "./src/app/routes",
      generatedRouteTree: "./src/app/routeTree.gen.ts",
    }),
    react(),
    tailwindcss(),
    swaggerUIAssets(),
    VitePWA({
      registerType: "autoUpdate",
      includeAssets: ["icon.svg"],
      manifest: {
        name: "Студия автора",
        short_name: "Студия",
        description: "Голос, черновик и рукопись в одном безопасном пространстве.",
        display: "standalone",
        start_url: "/projects",
        icons: [
          {
            src: "/icon.svg",
            sizes: "any",
            type: "image/svg+xml",
            purpose: "any maskable",
          },
        ],
      },
      workbox: {
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          {
            urlPattern: ({ url }) => url.pathname.startsWith("/api/"),
            handler: "NetworkOnly",
          },
        ],
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
