# «Студия автора»

Рабочая среда для создания длинных текстов: Canvas-доски для замысла, локально надёжная запись, live-текст, финальная расшифровка, проверка по аудио, полноценный manuscript-редактор, версии, поиск, замена и экспорт.

Статус реализации: dictation MVP и его AC-01…AC-55 реализованы и сведены с
проверками в [TRACEABILITY.md](./docs/implementation/TRACEABILITY.md). Assistant
Alpha реализован как implementation candidate: live Voice/Chat run, project MCP,
staged review/apply/undo, guide/memory/dictionary, dynamic ProjectStructure и
пользовательские OpenAI-compatible providers. Нормативный scope дизайна и runtime —
WF-20…WF-33 / AC-56…AC-142, включая docked manuscript + Assistant workspace,
selection attachment, live non-canonical preview и review внутри того же Studio.
WF-34…WF-36 / AC-143…AC-167 также реализованы: inline-ссылки словаря, заметки на
полях и общий проектный список правок описаны в
[MANUSCRIPT_ANNOTATIONS.md](./docs/specification/MANUSCRIPT_ANNOTATIONS.md); они
хранятся отдельно от Tiptap JSON и переносятся в `.wstudio` v2 с поддержкой импорта
v1. Assistant может прочитать все заметки проекта через отдельные MCP-функции,
подготовить до 50 точных исправлений и атомарно обновить текст, якоря и audit
history с полноценным Undo. Действия над выделением появляются только в контекстном меню по правому клику,
`Shift+F10`/клавише меню или долгому нажатию — постоянной панели над текстом нет.
Редактор документов v2 добавляет таблицы, блоки и строчный код, изображения
проекта, Mermaid-схемы и callout-блоки во всех типах проектов, включая
`game_design` и заметки. Старые документы остаются на schema v1, пока автор не
вставит новый rich-блок; перенос изображений появился в `.wstudio` v3. Текущий
формат `.wstudio` v6 также переносит Canvas-доски и продолжает принимать архивы
v1–v5. Контракт и гарантии совместимости описаны в
[RICH_DOCUMENT_EDITOR.md](./docs/specification/RICH_DOCUMENT_EDITOR.md).
Известные release/design gaps перечислены в traceability. Финальный
Docker-прогон и актуальные coverage-данные описаны в
[VERIFICATION_REPORT.md](./docs/implementation/VERIFICATION_REPORT.md).

## Архитектура

- React 19 + TypeScript + Vite PWA.
- React Flow (`@xyflow/react`) и JSON Canvas-compatible snapshots для досок.
- Tiptap 3 / ProseMirror с source references на уровне блоков.
- Go modular monolith: HTTP API, WebSocket live gateway и background worker в одном процессе.
- PostgreSQL для проектов, revisions, transcript artifacts, jobs и пользовательских настроек.
- S3-compatible object storage (MinIO локально) для чанков, собранного аудио и
  изображений проекта.
- IndexedDB для локального manifest и аудиочанков до подтверждённой синхронизации.
- Capability-based STT interface: Deepgram Nova-3 для настоящего live-streaming,
  OpenRouter для явной повторной расшифровки/fallback и детерминированный fake adapter
  для тестов.
- Server-side Assistant Orchestrator с Responses/Chat Completions adapters и
  локальным in-process MCP server поверх application services.
- Семантический `assistant.begin_turn` без regex/verb routing; exact text changes
  используют verified anchors, expected text, staged diff и protected ranges.

Аудио, immutable source transcript, редактируемый transcript draft и manuscript revision хранятся как разные сущности. Поздний batch-результат не перезаписывает авторскую правку.

## Быстрый запуск

Требуются Docker Engine и Docker Compose v2.

```bash
cp .env.example .env
docker compose up --build -d
docker compose ps
```

Приложение: <http://localhost:8080>. Создайте локальный аккаунт на экране входа.

Проверка здоровья:

```bash
curl -fsS http://localhost:8080/api/health
```

Остановка без удаления данных:

```bash
docker compose down
```

Полностью чистый запуск удаляет локальные PostgreSQL/MinIO volumes:

