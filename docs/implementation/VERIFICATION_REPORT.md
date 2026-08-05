# Verification report

Дата: 2026-08-04  
Runtime: Docker Desktop, PostgreSQL 17, MinIO, Go 1.26, Bun 1.3.14, Chromium/Playwright.

## Delta 0.2.0 — 2026-08-05

Rich document editor проверен локально: Biome PASS, production Vite/TypeScript
build PASS, frontend 114/114 PASS и полный `go test ./...` PASS. Отдельные
регрессии подтверждают сохранение schema v1 при обычной правке, ленивый переход
на v2, безопасный assistant transfer, HTML allowlist и `.wstudio` v1/v2/v3 с
checksum/MIME/dimension validation и remap project media. Migration 019 additive;
LAN deployment выполнен 2026-08-05 на `127.0.0.1`: pre-switch и deploy
PostgreSQL/MinIO backups проверены, migration 19/19 применена, app/db/storage
healthy, публичный HTTPS health PASS, а counts всех существующих project/document,
dictation, annotation, Assistant и memory records совпали до/после. Имена и
SHA-256 всех 456 application objects в MinIO также совпали. Runbook:
[`RELEASE_0.2.0.md`](./RELEASE_0.2.0.md).

## Итог

Функциональный dictation MVP реализован по P-01…P-04, S-01…S-17 и AC-01…AC-55.
Assistant Alpha implementation candidate покрывает S-18…S-41, Mobile P-07/P-10/P-11
и 176 из 176 сценариев baseline, включая WF-33 / AC-132…AC-142: floating Studio
без панели, docked Assistant рядом с mounted manuscript, selection attachment,
live non-canonical preview, compact panel typography, direct Apply + Undo и modal
review без route/URL transition, а также WF-34…WF-36 / AC-143…AC-167: словарные
ссылки, заметки на полях, общий список проекта, exact deep links и безопасный
перенос, Assistant MCP mass-fix, versioned anchors и audit-safe Apply/Undo, а также
WF-37 / AC-168…AC-176: несколько независимых чатов проекта, atomic first turn,
переключение без остановки работы, изолированные draft/mode/selection/review,
responsive switcher и server-enforced stale/rebase flow.
Действия выделения открываются только контекстно; постоянной панели над текстом
нет. Локальная production-сборка и изолированная четырёхразмерная E2E-матрица
прошли финальную проверку.
Подробная связь требований с кодом и доказательствами:
[`TRACEABILITY.md`](./TRACEABILITY.md).

Финальный статус проверок:

