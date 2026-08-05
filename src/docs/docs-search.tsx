import { create } from "@orama/orama";
import { useDocsSearch } from "fumadocs-core/search/client";
import { oramaStaticClient } from "fumadocs-core/search/client/orama-static";
import {
  SearchDialog,
  SearchDialogClose,
  SearchDialogContent,
  SearchDialogHeader,
  SearchDialogIcon,
  SearchDialogInput,
  SearchDialogList,
  SearchDialogOverlay,
  type SharedProps,
} from "fumadocs-ui/components/dialog/search";

const client = oramaStaticClient({
  from: "/docs/search.json",
  initOrama: () => create({ schema: { _: "string" }, language: "russian" }),
});

export function DocsSearchDialog(props: SharedProps) {
  const { search, setSearch, query } = useDocsSearch({ client });

  return (
    <SearchDialog search={search} onSearchChange={setSearch} isLoading={query.isLoading} {...props}>
      <SearchDialogOverlay />
      <SearchDialogContent>
        <SearchDialogHeader>
          <SearchDialogIcon />
          <SearchDialogInput
            placeholder="Найти в документации…"
            aria-label="Поиск по документации"
          />
          <SearchDialogClose>Закрыть</SearchDialogClose>
        </SearchDialogHeader>
        <SearchDialogList
          items={query.data !== "empty" ? query.data : null}
          Empty={() => (
            <p className="p-6 text-center text-sm text-fd-muted-foreground">
              Ничего не нашлось. Попробуйте сформулировать запрос короче.
            </p>
          )}
        />
      </SearchDialogContent>
    </SearchDialog>
  );
}
