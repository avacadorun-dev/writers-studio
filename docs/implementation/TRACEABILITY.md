# Traceability: workflow → acceptance criterion → implementation → verification

Дата контрольного прохода: 2026-08-04. Источник требований:
[`WORKFLOWS.md`](../specification/WORKFLOWS.md).

Этот отчёт фиксирует dictation MVP AC-01…AC-55 и локально проверенный Assistant
Alpha AC-56…AC-176. `PASS` означает наличие кода и автоматического доказательства;
ручная продуктовая приёмка и серверная выкладка учитываются отдельно и не меняют
статус реализованных invariants. LAN production deployment на
`https://studio.example.test` обновлён 2026-08-04 с двумя проверенными комплектами
PostgreSQL/MinIO backup, миграцией 017, побайтовой сверкой project data/object
storage и внешним HTTPS healthcheck.

WF-33 / AC-132…AC-142 реализуют docked Assistant рядом с mounted manuscript
editor, selection attachment, inline non-canonical live preview и persistent
review context. Обычный Studio сохраняет floating manuscript; только открытая
Assistant panel переводит shell в выровненный трёхколоночный режим. ADR-007
нормативно закрепляет компактную panel typography, direct Apply + Undo и modal
review без route/URL transition как защиту непрерывного авторского flow.

WF-34…WF-36 / AC-143…AC-167 добавляют отдельные от документа Unicode-safe
пометки: реальные ссылки на словарь, заметки на полях, общий список проекта и
точный deep link. Assistant читает и исправляет их через server-owned anchors с
атомарным Apply/Undo и immutable history. Выделение не создаёт постоянную панель:
действия открываются контекстно правым кликом, `Shift+F10`/клавишей меню или
долгим нажатием.

WF-37 / AC-168…AC-176 заменяет singleton-представление на проектный список
независимых Assistant runs. Visible run остаётся только UI-selection; новый чат
создаётся атомарно вместе с первым текстовым turn, незавершённые черновики и режим
общения сохраняются по run id, а stale proposal блокируется по base revision.
S-38/S-39/S-40/S-41 и Mobile P-11 реализованы в Studio без remount рукописи.

## Обозначения доказательств

