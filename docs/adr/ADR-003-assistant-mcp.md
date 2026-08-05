# ADR-003: Локальный MCP-контур для режима «Ассистент»

> Статус: принято; implementation baseline реализован  
> Дата: 31 июля 2026  
> Владельцы: Product + Engineering + Security

## Контекст

«Студия автора» добавляет live-режим, в котором автор голосом или текстом поручает
модели прочитать материалы проекта и выполнить работу: найти фрагменты, создать
заметку, дописать сцену, изменить структуру или подготовить редактуру. Run может
быть целевым — из конкретной главы/сцены — или глобальным — из проекта без заранее
заданного mutation target.

В `.env` уже предусмотрены `LLM_API_KEY` и `LLM_BASE_URL`. Пользователь также
должен иметь возможность подключить собственный OpenAI-compatible endpoint.

У приложения должен быть MCP-контракт, покрывающий проект, дерево, документы,
черновики, заметки, словарь, версии, расшифровки, поиск, экспорт и recovery.
Одновременно рукопись является чувствительным содержимым, а существующие
продуктовые правила запрещают скрыто перезаписывать авторский текст.

OpenAI-compatible endpoints не обладают единым уровнем совместимости: часть
реализует Responses API, часть — Chat Completions, часть поддерживает только
простые сообщения, а remote MCP не является общей гарантией. Поэтому архитектура
не может требовать provider-hosted MCP как единственный путь.

## Решение

Использовать server-side **Assistant Orchestrator**, который:

1. принимает подтверждённый transcript turn, nullable target и фиксированную
   project-context policy;
2. серверно загружает active revision проектного `ASSISTANT.md` либо фиксирует
   явное отсутствие guide;
3. выбирает сохранённый provider/model adapter;
4. получает MCP tool schemas от внутреннего Writer’s Studio MCP Server;
5. преобразует их в Responses или Chat Completions function/tool format;
6. выполняет tool calls локально через MCP client;
7. применяет ownership, scope, risk и approval policy;
8. накапливает mutations в изолированном `AssistantChangeSet`;
9. возвращает модели read/tool results;
10. завершает run ответом или предложением изменений;
11. передаёт commit в обычный application service только после UI approval.

Внешний LLM-провайдер не соединяется с PostgreSQL, object storage или внутренним
MCP endpoint и не получает сетевые credentials приложения.

## Семантический protocol turn

Host не определяет намерение поручения регулярными выражениями, списками
глаголов, синонимов/гипонимов или keyword-based выбором tools. Такой router
неустойчив к естественной речи, языкам и контексту произведения.

Вместо него первый изолированный provider request принудительно предоставляет
только `assistant.begin_turn`. Модель обязана ровно один раз объявить:

- `answer` — прочитать/объяснить без изменения проекта;
- `change` — подготовить хотя бы одну mutation, в том числе «только показать diff»;
- `clarify` — задать прямой вопрос, если недостающее нельзя получить read tools.

Декларация получает текущее поручение и структурный scope, но не старую chat
prose. После неё модель получает полный policy-разрешённый MCP catalog. Для
`change` основной loop также начинает с текущего авторитетного поручения;
история предыдущего proposal читается только через
`assistant.get_recent_change_set`, а не реконструируется из старого ответа.

`document.find_exact` индексирует одну уже известную буквальную строку, а
`document.find_exact_batch` — до 50 строк в одной exact revision и возвращает
упорядоченные block-local Unicode anchors. Они не понимают смысл фразы.
`document.replace_range` принимает эти anchors, `expectedText`, base revision и
protected ranges; сервер отклоняет любое несовпадение до staging. По умолчанию
turn не ограничен числом tool calls или provider cycles. Автор может задать
положительный tool-call limit в настройках конкретного проекта; host фиксирует
его в начале turn и завершает превышающий бюджет loop с `tool_limit_exceeded`.
Для поручений с 2–100 новыми терминами или фактами используются
`dictionary.add_batch` и `project_memory.add_batch`. Они не расширяют authority и
не коммитят данные напрямую: host разворачивает batch в отдельные staged operations
с per-item conflict detection. Это сохраняет единый авторский turn и сокращает
стоимость даже в проектах, где автор включил собственный лимит шагов.

Для создания 1–50 content-bearing sibling-узлов используется
`tree.create_nodes_batch`: каждый item содержит title и полное initial content,
а host разворачивает пакет в обычные `tree.create_node` staged operations. Это
устраняет ошибочный create-then-append flow: staged node ID до Apply отсутствует в
canonical tree и не может быть target следующего tool call.

