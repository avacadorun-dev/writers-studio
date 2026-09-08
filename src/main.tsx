import "@fontsource/literata/400.css";
import "@fontsource/literata/400-italic.css";
import "@fontsource/literata/500.css";
import "@fontsource/literata/600.css";
import "@fontsource/philosopher/700.css";
import "@fontsource/inter/400.css";
import "@fontsource/inter/500.css";
import "@fontsource/inter/600.css";
import "@fontsource/source-serif-4/400.css";
import "@fontsource/source-serif-4/400-italic.css";
import "@fontsource/source-serif-4/500.css";
import "@fontsource/source-serif-4/600.css";
import "@fontsource/sofia-sans/400.css";
import "@fontsource/sofia-sans/400-italic.css";
import "@fontsource/sofia-sans/500.css";
import "@fontsource/sofia-sans/600.css";
import "@fontsource/ibm-plex-mono/400.css";
import "katex/dist/katex.min.css";
import "./shared/styles/globals.css";

import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { AppProviders } from "~app/providers";

import { routeTree } from "./app/routeTree.gen";

const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
  scrollRestoration: true,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const root = document.getElementById("root");
if (!root) {
  throw new Error("Application root is missing");
}

createRoot(root).render(
  <StrictMode>
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
);
