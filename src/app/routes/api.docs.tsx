import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";

const swaggerAssetBase = "/swagger-assets/5.32.12";

type SwaggerRequest = {
  credentials?: RequestCredentials;
};

type SwaggerUIFactory = ((configuration: Record<string, unknown>) => unknown) & {
  presets: { apis: unknown };
};

declare global {
  interface Window {
    SwaggerUIBundle?: SwaggerUIFactory;
    SwaggerUIStandalonePreset?: unknown;
  }
}

export const Route = createFileRoute("/api/docs")({
  component: OpenAPIDocsPage,
});

function OpenAPIDocsPage() {
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    ensureStylesheet(`${swaggerAssetBase}/swagger-ui.css`);
    Promise.all([
      loadScript("swagger-ui-bundle", `${swaggerAssetBase}/swagger-ui-bundle.js`, () =>
        Boolean(window.SwaggerUIBundle),
      ),
      loadScript(
        "swagger-ui-standalone-preset",
        `${swaggerAssetBase}/swagger-ui-standalone-preset.js`,
        () => Boolean(window.SwaggerUIStandalonePreset),
      ),
    ])
      .then(() => {
        if (!active || !window.SwaggerUIBundle) return;
        window.SwaggerUIBundle({
          url: "/api/openapi.json",
          dom_id: "#swagger-ui",
          deepLinking: true,
          displayRequestDuration: true,
          filter: true,
          layout: "StandaloneLayout",
          presets: [window.SwaggerUIBundle.presets.apis, window.SwaggerUIStandalonePreset],
          requestInterceptor: (request: SwaggerRequest) => {
            request.credentials = "same-origin";
            return request;
          },
          withCredentials: true,
        });
      })
      .catch(() => {
        if (active) setError("Не удалось загрузить Swagger UI");
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <main className="min-h-screen bg-[var(--surface-raised)]">
      {error ? (
        <p className="p-8 font-sans text-[var(--danger)]">{error}</p>
      ) : (
        <div id="swagger-ui" />
      )}
    </main>
  );
}

function ensureStylesheet(href: string) {
  if (document.querySelector(`link[href="${href}"]`)) return;
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = href;
  document.head.append(link);
}

function loadScript(id: string, src: string, isReady: () => boolean): Promise<void> {
  if (isReady()) return Promise.resolve();
  return new Promise((resolvePromise, reject) => {
    const existing = document.getElementById(id) as HTMLScriptElement | null;
    const script = existing ?? document.createElement("script");
    const onLoad = () => resolvePromise();
    const onError = () => reject(new Error(`Failed to load ${src}`));
    script.addEventListener("load", onLoad, { once: true });
    script.addEventListener("error", onError, { once: true });
    if (!existing) {
      script.id = id;
      script.src = src;
      script.async = true;
      document.head.append(script);
    }
  });
}