| Проверка | Результат |
|---|---|
| Biome | PASS |
| TypeScript | PASS |
| Frontend unit | PASS, 107 tests |
| Frontend coverage | 69.36% functions, 73.06% lines для всего legacy-inclusive frontend; новая критическая логика: Assistant annotation review 100%, context-menu 100%, annotation decorations/anchors 99.40% lines и autosave anchor mapping 96.69% lines |
| Go unit | PASS |
| Go coverage | `internal/documentannotation` — 96.9%, `internal/projectmcp` — 91.0% statements; package-wide legacy wiring ниже и не выдаётся за 90% |
| PostgreSQL integration | PASS, включая annotation MCP reads/staging, atomic document+anchor+lifecycle Apply/Undo/history, stale guards, lifecycle/ownership/revision safety, deleted-term snapshot и `.wstudio` v1/v2 round-trip/remap; проверены 69/69 Assistant tools и durable voice reconnect |
| Production frontend build | PASS |
| Docker image/build/health | PASS, production Compose rebuilt and healthy; isolated E2E Compose used clean volumes and removed them after the run |
| LAN production deployment | PASS, 2026-08-04, `127.0.0.1` / `https://studio.example.test`; два проверенных PostgreSQL/MinIO backup перед switch, migration 17/17, app/db/storage healthy после контрольного restart, Caddy HTTPS `/api/health` и новый JS asset — 200 |
| 0.2.0 rich editor LAN deployment | PASS, 2026-08-05: migration 19/19, app/db/storage healthy, public HTTPS health и root 200, media auth guard 401, production bundle содержит Mermaid/table UI; counts до/после совпали (`9` projects, `79` nodes, `633` revisions и все связанные dictation/annotation/Assistant/memory records), как и SHA-256 всех `456` application objects MinIO |
| WF-37 data preservation | PASS: до/после deployment совпали counts и MD5 всех основных project tables — 5 projects, 20 nodes, 557 document revisions, transcripts, audio metadata, memory, annotations и Assistant turns; MinIO bucket — 456 files и тот же SHA-256 |
| Playwright 4-viewports | PASS, 112/112, retries disabled, 6.5 min; чистые временные DB/storage удалены после прохода |
| Multi-session Assistant | PASS, 8/8: два независимых чата и atomic first turns, per-run draft/mode/selection, responsive desktop/tablet/mobile switcher, параллельные proposals, серверный `409 stale_change_set` и refresh turn |
| Multi-session visual QA | PASS: локальная production-сборка вручную сверена на 1440×1024 и 390×844; desktop panel 360 px, mobile panel 390 px без horizontal overflow, manuscript остаётся mounted |
| Manuscript annotations / context menu | PASS: обычное выделение не показывает кнопок; right-click открывает 3 `menuitem`, `Escape` закрывает, `Shift+F10` открывает повторно; общий список и exact deep link проверены E2E × 4 и вручную в production build |
| Annotation design | PASS: `design.pen` S-31 показывает компактное контекстное меню, S-31B — выбор термина, S-37 — Assistant review note/exact before-after/per-note resolve; persistent selection toolbar удалён, Pencil diagnostics без layout problems |
| Global project-scope delta | PASS, 4/4 viewports внутри consolidated 96-test run: dynamic root label, project return path, `targetKind=project`, `targetNodeId=null` |
| Provider-form regression | PASS, 4/4 viewports после финального autocomplete fix |
| Axe WCAG A/AA | PASS в P-01, P-04, S-17 settings и S-27 Project Memory |
| Реальная MiniMax-M3 Assistant probe | PASS на свежей production-сборке: полный strict tool loop, staged review, точный Apply и byte-equal Undo; streaming не подтверждён |
| Реальная MiniMax-M3 context probe | PASS: в одной продолженной Chat-сессии модель точно прочитала always-memory и pinned `ASSISTANT.md` r1; Markdown отрисован, canonical document не изменён |
| Реальная OpenRouter `openai/gpt-5.6-luna` MCP probe | PASS: полный strict catalog, safe wire names, project/memory reads и staged `project_memory.add`; canonical state не применялся |
| Assistant live layout | PASS вручную в локальной production-сборке: page `720/720`, `window.scrollY=0`, отдельный transcript `602/377`, smart autoscroll/jump и 0 Philosopher descendants в transcript |
| Assistant continuous flow | PASS: docked Voice/turn `14 px`, 0 Philosopher descendants в panel; ready card имеет `Проверить изменения` и `Применить сразу`; review — `role=dialog` `1120×688` внутри `1280×720`, Studio и ProseMirror остаются mounted под scrim |
| UX consistency delta | PASS: Settings имеет явный `Назад`; live recording показывает раскрытый список/кнопки команд; форма уточнения скрывает Apply/Reject; `Проверить изменения` заменяет технический label. Изолированный E2E: 4/4 на desktop+mobile без внешнего LLM/STT |
| Display typography / brand | PASS: `Philosopher` Bold 700 загружен локально, title и PWA metadata используют «Студия автора», manuscript/transcript сохраняют собственные font roles |
| Integrated manuscript workspace | PASS вручную и E2E: floating полотно без панели; при docked Assistant три headers `top=56`, `height=56`, `bottom=112`, manuscript `margin=0`, `radius=0`; canonical text не меняется до Apply |
| Реальный system-provider Assistant flow | PASS: `openai/gpt-5.6-sol`, 61 provider tools, selection attachment, точный live proposal, readable review и Reject без canonical commit |
| Большое поручение / дешёвый OpenRouter smoke | PASS: `qwen/qwen3-30b-a3b-instruct-2507`, полный пользовательский fixture 20 803 символа / 37 233 байта, 64 project tools и 3 последовательных provider steps; только принудительные read calls, canonical state не затрагивался |
| Реальный OpenRouter adapter smoke | PASS, timed segment + 12 words |
| Реальный Deepgram browser E2E | PASS, continuous live + склейка + без автоматического batch, 1/1 за 10.3 s |

## Assistant Alpha: проверенный контур

Автоматически проверены:

- обязательный `assistant.begin_turn(answer|change|clarify)` до project tools;
- отсутствие regex/verb/synonym/hyponym и instruction-keyword routing;
- полный policy-разрешённый MCP catalog: 69/69 schemas совпадают с LLM surface,
  каждый tool имеет runtime integration evidence; recursive contract test требует
  strict `additionalProperties=false` и все `required` для каждого вложенного object;
- current-only authoritative input для change и
  `assistant.get_recent_change_set` для исправления предыдущего proposal;
- `document.find_exact` и пакетный `document.find_exact_batch` с block-local
  Unicode anchors; regression loop проходит 13 отдельных чтений и затем staged
  mutation, не упираясь в прежний 12-step guard;
- атомарный `document.replace_range` с `expectedText`, base revision и
  protected ranges; ошибочные offsets не меняют canonical document;
- read-only project tools, structure-derived create/move для seven templates,
  content-bearing free/custom nodes;
- `tree.create_nodes_batch`: шесть коротких content-bearing siblings создаются
  одним scripted-provider call, сохраняют порядок, проходят обычные Apply/Undo;
- persisted `working` после process restart идемпотентно получает recoverable
  `execution_interrupted`; полный пример большого поручения проверен отдельным
  дешёвым read-only smoke без генерации рукописи и без canonical mutations;
