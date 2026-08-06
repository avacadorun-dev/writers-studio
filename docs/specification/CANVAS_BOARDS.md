# Canvas-доски

Статус: реализованная спецификация первого card-first выпуска.  
Версия продукта: 0.4.0.  
Дата: 5 августа 2026.

## 1. Scope

Проект содержит несколько независимых досок. Первый выпуск включает Markdown-
заметки, ссылки на документы проекта, изображения, spatial-группы, цвета и
направленные связи. Desktop поддерживает компоновку; mobile — просмотр, pan/zoom
и открытие документов.

Не входят: freehand/фигуры/eraser, web/PDF/audio/video cards, вложенные canvas,
`.canvas` import/export, image export, collaboration, Assistant tools и глобальный
поиск по standalone-тексту доски.

## 2. Клиентская модель

- viewport и selection реализованы на официальном пакете React Flow
  `@xyflow/react`;
- канонический snapshot совместим с JSON Canvas 1.0: `text`, `file`, `group` и
  edges;
- document/media reference живёт в `x-writers-studio.resource`; неизвестные
  top-level и namespaced поля codec сохраняет без потерь;
- SVG drawing-layer в следующем этапе может использовать ту же систему координат
  через `ViewportPortal`, не меняя движок карточек и viewport.

Поддерживаются grid snapping, multiselect/lasso, resize, z-order, четыре connection
handles, reconnect, label/color, fit-all/fit-selection, copy/cut/paste, duplicate и
history на 100 операций. Копирование создаёт новые IDs и переносит только связи,
оба конца которых входят в selection. Перемещение spatial-группы переносит
полностью заключённые карточки; удаление группы оставляет карточки.

## 3. Persistency и конфликты

Таблицы `canvases` и `canvas_revisions` хранят summary и immutable JSONB snapshots.
Mutation отправляет `baseRevision`. При stale base сервер атомарно создаёт доску
`— конфликтующая копия`; ни один вариант не теряется.

Autosave:

- IndexedDB — через 300 мс;
- server revision — через 1,5 с покоя;
- максимум между попытками server save — 15 с;
- online event повторяет несинхронизированную попытку;
- reload восстанавливает несинхронизированный `canvasSnapshots` record.

## 4. API

- `GET/POST /api/projects/{projectId}/canvases`;
- `GET/PATCH/DELETE /api/canvases/{canvasId}`;
- `POST /api/canvases/{canvasId}/duplicate`;
- `POST /api/canvases/{canvasId}/restore`;
- `PUT /api/canvases/{canvasId}/revisions`.

`GET` одной доски возвращает текущий snapshot и разрешённые previews всех
связанных документов/media батчами без N+1. Сервер проверяет ownership, уникальные
node/edge IDs, геометрию, dangling edges и принадлежность resources проекту.
Лимит snapshot: 2 000 nodes, 4 000 edges, 100 000 Unicode code points в text card.

## 5. Перенос проекта

`.wstudio` v6 содержит все `canvases` и `canvasRevisions`, включая корзину.
При импорте canvas/document/media IDs создаются заново; namespaced resource IDs и
локальные `writers-studio://…` file references переписываются атомарно. Импорт
legacy v1–v5 остаётся допустимым и создаёт проект без досок.

## 6. Acceptance и coverage

- CAN-01: несколько досок создаются, открываются, переименовываются,
  дублируются, удаляются и восстанавливаются независимо;
- CAN-02: text/document/image/group cards и edges переживают reload без потери
  extensions;
- CAN-03: copy/paste remap, spatial group geometry и undo/redo детерминированы;
- CAN-04: offline snapshot восстанавливается, stale revision создаёт отдельный
  конфликтующий canvas;
- CAN-05: mobile не мутирует layout, но позволяет pan/zoom и переход в Studio;
- CAN-06: `.wstudio` v6 round-trip сохраняет все revisions и remap resources;
- CAN-07: 500 cards / 1 000 edges используют visible-only rendering;
- CAN-08: line/statement coverage новой критической Canvas-логики не ниже 90%.

Команда `bun run test:canvas:coverage` проверяет порог автоматически. Текущий
результат: frontend 99,51% lines, Go canvas validator 97,2% statements.