```bash
docker compose down -v
docker compose build --no-cache
docker compose up -d
```

## Развёртывание в локальной сети

Production-стек для сервера `127.0.0.1` описан в
`docker-compose.server.yml`. После добавления
`127.0.0.1 studio.example.test` в `/etc/hosts` клиентского устройства
приложение доступно по <https://studio.example.test>. HTTPS обязателен для
браузерного доступа к микрофону.

Первичная инициализация создаёт отдельный production `.env` со случайными
паролями PostgreSQL и MinIO, сохраняя серверные ключи провайдеров:

```bash
cp .env .env.providers
./deploy/init-server-env.sh
./deploy/update-server.sh
```

Перед каждым последующим обновлением запускайте:

```bash
./deploy/update-server.sh
```

Скрипт сначала сохраняет PostgreSQL и MinIO в `backups/`, затем пересобирает
приложение и ждёт успешный healthcheck. Ручной backup:

```bash
./deploy/backup-server.sh
```

Рукописи и аудио находятся не в контейнерах, а в именованных томах
`writers-studio-postgres-data` и `writers-studio-object-data`. Обычные
`docker compose up`, пересборка и пересоздание контейнеров эти тома не удаляют.
На production-сервере нельзя выполнять `docker compose down -v` или удалять эти
тома вручную.

Для доверенного HTTPS скопируйте с сервера
`writers-studio-local-ca.crt`, импортируйте его в системное хранилище сертификатов
клиентского устройства и пометьте доверенным. Без доверия к локальному CA браузер
не разрешит микрофон на сетевом IP.

## Переменные окружения

Два STT-контура выбираются независимо. `LIVE_STT_ADAPTER=auto` использует Deepgram,
когда в `.env` есть `DEEPGRAM_API_KEY`; `STT_ADAPTER=auto` выбирает OpenRouter для
явной повторной расшифровки и аварийной финализации, когда есть
`OPENROUTER_API_KEY`. Успешно завершённая Deepgram-сессия не отправляет аудио в
OpenRouter автоматически. Без соответствующих ключей включается adapter
`disabled`: preflight честно сообщает, что распознавание не настроено, и не
подставляет fixture-текст. `.env` исключён из Git; секреты не должны попадать в
логи, traces или screenshots.

| Переменная | Назначение | Значение по умолчанию |
|---|---|---|
| `APP_ENV` | Имя runtime-окружения | `development` |
| `HTTP_ADDR` | Адрес Go HTTP server | `:8080` |
| `DATABASE_URL` | PostgreSQL DSN | локальный `writers_studio` |
| `OBJECT_STORAGE_ENDPOINT` | S3/MinIO endpoint | `localhost:9000` |
| `OBJECT_STORAGE_ACCESS_KEY` | S3 access key | локальное значение |
| `OBJECT_STORAGE_SECRET_KEY` | S3 secret key | локальное значение |
| `OBJECT_STORAGE_BUCKET` | Bucket исходного аудио | `writers-studio` |
| `OBJECT_STORAGE_USE_SSL` | TLS для object storage | `false` |
| `ALLOW_TEST_ADAPTERS` | Разрешить `fake`/`fixture` только в non-production test runtime | `false` |
| `STT_ADAPTER` | `auto`, `fake` или `openrouter` | `auto` |
| `LIVE_STT_ADAPTER` | `auto`, `fake`, `deepgram` или `openrouter` fallback | `auto` |
| `OPENROUTER_API_KEY` | Серверный ключ OpenRouter | пусто |
| `OPENROUTER_STT_MODEL` | Модель финального file transcription | `openai/whisper-large-v3` |
| `DEEPGRAM_API_KEY` | Серверный ключ Deepgram | пусто |
| `DEEPGRAM_STT_MODEL` | Модель настоящего live-streaming | `nova-3` |
| `LLM_ADAPTER` | `auto`, `disabled`, `openai_compatible`; `fixture` только для test/CI и запрещён в production | `auto` |
| `LLM_BASE_URL` | System OpenAI-compatible endpoint для Assistant | пусто |
| `LLM_API_KEY` | Server-only credential Assistant | пусто |
| `LLM_MODEL` | Точный model ID системного Assistant provider | пусто |
| `LLM_DIALECT` | `chat_completions` или `responses` | `chat_completions` |
| `PUBLIC_BASE_URL` | Публичный origin приложения | `http://localhost:8080` |
| `PUBLIC_HOST` | Хост HTTPS-прокси без схемы | `studio.example.test` для LAN |
| `MCP_ENABLED` | Включить stateful Streamable HTTP endpoint `/mcp` | `false` |
| `MCP_OAUTH_SECRET` | Отдельный secret для OAuth consent; минимум 32 символа при включённом MCP | пусто |