| Код | Проверка |
|---|---|
| `E-P01` | `tests/e2e/critical-path.spec.ts`, основной путь P-01 |
| `E-TOOLS` | `tests/e2e/critical-path.spec.ts`, dictionary/version/search/export/backup |
| `E-TRANSFER` | `tests/e2e/critical-path.spec.ts`, видимые Back controls и `.wstudio` download → import → новая Studio-копия с тем же текстом |
| `E-P02`, `E-P03` | `tests/e2e/recovery-and-safety.spec.ts` |
| `E-P04` | `tests/e2e/mobile-capture.spec.ts`: mobile capture, видимый command panel, кнопочные команды + Undo |
| `E-NEG` | `tests/e2e/negative-states.spec.ts`: denied/missing/pending microphone permission, insecure origin, storage/network/revision failures |
| `E-CMD` | `tests/e2e/commands-and-shortcuts.spec.ts` |
| `E-REPLACE`, `E-LIFE` | `tests/e2e/lifecycle-and-replace.spec.ts` |
| `E-NAV` | `tests/e2e/assistant-discoverability.spec.ts`: отдельные Context, recoverable/template-based `ASSISTANT.md`, Project Memory create/search/filter/edit/history/restore/deactivate и AI settings, password-manager-safe provider form + Axe |
| `E-A-LIVE` | `tests/e2e/assistant-live-flow.spec.ts`: видимый voice draft, Chat → Voice → Chat без auto-submit, configurable/durable silence checkpoint, безопасный writer-preferences snapshot, edit/reconnect/continue, отмена connecting и позднего microphone result; 3 сценария × 4 viewport |
| `E-A-SCOPE` | `tests/e2e/assistant-live-flow.spec.ts`: глобальный run показывает schema-derived root label, возвращается в проект и сохраняется как `targetKind=project`, `targetNodeId=null`; 1 сценарий × 4 viewport |
| `E-A-CHANGE` | `tests/e2e/assistant-change-flow.spec.ts`: `turn → exact modal diff → Apply → grouped Undo`, затем второй turn → direct Apply из panel → toast Undo; форма уточнения показывает Send без Apply; canonical text неизменен до каждого Apply и полностью восстановлен после Undo; 1 сценарий × 4 viewport |
| `E-A-WORKSPACE` | `tests/e2e/assistant-change-flow.spec.ts`: floating manuscript до запуска, docked layout при открытой панели, единая desktop header line, responsive close/reopen без потери selection, inline preview и bounded modal review без URL transition; 1 сценарий × 4 viewport |
| `E-A-THREADS` | `tests/e2e/assistant-multi-session.spec.ts`: два независимых чата, atomic first turn, project list/title, per-run draft и mode restoration, URL selection, mounted editor и desktop/tablet/mobile geometry; отдельный параллельный proposal conflict проверяет successful Apply первого change set, серверный `409 stale_change_set` второго, `r2 → r3`, refresh и независимость остальных чатов; 2 сценария × 4 viewport |
| `E-ANNOTATIONS` | `tests/e2e/annotations.spec.ts`: контекстное меню выделения, создание ссылки/заметки, общий список проекта, exact deep link, inline decorations и Axe; 1 сценарий × 4 viewport |
| `E-STT` | `tests/real-stt/deepgram-live.spec.ts`: настоящий WAV → Deepgram continuous live → один prose-run/draft paragraph + timestamps; без автоматического batch artifact |
| `F-UNIT` | 107 Bun unit tests: Assistant live preview/review/activity/Markdown, thread grouping/status/title/count и annotation-fix presentation, guide-draft recovery, Project Memory filtering/payload allowlist, project outline, autosave, editor schema/undo/provenance, annotation anchors, context-menu placement, paste sanitization, preferences, IndexedDB и controls |
| `F-ANNOTATIONS` | `annotations.test.ts`, `selection-context-menu.test.ts`, `use-editor-autosave.test.tsx`, `project-api-annotations.test.ts`: transient decorations, Unicode anchors, mapping/detach, viewport-safe menu и атомарный save contract |
| `G-UNIT` | Go unit tests: editor, commands, export, STT, object storage, transitions |
| `G-INT` | `internal/store/store_integration_test.go` с PostgreSQL |
| `G-TRANSFER` | `project_transfer_test.go`, `project_transfer_integration_test.go` и HTTP archive tests: version/reference/tree/checksum validation, SQL round-trip и ID remap |
| `G-ANNOTATIONS` | `internal/documentannotation`, `internal/editor/document_test.go`, `internal/store/annotations_integration_test.go`, `assistant_annotations_integration_test.go`: deterministic anchor mapping, ownership, lifecycle/anchor separation, Assistant Apply/Undo/history, deleted-term snapshot и revision safety |
| `M-DESKTOP` | Ручной проход в in-app Browser: desktop workspace, console/network, основные состояния |
| `M-MOBILE` | Ручной проход 390×844 и 834×1112: studio/settings, доступность CTA, отсутствие horizontal overflow |
| `M-RESTART` | Рестарт app/db/storage с проверкой health и неизменности persisted counts |
| `M-OR` | Реальный OpenRouter verbose-transcription smoke: timed segments/words без вывода текста, секрета или provider body |
| `A-MCP` | `internal/projectmcp/server_test.go`: 69/69 policy tools, включая `annotation.list/get/propose_fixes`, recursive strict schemas, execution context и отсутствие catalog/LLM drift |
| `A-ORCH` | `internal/assistantapp/backend_test.go` и `backend_integration_test.go`: semantic declaration, no lexical routing, prompt/current-turn invariants, guide revision per confirmed turn, sanitization |
| `A-INT` | `internal/assistantapp/backend_integration_test.go` с PostgreSQL: runtime-аудит всех 69 tools, project-wide annotation reads и staged server-anchor fixes, document/transcript search, native handoffs, staged changes, apply/undo, guide/memory/dictionary, 13 последовательных read steps до mutation, high-cardinality context batch на 80 operations, compact 6-node content batch, accumulated long-form chunks, partial-timeout preservation и все structure templates |
| `A-STORE` | `internal/store/assistant_integration_test.go`, `assistant_apply_test.go`: ownership, recent change set, atomic apply/reject/undo, memory history, restore-as-new-revision, идемпотентный recovery `working` после restart и terminal run/turn consistency |
| `A-VOICE` | `internal/httpapi/assistant_voice_integration_test.go` + `assistant_voice_test.go`: durable transcript/checkpoint/edit, ready/finalize handshake, reconnect/continue/discard и только две точные reserved-команды |
| `A-HTTP` | `internal/httpapi/assistant_test.go`: run/turn/change-set/provider API |
| `A-THREADS` | `internal/httpapi/assistant_test.go`, `internal/store/assistant_integration_test.go`, migration `017_assistant_threads.sql`: parallel execution manager, atomic run+first turn, durable title/view marker/unread, null-safe working state, ordered project list и latest change set |
| `A-LLM` | `internal/llm/*_test.go`: Responses/Chat Completions, probe, credentials, deterministic strict-tool fixture; config test запрещает fixture в production |
| `A-LARGE` | Дешёвый OpenRouter smoke на полном пользовательском fixture 20 803 символа / 37 233 байта: `qwen/qwen3-30b-a3b-instruct-2507`, 64 project tools на момент smoke, 3 последовательных provider steps, только read-only forced calls; scripted regression отдельно проверяет текущий 69-tool catalog без внешних токенов |
| `A-UI` | Assistant presentation/review/Markdown Bun tests и production build |
| `A-DESIGN` | Pencil diagnostics/exports: S-19/S-20 integrated manuscript + compact typography, S-21 in-Studio modal review, S-22–S-29, S-38–S-41 и Mobile P-07/P-10/P-11 |
| `D-THREADS` | `design.pen`: S-38 `fF5xj`, S-39 `X3XsxT`, S-40 `U9Ixmc`, S-41 `y4BlnR`, Mobile P-11 `Q9k6d`; локальная visual QA сверила desktop 1440×1024 и mobile 390×844 с production bundle |
| `D-ANNOTATIONS` | `design.pen` S-31, S-31B, S-32…S-36 и S-37: контекстное меню, выбор термина, поля заметок, recovery, общий список/deep link и Assistant annotation-fix review |
| `A-NATIVE` | `backend_integration_test.go` + `assistant-presentation.test.ts`: prepare-only A3 payloads превращаются только в server-authored native destinations, точно переносят export/version prefill и отклоняют stale restore |
| `A-LIFE` | `assistant_integration_test.go`, `assistant_apply_test.go`, review tests: run-history deletion, guide/memory restore-as-new-revision, typed duplicate conflicts и dependency-aware selection |
| `A-REAL` | MiniMax-M3 на свежей production-сборке и `free`-проекте: `begin_turn → project.search → document.get → document.find_exact → document.replace_range`; все MCP-вызовы успешны, `staged → applied → undone`, revisions до Apply и после Undo byte-equal |
| `A-CTX-REAL` | MiniMax-M3 в одной продолженной Chat-сессии: точные ответы из always-memory и pinned `ASSISTANT.md` r1, безопасный Markdown, два сохранённых turn и неизменный canonical document |
| `A-LUNA-REAL` | OpenRouter `openai/gpt-5.6-luna`: полный strict catalog с provider-safe wire names, project/memory reads, staged `project_memory.add` и review без canonical commit |
| `A-LIVE-LAYOUT` | In-app Browser на production-сборке: root/body без scroll, отдельный transcript scroll, Voice/turn `14 px`, 0 Philosopher descendants в panel, modal `1120×688` внутри `1280×720` и смонтированный ProseMirror под scrim |