Process restart не считается продолжением in-memory provider/tool loop. При
старте persisted `working` turn идемпотентно помечается recoverable ошибкой
`execution_interrupted`, его run снова становится `active`, а canonical project
остаётся неизменным. Автор явно повторяет поручение; host не отправляет его модели
автоматически и не оставляет бесконечный `working`.

Таймаут одного provider request установлен в 10 минут. Наблюдаемый двухминутный
cutoff дважды обрывал длинную генерацию точно на границе transport timeout, а не
из-за MCP/tool limits. Host не делает автоматический retry, чтобы не удваивать
токены. Если до timeout уже были успешные mutation tools, их operations
финализируются в явно частичный `AssistantChangeSet`; если их не было, пустой
review не создаётся. Stop обновляет terminal run и текущий `working` turn одной
транзакцией.

После увеличения client timeout отдельный реальный Opus-run успешно выполнил семь
read tools, затем его первый генеративный шаг снова был закрыт внешним
OpenRouter/provider-контуром ровно через 120 секунд. Следовательно, long-form нельзя
строить как один нестриминговый JSON tool payload. Для существующего документа
`document.append_chunk` принимает ограниченные фрагменты (до 9 000 символов и
1 500 слов), проверяет sequence/target/final marker и накапливает их в одной staged
operation. Следующий provider cycle получает только метаданные, accumulated word
count и bounded tail; полный принятый чанк удаляется из replay. Apply создаёт одну
revision, Undo восстанавливает исходную, а обрыв оставляет partial review.

Повторный сбой показал границу уровнем раньше: после девяти read tools provider не
успел вернуть сам первый чанк. Поэтому OpenAI-compatible Chat Completions transport
работает в SSE-режиме. OpenRouter processing comments читаются как keepalive,
фрагментированные имя/аргументы tool call собираются по `index`, а opaque
`reasoning_details` сохраняются для следующего provider cycle. Частичный или
невалидный JSON не вызывает tool. Это решение дополняет накапливающие чанки:
streaming защищает отдельный чанк во время генерации, а `document.append_chunk` —
весь длинный документ между ходами модели.

## Почему MCP остаётся отдельным сервером

- контракт доступен не только одному provider adapter;
- tool schemas и capability metadata версионируются независимо от prompts;
- все функции проходят единый ownership/policy слой;
- можно тестировать MCP tools без реальной LLM;
- в дальнейшем тот же сервер можно безопасно опубликовать для доверенного клиента
  через отдельный transport, не меняя доменные сервисы;
- coverage существующих workflow можно проверять автоматически.

MCP server может быть in-process или отдельным локальным process в MVP. Его
логическая граница обязательна; deployment boundary — нет.

## Provider adapters

```go
type LLMProvider interface {
    Capabilities(ctx context.Context) LLMCapabilities
    StartRun(ctx context.Context, request RunRequest) (RunStream, error)
    ContinueWithToolResults(
        ctx context.Context,
        run ProviderRunState,
        results []ToolResult,
    ) (RunStream, error)
}
```

```ts
interface LLMCapabilities {
  dialect: "responses" | "chat_completions";
  streaming: boolean;
  functionCalling: boolean;
  strictArguments: boolean;
  sequentialToolCalls: boolean;
  parallelToolCalls: boolean;
  structuredOutputs: boolean;
  remoteMCP: boolean;
  usage: "none" | "tokens" | "detailed";
  maxContextTokens?: number;
}
```

Для Assistant обязательны:

- text generation;
- function/tool calling;
- строгий разбор arguments на стороне приложения;
- возврат tool results в последующий turn;
- минимум два последовательных tool calls.

`remoteMCP`, parallel calls и structured outputs являются optional capabilities.

## Почему не отдавать remote MCP провайдеру по умолчанию

### Плюсы provider-hosted remote MCP

- меньше собственного orchestration кода;
- provider может выполнить несколько calls в одном request;
- стандартные MCP lifecycle items и approvals у поддерживающих providers.

### Причины не выбирать baseline

- это не общая OpenAI-compatible capability;
- private MCP пришлось бы публиковать или прокладывать tunnel;
- provider получает прямой сетевой путь к server и его schemas;
- сложнее гарантировать единый approval/change-set semantics;
- поведение, retention и tool loading отличаются между providers;
- пользовательский self-hosted endpoint может не поддерживать remote MCP.

Remote MCP adapter допустим позже только при сохранении тех же policy и commit
invariants.

## Staged mutations

MCP mutation tool никогда не меняет canonical revision напрямую в assistant run.