- два наблюдаемых локальных provider failures совпали с прежним двухминутным
  timeout до миллисекунд; timeout увеличен до 10 минут без auto-retry, а короткий
  PostgreSQL fixture подтверждает сохранение staged partial review и атомарный Stop;
- полный пользовательский fixture размером 20 803 символа / 37 233 байта прошёл
  OpenRouter Chat Completions на `qwen/qwen3-30b-a3b-instruct-2507`: declaration,
  полный catalog из 64 project tools и продолжение после synthetic tool result;
  smoke принудил только `project.get_overview` и `tree.list`, поэтому проект не
  изменялся и длинная художественная генерация не оплачивалась;
- реальный Opus-run с запросом примерно на 10 000 слов успешно выполнил семь read
  tools, но первый генеративный provider step был закрыт upstream ровно через
  120 секунд даже при локальном timeout 10 минут; это зафиксировало отдельную
  причину и привело к `document.append_chunk`, compact replay и accumulated preview;
- PostgreSQL scripted regression объединяет два long-form чанка в одну operation и
  одну revision после Apply, проверяет Undo и сохраняет первый чанк как partial
  review при последующем provider timeout; реальная дорогая генерация не повторялась;
- после production Compose rebuild реальный пользовательский Opus-turn подтвердил
  новый протокол без отдельного платного smoke: два последовательных
  `document.append_chunk` (545 + 471 слово) были объединены в одну staged operation
  на 1 016 слов и завершились в `awaiting_review`; до пользовательского Apply
  canonical документ остался неизменным;
- последующий более тяжёлый Opus-turn прочитал инструкцию, заметки и две сцены через
  девять успешных tools, но первый генеративный вызов снова был оборван ровно через
  120 секунд до первого чанка. Поэтому Chat Completions переведён на SSE; unit
  regression проверяет OpenRouter keepalive comments, фрагментированные text/tool
  arguments, сборку opaque `reasoning_details`, terminal marker, неоконченный stream
  и безопасную mid-stream диагностику 524;
- staged document/tree/dictionary/memory/guide changes, atomic Apply, reject и
  grouped Undo;
- единый browser E2E `turn → document.find_exact → staged exact diff → Apply →
  grouped Undo`: canonical document неизменен до Apply, содержит только точную
  замену после Apply и byte-for-byte возвращается к исходному тексту после Undo;
- prepare-only A3 handoffs для archive/restore/retranscribe/export без прямого
  canonical commit, с точным export/version prefill и stale restore guard;
- удаление Assistant run history без отката applied revisions, restore guide как
  новая revision, typed dictionary/memory conflicts и dependency-aware partial apply;
- Responses и Chat Completions adapters, encrypted credentials и sequential
  tool-loop probe;
- bounded second probe для транзиентного provider failure и password-manager-safe
  custom-provider form (`new-password`, scoped names и ignore contracts);
- durable Assistant run/events, project-wide activity, document+transcript search;
- voice-ready/finalize handshake, configurable/durable silence checkpoint,
  безопасный run-pinned preferences snapshot и manual edit,
  reconnect/continue/discard, Chat → Voice → Chat handoff без auto-submit;
- две точные protocol-команды «Команда, выполнить поручение» и «Команда,
  продолжить»; похожая естественная речь не маршрутизируется регулярками;
- безопасный GFM Markdown, stripping `<think>/<analysis>`, human-readable
  activity и exact-fragment review presentation.

Дополнительный real-provider проход на локальной production-сборке проверил не
только tool mutation loop, но и проектный контекст. В одной продолженной Chat-
сессии MiniMax-M3 назвал Элиару, правило возвращения до рассвета и правило
непрямолинейных диалогов из Project Memory, затем отдельно воспроизвёл pinned
правило `ASSISTANT.md` r1 о последней реплике. Ответ отрисован как Markdown,
сессия сохранила оба turn, исходные второй абзац и последняя реплика документа
остались неизменными.

OpenRouter `openai/gpt-5.6-luna` отдельно прошёл объявление полного strict MCP
catalog после преобразования внутренних dotted names в provider-safe wire names.
В реальном Assistant run модель прочитала Project Memory и ответила именем
персонажа; во втором run вызвала `project_memory.add`, создала staged change set и
перешла в `awaiting_review`. Тестовый proposal был отклонён без canonical commit.
Отсутствие прежнего глобального `provider_step_limit` закреплено интеграционным
тестом: `begin_turn → 25 read calls → project_memory.add → final response`.
Отдельный сценарий проверяет остановку с `tool_limit_exceeded`, когда в проекте
явно задан меньший бюджет.
Отдельная high-cardinality регрессия передаёт 42 dictionary items и 42 memory
items двумя batch tool calls. Четыре внутрипакетных дубликата отсекаются с
per-item conflict semantics, остальные 80 записей становятся независимыми
operations; Apply и grouped Undo проходят через обычный canonical write path.