Все E2E-сценарии исполняются в Chromium на `1440×1024`, `1024×768`,
`834×1194` и `390×844`. `E-P01`, `E-P04`, `E-NAV` и сценарий S-17 настроек
дополнительно запускают axe WCAG 2 A/AA, 2.1 AA и 2.2 AA.

## Покрытие workflow

| Workflow | Реализация | Основное доказательство | Статус |
|---|---|---|---|
| WF-01 Первый вход | `pages/sign-in`, `pages/projects`, постоянная ссылка Recovery | `E-P01`, `E-NEG` | PASS |
| WF-02 Создание проекта | `pages/new-project`, `store/projects.go` atomic transaction | `E-P01`, `G-INT` | PASS |
| WF-03 Структура | `widgets/project-tree`, document move/trash/restore APIs | `E-LIFE`, `G-INT` | PASS |
| WF-04 Редактор | Версионированная Tiptap schema, paste allowlist, stable block ids/source refs, autosave, conflict copy, local snapshot | `E-P01`, `E-NEG`, `F-UNIT` | PASS |
| WF-05 Preflight | permission/device/storage/network checks | `E-P01`, `E-NEG`, `M-MOBILE` | PASS |
| WF-06 Live-диктовка | local MediaRecorder + continuous AudioWorklet PCM16 + Deepgram WebSocket live channel; раскрытый command panel | `E-P01`, `E-P02`, `E-P04`, `E-STT` | PASS |
| WF-07 Голосовые команды | exact grammar, source annotation, reversible voice/button execution, начало русской реплики с `—` | `E-CMD`, `E-P04`, `F-UNIT`, `G-UNIT` | PASS |
| WF-08 Pause/resume | one recorder/session, `requestData()` boundary, dual duration | `E-P01` | PASS |
| WF-09 Network degradation | IndexedDB chunks, upload replay, degraded UI | `E-P02`, `E-NEG` | PASS |
| WF-10 Finalization | idempotent durable background job + manifest/SHA verification | `E-P02`, `G-INT` | PASS |
| WF-11 Review | editable draft, immutable artifacts, source-range audio | `E-P01`, `E-P03`, `G-INT` | PASS |
| WF-12 Late batch/diff | suggestion artifact, opt-in groups, one-action undo | `E-P03`, `G-INT` | PASS |
| WF-13 Transfer | append default, provenance, one revision/operation, undo | `E-P01`, `E-P03`, `G-UNIT` | PASS |
| WF-14 Dictionary | project CRUD + batch/retranscription hints | `E-TOOLS`, `E-P03`, `G-UNIT` | PASS |
| WF-15 Versions | named snapshot, read-only compare, restore-as-new-revision | `E-P03`, `E-TOOLS` | PASS |
| WF-16 Search/replace | project search, transient highlight, preview token, operation undo | `E-REPLACE`, `G-INT`, `F-UNIT` | PASS |
| WF-17 Export | repeatable-read depth-first snapshot + TXT/MD/DOCX renderer | `E-TOOLS`, `G-INT`, `G-UNIT` | PASS |
| WF-18 Crash recovery | IndexedDB manifest/chunks, new-tab recovery, local download | `E-NEG`, `E-P02`, `F-UNIT` | PASS |
| WF-19 Project/account deletion | distinct archive/trash, exact-confirm project/account purge, deletion audit, no orphan jobs | `E-LIFE`, `G-INT` | PASS |