## Удалённый MCP в LAN/VPN

Remote MCP включается только явно и рассчитан на доверенную локальную сеть или
VPN. Публичный интернет и облачные агенты в этот контур не входят. Клиент должен
разрешать `studio.example.test` на адрес сервера и доверять тому же локальному CA,
который используется веб-приложением.

После установки `MCP_ENABLED=true` откройте «Настройки → Агенты и MCP», выберите
существующие проекты и права, затем сохраните одноразово показанный bearer-токен.
Минимальная конфигурация MCP-клиента:

```json
{
  "writers-studio": {
    "url": "https://studio.example.test/mcp",
    "headers": {
      "Authorization": "Bearer wsmcp_..."
    }
  }
}
```

Клиенты с OAuth используют discovery по
`/.well-known/oauth-protected-resource` и Authorization Code + обязательный
S256 PKCE. Сервер поддерживает Client ID Metadata Documents и Dynamic Client
Registration; проверка OAuth-запроса выполняется ORY Fosite. Access token
действует час, refresh token — 30 дней и ротируется при использовании. Grant
неизменно ограничен выбранными проектами; для изменения allowlist старый grant
отзывается и создаётся новый.

Рабочий цикл полностью доступен через MCP: `workspace.open_project` →
`assistant.begin_turn` → project tools → `assistant.finish_turn` →
`change_set.review` → `change_set.apply`/`reject`; для applied change set новый
review token разрешает `change_set.undo`. Ни один staged-вызов не меняет
канонический проект до Apply.

## STT

`fake` — только явный детерминированный adapter для CI и failure/retry сценариев.
Он требует `ALLOW_TEST_ADAPTERS=true` и non-production `APP_ENV`; production
конфигурация с тестовым adapter отклоняется при запуске.

`deepgram` — настоящий live-контур. Browser AudioWorklet отправляет небольшие mono
PCM16 transport-frames защищённому WebSocket gateway, но сервер держит одно
непрерывное соединение с Deepgram на всю запись. Nova-3 сохраняет контекст между
frames и возвращает изменяемые `partial`, финальные фрагменты, пунктуацию,
word timestamps и confidence. Partial заменяется на месте, а соседние финальные
фрагменты склеиваются пробелами в один prose-run и один абзац draft; новую границу
абзаца создаёт только структурная команда.

После успешного Deepgram `Finalize` сохранённые `segment_final` становятся
immutable `live_final` и источником первой draft revision. Автоматического
повторного распознавания и расхода OpenRouter-токенов нет. `openrouter` остаётся
отдельным file-STT adapter для кнопки «Повторить расшифровку со словарём» и fallback,
если live-поток отсутствовал или не подтвердил финализацию. Оба ключа используются
только сервером:

```bash
LIVE_STT_ADAPTER=deepgram STT_ADAPTER=openrouter docker compose up --build -d
```

Изолированный smoke test принимает короткий аудиофайл и выводит только provider/model и размеры результата:

```bash
set -a
source .env
set +a
go run ./cmd/openrouter-smoke /absolute/path/to/short-audio.wav
```

Если ключ Deepgram отсутствует, OpenRouter можно оставить как честно обозначенный
`chunked_live` fallback с независимыми WAV-окнами. Локальная MediaRecorder-запись не
зависит ни от одного live-провайдера и продолжается при сбое live-текста.