```text
project snapshot + base revisions
              ↓
        AssistantChangeSet
       ├── document patches
       ├── tree operations
       ├── dictionary operations
       └── snapshot/note operations
              ↓ user approval
      Application transaction
              ↓
 revisions + operation ID + audit summary
```

`assistant.apply_changeset` не является model tool. Его вызывает authenticated
UI/API после явного пользовательского подтверждения.

Если application transaction не может быть атомарной для конкретного внешнего
эффекта, такой эффект относится к A3 и остаётся в native flow.

## Input modes: Voice и Chat

Оба режима создают ordered `AssistantTurn` одного run и передают Orchestrator
нормализованный text instruction. Они не имеют разных MCP registries, providers,
policy или history.

### Voice

Голосовое поручение использует существующий STT pipeline и передаёт LLM текст.
Assistant не требует audio-capable LLM. Это:

- сохраняет совместимость с text-only providers;
- повторно использует dictionary и uncertain-word UX;
- не отправляет дополнительное аудио LLM-провайдеру;
- отделяет стоимость/retention STT от LLM.

STT и Orchestrator имеют явную границу подтверждения:

1. partial/stable transcript отображается и восстанавливается локально;
2. длительная тишина переводит UI в `silence_checkpoint`, но не создаёт provider
   request;
3. только явное `Выполнить` стабилизирует turn, ставит microphone capture на паузу
   и передаёт текст Orchestrator;
4. activity и provider output дописываются в тот же ordered session feed;
5. следующий голосовой turn продолжает тот же run.

Таким образом, silence detection является UX-сигналом готовности, а не источником
полномочия на tool calls.

### Chat

- text composer создаёт turn только после explicit submit;
- STT/audio artifacts отсутствуют;
- active voice draft при переключении паузится и может быть скопирован в composer,
  но не отправляется автоматически;
- unconfirmed voice draft и transcript durable между WebSocket reconnect; переход
  в Chat требует выбора `перенести / вернуться / удалить`;
- input mode можно менять между turns без нового run;
- во время provider/tool loop новый turn не queue-ится в baseline.

## Data model

Новые сущности:

### `LLMConnection`

- `id`, `ownerUserId`, display name;
- normalized base URL;
- dialect preference;
- encrypted credential reference;
- model ID и optional headers;
- discovered capabilities и checkedAt;
- status;
- createdAt/updatedAt.

Системное env-подключение представляется read-only virtual connection и не
сохраняет env credential в БД.

### `AssistantRun`

- `id`, `ownerUserId`, `projectId`;
- nullable default target и project-context policy;
- provider connection ID, model ID и capability snapshot;
- status, timestamps, stop reason;
- token/latency metadata без content logs.

### `AssistantGuide`

- один optional artifact на `projectId`, reserved filename `ASSISTANT.md`;
- stable artifact ID и active revision ID;
- Markdown/structured-text content до 24 000 символов;
- typed stable references на dictionary/note/document;
- revision history, author/source и timestamps;
- не входит в manuscript/export/search corpus по умолчанию.

### `ProjectMemoryRecord`

- `id`, `projectId`, kind и canonical statement;
- optional scope node IDs, typed source refs и tags;
- active/alwaysRelevant;
- optimistic revision и provenance user/assistant operation;
- visible CRUD/search/history; provider-side memory не используется.

### `AssistantTurn`

- `id`, `runId`, ordinal;
- `inputMode`: `voice` или `text`;
- stable instruction transcript и optional uncertain spans;
- guide artifact/revision ID или explicit `none`;
- state: draft/listening/checkpoint/confirmed/working/completed/failed/stopped;
- microphone state и timestamps;
- answer/proposal references;
- immutable confirmed text; исправление после запуска создаёт следующий turn.

### `AssistantToolEvent`

- run ID, ordinal, tool name, risk level;
- redacted argument summary;
- affected object IDs/revision IDs;
- status/error class;
- user-visible summary.

### `AssistantChangeSet`

- run ID, base project/tree version;
- ordered typed operations;
- per-document base revision;
- preview summary и diff;
- status: draft/ready/stale/applied/rejected;
- application operation ID.

Dictionary, `ProjectMemoryRecord` и `AssistantGuide` mutations используют тот же
change set. MCP и UI/API вызывают один application service, поэтому голосовое
`добавь в словарь / запомни` не создаёт обходного write path.

## Security

- credentials только на backend, encrypted at rest;
- base URL проходит SSRF-защиту, DNS/IP revalidation и redirect policy;
- `http://` допустим только для явно разрешённого local/self-hosted режима;
- model output не определяет URL tool или storage object;
- каждый tool повторно проверяет owner и server-pinned
  `owner/project/run/turn` execution context; при отдельном transport тот же
  контекст подписывается короткоживущим capability token;