Реальный configured OpenAI-compatible provider MiniMax-M3 на свежей production-сборке
и проекте типа `free` прошёл цепочку `begin_turn → project.search → document.get →
document.find_exact → document.replace_range`. Все пять MCP-вызовов завершились без
ошибок; run перешёл в `awaiting_review`, а журнал сохранил последовательность
`staged → applied → undone`. Изменена только точная фраза во втором абзаце,
последняя реплика осталась нетронутой. После grouped Undo PostgreSQL revisions до
Apply и после Undo совпали и по `content`, и по `plain_text`; до Apply canonical
revision не менялась.

Targeted frontend modules:

| Module | Functions | Lines |
|---|---:|---:|
| Assistant review presentation | 100% | 100% |
| Assistant session presentation | 100% | 95.03% |
| Annotation decorations/anchors | 100% | 99.40% |
| Deterministic Go anchor mapping | — | 96.9% statements |
| MCP catalog/policy | — | 91.0% statements |
| Assistant Markdown | 100% | 100% |
| Project Memory UI model/payload | 100% | 100% |
| `ASSISTANT.md` local draft recovery | 100% | 97.56% |

Package-wide frontend/HTTP/store percentages ниже 90%, потому что denominator
включает legacy UI barrels, wiring и unrelated dictation surface. Порог 90%
проверяется также по нормативным acceptance scenarios; он не подменяет
package-wide line coverage.

Критерии AC-76, AC-80, AC-93, AC-99 и AC-117, ранее отмеченные как неполные,
закрыты отдельными backend/store/UI regression tests. WF-37 / AC-168…AC-176
дополнительно закрыты server/store/client/unit/E2E проверками. Итоговое coverage
реализованного baseline — 176/176 PASS.
Это не подменяет опубликованные package-wide coverage percentages.

## Контрольный Docker lifecycle

Выполнен чистый lifecycle:

```bash
docker compose -p writers-studio-final down -v
docker compose -p writers-studio-final build --no-cache app
docker compose -p writers-studio-final up -d
# После сбора доказательств временный проект обязательно удаляется:
docker compose -p writers-studio-final down -v --remove-orphans
```

`writers-studio-final` — только временное имя изолированного verification stack.
Оно не должно оставаться рядом с основным `writers-studio` после завершения
прохода. Та же teardown-гарантия применяется к любому другому тестовому project
name; основной Compose и его volumes такой командой не затрагиваются.

Результат:

- PostgreSQL, MinIO и app перешли в `healthy`;
- в текущем clean-room stack применены 17 additive embedded SQL migrations;
- production Vite bundle и статический Go binary собраны внутри multi-stage image;
- после рестарта app и повторного одновременного рестарта services приложение
  восстановилось по `restart: unless-stopped`;
- контрольные counts до/после рестарта не изменились:
  `projects=54`, `document_revisions=109`, `dictation_sessions=24`,
  `transcript_artifacts=44`, `audio_assets=20`, `background_jobs=36`;
- после возврата из детерминированного fake-регресса runtime повторно запущен без
  override и сообщает cloud-capabilities
  `realtime,file`, protocol `pcm16`, partial results и segment/word timestamps.

## Автоматизированные проверки

Команды:

```bash
bun install --frozen-lockfile
bun run check
bun run type-check
bun run test
bun run test:coverage
bun run build
go test ./...
go test -coverprofile=/tmp/writers-studio-cover.out ./...
go tool cover -func=/tmp/writers-studio-cover.out

docker run --rm \
  --network <compose-network> \
  -e TEST_DATABASE_URL='<docker-internal-test-url>' \
  writers-studio-integration-test \
  go test -tags=integration -p=1 ./internal/store ./internal/assistantapp ./internal/httpapi -count=1

bunx --bun playwright test --retries=0
REAL_STT_AUDIO_FILE=/absolute/path/to/russian-speech.wav \
  bun run e2e:real-stt
```

Текущая E2E-матрица содержит 28 сценариев × 4 viewport = 112 tests. Последний
полностью объединённый проход выполнен в отдельном fixture-based Compose project
с чистыми volumes и охватил все 112 tests:

- P-01: project → dictation → review → transfer → edit → durable reload;
- P-02: WebSocket/network loss, continued local capture, retryable batch failure;
- P-03: protected author draft, late batch, diff accept/undo, retranscribe,
  transfer undo, named version, compare, restore;
- P-04: mobile quick action → capture → review;
- offline/local-only, crash/new-tab recovery, local audio download;
- denied/missing microphone, low storage, stale revision conflict copy;
- явное ожидание browser permission в Dictation и отмена позднего microphone
  result в Assistant Voice без ложного состояния «Слушаю»;
- отдельное secure-origin состояние для LAN HTTP с переходом на localhost;
- discoverability Context/`ASSISTANT.md`/Project Memory/AI settings и Axe;
- Assistant Live: видимая стенограмма, Chat → Voice → Chat без отправки модели,
  configurable/durable silence checkpoint, безопасный preferences snapshot,
  edit/reconnect/continue того же поручения;
