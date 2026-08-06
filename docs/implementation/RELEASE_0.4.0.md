# Release 0.4.0 — Canvas-доски

Дата подготовки: 2026-08-05. Статус: implementation candidate, не развёрнут.

## Состав

- несколько Canvas-досок на проект на базе `@xyflow/react`;
- Markdown-, document- и image-карточки, spatial-группы, цвета и связи;
- pan/zoom, fit, lasso/multiselect, move/resize, grid, z-order и reconnect;
- copy/cut/paste, duplicate, undo/redo и безопасный Markdown preview;
- локальный recovery через 300 мс и server autosave 1,5 с / максимум 15 с;
- конфликтующая копия при stale `baseRevision`;
- mobile read-only layout с pan/zoom и переходом в Studio;
- `.wstudio` v6 со всеми canvas revisions и remap document/media resources;
- OpenAPI Canvas API и migration 023.

Свободное рисование намеренно отложено. Будущий SVG/custom-node layer использует
тот же viewport; миграция существующих досок не требуется.

## Обновление и откат

Migration 023 только добавляет `canvases`, `canvas_revisions` и индексы. Она не
переписывает проекты, документы, media или старые revisions и может остаться в БД
при откате binary. Старый binary проигнорирует новые таблицы, но не сможет открыть
или экспортировать доски. Архив `.wstudio` v6 требует версии 0.4.0+; importer 0.4.0
продолжает принимать v1–v5.

Перед production switch использовать `./deploy/update-server.sh`; удалять volumes
или выполнять `docker compose down -v` нельзя.

## Проверка перед выпуском

```bash
bun run check
bun run type-check
bun run test
bun run test:canvas:coverage
bun run build
go test ./...
```

PostgreSQL integration отдельно проверяет экспорт/import v6 с новым canvas ID и
переназначенной document reference. Browser E2E должен пройти создание карточек,
resize/connect/group/color, reload, offline recovery, переход в Studio и mobile
read-only viewport.
