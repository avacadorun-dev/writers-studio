import { createFileRoute, Link } from "@tanstack/react-router";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { DocsBody, DocsDescription, DocsPage, DocsTitle } from "fumadocs-ui/page";
import { RootProvider } from "fumadocs-ui/provider/tanstack";
import { ArrowLeft, BookOpen } from "lucide-react";
import { useEffect } from "react";

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

  useEffect(() => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = docsStylesHref;
    link.dataset.writersStudioDocsStyles = "true";
    document.head.append(link);

    return () => {
      link.remove();
    };
  }, []);

  useEffect(() => {
    const previous = document.title;
    document.title = page
      ? `${page.data.title} — Студия автора`
      : "Материал не найден — Студия автора";
    return () => {
      document.title = previous;
    };
  }, [page]);

  return (
    <div className="ws-docs">
      <RootProvider
        theme={{ enabled: false }}
        i18n={{
          locale: "ru",
          translations: {
            "Search(search dialog)": "Поиск",
            "Search(search trigger)": "Поиск",
            "Open Search(search trigger)(aria-label)": "Открыть поиск",
            "Close Search(search dialog)(aria-label)": "Закрыть поиск",
            "On this page(table of contents)": "На этой странице",
            "No Headings(table of contents)": "На странице нет разделов",
            "Copy Anchor Link(heading anchor)(aria-label)": "Скопировать ссылку на раздел",
            "Open Sidebar(aria-label)": "Открыть оглавление",
            "Open Sidebar(sidebar)(aria-label)": "Открыть оглавление",
            "Close Sidebar(aria-label)": "Закрыть оглавление",
            "Close Sidebar(sidebar)(aria-label)": "Закрыть оглавление",
            "Collapse Sidebar(sidebar)(aria-label)": "Свернуть оглавление",
            "Show Sidebar(sidebar)": "Показать оглавление",
            "Hide Sidebar(sidebar)": "Скрыть оглавление",
            "Toggle Menu(mobile menu)(aria-label)": "Открыть меню",
            "Next Page(pagination)": "Следующая статья",
            "Previous Page(pagination)": "Предыдущая статья",
            "No results found(search dialog)": "Ничего не найдено",
          },
        }}
        search={{
          SearchDialog: DocsSearchDialog,
          links: [
            ["Начало работы", "/docs"],
            ["Если что-то пошло не так", "/docs/help"],
          ],
        }}
      >
        <DocsLayout
          tree={docsSource.pageTree}
          nav={{
            title: (
              <span className="flex items-center gap-2 font-semibold">
                <BookOpen className="size-5" aria-hidden="true" />
                Помощь по Студии
              </span>
            ),
            url: "/docs",
          }}
          links={[
            {
              type: "main",
              text: "Вернуться в Студию",
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
        <p className="text-sm font-semibold text-fd-muted-foreground">Материал не найден</p>
        <h1 className="mt-3 text-3xl font-semibold">Похоже, эта страница переехала</h1>
        <p className="mt-4 leading-7 text-fd-muted-foreground">
          Вернитесь к началу документации или воспользуйтесь поиском — нужная подсказка, скорее
          всего, осталась рядом.
        </p>
        <Link
          to="/docs/$"
          params={{ _splat: "" }}
          className="mt-6 inline-flex min-h-11 items-center rounded-md bg-fd-primary px-4 text-sm font-medium text-fd-primary-foreground"
        >
          Открыть начало документации
        </Link>
      </section>
    </main>
  );
}