- Assistant exact change: semantic turn, точный staged diff, отсутствие canonical
  mutation до review, modal Apply/Undo без URL transition и второй direct
  Apply/toast Undo в одном browser-сценарии;
- Assistant multi-session: два независимых project chats с атомарным первым turn,
  переключением без Stop, восстановлением per-run draft/mode/URL selection и
  responsive 360 px/full-width panel; конфликт двух proposals подтверждает
  серверный stale guard, `r2 → r3`, defer и refresh через новый turn;
- manuscript annotations: контекстное меню без persistent toolbar, реальная
  dictionary link, заметка, общий список проекта и exact deep link;
- exact voice-command grammar/undo and keyboard map;
- голосовая навигация к произнесённому предложению и возврат в live-диктовку;
- project-wide replace preview/apply/highlight/undo;
- document/project lifecycle, structure move/caret/undo, archive/trash/restore/purge;
- dictionary, versions, search, TXT/Markdown/DOCX export, persistent writer
  preferences, backup и полное удаление аккаунта.

Финальная команда `bun run e2e:compose` завершилась:

```text
112 passed (6.5m)
```

P-01 измеряет browser-clock latency от click по `Начать запись` до видимого recorder UI
и требует `≤300 ms` во всех четырёх viewport. Он также сравнивает selection/scroll до
и после server acknowledgement и проверяет source refs после reload.

## Визуальная проверка

Production screenshots live/review и responsive acceptance были сопоставлены с
desktop/mobile frames в `design.pen` через Pencil. По результату:

- live-экран получил отдельную строку online/save status, бумажную поверхность
  transcript и нижний inset recorder dock;
- review получил общий нижний audio/action bar и desktop transfer action;
- Pencil layout inspection не обнаружил clipping/overflow;
- повторная диагностика охватила 30 актуальных desktop, compact, tablet и mobile
  frames; все прошли без layout problems;
- отдельная повторная диагностика 21 Assistant frame S-18…S-30 также вернула
  0 layout problems;
- добавлены и отдельно проверены три состояния текущего прохода: S-30 Assistant
  Context Hub, desktop insecure-origin preflight и mobile insecure-origin preflight;
  Pencil diagnostics для каждого вернули 0 layout problems;
- повторный критический responsive pass: 12/12, включая Axe;
- S-17 settings сопоставлен с Pencil hierarchy: секционная навигация, dictation/editor
  preferences, local storage, retention и privacy actions.

В actual browser screenshots проверено:

- desktop workspace: editor, tree, header и actions визуально согласованы;
- в локальной production-сборке вручную открыты отдельные Context,
  `ASSISTANT.md`, Project Memory, AI providers и Assistant Voice/Chat; реальный
  MiniMax turn прошёл поиск и точную правку через пять MCP-шагов, review, Apply и
  Undo без browser warnings/errors; отдельная продолженная Chat-сессия прочитала
  Project Memory и pinned guide без скрытой мутации документа;
- `390×844` и `834×1112`: нет horizontal overflow, preflight использует dynamic
  viewport height, settings и manuscript остаются доступны;
- после исправления PWA-кэширования новых console warnings/errors не возникло;
- responsive tree и compact controls не перекрывают manuscript/editor;
- опасные действия имеют явное подтверждение и не маскируются обычным primary action.
- S-38/S-39/S-40/S-41 вручную сверены на `1440×1024` и `390×844`: секции и
  статусы читаемы, duplicate close отсутствует, счётчик использует корректное
  склонение, desktop panel имеет ширину 360 px, mobile не создаёт overflow.

Screenshots основных E2E-состояний сохраняются в `artifacts/`; traces/video создаются
для ошибок в `test-results/`.

## Реальный Deepgram live как финальный source transcript

Реальный запрос к OpenRouter прошёл:

```text
OpenRouter STT smoke passed: provider=openrouter model=openai/whisper-large-v3 characters=98 segments=1 timed_segments=1 words=12
```

Отдельный Playwright-сценарий передал тот же русский WAV как Chromium microphone input
в production UI. Live-аудио прошло как headerless mono PCM16 через один Deepgram
WebSocket-сеанс; после подтверждённого `Finalize` тот же Deepgram transcript стал
финальным источником без повторной отправки аудио в OpenRouter. Проверено:

- runtime сообщает `modes=realtime,file`, `streamingProtocol=pcm16`,
  `partialResults=true`, `vocabularyHints=keyterm`;
- узнаваемые слова `настоящего распознавания` появились в live transcript;
- соседние Deepgram final segments отображаются одним prose-run и сохраняются одним
  draft-абзацем, а не сетевыми строками;