## Дополнение: навигация и переносимый проект

Источник требований: [`PROJECT_NAVIGATION_AND_TRANSFER.md`](../specification/PROJECT_NAVIGATION_AND_TRANSFER.md).

| Критерий | Реализация | Доказательство | Статус |
|---|---|---|---|
| AC-NAV-01…02 | Явный Back на Search/Recovery; остальные вторичные экраны получают Back через свой header или `ProjectUtilityShell` | `E-TRANSFER`, `studio-header.tsx` | PASS |
| AC-PT-01 | Versioned ZIP/MIME `.wstudio`, manifest и SHA-256 аудио | `project_transfer.go`, `G-TRANSFER`, `E-TRANSFER` | PASS |
| AC-PT-02…03 | Полный authoring state, новая active-копия и remap project/node/session/artifact/memory IDs | `project_transfer_integration_test.go`, `E-TRANSFER` | PASS |
| AC-PT-04 | Future version, broken references/tree, unsafe ZIP и checksum отклоняются до частичного проекта | `G-TRANSFER` | PASS |
| AC-PT-05 | Account/provider credentials отсутствуют в transfer DTO и export queries | `project_transfer.go`, code review | PASS |

## Acceptance criteria AC-01…AC-55

| AC | Реализация | Доказательство | Статус |
|---|---|---|---|
| AC-01 | Один основной CTA `Новый проект` после входа | `E-P01` | PASS |
| AC-02 | Recovery banner на active projects не имеет dismiss и центр всегда доступен | `E-NEG`, `M-DESKTOP` | PASS |
| AC-03 | Созданный project/node читаются из PostgreSQL после refresh | `E-P01`, `E-TOOLS` | PASS |
| AC-04 | Project и initial chapter создаются одной DB transaction | `store/projects.go`, `G-INT` | PASS |
| AC-05 | Same node/revision refetch не вызывает `setContent`; caret сохраняется при move | `E-LIFE` | PASS |
| AC-06 | Move и trash дают toast Undo; restore сохраняет сущность | `E-LIFE` | PASS |
| AC-07 | Acknowledged revision и форматирование переживают reload | `E-P01`, `E-TOOLS`, `F-UNIT` | PASS |
| AC-08 | Transcript transfer — одна revision с operation id; undo восстанавливает previous content | `E-P03`, `G-INT` | PASS |
| AC-09 | Autosave не заменяет editor content; selection и scroll сравниваются до/после ack | `E-P01`, `F-UNIT` | PASS |
| AC-10 | Явный/fallback late quality pass создаёт artifact/suggestion и не выполняет Tiptap transaction | `E-P03`, `G-INT` | PASS |
| AC-11 | Click→видимое recording state измеряется browser clock и проверяется `≤300 ms` | `E-P01` × 4 viewport | PASS |
| AC-12 | Destination и language доступны до старта | `E-P01`, `E-NEG` | PASS |
| AC-13 | Mic denial/missing device и STT failure имеют разные состояния/тексты | `E-NEG`, `E-P02` | PASS |
| AC-14 | Partial имеет label `Черновик`, dotted border/caret и screen-reader prefix | `recording-page.tsx`, `E-P01`, axe | PASS |
| AC-15 | Autoscroll действует только при `followLive=true`; ручной scroll выключает follow | `recording-page.tsx`, `M-DESKTOP` | PASS |
| AC-16 | Recorder state — `role=status` live region; обе duration имеют текстовую семантику | `recording-page.tsx`, axe | PASS |
| AC-17 | Controller отклоняет `start()` при active state | `recorder-controller.ts`, `E-P01` | PASS |
| AC-17a | Live footer по умолчанию показывает prefix, примеры и клавиатурно доступные quick buttons | `recording-page.tsx`, `E-P04` desktop/mobile | PASS |
| AC-18 | Executed command отображается отдельным status-card с Undo | `E-P01`, `E-CMD` | PASS |
| AC-19 | `command.rawText` остаётся в stored live segment/source artifact | `E-CMD`, `G-INT` | PASS |
| AC-20 | Disabled command configuration возвращает phrase как обычный segment | `E-CMD`, `G-UNIT` | PASS |
| AC-20a | При disabled voice commands кнопки остаются allowlisted и используют те же persistence/Undo semantics | `recorder-controller.ts`, `dictation.go`, `E-P04` | PASS |
| AC-21 | Pause сначала вызывает `requestData()`, потом ставит recorder на pause | `recorder-controller.ts`, `E-P01` | PASS |
| AC-22 | Resume использует тот же local/server session id | `E-P01`, `G-INT` | PASS |
| AC-23 | Header показывает total session, footer — recorded audio; оба значения persisted | `recording-page.tsx`, `E-P01`, `G-INT` | PASS |
| AC-24 | Offline/WebSocket loss не меняет MediaRecorder state; число local chunks растёт | `E-P02` | PASS |
| AC-25 | Degraded banner отдельно сообщает local recording и live-text outage | `E-P02` | PASS |
| AC-26 | Reconnect перебирает все `uploaded=false` chunks и дозагружает их | `E-P02`, `recovery-sync.ts` | PASS |
| AC-27 | Stop promise и DB idempotency key дают одну finalization-задачу; успешный Deepgram не запускает batch STT | `G-INT`, `E-STT`, `recorder-controller.ts` | PASS |
| AC-28 | Finalization job хранится в PostgreSQL и выполняется независимо от страницы | `worker.go`, `G-INT`, `M-RESTART` | PASS |
| AC-29 | Failed finalization оставляет audio endpoint, live draft и retry | `E-P02` | PASS |
| AC-30 | Paragraph source ref выбирает segment и проигрывает только start–end range; Deepgram/OpenRouter timestamps нормализуются | `E-P01`, `E-STT`, `G-UNIT` | PASS |
| AC-31 | Manual edit пишет transcript draft revision; source artifact immutable | `E-P01`, `E-P03`, `G-INT` | PASS |
| AC-32 | Gateway передаёт active dictionary как Deepgram keyterms; worker использует hints для batch/retranscription | `E-TOOLS`, `E-P03`, `E-STT`, `G-UNIT` | PASS |
| AC-33 | Playback не меняет editor read-only state; E2E продолжает ввод после play | `E-P01` | PASS |
| AC-34 | Draft JSON сравнивается byte-for-byte до Accept; retry его не меняет | `E-P03` | PASS |
| AC-35 | Accepted diff revision получает source `Применена финальная расшифровка` | `E-P03`, `G-INT` | PASS |
| AC-36 | Каждая diff group связывается с bounded source audio range | `transcript-diff-page.tsx`, `E-P03` | PASS |
| AC-37 | Block-level `sourceRefs` валидируются, сохраняются и проверяются после reload | `E-P01`, `F-UNIT`, `G-UNIT` | PASS |
| AC-38 | Undo transfer сохраняет previous content новой revision и не затрагивает остальной текст | `E-P03`, `G-INT` | PASS |
| AC-39 | Transfer mode по умолчанию `append`; replace требует явного выбора | `review-page.tsx`, `E-P01` | PASS |
| AC-40 | Dictionary UI прямо сообщает, что старый текст автоматически не меняется | `dictionary-page.tsx`, `E-TOOLS` | PASS |
| AC-41 | Retranscribe создаёт новый `cleaned_suggestion`, original batch/draft не меняются | `E-P03`, `G-INT` | PASS |
| AC-42 | Restore добавляет revision; более новые revisions остаются в history | `E-P03` | PASS |
| AC-43 | Named version — отдельная immutable row, не часть autosave revisions | `E-P03`, `E-TOOLS` | PASS |
| AC-44 | Compare не вызывает mutation; revision/text сравниваются до/после | `E-P03` | PASS |
| AC-45 | Search result открывает studio и ставит transient ProseMirror selection/highlight | `E-REPLACE`, `F-UNIT` | PASS |
| AC-46 | Replace требует свежий preview token; stale token отклоняется | `E-REPLACE`, `G-INT` | PASS |
| AC-47 | Snapshot использует depth-first recursive outline и selected revisions | `G-INT`, `E-TOOLS`, `G-UNIT` | PASS |
| AC-48 | Snapshot JSON фиксируется в repeatable-read transaction; worker читает только его | `G-INT` | PASS |
| AC-49 | TXT/Markdown доступны без billing/paywall guard | `E-TOOLS`, `G-UNIT` | PASS |
| AC-50 | Новая вкладка читает IndexedDB и синхронизирует после закрытия исходной | `E-NEG` | PASS |
| AC-51 | Recovery собирает local chunks в Blob; download проверяется до delete | `E-NEG` | PASS |
| AC-52 | Failed upload оставляет chunk с `uploaded=false`; удаление только явное | `E-NEG`, `E-P02`, `F-UNIT` | PASS |
| AC-53 | Archive и trash — разные status/filter/action | `E-LIFE` | PASS |
| AC-54 | Restore меняет только project status; tree/revisions не удаляются | `E-LIFE`, `G-INT` | PASS |
| AC-55 | Purge UI показывает retention/backup caveat; project/account purge проверяют exact confirmation, audit и отсутствие orphan jobs | `settings-page.tsx`, `E-LIFE`, `G-INT`, `M-DESKTOP` | PASS |