- MCP output не содержит secrets и raw signed URLs;
- project content считается untrusted data, а не instructions;
- active guide считается user-authored instruction только если загружен по
  reserved project artifact ID; файл с тем же именем в заметке/тексте не подходит;
- guide ниже system/security policy и не может расширить scope/tools/approvals;
- system/developer policy отделена от retrieved content;
- unknown tool/argument fields отклоняются;
- лимиты на calls, duration, tokens, output size и recursion обязательны;
- Stop закрывает server execution context и отзывает capability token там, где
  используется отдельный transport;
- application logs не содержат content.

## Совместимость с существующими решениями

- ADR-001 остаётся источником истины для STT; LLM provider не заменяет STT adapters.
- ADR-002 остаётся источником истины для текста; mutations преобразуются в
  ProseMirror-safe operations и revisions.
- Restore остаётся restore-as-new-revision.
- Source transcripts остаются immutable.
- Search/replace и export сохраняют preview/snapshot semantics.
- Account/project deletion остаются вне model tools.

## Последствия

### Положительные

- полный provider-neutral проектный assistant;
- MCP покрывает доменные функции без дублирования логики;
- автор контролирует каждое содержательное изменение;
- пользовательские providers не требуют remote MCP;
- tool contract, provider adapter и UI можно тестировать отдельно;
- multi-document изменения применяются согласованно и отменяются группой;
- большие структурные поручения требуют меньше provider/tool round trips, не
  создавая второго apply path;
- restart не оставляет run в ложном бесконечном `working`.

### Отрицательные

- требуется собственный agent loop и нормализация двух API dialects;
- change-set overlay сложнее прямых CRUD calls;
- нужно поддерживать capability probe разных providers;
- tool schemas и prompts становятся versioned product surface;
- полная функциональность увеличивает риск prompt injection и требует threat model.

## Отвергнутые варианты

### Прямые CRUD-вызовы модели

Отвергнуто: дублирует business rules, не даёт единого coverage/audit и повышает
риск обхода ownership.

### Разрешить модели коммитить каждую mutation сразу

Отвергнуто: создаёт частичные изменения, approval fatigue и противоречит diff-first
правилу авторских правок.

### Требовать только Responses remote MCP

Отвергнуто: исключает многие OpenAI-compatible и self-hosted providers.

### Передавать модели полный проект в одном prompt

Отвергнуто: дорого, плохо масштабируется и нарушает data minimization.

## Проверки решения

- [x] UX режимов, scope, exact diff и multi-object preview зафиксирован в
  `design.pen`, включая desktop и mobile Voice → Chat handoff.
- [x] MCP schemas и coverage map WF-02–WF-19 зафиксированы contract tests.
- [x] Responses и Chat Completions adapters исполняют единый tool corpus.
- [x] Provider capability probe проверяет последовательный tool loop и не делает
  invalid credential активным.
- [x] Semantic declaration, отсутствие lexical router, current-only change input
  и native recent-change-set context покрыты тестами.
- [x] Exact literal anchors, wrong offsets/`expectedText`, protected ranges,
  stale revisions и atomic apply/undo проверяются server-side.
- [x] Пакетное создание content-bearing nodes проверяет порядок, preview,
  Apply и Undo коротким scripted-provider regression без реальной LLM.
- [x] Long-form chunk regression проверяет compact replay, накопление нескольких
  чанков в одну operation/revision, Apply/Undo и partial review при provider timeout.
- [x] Recovery persisted `working` после restart идемпотентно проверен на
  PostgreSQL и не меняет canonical project.
- [x] Короткий deterministic timeout regression сохраняет staged partial proposal,
  но не canonical state; отдельный regression не допускает working turn после Stop.
- [x] Activity скрывает thinking/raw payload, а Markdown и review имеют
  отдельные presentation tests.
- [x] Credential encryption, project/run ownership и private-endpoint policy
  покрыты unit/integration tests.
- [ ] Перед production необходимо завершить внешний threat-model review,
  ротацию/backup encryption key и эксплуатационный smoke Stop/cancel на сервере.

## Источники

- [OpenAI: Using tools](https://developers.openai.com/api/docs/guides/tools)
- [OpenAI: MCP and Connectors](https://developers.openai.com/api/docs/guides/tools-connectors-mcp)
- [Model Context Protocol](https://modelcontextprotocol.io/introduction)