- live segments содержат word/segment timestamps;
- fixture `Дом стоял у самой воды` отсутствует в live;
- live artifact содержит ненулевой source range;
- `batch_source` отсутствует;
- история draft содержит одну revision с source `live_source`;
- review открылся без console errors и failed requests.

Результат:

```text
1 passed (10.3s)
```

Ключ не выводился и не передавался клиенту. Adapter также имеет tests на redaction
provider body/credential, verbose timestamp normalization и безопасный fallback при
несовместимом `verbose_json`. Использован официальный контракт OpenRouter:
<https://openrouter.ai/docs/guides/overview/multimodal/stt>.

## Дефекты, найденные контрольным проходом

Во время проверки были найдены и исправлены:

1. Bun подхватывал Playwright specs как unit tests — scripts ограничены `bun test src`.
2. Source highlight менял ProseMirror JSON — заменён на transient Decoration.
3. Mobile preflight мог обрезать нижний CTA — применены `100dvh`, flex min-height и
   fixed footer.
4. Цвет secondary text не проходил требуемый contrast — token скорректирован.
5. Дублированный live-region text делал E2E и screen-reader state неоднозначными —
   роли и exact locators разведены.
6. Structural `part` с revision 0 возвращал SQL `NULL` в non-null Go поля —
   `LEFT JOIN` обёрнут `COALESCE`, добавлен integration regression.
7. Refetch после structural move мог сбросить selection — editor больше не вызывает
   `setContent` для уже загруженных node/revision; добавлен caret regression.
8. Export сортировал sibling position глобально и мог нарушить outline —
   snapshot теперь строится recursive depth-first path; добавлен order/snapshot test.
9. Одновременный restart DB/app мог оставить app остановленным — services получили
   restart policy и self-recovery проверен.
10. Recorder partial/state/duration и source-range playback были недостаточно явными —
    добавлены non-color partial label, screen-reader status, dual-duration semantics и
    bounded playback.
11. Production runtime по умолчанию оставался на fake adapter, а WebSocket игнорировал
    аудио при cloud STT — добавлены capability-based `auto` selection и AudioWorklet
    transport через server gateway.
12. Fixture-фраза могла появляться после любого binary WebSocket blob — теперь этот
    путь доступен только при явном `STT_ADAPTER=fake`; production OpenRouter сохраняет
    исключительно provider result.
13. OpenRouter artifact сворачивался в диапазон `0–0` — включён `verbose_json`,
    нормализованы segment/word timestamps и добавлен duration fallback.
14. Первый real browser assertion ошибочно использовал ASCII-семантику `\w` для
    кириллицы — тест исправлен и повторно прошёл.
15. Временный provider 429/5xx мог перевести финализацию в failed с первой попытки —
    adapter получил ограниченный retry с context-aware backoff, а экран финализации
    сохраняет явный повтор без потери аудио.
16. OpenRouter live fallback резал контекст на независимые WAV-окна — добавлен
    Deepgram Nova-3 native streaming: один upstream WebSocket и 100 ms PCM16
    transport-frames.
17. Каждый provider final создавал визуальный абзац — partial теперь заменяется на
    месте, а соседние финальные фрагменты группируются в один prose-run; команды
    остаются явными границами.
18. Нормальное закрытие Deepgram после browser stop могло выглядеть как provider
    outage в логах — lifecycle reader привязан к отдельному отменяемому контексту.
19. Автоматический OpenRouter quality pass ухудшал хороший Deepgram live-текст и
    расходовал токены — подтверждённый `live_final` теперь сразу становится source
    transcript; OpenRouter оставлен для явного retry и fallback.
20. Batch/fallback мог записать произнесённую структурную команду как прозу —
    финальные segments проходят детерминированное выделение команд перед draft.
21. Provider finals могли стать отдельными draft-абзацами — соседняя проза теперь
    склеивается, а границу создаёт только структурная команда.
22. Добавлена команда «Команда, начало диалога»: она не входит в текст, а следующая
    реплика начинается с русского длинного тире.
23. Встроенный SPA fallback возвращал `index.html` для отсутствующего hashed
    JavaScript и после обновления PWA получался `Unexpected token '<'` — assets теперь
    имеют настоящий 404 и immutable cache, а index/fallback — `no-cache`; добавлен
    HTTP regression test.
24. Production editor не фиксировал schema version и мог занести небезопасную
    разметку из Word/Google Docs — добавлены version gate, semantic paste allowlist,
    stable block ids и тесты provenance через join/split/undo.
25. S-17 хранил только language/retention — добавлены microphone, command/live
    switches, font/size/line-height/width/theme/spellcheck/word goal, сохранение после
    reload и selected-device fallback.
26. Accent/danger foreground tokens в тёмной теме использовались как заливка и
    проваливали Axe contrast — surface tokens разделены от текстовых.
27. Project/account purge мог оставить фоновые задачи — deletion order исправлен;
    полное удаление аккаунта теперь транзакционно очищает БД, object storage и
    локальные browser recovery records, сохраняя обезличенный audit event.