## Assistant Alpha: workflow coverage

| Workflow | Реализация | Доказательство | Статус |
|---|---|---|---|
| WF-20 Выбор режима | Отдельные Dictation/Assistant entry points; node/project scope; Assistant не создаёт dictation session | `A-HTTP`, `A-UI`, `A-DESIGN`, `E-NAV`, `E-A-SCOPE` | PASS |
| WF-21 Live-поручение | Voice WS, stable transcript, durable silence checkpoint/edit, explicit confirm/continue, reconnect/discard | `A-VOICE`, `A-HTTP`, `A-DESIGN`, `E-A-LIVE` | PASS |
| WF-22 MCP tool loop | Semantic declaration, 69/69 catalog, scoped read/mutation loop, batch exact/context/node/annotation/long-form operations, 13-step, 80-operation, compact 6-node и accumulated-chunk regressions, Stop limits | `A-MCP`, `A-ORCH`, `A-INT`, `A-LUNA-REAL` | PASS |
| WF-23 Change review/apply | Staging, in-Studio modal exact diff без URL transition, direct Apply, stale check, atomic apply/reject/undo | `A-STORE`, `A-INT`, `A-UI`, `A-DESIGN`, `E-A-CHANGE` | PASS |
| WF-24 A3/A4 | Registry не содержит commit/purge/credential read; prepare tools создают только server-authored native handoff с точным prefill и stale guard | `A-MCP`, `A-ORCH`, `A-NATIVE` | PASS |
| WF-25 Activity/recovery | Durable run/events/change sets, project-wide activity, reopen, идемпотентный `execution_interrupted` для orphaned `working` и удаление run history без отката applied revisions | `A-STORE`, `A-HTTP`, `A-UI`, `A-LIFE`, `A-INT` | PASS |
| WF-26 Providers | System env, encrypted user connection, probe, Responses/Chat adapters, disable/default | `A-LLM`, `A-HTTP`, `A-UI` | PASS |
| WF-27 Guide | Optional reserved guide, pinned revision, staged create/update, history и restore-as-new-revision | `A-INT`, `A-STORE`, `A-UI`, `A-DESIGN`, `A-LIFE`, `A-CTX-REAL`, `E-NAV` | PASS |
| WF-28 Dictionary/Memory | Typed read/write, one change set, revisions, undo и явный duplicate conflict | `A-INT`, `A-STORE`, `A-DESIGN`, `A-LIFE`, `A-CTX-REAL`, `E-NAV` | PASS |
| WF-29 Voice/Chat | Один run/history, explicit send, durable editable draft handoff без auto-submit | `A-VOICE`, `A-UI`, `A-DESIGN`, `E-A-LIVE` | PASS |
| WF-30 Addressed edit | Exact anchors, expected text, protected range, dependency-aware groups, readable review и реальный Apply/Undo | `A-INT`, `A-ORCH`, `A-UI`, `A-LIFE`, `A-REAL`, `E-A-CHANGE` | PASS |
| WF-31 Dynamic structure | Семь templates, schema-derived nesting/root label, content-bearing/free nodes, stale revision | `A-INT`, `internal/store/structure_test.go`, `A-UI`, `E-A-SCOPE` | PASS |
| WF-32 Semantic protocol | Forced begin-turn, no lexical router, current-only change input, native prior change set | `A-ORCH`, `A-MCP`, `A-INT` | PASS |
| WF-33 Integrated workspace | Studio route сохраняет mounted editor; закрытое состояние остаётся floating, открытая panel включает docked layout и compact type scale, stable selection attachment, live preview/stale и modal review | `E-A-WORKSPACE`, `E-A-CHANGE`, `F-UNIT`, `A-HTTP`, `A-STORE`, `A-DESIGN`, `ADR-007` | PASS |
| WF-34 Ссылки и заметки рукописи | Отдельные от Tiptap JSON annotations, transient decorations, exact Unicode anchors, атомарное mapping-сохранение и контекстное меню без постоянной панели | `E-ANNOTATIONS`, `F-ANNOTATIONS`, `G-ANNOTATIONS`, `D-ANNOTATIONS` | PASS |
| WF-35 Все правки и перенос | Проектный список/фильтры, exact deep link, additive migration, `.wstudio` v1 compatibility и v2 atomic remap | `E-ANNOTATIONS`, `G-ANNOTATIONS`, `G-TRANSFER`, `D-ANNOTATIONS` | PASS |
| WF-36 Assistant исправляет заметки | Project-wide A0 reads, до 50 server-owned exact fixes, per-note resolve choice, atomic document/anchor/history Apply и snapshot Undo | `A-MCP`, `A-INT`, `A-STORE`, `G-ANNOTATIONS`, `A-UI`, `D-ANNOTATIONS` | PASS |
| WF-37 Несколько чатов проекта | Project-scoped switcher/new-chat panel, независимые run execution/drafts/modes/attachments, durable unread/viewed/title, responsive overlay/full-width state и revision-safe conflict refresh | `E-A-THREADS`, `A-THREADS`, `A-STORE`, `A-HTTP`, `D-THREADS` | PASS |