Настоящий browser E2E принимает WAV как Chromium microphone input, проверяет узнаваемые
слова в Deepgram live UI, `realtime/pcm16` capabilities, один склеенный prose-run,
отсутствие fixture-текста, отсутствие автоматического `batch_source`, одну revision
`live_source` и timestamps:

```bash
REAL_STT_AUDIO_FILE=/absolute/path/to/russian-speech.wav \
  bun run e2e:real-stt
```

## Тестирование

Установка frontend dependencies:

```bash
bun install --frozen-lockfile
```

Статические проверки, unit tests и production build:

```bash
bun run check
bun run type-check
bun run test
bun run test:coverage
bun run test:canvas:coverage
bun run build
go test ./...
```

Integration test требует PostgreSQL. Для работающего Compose-проекта:

```bash
docker run --rm \
  --network writers-studio_default \
  -v "$PWD:/src" \
  -w /src \
  -e TEST_DATABASE_URL='postgres://writers:writers@db:5432/writers_studio?sslmode=disable' \
  golang:1.26-alpine \
  go test -tags=integration ./internal/store -count=1
```

Полная E2E-матрица запускается в отдельном временном Compose project на
`http://127.0.0.1:18080`; скрипт сам поднимает gated fixture adapters и удаляет
тестовые containers/volumes после прохода, не затрагивая ручной стенд `:8080`:

```bash
bun run e2e:compose
```

Viewports: `1440×1024`, `1024×768`, `834×1194`, `390×844`. Screenshots критических сценариев сохраняются в `artifacts/`, traces ошибок — в `test-results/`.

## Данные, backup и удаление

- Настройки позволяют выбрать срок хранения исходного аудио: 30, 90 или 365 дней.
- Применение срока удаляет только истёкшие server-side audio assets; локальные unsynced chunks остаются под контролем Recovery Center.
- Account backup скачивается как ZIP с `backup.json` и доступным полным аудио.
- Archive и trash — разные состояния. Физический purge требует точного подтверждения и создаёт audit event.
- Удаление аккаунта требует точного ввода email, транзакционно удаляет проекты,
  ревизии, сессии, артефакты и фоновые задачи, затем удаляет связанные object-storage
  assets и очищает локальные IndexedDB/localStorage. В audit остаётся только
  обезличенное событие `account_purged`.
- UI не обещает мгновенное исчезновение копий из внешних backup-систем.

## Документы

| Документ | Назначение |
|---|---|
| [PRODUCT_BRIEF.md](./PRODUCT_BRIEF.md) | Видение, аудитория и границы MVP |
| [WORKFLOWS.md](./docs/specification/WORKFLOWS.md) | Workflow, правила и acceptance criteria |
| [ASSISTANT_MODE.md](./docs/specification/ASSISTANT_MODE.md) | Assistant scope, MCP catalog, risk и provider contract |
| [PROJECT_NAVIGATION_AND_TRANSFER.md](./docs/specification/PROJECT_NAVIGATION_AND_TRANSFER.md) | Кнопка «Назад» и переносимый формат `.wstudio` |
| [MARKDOWN_TREE_MIGRATION.md](./docs/specification/MARKDOWN_TREE_MIGRATION.md) | Двусторонняя миграция Markdown / Obsidian с деревом папок |
| [DESIGN_HANDOFF.md](./docs/design/DESIGN_HANDOFF.md) | Компоненты, responsive и accessibility |
| [ARC42.md](./docs/architecture/ARC42.md) | Архитектурная спецификация |
| [ADR-001](./docs/adr/ADR-001-speech-to-text.md) | Live/batch STT |
| [ADR-002](./docs/adr/ADR-002-editor-engine.md) | Tiptap/ProseMirror |
| [ADR-003](./docs/adr/ADR-003-assistant-mcp.md) | Local MCP bridge и staged change sets |
| [ADR-007](./docs/adr/ADR-007-assistant-continuous-workspace.md) | Непрерывный Assistant flow, compact panel и modal review |
| [TRACEABILITY.md](./docs/implementation/TRACEABILITY.md) | WF/AC → код → тест → ручная проверка |
| [VERIFICATION_REPORT.md](./docs/implementation/VERIFICATION_REPORT.md) | Финальные команды, результаты и ограничения |