28. Подписи полей входа и создания проекта были визуальными, но не имели устойчивой
    связи с контролами — добавлены `id`/`htmlFor`, а тип проекта оформлен как
    `fieldset` с доступной легендой.
29. Recovery мог повторно показать закэшированное пустое состояние после offline
    записи — запрос теперь всегда обновляется при входе в центр восстановления.
30. На мобильном review статус сохранения был скрыт вместе с desktop header —
    utility shell получил компактный видимый save status.
31. Явно развёрнутая пользователем структура проекта могла снова свернуться после
    reload на compact viewport — сохранённый `false` теперь имеет приоритет над
    breakpoint-default.
32. Системный provider без точного `LLM_MODEL` молча получал несвязанный default —
    конфигурация теперь требует точный model ID; MiniMax-M3 проверена strict loop.
33. LAN HTTP выглядел как поломанный микрофон — preflight теперь отдельно объясняет
    secure-origin правило, блокирует Start и предлагает HTTPS/localhost или Chat.
34. Context, Project Memory, `ASSISTANT.md` и AI settings были спрятаны в общей
    навигации — добавлены самостоятельные маршруты, tabs и явные entry points.
35. Пустая Project Memory возвращалась как `null` и ломала список — API и UI
    нормализуют коллекцию в `[]`, добавлен regression.
36. Старый change set мог содержать `dependencies:null` и падал до review — store
    и presentation нормализуют список, dependency graph проверяется до Apply.
37. Leaf-типы старой project schema сериализовались с `allowedChildren:null` и
    ломали dynamic tree — API-контракт теперь всегда отдаёт iterable `[]`.
38. Lifecycle E2E сохранял старые manuscript-only labels и недопустимую вложенность —
    сценарий переведён на schema-derived parent/type и общую терминологию «материал».
39. Native export/version handoff терял часть подготовленных ассистентом параметров —
    destination и route schemas теперь переносят точный prefill, а restore проверяет
    `expectedCurrentRevision` и не применяет устаревшее предложение.
40. Silence checkpoint жил только в клиентском состоянии — server сначала сохраняет
    `awaiting_confirmation`, затем показывает prompt; состояние восстанавливается
    после разрыва WebSocket.
41. Редактирование voice-стенограммы могло остаться только в React state — изменения
    debounce-сохраняются в тот же durable draft, включая состояние ожидания.
42. Пауза могла сработать внутри продолжающейся речи, а flush завершиться раньше
    provider final — partial/speech events сбрасывают таймер, а stop ждёт `finalized`.
43. Голосовой execute/continue не имел явного протокола — добавлены две точные
    reserved-команды с визуальным acknowledgement; похожие фразы остаются текстом.
44. `project.get_activity` показывал только события текущего запуска — выдача стала
    проектной и объединяет безопасные document revisions, named versions и Assistant
    change/event metadata.
45. `project.search` и `mentions.find` не видели стенограммы — retrieval теперь ищет
    document nodes, transcript drafts/artifacts/live segments и возвращает безопасные
    source coordinates.
46. Пустой список Assistant turns сериализовался как `null` — HTTP-контракт всегда
    возвращает iterable `[]`; браузерный regression воспроизводит чистый run.
47. `audio.get_source_range` мог обещать источник без durable asset, а
    `version.compare` принимать отсутствующую сторону — оба инструмента теперь
    проверяют реальное наличие и возвращают честный результат/ошибку.
48. `note.update` и `draft.prepare_transfer` принимали node несовместимого типа —
    backend теперь проверяет schema-compatible kind до staging.
49. Один транзиентный ответ MiniMax мог закэшировать системный provider как
    `unavailable`, хотя немедленный повтор проходил полный strict loop — probe теперь
    повторяет полную последовательность ровно один раз, после восстановления честно
    помечает provider как `limited/нестабильный` и не повторяется при cancel.
50. Password manager подставлял сохранённые sign-in credentials в `Model ID` и
    `API-ключ` нового OpenAI-compatible подключения — form/input получили отдельные
    имена, `new-password` и vendor ignore contracts; реальный browser и 4-viewports
    regression подтверждают пустые поля без отправки данных.
51. Silence checkpoint ассистента был зафиксирован в launcher на 5 секундах, хотя
    спецификация требовала preference `3/5/8/выключено` — настройка добавлена в
    AI-раздел, сохраняется после reload и фиксируется в durable run.
52. `settings.get_writer_preferences` возвращал только audio retention и не видел
    фактические параметры текущего Assistant — run теперь получает безопасный
    нормализованный snapshot языка, command/live/editor и silence preferences;
    microphone ID, credentials, retention/privacy и account controls исключены.
53. Run, открытый из Chat, сохранял `silenceCheckpointMs=0`, поэтому последующее
    переключение в Voice бесшумно отключало checkpoint — threshold теперь является
    настройкой всего run, а отсутствие STT/checkpoint в Chat обеспечивается состоянием
    режима; regression проходит полный Chat → Voice → Chat цикл.
