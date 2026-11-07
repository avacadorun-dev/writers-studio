import { createFileRoute, Link } from "@tanstack/react-router";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { DocsBody, DocsDescription, DocsPage, DocsTitle } from "fumadocs-ui/page";
import { RootProvider } from "fumadocs-ui/provider/tanstack";
import { ArrowLeft, BookOpen } from "lucide-react";
import { useEffect, useState } from "react";

import docsStylesHref from "../../docs/docs.css?url";
import { DocsSearchDialog } from "../../docs/docs-search";
import { getDocsMdxComponents } from "../../docs/mdx-components";
import { docsSource } from "../../docs/source";

export const Route = createFileRoute("/docs/$")({
  component: DocsRoute,
});

function DocsRoute() {
  const { _splat = "" } = Route.useParams();
  const page = docsSource.getPage(_splat ? _splat.split("/").filter(Boolean) : []);
  const [stylesReady, setStylesReady] = useState(false);

  useEffect(() => {
    let mounted = true;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = docsStylesHref;
    link.dataset.writersStudioDocsStyles = "true";
    const revealDocs = () => mounted && setStylesReady(true);
    link.addEventListener("load", revealDocs);
    link.addEventListener("error", revealDocs);
    document.head.append(link);

    return () => {
      mounted = false;
      link.removeEventListener("load", revealDocs);
      link.removeEventListener("error", revealDocs);
      link.remove();
    };
  }, []);

  useEffect(() => {
    const previous = document.title;
    const previousLanguage = document.documentElement.lang;
    document.documentElement.lang = "en";
    document.title = page
      ? `${page.data.title} — Writer’s Studio`
      : "Page not found — Writer’s Studio";
    return () => {
      document.title = previous;
      document.documentElement.lang = previousLanguage;
    };
  }, [page]);

  if (!stylesReady) {
    return (
      <main
        aria-busy="true"
        aria-label="Loading documentation"
        className="grid min-h-dvh place-items-center bg-[var(--surface-app)] text-sm text-[var(--ink-secondary)]"
      >
        Opening documentation…
      </main>
    );
  }

  return (
    <div className="ws-docs" lang="en">
      <RootProvider
        theme={{ enabled: false }}
        i18n={{
          locale: "en",
          translations: {
            "Search(search dialog)": "Search",
            "Search(search trigger)": "Search",
            "Open Search(search trigger)(aria-label)": "Open search",
            "Close Search(search dialog)(aria-label)": "Close search",
            "On this page(table of contents)": "On this page",
            "No Headings(table of contents)": "No sections on this page",
            "Copy Anchor Link(heading anchor)(aria-label)": "Copy section link",
            "Open Sidebar(aria-label)": "Open contents",
            "Open Sidebar(sidebar)(aria-label)": "Open contents",
            "Close Sidebar(aria-label)": "Close contents",
            "Close Sidebar(sidebar)(aria-label)": "Close contents",
            "Collapse Sidebar(sidebar)(aria-label)": "Collapse contents",
            "Show Sidebar(sidebar)": "Show contents",
            "Hide Sidebar(sidebar)": "Hide contents",
            "Toggle Menu(mobile menu)(aria-label)": "Open menu",
            "Next Page(pagination)": "Next article",
            "Previous Page(pagination)": "Previous article",
            "No results found(search dialog)": "No results found",
          },
        }}
        search={{
          SearchDialog: DocsSearchDialog,
          links: [
            ["Getting started", "/docs"],
            ["Troubleshooting", "/docs/help"],
          ],
        }}
      >
        <DocsLayout
          tree={docsSource.pageTree}
          nav={{
            title: (
              <span className="flex items-center gap-2 font-semibold">
                <BookOpen className="size-5" aria-hidden="true" />
                Studio help
              </span>
            ),
            url: "/docs",
          }}
          links={[
            {
              type: "main",
              text: "Return to Studio",
              url: "/projects",
              icon: <ArrowLeft className="size-4" aria-hidden="true" />,
            },
          ]}
          themeSwitch={{ enabled: false }}
        >
          {page ? <DocumentPage page={page} /> : <DocsNotFound />}
        </DocsLayout>
      </RootProvider>
    </div>
  );
}

function DocumentPage({ page }: { page: NonNullable<ReturnType<typeof docsSource.getPage>> }) {
  const Content = page.data.body;
  return (
    <DocsPage toc={page.data.toc}>
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
      <DocsBody data-docs-body>
        <Content components={getDocsMdxComponents()} />
      </DocsBody>
    </DocsPage>
  );
}

function DocsNotFound() {
  return (
    <main className="mx-auto grid min-h-[70vh] max-w-xl place-items-center px-6 text-center">
      <section>
        <p className="text-sm font-semibold text-fd-muted-foreground">Page not found</p>
        <h1 className="mt-3 text-3xl font-semibold">This page may have moved</h1>
        <p className="mt-4 leading-7 text-fd-muted-foreground">
          Return to the documentation home or use search to find the guide you need.
        </p>
        <Link
          to="/docs/$"
          params={{ _splat: "" }}
          className="mt-6 inline-flex min-h-11 items-center rounded-md bg-fd-primary px-4 text-sm font-medium text-fd-primary-foreground"
        >
          Documentation home
        </Link>
      </section>
    </main>
  );
}