## Assistant Alpha: acceptance coverage

| Критерии | Покрытый invariant | Доказательство | Статус |
|---|---|---|---|
| AC-56…AC-75 | Entry, node/project scope, live transcript/checkpoint, scoped document+transcript reads, staged/atomic changes, source immutability | `A-HTTP`, `A-VOICE`, `A-MCP`, `A-INT`, `A-STORE`, `A-UI`, `E-A-LIVE`, `E-A-SCOPE` | PASS |
| AC-76 | A3 всегда заканчивается native confirmation | `A-MCP`, `A-NATIVE`: archive/restore/retranscribe/export не получают canonical commit path | PASS |
| AC-77…AC-79 | Policy injection boundary, durable run, disabled provider не ломает history | `A-MCP`, `A-STORE`, `A-HTTP` | PASS |
| AC-80 | Удаление run history не откатывает applied project changes | `A-HTTP`, `A-LIFE`: endpoint + ownership + applied-content regression | PASS |
| AC-81…AC-92 | Credentials/providers и optional pinned/staged guide | `A-LLM`, `A-HTTP`, `A-INT`, `A-STORE`, `A-CTX-REAL` | PASS |
| AC-93 | Restore guide создаёт новую revision | `A-LIFE`: restore сохраняет новую активную revision и историю | PASS |
| AC-94…AC-98 | Guide exclusion; typed dictionary/memory staging, visible history и revisions | `A-INT`, `A-STORE`, `A-CTX-REAL`, `E-NAV`, `F-UNIT` | PASS |
| AC-99 | Duplicate/conflict предлагает merge/update | `A-INT`, `A-LIFE`: dictionary/memory conflict не создаёт silent operation | PASS |
| AC-100…AC-116 | Typed retrieval, voice/chat draft continuity, exact addressed edit, protected review/trace | `A-VOICE`, `A-INT`, `A-STORE`, `A-UI`, `A-REAL`, `E-A-LIVE`, `E-A-CHANGE` | PASS |
| AC-117 | Partial groups соблюдают dependencies | `A-LIFE`, `A-UI`: transitive prerequisites выбираются, required groups нельзя снять | PASS |
| AC-118…AC-131 | Atomic Undo, linked memory change, recovery, dynamic structure, semantic/no-regex protocol, Markdown/activity | `A-STORE`, `A-INT`, `A-ORCH`, `A-MCP`, `A-VOICE`, `A-UI`, `E-A-CHANGE` | PASS |
| AC-132…AC-138 | Manuscript + Assistant одновременно, mounted editor state, floating↔docked shell, selection attachment, inline preview/stale, persistent review transcript | `E-A-WORKSPACE`, `E-A-CHANGE`, `F-UNIT`, `A-HTTP`, `A-STORE`, `design.pen` | PASS |
| AC-139…AC-142 | Compact panel typography, modal diff без route/URL transition, direct Apply + grouped Undo и instruction/groups/target context | `E-A-WORKSPACE`, `E-A-CHANGE`, `A-LIVE-LAYOUT`, `A-DESIGN`, `ADR-007` | PASS |
| AC-143…AC-151 | Контекстное создание exact anchors, реальные dictionary links, editable note lifecycle, non-canonical decorations, transactional remap, safe detach/old-client/conflict/duplicate и desktop/mobile parity | `E-ANNOTATIONS`, `F-ANNOTATIONS`, `G-ANNOTATIONS`, `D-ANNOTATIONS` | PASS |
| AC-152…AC-160 | Общий список и exact deep link, resolved history, ownership/privacy, v1/v2 transfer safety, additive schema и ≥90% покрытия новой критической логики | `E-ANNOTATIONS`, `F-ANNOTATIONS`, `G-ANNOTATIONS`, `G-TRANSFER`, `D-ANNOTATIONS` | PASS |
| AC-161…AC-167 | Scoped annotation reads, no model offsets, exact review, atomic Apply/history, deterministic detach, stale safety и grouped snapshot Undo | `A-MCP`, `A-INT`, `A-STORE`, `G-ANNOTATIONS`, `A-UI`, `D-ANNOTATIONS` | PASS |
| AC-168…AC-170 | Несколько active/working runs; visible id только выбирает панель; hide/switch не останавливает execution и не отправляет чужой draft | `E-A-THREADS`, `A-THREADS`, `A-HTTP` | PASS |
| AC-171…AC-173 | S-39 показывает target/scope/provider/context до Send; S-38 имеет текстовые секции/status; draft, mode, selection и review изолированы по run | `E-A-THREADS`, `F-UNIT`, `D-THREADS` | PASS |
| AC-174 | Base revision атомарно блокирует второй Apply; S-40 показывает base/current и создаёт новый refresh turn | `E-A-THREADS`, `A-STORE`, `A-HTTP` | PASS |
| AC-175…AC-176 | 360 px overlay на 1024/834, full-width mobile state, mounted editor, semantic buttons/sections и ≥44 px touch actions | `E-A-THREADS`, `D-THREADS` | PASS |

Автоматическими unit/integration/E2E доказательствами покрыты 176 из 176
acceptance scenarios нормативного baseline. Package-wide
line coverage не используется как замена scenario coverage: она отдельно
публикуется в verification report.

## Граница ручной проверки

Ручными оставлены только свойства, для которых browser automation не является полной
заменой: субъективная композиция и читаемость длинной live-сессии, реальное удобство
голосового потока, OS/browser permission presentation и авторская оценка diff.
Серверная выкладка выполняется только после этой локальной приёмки. Все state
transitions, persistence invariants, destructive confirmations и критические recovery
ветки имеют автоматизированное доказательство.