54. Revision `ASSISTANT.md` закреплялась на весь run, хотя спецификация требует,
    чтобы изменение guide влияло на следующее подтверждённое поручение — active
    revision теперь атомарно pin-ится на turn при подтверждении, используется и в
    prompt, и в `assistant_guide.get`; более поздняя правка не меняет текущий turn.
55. Несохранённый `ASSISTANT.md` терялся при reload и не предлагал автору структуру —
    project-scoped локальный черновик теперь восстанавливается только поверх той же
    base revision, stale draft требует явного выбора, а встроенный шаблон не может
    молча заменить уже набранный текст.
56. S-27 показывал лишь add/deactivate списка и скрывал обязательные scope, tags,
    sources, edit, search, filters и revision history — экран Project Memory теперь
    управляет всеми полями, показывает provenance и историю, а restore rN создаёт
    следующую revision с optimistic concurrency и ownership check.
57. Старый клиент `updateProjectMemory` отправлял строгому endpoint весь read-model
    вместе с `id/projectId/source/timestamps`, получал 400 и ломал edit/reactivate —
    payload заменён явным writable allowlist; unit contract и полный 4-viewport
    create/edit/restore/deactivate regression проходят.
58. Pending browser permission выглядел как неработающая кнопка диктовки, а Assistant
    Voice называл ещё не открытый микрофон состоянием «Слушаю» — оба интерфейса теперь
    отдельно показывают ожидание разрешения. Voice-подключение можно отменить; поздно
    разрешившийся `getUserMedia` немедленно останавливается и не возобновляет запись.
59. Полный UI-E2E зависел от доступности внешней MiniMax capability-probe и мог падать
    без изменения продукта — добавлен in-process strict-tool fixture provider для
    детерминированного CI. `APP_ENV=production` конфигурационно запрещает fixture;
    реальный MiniMax strict loop остаётся отдельным обязательным smoke и после
    возврата текущего runtime снова имеет `systemProviderAvailable=true`.
60. Второй рабочий чат мог вернуть HTTP 500: nullable SQL-выражение unread
    сканировалось в Go `bool` — запрос получил `COALESCE`, а integration regression
    проверяет working run без error/change set; cursor списка закрывается до
    вложенной hydration.
61. После скрытия панели Chat мог открыться как Voice — выбранный input mode теперь
    сохраняется в `localStorage` отдельно для каждого run и проверяется E2E.
62. На compact/mobile после скрытия и повторного открытия панели URL терял
    `assistantRunId` — восстановление visible run теперь синхронизирует URL без
    remount editor; регрессия проходит на всех четырёх viewport.
63. В первом пустом switcher одновременно показывались два способа закрытия, а
    счётчик писал «2 чатов» — лишний интерактивный close удалён, число чатов
    склоняется по русским правилам и покрыто unit/visual QA.
64. Первый server start release-кандидата отклонил миграцию до изменения БД:
    macOS tar добавил `._*` AppleDouble entries рядом с embedded SQL. Все 494
    служебных файла удалены из server source, image пересобрана с чистым context;
    migration 017 затем применена штатно, а data hashes остались неизменными.

## Известные границы

- Deepgram даёт native realtime partial/final contract, но сетевой PCM всё равно
  физически передаётся frames; это transport detail, а не текстовые чанки или границы
  абзацев. OpenRouter `chunked_live` сохранён только как fallback, когда Deepgram
  явно не выбран. Fake fixture доступен только в явном test/CI режиме.
- LLM fixture проходит тот же трёхшаговый provider probe и обычный registry/run path,
  но доступен только вне production; production startup с `LLM_ADAPTER=fixture`
  завершается ошибкой.
- Media tests используют Chromium fake media device. Реальный OpenRouter file request
  и полный Deepgram browser media flow выполнены с реальным WAV, но OS-specific
  microphone hardware/permission UI на Safari/Firefox не входит в этот локальный
  прогон.
- Голосовой capture в браузере требует HTTPS или localhost. Текущий локальный
  `http://localhost:8080` пригоден для ручной проверки; LAN production доступен
  через доверенный `https://studio.example.test`, а multi-session delta развёрнута
  и проверена 2026-08-04.
- В MVP нет billing/subscription subsystem. Поэтому AC-49 выполнен отсутствием
  paywall/entitlement guard для TXT/Markdown, а не имитацией отмены подписки.
- Внешний backup provider отсутствует; UI честно описывает retention и не обещает
  мгновенное физическое исчезновение внешних копий.
- Общий Go coverage ниже domain coverage из-за HTTP wiring и long-running worker glue;
  safety-critical editor/export/STT/store invariants покрыты unit/integration/E2E.

## Статический аудит

Проведён поиск незавершённых implementation markers, skipped/focused tests и случайных
секретов. Таких markers и отключённых tests в исходниках нет. `.env` не включён в
artifacts, логи или документацию.
