# Release 0.2.0 — rich document editor

Дата подготовки: 2026-08-05.

## Состав

- таблицы и команды строк/столбцов;
- block/inline code;
- project images с alt-текстом и подписью;
- Mermaid-схемы с редактируемым исходником;
- callout `info`, `idea`, `warning`, `todo`;
- единая доступность блоков в `game_design`, заметках и остальных режимах;
- document schema v2 с ленивым переходом с v1;
- `.wstudio` v3 с бинарными изображениями и импортом v1/v2/v3;
- Markdown/TXT/DOCX textual fallback для новых семантических блоков;
- allowlist paste sanitizer, server allowlist и media integrity checks.

## Обновление сетевого инстанса

Перед переключением release использовать только штатный сценарий:

```bash
./deploy/update-server.sh
```

Он вызывает backup PostgreSQL и MinIO до rebuild. Нельзя выполнять
`docker compose down -v`, удалять production volumes или запускать clean-room
Compose под production project name.

При старте приложение применяет migration 019. Миграция только создаёт
`project_media` и два индекса; существующие таблицы и revisions не переписываются.

После healthcheck проверить:

1. Открытие существующего сетевого проекта и старой revision.
2. Обычную текстовую правку: документ должен остаться schema v1.
3. Rich-блок в отдельной тестовой заметке: новая revision должна стать schema v2.
4. Загрузку и повторное открытие изображения.
5. Экспорт `.wstudio` v3 и импорт его как независимой копии.
6. Counts ключевых таблиц и количество MinIO objects до/после обновления.

## Откат

Additive migration 019 может оставаться в БД при откате binary. Все документы,
которые не использовали новый блок, остаются v1 и доступны старому release.
Revision с rich-блоком имеет schema v2 и требует 0.2.0+; для полного отката после
начала использования v2 нужно восстановить предрелизный PostgreSQL/MinIO backup.

## Локальная верификация

- `bun run check` — PASS;
- `bun test ./src/` — PASS, 114 tests;
- `bun run build` — PASS;
- `go test ./...` — PASS;
- targeted compatibility, sanitizer, assistant transfer, `.wstudio` media и
  OpenAPI tests — PASS.

## LAN deployment

Production switch выполнен 2026-08-05 на `127.0.0.1` /
`https://studio.example.test` штатным `./deploy/update-server.sh`.

- PostgreSQL и MinIO сохранены до переключения в
  `postgres-20260805T084557Z.sql.gz` и
  `object-data-20260805T084557Z.tar.gz`; сам deploy дополнительно создал
  `postgres-20260805T085500Z.sql.gz` и
  `object-data-20260805T085500Z.tar.gz`. Последняя пара прошла `gzip -t` /
  `tar -tzf`.
- Предыдущий source оставлен для rollback в
  `/path/to/apps/writers-studio.rollback-20260805T084632Z`; отдельная полная
  source-копия — `/path/to/apps/writers-studio.previous-20260805T084632Z`.
- Migration 019 применена (`migration_max=19`), `project_media=0` сразу после
  обновления.
- До/после совпали counts существующих данных: `projects=9`,
  `document_nodes=79`, `document_revisions=633`, `dictation_sessions=7`,
  `transcript_artifacts=5`, `audio_assets=6`, `document_annotations=12`,
  `assistant_runs=16`, `project_memory_records=22`.
- В MinIO application bucket до/после совпали имена и SHA-256 всех `456`
  файлов; изменения служебных `.minio.sys` usage/trash metadata исключены из
  сравнения пользовательских объектов.
- App, PostgreSQL и MinIO healthy; внутренний и публичный HTTPS health отвечают
  `{"status":"ok"}`, корневая страница — HTTP 200. После switch существующий
  сетевой проект продолжил успешные API-запросы.
- Production bundle содержит Mermaid и table UI; закрытый media endpoint без
  сессии отвечает 401.
