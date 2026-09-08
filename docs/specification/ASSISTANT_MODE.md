# «Студия автора»: режим «Ассистент» и проектный MCP

> Версия: 1.0
> Статус: нормативный product/design baseline; Assistant UX release 0.7.0 реализован и проверен
> Дата: 28 августа 2026
> Нормативное поведение: [WORKFLOWS.md](./WORKFLOWS.md), WF-20–WF-37
> Архитектурные решения: [ADR-003](../adr/ADR-003-assistant-mcp.md) и [ADR-007](../adr/ADR-007-assistant-continuous-workspace.md)
> Реализация и доказательства: [TRACEABILITY.md](../implementation/TRACEABILITY.md)

## 1. Назначение

«Ассистент» — второй live-голосовой режим «Студии автора» рядом с диктовкой.

- **Диктовка** превращает речь автора в исходный текст и сохраняет связь с аудио.
- **Ассистент** превращает сформулированное голосом или текстом намерение в
  контролируемую последовательность чтения и изменения материалов проекта.

Ассистент может находить контекст, объяснять, предлагать текст, дописывать,
редактировать и организовывать материалы, но не получает отдельный неограниченный
доступ к базе данных. Все проектные действия проходят через тот же доменный слой,
что и обычный интерфейс, и доступны модели как MCP-инструменты.

## 2. Непересекающиеся режимы

| Свойство | Диктовка | Ассистент |
|---|---|---|
| Основная единица | Аудиосессия и расшифровка | Live assistant run из одного или нескольких turns |
| Интерпретация речи | STT + детерминированные команды | STT → LLM → инструменты |
| Результат | Transcript draft / вставка в рукопись | Ответ или staged change set в том же session feed |
| Запись в проект | Пользователь переносит текст | Пользователь применяет preview |
| Исходник | Immutable audio и source transcript | Текст поручения, журнал действий и revisions |
| Кнопка остановки | Завершает аудиосессию | Останавливает дальнейшее планирование и tool calls |

Фраза в режиме диктовки не становится свободным LLM-поручением. Поручение в
режиме ассистента не попадает в рукопись как проза без явного действия.

## 3. Продуктовые границы первой версии

### Входит

- запуск из текущего проекта, документа или выделенного фрагмента;
- два способа общения внутри одного Assistant run: `Голосом` и `Чат`;
- переключение между ними без потери history, target, context или change set;
- live transcript поручения в той же визуальной модели, что диктовка;
- silence checkpoint `Выполнить поручение?` без автоматической отправки;
- несколько последовательных voice/text turns в одной assistant session;
- необязательный корневой `ASSISTANT.md` — версионируемое руководство ассистента
  для конкретного произведения;
- управляемая память проекта для канонических фактов и решений;
- чтение рукописи, черновиков, заметок, словаря, структуры, версий и
  расшифровок в пределах проекта;
- поиск по всему проекту;
- ответы по материалам проекта со ссылками на источники;
- создание и изменение текста, заметок, черновиков, структуры и словаря;
- подготовка изменений в изолированном change set;
- diff/preview по всем затронутым объектам;
- атомарное применение, revisions, provenance и групповая отмена;
- журнал поручений и действий;
- системный LLM-провайдер из server env;
- пользовательские OpenAI-compatible провайдер, model ID и API key;
- проверка совместимости до выбора провайдера для ассистента.

### 3.1. Критерий самостоятельной продуктовой ценности

Baseline не допускает реализацию «микрофон над обычным чатом»: STT → один prompt
→ вставка ответа. Assistant mode обязан одновременно поддерживать:

1. продолжение работы с ранее надиктованным stable text в том же ordered run;
2. разрешение структурных, семантических и позиционных ссылок автора в stable
   IDs/anchors;
3. канонический контекст проекта из дерева, словаря, guide и Project Memory;
4. видимый trace от исходного поручения до каждой proposed operation;
5. preview и выбор независимых change groups до commit;
6. atomic Apply и grouped compensating Undo;
7. длительные durable sessions с recovery и ограниченной задержкой.

Если провайдер вернул prose без project tools, такой ответ может быть показан как
совет, но не выдаётся за выполненную правку и не имеет пути прямой вставки в
canonical manuscript.

### Не входит в первую версию

- автономная фоновая работа после завершения поручения;
- действия вне «Студии автора»;
- совместное редактирование;
- самостоятельная публикация, отправка писем или размещение файлов;
- окончательное удаление проекта, документа, аккаунта или исходного аудио;
- изменение privacy, retention, credentials и других security-настроек моделью;
- обучение модели на рукописи средствами продукта;
- скрытое автоматическое «улучшение» текста;
- прямой доступ провайдера к внутренней сети или базе данных.

## 4. Модель доверия и уровни действий

| Уровень | Примеры | Поведение |
|---|---|---|
| A0 — чтение | открыть документ, искать, прочитать словарь, версии, заметки | Выполняется без отдельного подтверждения внутри выбранного scope |
| A1 — создание | создать заметку/черновик, добавить термин, сделать snapshot | Попадает в change set; применяется общей кнопкой |
| A2 — изменение | переписать фрагмент, переместить сцену, переименовать узел, массовая замена | Обязательный diff/preview и явное «Применить» |
| A3 — чувствительное | экспорт, архив, корзина, восстановление, повторная расшифровка | Ассистент только готовит параметры и открывает обычный native flow |
| A4 — необратимое | окончательное удаление, удаление аккаунта, очистка local-only recovery | Недоступно модели |

Пользователь подтверждает **понятный change set**, а не сырые JSON-аргументы
инструментов. Для A3/A4 модель не может получить обходной tool. Provider,
credentials, privacy и writer settings меняются только пользователем в native UI;
baseline model surface умеет прочитать лишь несекретные preferences/capabilities.

## 5. Два способа общения внутри Assistant

`Голосом` и `Чат` — два input modes одного `AssistantRun`, а не разные агенты,
provider contexts или истории. У них общие target, project context, guide revision,
Project Memory access, MCP policy, activity и change sets.

### 5.1. Live-голосовое поручение

Assistant workspace является состоянием Studio, а не отдельной полноэкранной
страницей. Для targeted run на desktop одновременно видны:

- дерево проекта слева;
- полноценный manuscript editor в центре, сохраняющий cursor, selection,
  formatting, undo/redo и autosave;
- docked assistant panel справа со stable/partial transcript, ordered history,
  activity, result и recorder/status dock;
- постоянные target, project context, guide и provider/model в compact header.

До открытия docked Assistant обычный Studio сохраняет прежнюю спокойную
композицию: manuscript — отдельное плавающее полотно с верхним воздухом,
скруглённой рамкой и ограниченной шириной. Открытие панели переключает только
layout presentation: внешний отступ и радиус manuscript исчезают, а headers
project tree, editor toolbar и Assistant образуют одну прямоугольную строку
высотой 56 px сразу под главным header. Закрытие панели возвращает плавающее
полотно без remount editor и потери его состояния.

Общий `AssistantDockShell` используется в Studio и Canvas. На desktop он занимает
правые 380 px workspace без внешнего отступа, радиуса и плавающей тени; Canvas
пересчитывает доступную ширину, но не меняет canonical координаты узлов. На tablet
это управляемый overlay от правого края, на mobile — полноширинное presentation. Закрытие
возвращает focus в control, который открыл panel. Один run и выбранное представление
сохраняются при переходе Studio ↔ Canvas; background run не останавливается.

Открытие панели не демонтирует editor и не создаёт второй документ. Voice и Chat меняют
только input/feed внутри этой панели. Для global run в центральной области остаётся
последний открытый документ или project overview; когда target разрешён, нужный
документ открывается там же без потери истории run.

Этот режим принят как единый непрерывный авторский flow. Поэтому docked panel не
использует display-кегль для отдельных voice turns: основной transcript/chat имеет
`14 px`, activity `13 px`, supporting/meta `10–11 px`, service labels `9 px`.
Готовый свежий change set предлагает `Применить сразу` с grouped Undo либо
`Проверить изменения`. Подробный S-21 открывается modal поверх Studio без изменения
route/URL; полноэкранный переход из panel считается регрессией. Мотивация и запрет
на откат закреплены в `ADR-007-assistant-continuous-workspace.md`.

В S-21 действие `Попросить исправить` заменяет строку review-действий формой с
двумя кнопками: `Отмена` и `Отправить`. `Применить изменения` не показывается рядом
с `Отправить`: сначала автор выбирает, применяет ли текущий вариант или формулирует
новое уточнение.

Transcript и history имеют собственную прокрутку: header, target/mode и composer остаются
на месте, wheel/touch не передаётся Canvas или Studio. Auto-follow работает только у конца
feed; позиция хранится отдельно для каждого run. В review независимо прокручиваются
группы, точный diff и контекс, а footer с единственным primary action остаётся видимым.

Общий message renderer переносит длинные token/URL, показывает valid JSON как
форматированный, сворачиваемый и точно копируемый code surface. Сырой HTML не
интерпретируется; fenced code имеет отдельную keyboard-accessible прокрутку.

До voice capture применяется тот же security preflight, что для диктовки.
`getUserMedia` доступен только на HTTPS или browser-trusted loopback origin.
Raw LAN HTTP не считается voice-ready: интерфейс не имитирует permission error,
а показывает отдельное объяснение, `Открыть localhost` и доступный Chat mode.

Поток:

1. Приложение использует существующий STT-контур для преобразования голоса в текст.
2. LLM получает текст, а не обязано поддерживать audio input.
3. Во время захвата аудио и transcript turn хранятся локально для recovery.
4. Короткие паузы только стабилизируют partial.
5. После продолжительной тишины интерфейс показывает checkpoint
   `Выполнить поручение?`, но не отправляет текст и не запускает LLM.
6. Обычная речь после checkpoint продолжает тот же transcript turn.
7. `Выполнить` или точная голосовая команда `Команда, выполнить поручение`
   flush/stabilize transcript, ставит microphone capture на паузу и запускает turn.
8. `Продолжить говорить` или `Команда, продолжить` закрывает checkpoint без
   запуска LLM.
9. Если пользователь ничего не выбирает, состояние остаётся waiting; timeout не
   применяет действие автоматически.
10. Во время работы стенограмма остаётся в панели read-only, а activity и
    streaming answer/proposal появляются ниже в том же feed.
11. Текстовая mutation по текущему документу может показываться как встроенный
    live preview в manuscript surface. Preview обязан иметь постоянную пометку
    `Предложение · не применено`, не участвует в canonical revision, autosave или
    ручном undo history и исчезает/становится stale при несовместимой ручной правке.
12. Готовый свежий change set можно атомарно применить прямо из panel; подробная
    проверка открывается modal поверх текущего workspace и не меняет route/URL.
13. После ответа `Продолжить голосом` создаёт следующий turn того же run с теми же
    target/context, provider и model.

Silence threshold — preference `3 / 5 / 8 секунд / выключено`, default 5 секунд.
Он применяется только после обнаруженной речи и не считается подтверждением.

По умолчанию аудио поручения удаляется после durable transcript и не попадает в
список голосовых сессий. Настройка «Хранить аудио поручений» включается отдельно.
Сомнительные слова и destination должны быть исправимы до `Выполнить`; словарь
проекта используется STT-контуром.

### 5.2. Привычный чат

Chat mode нужен, когда говорить неудобно/невозможно или точное текстовое уточнение
быстрее голоса.

- UI показывает привычный chronological message feed и sticky text composer в
  той же docked-панели рядом с рукописью;
- отправка только явная: button или `Ctrl/Cmd+Enter`; plain `Enter` настраивается;
- Chat turn не запускает STT, не создаёт аудио и не использует silence checkpoint;
- user message становится durable `AssistantTurn(inputMode=text)`;
- activity, sources, answer и change-set summary появляются в том же run;
- attachments — только ссылки на разрешённые project objects/selection, не
  произвольная загрузка файлов в baseline;
- target/context/guide/provider всегда видны и совпадают с voice mode;
- completed voice turn можно уточнить текстом, затем продолжить голосом;
- переключение Voice/Chat сохраняет открытый документ, selection, cursor,
  scroll position и неприменённый live preview;
- переключение во время provider/tool work не отменяет request, но composer
  disabled до terminal/recoverable state.

Переключение из active listening:

1. microphone capture ставится на паузу;
2. partial flush выполняется best effort;
3. видимый transcript сохраняется как unconfirmed draft;
4. UI предлагает `Перенести в чат`, `Вернуться к голосу`, `Удалить черновик`;
5. ничего не отправляется LLM автоматически;
6. `Перенести в чат` копирует текст в composer, но не создаёт turn до Send.
7. `Вернуться к голосу` продолжает тот же durable draft; `Удалить` удаляет только
   unconfirmed turn и не затрагивает историю run.

Возврат в `Голосом` создаёт следующий voice turn того же run. History маркирует
input mode (`голосом` / `текстом`), но не разделяется на две вкладки.

### 5.3. Несколько чатов в одном проекте

Один Assistant chat соответствует одному `AssistantRun`. Это не противоречит
правилу Voice/Chat: голос и текст продолжают выбранный run, а `Новый чат` создаёт
новый независимый run только после отправки первого turn.

- все чаты используют один `projectId`, но имеют независимые target, drafts,
  ordered turns, activity, change sets и review state;
- `visibleRunId` определяет только содержимое docked panel и не является lock на
  выполнение других runs;
- working run продолжает работу при переключении чата или скрытии панели;
- unread/status marker сообщает о готовом результате, ошибке или review;
- новый чат наследует текущий document context, provider/model и active guide,
  но показывает их до Send и позволяет изменить scope;
- параллельные write proposals не сливаются автоматически: каждый хранит base
  revision, а stale proposal требует refresh/rebase через новый model/tool turn;
- на desktop и tablet manuscript остаётся mounted, на mobile переключается только
  presentation state `Рукопись / Ассистент`.

Design reference: S-38 thread switcher, S-39 new chat, S-40 stale proposal,
S-41 tablet overlay и Mobile P-11. Implementation evidence: `E-A-THREADS` и
`A-THREADS` в [TRACEABILITY.md](../implementation/TRACEABILITY.md).

## 6. Контекст поручения

Перед и во время assistant session интерфейс отдельно показывает:

### Target — куда писать

- текущая глава/сцена — default при запуске из документа;
- `Не задано` — допустимый default глобального запуска из проекта;
- выделение — если поручение относится к фрагменту; оно прикрепляется через
  контекстное меню выделенного текста (правая кнопка, `Shift+F10`/клавиша меню
  или долгое нажатие), без постоянной панели над рукописью;
- новый draft/note — явный выбор;
- дополнительные targets появляются только в change-set preview;
- глобальный run не означает право незаметно писать во все файлы: конкретные
  targets определяются из поручения и подтверждаются в preview.

### Project context — что можно читать

- текущий проект — обязателен;
- весь проект через MCP по мере необходимости — default;
- ограничить текущим разделом/выбранными материалами — optional privacy mode.

`Весь проект` не означает mutation всего проекта и не отправляет его одним prompt.
Модель получает:

- системные правила и краткое описание проекта;
- выбранный фрагмент, если он разумного размера;
- идентификаторы и breadcrumbs доступных объектов;
- минимальный manifest доступных инструментов;
- результаты read tools по мере необходимости.

Tool outputs пагинируются и ограничиваются по размеру. Секреты, credentials,
private storage URLs, чужие проекты и внутренние служебные поля никогда не
включаются в контекст.

### 6.1. Разрешение точных ссылок и ограничения правки

Ассистент не передаёт в mutation tool расплывчатую фразу пользователя. Перед
staging он преобразует её в `ResolvedTarget`:

```json
{
  "nodeId": "stable-node-id",
  "baseRevision": 17,
  "ranges": [
    {"blockId": "dialogue-2", "start": 0, "end": 184, "reason": "все реплики сцены"}
  ],
  "protectedRanges": [
    {"blockId": "dialogue-last", "start": 0, "end": 41, "reason": "не менять последнюю реплику"}
  ],
  "sourcePhrase": "сделай диалог менее прямолинейным, но не меняй последнюю реплику"
}
```

Смысл ссылки определяет LLM, читая структуру и содержимое через MCP. Сервер не
пытается угадать намерение по регулярным выражениям, спискам глаголов, синонимов,
гипонимов или «похожих» слов. В частности, фразы `поправь`, `сделай`,
`вернись`, `во всех репликах` и `запомни` не маршрутизируются отдельным
лексическим классификатором.

Каждый confirmed turn начинается с изолированного семантического шага:

```json
{
  "tool": "assistant.begin_turn",
  "arguments": {
    "mode": "answer | change | clarify",
    "summary": "краткое понимание результата или прямой уточняющий вопрос"
  }
}
```

На этом шаге модель получает только текущее авторитетное поручение и структурный
scope run. `mode=change` означает требуемое изменение состояния проекта,
`mode=answer` — чтение/объяснение без mutation, `mode=clarify` — действительно
недостающую информацию, которую нельзя получить read tools. Классификация
основана на желаемом результате, а не на словаре формулировок.

После декларации модель получает полный доступный по policy каталог проекта и
сама выбирает read tools. Позиционные (`второй абзац`), scope- (`в этой сцене`) и
entity-ссылки (`во всех репликах Марины`) разрешаются последовательностью
`project.get_structure` / `tree.*` / `project.search` / `document.*` /
`mentions.find`. При нескольких кандидатах модель выбирает `clarify`; mutation
до ответа не создаётся.

Когда уже известна буквальная строка, `document.find_exact` выполняет только
точный регистрозависимый поиск в указанной revision и возвращает проверенные
Unicode-координаты `blockId/start/end/expectedText`. Это безопасная индексация,
а не смысловой resolver. Модель не считает offsets вручную и копирует anchors в
`document.replace_range`.

Если изменение затрагивает несколько уже найденных абзацев или реплик, модель
использует `document.find_exact_batch`: один read-вызов принимает до 50
буквальных запросов одной revision и возвращает результаты в исходном порядке.
Все полученные anchors передаются в один атомарный `document.replace_range`.
По умолчанию turn не имеет лимита tool calls или provider cycles. В настройках
проекта автор может задать положительный лимит tool calls на одно поручение;
пустое значение означает отсутствие ограничения.
Один OpenAI-compatible provider request может ждать до 10 минут. Эффективное
значение задаётся только server-side переменной `LLM_REQUEST_TIMEOUT` (default
`10m`) и одинаково применяется к системному провайдеру, пользовательскому
OpenRouter и любому другому пользовательскому OpenAI-compatible подключению.
Сохранённое подключение не может переопределить этот timeout; скрытый default
`120000 ms` запрещён. Однако внешний
gateway/provider может завершить отдельный нестриминговый ответ раньше: реальные
Opus-запуски после успешного чтения проекта обрывали первый генеративный шаг ровно
через 120 секунд уже при локальном лимите 10 минут. В одном из повторных случаев
модель успела выполнить девять read-вызовов (инструкция, заметки, две сцены и целевой
документ), но не успела вернуть даже первый `document.append_chunk`. Поэтому
увеличение client timeout не считается long-form стратегией. Chat Completions
запросы отправляются с `stream=true`: host читает OpenRouter SSE keepalive comments,
накапливает текст и фрагментированные `tool_calls[].function.arguments`, а tool
исполняет только после получения полного валидного JSON-вызова. `reasoning` и
`reasoning_details` собираются в единый assistant continuation без перестановки
opaque-блоков. Mid-stream provider error нормализуется в безопасный класс, включая
edge status 524. Timeout не запускает автоматический retry и не удваивает
стоимость/токены без решения автора.
Многоэлементные обновления контекста не обходят этот защитный лимит серией
одиночных write-вызовов: для 2–100 новых терминов или записей памяти модель обязана
использовать `dictionary.add_batch` или `project_memory.add_batch`. Один пакетный
вызов разворачивается сервером в независимые staged operations с теми же preview,
частичным Apply, provenance и Undo, что и одиночные добавления.

Новый content-bearing узел должен получить заголовок и полное начальное содержимое
в одном `tree.create_nodes_batch`; один вызов принимает 1–50 sibling-узлов и
сохраняет порядок массива. Host разворачивает пакет в обычные независимые
`tree.create_node` staged operations. Последовательность «создать пустой узел →
дописать его через `document.append` до Apply» запрещена: staged ID ещё не
существует в canonical tree и не может быть target следующего tool call.

Если новое содержимое существующего документа может превысить примерно 1 500 слов,
модель обязана использовать `document.append_chunk`, а не формировать весь текст в
одном `document.append`. Один вызов содержит не более 9 000 символов и 1 500 слов;
рекомендуемый размер — 600–1 200 слов. `sequence` начинается с 1, `targetWords`
остаётся неизменным, а `finalChunk=true` допустим после накопления не менее 80% от
приблизительной цели автора. Host объединяет все чанки в одну in-memory staged
операцию поверх одной base revision, обновляет live non-canonical preview и при
Apply создаёт ровно одну новую revision. В provider replay полный текст уже
принятого чанка заменяется компактной метаинформацией; следующий шаг получает
счётчики и хвост до 800 символов. Это удерживает стоимость от квадратичного роста.
Сбой после первого или последующего чанка финализирует накопленное как partial
review. Незавершённая серия без `finalChunk` не может выглядеть готовым результатом.

`protectedRanges` проверяются сервером после генерации proposal: соответствующие
plain-text ranges и ProseMirror subtrees должны совпадать с base. Каждая замена
дополнительно содержит `expectedText`; несовпадение base revision, block ID,
границ или ожидаемого текста отклоняется до staging. Проверка не может быть
оставлена обещанию модели.

Исправление uncertain/ошибочно распознанного имени до `Выполнить` обновляет
только transcript draft текущего turn. Оно не создаёт дополнительный LLM request.
При желании пользователь отдельным staged действием добавляет исправление как
variant/canonical term словаря.

### 6.2. Динамическая структура произведения

Assistant и MCP не имеют глобально зашитой иерархии `часть → глава → сцена`.
Каждый проект хранит версионируемую `ProjectStructure`:

- template ID и понятный root label;
- доступные node types с stable keys, labels и icons;
- `contentBearing` для наличия редакторского документа;
- `canBeRoot` и `allowedChildren`;
- structure revision, на которой готовился change set.

`project.get_structure` включается в manifest любого run. `tree.create_node`,
`tree.create_nodes_batch`, `tree.move_node`, семантическое разрешение target
моделью, breadcrumbs, preview и export читают эту schema через application
service и повторно проверяют revision перед Apply.

Текущий шаблон manuscript сохраняет существующее поведение. `screenplay`,
`game_design`, `free/custom` могут задавать иные типы и nesting rules. Неизвестный
будущий node type остаётся доступен Assistant как schema data и не требует нового
tool name или изменения prompt-кода.

### `ASSISTANT.md` — постоянный авторский контекст

У каждого проекта может быть ровно один специальный root artifact `ASSISTANT.md`.
В UI он называется `Руководство ассистента`; это не системный файл сервера и не
скрытый prompt.

Назначение:

- голос автора, тон, ритм и композиционные предпочтения;
- лицо, время, дистанция повествователя и формат диалогов;
- желательная/запрещённая лексика, клише и повторяющиеся конструкции;
- канон и приоритеты источников;
- правила обращения к словарю, карточкам персонажей, заметкам и черновикам;
- короткие положительные/отрицательные примеры;
- проектные ограничения вроде `не объяснять магию до третьей части`.

Правила:

- файл необязателен; при его отсутствии система не изобретает стиль автора;
- создаётся пустым, из шаблона или как assistant-generated draft с preview;
- редактируется в обычном native editor, имеет revisions/history/restore-as-new;
- не входит в рукопись, поиск читательского текста и экспорт по умолчанию;
- ссылки на словарь/заметки хранят stable IDs, а не копии их содержимого;
- exact current revision автоматически включается в каждый confirmed turn проекта,
  целевой и глобальный;
- в session header видны `Руководство активно` и revision; файл можно открыть;
- явное поручение текущего turn может переопределить stylistic default, но не
  system/security policy;
- лимит baseline — 24 000 символов; UI показывает размер и требует сократить
  превышение до сохранения.

Иерархия инструкций:

```text
system/security/developer policy
  > подтверждённое поручение текущего turn
  > активная revision ASSISTANT.md
  > извлечённые manuscript/notes/tool results как недоверенные данные
```

`ASSISTANT.md` не может расширить project scope, tool allowlist, уровни A0–A4 или
обойти preview/Apply. Содержимое других документов с текстом вроде «игнорируй
правила» не становится руководством.

### Словарь, память и guide — разные хранилища

| Намерение автора | Хранилище | Пример |
|---|---|---|
| Как писать слово/имя | Словарь | `Эли́на`, варианты `Элина/Elina`, тип `character` |
| Что является фактом/решением | Память проекта | `Элина не умеет плавать`; источник — заметка персонажа |
| Как ассистент должен писать | `ASSISTANT.md` | `близкая третья персона; не объяснять магию напрямую` |

Память проекта — не скрытое состояние LLM. Это видимые автору, версионируемые
records:

```text
id
kind: canon_fact | character_fact | world_rule | plot_promise |
      continuity | editorial_decision | other
statement
scopeNodeIds[]
sourceRefs[]
tags[]
active
alwaysRelevant
revision
```

`alwaysRelevant` используется редко и только по явному выбору пользователя:
компактные active records могут входить в prompt summary. Остальная память
доступна через `project_memory.search/get` и извлекается по необходимости.

Маршрутизация естественного поручения:

- `Слово «аэросад» всегда пиши через “о”, добавь в словарь` →
  `dictionary.add`;
- `Персонажа зовут Элина, варианты Эля и Lina тоже её имя` →
  dictionary record типа `character`;
- `Запомни: Элина не умеет плавать` → `project_memory.add`;
- `Всегда пиши сцены Элины в близкой третьей персоне` →
  предложение обновить `ASSISTANT.md`.

Ассистент может уточнить неоднозначное назначение. Любой add/update остаётся
staged: для единичной A1-записи допускается компактный inline preview, для
нескольких объектов используется S-21. Никакой provider-side memory не считается
источником истины проекта.

## 7. Assistant run и change set

### Состояния run и turn

```text
run_active
  voice_draft → listening → silence_checkpoint → listening
                                  └→ confirmed → microphone_paused
  text_draft → explicit_send → confirmed
  microphone_paused → planning → reading → proposing
                                            ├→ awaiting_review
                                            ├→ completed_no_changes
                                            └→ failed_recoverable
  awaiting_review → applying → applied → next_turn/listening
  awaiting_review → rejected → next_turn/listening
  planning/reading/proposing → stopped
```

### Инварианты

- provider и model фиксируются в момент запуска run;
- run всегда привязан к `ownerUserId`, одному `projectId`, nullable default target
  и project-context policy;
- run/turn фиксирует ID и revision `ASSISTANT.md`, использованные при prompt
  assembly; изменение guide влияет только на следующий confirmed turn;
- run содержит ordered `AssistantTurn`; каждый turn фиксирует
  `inputMode=voice|text` и видимый transcript/message;
- проект может иметь несколько active/working runs; видимость run в UI не меняет
  его execution status;
- draft, target, activity и change sets изолированы по run ID;
- смена input mode не создаёт новый run и не меняет provider/target/context;
- unconfirmed voice/chat drafts не вызывают provider/tools;
- read tools работают только в scope run;
- write tools формируют `AssistantChangeSet`, а не меняют canonical state;
- каждый change содержит base revision/version;
- canonical state byte-for-byte не меняется до «Применить»;
- применение выполняется одной серверной транзакцией либо не выполняется вовсе;
- stale base блокирует применение и предлагает пересчитать change set;
- каждая revision содержит `source=assistant`, run ID, operation ID и понятное
  пользовательское описание;
- Undo группирует весь operation ID и создаёт компенсирующие revisions;
- остановка run запрещает новые model/tool calls, но сохраняет уже собранный draft;
- provider timeout после успешных mutation tools не выбрасывает их: host завершает
  partial `AssistantChangeSet`, явно помечает его незавершённым и оставляет только
  для review; canonical state по-прежнему не меняется до Apply;
- long-form текст накапливается ограниченными `document.append_chunk` вызовами в
  одной staged document operation; каждый успешный чанк сразу доступен live preview,
  не создаёт отдельную canonical revision и сохраняется как partial при обрыве;
- при старте приложения persisted `working` turn считается прерванным процессом:
  turn получает recoverable `execution_interrupted`, run возвращается в `active`,
  canonical state не меняется, а автор может повторить поручение. Повторный старт
  идемпотентен и не создаёт дублирующих recovery events.

## 8. MCP: принцип интеграции

MCP является единой контрактной поверхностью функций проекта, но не отдельной
копией бизнес-логики.

```text
LLM provider
  ↕ OpenAI-compatible Responses или Chat Completions tool calling
Assistant Orchestrator
  ↕ локальный MCP client + policy/approval layer
Writer’s Studio MCP Server
  ↕ application services
Projects / Documents / Tree / Search / Dictionary / Memory / Guide / Versions /
Dictation / Export
```

Baseline не требует, чтобы внешний LLM-провайдер умел подключаться к remote MCP.
Orchestrator преобразует MCP tool schemas в поддерживаемый provider function/tool
calling contract, локально выполняет вызовы и возвращает результаты модели.

Это обеспечивает:

- работу с несколькими OpenAI-compatible провайдерами;
- отсутствие публичного MCP endpoint;
- единый ownership/policy check;
- независимость доменной модели от provider response format;
- полный журнал и управляемые approvals.

Remote MCP может появиться как отдельный deployment adapter после security review,
но не является условием первой реализации.

## 9. MCP authentication и policy

Каждая in-process MCP session создаётся сервером с неизменяемым
`ExecutionContext`:

```text
ownerUserId + projectId + runId + turnId + targetNodeId
```

Это логическая capability boundary. Для локального in-process transport она не
обязана быть сериализованным bearer token; если MCP будет вынесен в отдельный
process/transport, тот же контекст передаётся короткоживущим подписанным token.

Требования:

- tool повторно проверяет owner и project membership;
- tool не доверяет ID, предложенным моделью;
- write tool требует активный change set и не умеет commit;
- commit endpoint вызывается приложением после UI approval и недоступен модели;
- параметры валидируются строгой JSON Schema без неизвестных полей;
- provider tool-call ID и server-side change-set operation используются для
  дедупликации staging/apply; публичный tool schema не просит модель придумывать
  idempotency key;
- read results содержат stable IDs, breadcrumbs и revision IDs;
- application logs не содержат текст рукописи, prompts, tool results или API keys;
- user-visible activity хранит осмысленное действие и ссылки на затронутые объекты.

## 10. Каталог MCP-инструментов

Названия являются baseline контракта. Реализация может группировать schemas, но
не должна терять покрытие.

### 10.1. Протокол turn, проект и навигация

| Tool | Уровень | Назначение |
|---|---:|---|
| `assistant.begin_turn` | A0/protocol | Ровно один семантический `answer/change/clarify` в начале turn |
| `assistant.get_recent_change_set` | A0 | Последний native change set, включая rejected/applied, для запроса на исправление |
| `project.get_overview` | A0 | Название, тип, synopsis, язык, цель, status |
| `project.get_structure` | A0 | Версионируемая схема типов узлов, labels и nesting rules |
| `project.list_recent` | A0 | Недавние документы текущего проекта |
| `project.search` | A0 | Поиск по manuscript/drafts/notes/transcripts |
| `project.get_activity` | A0 | Значимые revisions и операции без секретов |
| `tree.list` | A0 | Иерархия, типы, статусы и порядок |
| `tree.get_node` | A0 | Metadata и current revision узла |
| `tree.create_node` | A1 | Создать узел разрешённого текущей ProjectStructure типа |
| `tree.create_nodes_batch` | A1 | Создать 1–50 упорядоченных узлов сразу с полным начальным содержимым; результат — обычные reviewable `tree.create_node` operations |
| `tree.rename_node` | A2 | Переименовать узел |
| `tree.move_node` | A2 | Изменить parent/order |
| `tree.set_status` | A2 | Идея/черновик/редактура/готово |
| `tree.duplicate_node` | A1 | Создать альтернативный вариант |
| `tree.prepare_archive` | A3 | Открыть native archive flow, без выполнения |

### 10.2. Документы, черновики и заметки

| Tool | Уровень | Назначение |
|---|---:|---|
| `document.get` | A0 | Получить Tiptap/plain-text представление и revision |
| `document.find_exact` | A0 | Найти буквальную строку и вернуть проверенные block-local Unicode anchors |
| `document.find_exact_batch` | A0 | Найти до 50 буквальных строк одной revision одним вызовом и вернуть упорядоченные exact anchors |
| `document.get_range` | A0 | Прочитать диапазон по block IDs/anchors |
| `document.list_references` | A0 | Source refs, связанные аудиодиапазоны и mentions |
| `document.insert` | A2 | Вставить block-safe содержимое в change set |
| `document.replace_range` | A2 | Атомарно заменить один или несколько диапазонов с exact anchors, `expectedText`, protected ranges и base revision |
| `document.append` | A2 | Добавить содержимое в конец |
| `document.append_chunk` | A2 | Добавить ограниченный long-form чанк в один накопительный staged proposal; 1–16 чанков, одна revision после Apply |
| `document.set_title` | A2 | Изменить заголовок content-bearing node |
| `document.apply_marks` | A2 | Разрешённое форматирование по schema |
| `document.create_draft` | A1 | Создать новый черновик |
| `note.create` | A1 | Создать заметку |
| `note.update` | A2 | Изменить заметку |
| `annotation.list` | A0 | Прочитать inline-пометки всего проекта с lifecycle/anchor filters и собственными revisions |
| `annotation.get` | A0 | Прочитать inline-пометку и immutable Assistant anchor history |
| `annotation.propose_fixes` | A1 | Подготовить до 50 исправлений по ID/revision заметок; сервер сам выводит exact anchors и группирует operation по документу |
| `document.prepare_transfer` | A2 | Подготовить перенос между любыми content-bearing узлами |
| `draft.prepare_transfer` | A2 | Подготовить перенос в manuscript с preview |
| `document.prepare_replace_all` | A2 | Подготовить project-wide replace с preview |

Write tools принимают ProseMirror-safe operations или структурированный текст.
Произвольный HTML не допускается.

`annotation.propose_fixes` намеренно не принимает offsets/block IDs. Assistant
сначала использует `annotation.list/get`, затем передаёт только stable ID,
annotation revision, исходную цитату, замену и `resolve`. Application service
повторно проверяет exact anchor и document revision, строит `document.replace_range`
на сервере и в Apply-транзакции версионирует текст, текущий anchor и историю.
Полное совпадение диапазона перепривязывается к replacement; частичное пересечение
становится `anchor_status=detached` без semantic guessing. Undo восстанавливает
оба состояния.

### 10.3. Словарь и знания проекта

| Tool | Уровень | Назначение |
|---|---:|---|
| `dictionary.list` | A0 | Активные/неактивные термины и варианты |
| `dictionary.find` | A0 | Поиск канонического написания |
| `dictionary.add` | A1 | Добавить термин в change set |
| `dictionary.add_batch` | A1 | Добавить 2–100 терминов одним tool call как независимые operations |
| `dictionary.update` | A2 | Обновить варианты/type/language |
| `dictionary.set_active` | A2 | Активировать или отключить |
| `mentions.find` | A0 | Найти упоминания имени, места, события или термина |
| `assistant_guide.get` | A0 | Получить revision `ASSISTANT.md`, зафиксированную при подтверждении текущего turn |
| `assistant_guide.propose_create` | A1 | Подготовить новый guide как change set |
| `assistant_guide.propose_update` | A2 | Предложить patch guide с preview |
| `project_memory.list` | A0 | Список active/inactive records с фильтрами |
| `project_memory.search` | A0 | Найти релевантные факты/решения |
| `project_memory.get` | A0 | Получить record, sources и revision |
| `project_memory.add` | A1 | Добавить record в change set |
| `project_memory.add_batch` | A1 | Добавить 2–100 records одним tool call как независимые operations |
| `project_memory.update` | A2 | Изменить statement/kind/scope/sources/tags |
| `project_memory.set_active` | A2 | Деактивировать или вернуть record |
| `project_memory.set_always_relevant` | A2 | Изменить явное включение в prompt summary |

Карточки персонажей/мира после их появления должны расширить этот же namespace,
а не создавать параллельный закрытый контур.

Нормативное решение для цельного Assistant flow: если одно подтверждённое
поручение содержит две и более новых записи одного типа, Assistant использует
соответствующий batch tool. Это сохраняет поручение единым turn и не заставляет
автора вручную дробить обновление словаря/памяти из-за внутреннего лимита шагов.
Сервер ограничивает один batch сотней элементов, проверяет конфликты для каждого
элемента и возвращает индекс и typed conflict; конфликтующие элементы не
стейджатся, остальные остаются отдельными карточками review. Canonical state до
Apply не меняется. Batch остаётся предпочтительным и без глобального лимита:
он уменьшает число provider round trips, расход токенов и стоимость. Если автор
задал проектный лимит шагов, batch также помогает уложиться в этот бюджет.

Минимальные mutation arguments:

```json
{
  "dictionary.add": {
    "canonical": "Элина",
    "variants": ["Эля", "Elina"],
    "kind": "character",
    "pronunciation": "э-ЛИ-на",
    "language": "ru",
    "active": true
  },
  "project_memory.add": {
    "kind": "character",
    "statement": "Элина не умеет плавать.",
    "scope": "project",
    "scopeNodeId": null,
    "sourceReferences": [{"kind": "note", "id": "...", "label": null, "quote": null, "start": null, "end": null}],
    "tags": ["Элина", "вода"],
    "active": true,
    "alwaysRelevant": false
  }
}
```

Для нескольких записей те же item schemas передаются массивом:

```json
{
  "dictionary.add_batch": {
    "items": [
      {"canonical": "Элина", "variants": ["Эля"], "kind": "character", "pronunciation": "э-ЛИ-на", "language": "ru", "active": true},
      {"canonical": "Северный тракт", "variants": [], "kind": "place", "pronunciation": "", "language": "ru", "active": true}
    ]
  },
  "project_memory.add_batch": {
    "items": [
      {"kind": "character", "statement": "Элина не умеет плавать.", "scope": "project", "scopeNodeId": null, "sourceReferences": [], "tags": ["Элина"], "active": true, "alwaysRelevant": false},
      {"kind": "world_rule", "statement": "Северный тракт закрывают после заката.", "scope": "project", "scopeNodeId": null, "sourceReferences": [], "tags": ["Северный тракт"], "active": true, "alwaysRelevant": false}
    ]
  }
}
```

MCP schemas и обычный project API вызывают один application service. Для UI/API
доступны CRUD/list/search endpoints с теми же ownership, revision, idempotency и
change-set semantics; второй обходной write path не создаётся.

Project Memory UI использует явный versioned REST-контракт: list может включать
inactive records, update требует `baseRevision`, history возвращает неизменяемые
snapshots, а restore выбранного snapshot создаёт следующую revision. Клиент отправляет
только writable allowlist (`kind`, `statement`, `scope`, `scopeNodeId`,
`sourceReferences`, `tags`, `active`, `alwaysRelevant`, `baseRevision`), но не
серверные `id/projectId/source/timestamps`.

### 10.4. Версии, расшифровки и аудио

| Tool | Уровень | Назначение |
|---|---:|---|
| `version.list` | A0 | Revisions и named snapshots |
| `version.compare` | A0 | Semantic diff двух versions |
| `version.create_snapshot` | A1 | Подготовить именованную версию |
| `version.prepare_restore` | A3 | Открыть native restore flow |
| `dictation.list_sessions` | A0 | Сессии, destination, status и duration |
| `transcript.get` | A0 | Source/draft текст и timestamps в scope |
| `transcript.get_uncertain` | A0 | Сомнительные места |
| `audio.get_source_range` | A0 | Метаданные диапазона; не raw URL |
| `transcript.prepare_transfer` | A2 | Подготовить перенос transcript в manuscript |
| `transcript.prepare_retranscribe` | A3 | Открыть native re-transcribe flow |

Модель не получает raw audio автоматически. Передача аудио LLM-провайдеру не
входит в assistant baseline.

### 10.5. Экспорт, recovery и settings

| Tool | Уровень | Назначение |
|---|---:|---|
| `export.preview` | A0 | Ordered outline и доступные форматы |
| `export.prepare` | A3 | Открыть native export flow с заполненными параметрами |
| `recovery.get_status` | A0 | Наличие local/partial recovery без содержимого chunks |
| `settings.get_writer_preferences` | A0 | Зафиксированный при старте run снимок несекретных editor/dictation preferences |
| `assistant.get_capabilities` | A0 | Доступные tools, provider limits и policy |

Снимок содержит `language`, `commandPrefix`, command/live switches, silence
threshold, font/size/line-height/width/theme/spellcheck и word goal. Он не меняется
посреди run, поэтому повторный tool call воспроизводим. Ассистент не получает
`microphoneId`, API keys, retention, account deletion, local-chunk deletion,
privacy consent или tools для изменения любых settings.

Каждый A3 `prepare_*` возвращает не произвольную ссылку, а типизированный
server-authored destination. Он переносит все выбранные параметры native flow
(например, `format`, ordered `nodeIds`/titles или `versionId`) и текущую base
revision. Native route повторно проверяет owner/project membership и отклоняет
stale restore; отсутствующие, чужие или подменённые IDs не превращаются в prefill.

## 11. Покрытие существующих workflow

| Существующий workflow | Покрытие ассистентом |
|---|---|
| WF-02/03 Проект и структура | Чтение дерева; staged create/rename/move/status/duplicate |
| WF-04 Редактор | Staged Tiptap-safe operations, revisions, conflict check |
| WF-05–10 Диктовка | Может объяснить/найти сессию; не управляет active recorder |
| WF-11–13 Transcript | Читает artifacts, готовит перенос; не меняет source |
| WF-14 Словарь | Полный read и staged add/update/disable |
| WF-15 Версии | Read/compare; staged snapshot; restore только native flow |
| WF-16 Поиск/замена | Read search; replace только как multi-document preview |
| WF-17 Экспорт | Preview/prepare; окончательный запуск подтверждает пользователь |
| WF-18 Recovery | Только status/navigation |
| WF-19 Удаление | Не выполняется моделью |
| WF-27 Руководство ассистента | Всегда читается; create/update только staged |
| WF-28 Знания проекта | Dictionary и project memory читаются; mutations staged |

## 12. Пользовательский OpenAI-compatible провайдер

### 12.1. Поля

- название подключения;
- base URL;
- API dialect: `Auto`, `Responses API`, `Chat Completions`;
- API key;
- model ID;
- дополнительные headers — optional advanced, secret by default;
- максимальное context window — ручное значение, если endpoint не сообщает;
- сделать провайдером ассистента по умолчанию.

Request timeout не является полем пользовательского подключения. Оператор задаёт
единый `LLM_REQUEST_TIMEOUT` для всех системных и пользовательских провайдеров;
baseline использует `10m`. Это намеренно пока негибкая общая policy: она исключает
незаметный двухминутный override и одинаково защищает все подключаемые модели.

Base URL относится к серверному connector. `localhost` означает host приложения,
а не браузер пользователя; UI должен это объяснять.

### 12.2. Проверка подключения

Проверка выполняется сервером и не сохраняет провайдер по умолчанию. Она проверяет:

1. URL и TLS policy;
2. авторизацию без возврата credentials клиенту;
3. доступность указанной модели;
4. text response;
5. streaming, если заявлен;
6. function/tool calling со strict JSON arguments;
7. возврат tool result в следующий model turn;
8. последовательность минимум из двух tool calls;
9. usage metadata, если доступна;
10. понятную нормализацию provider errors.

Remote MCP support, parallel tool calls и structured outputs фиксируются как
capabilities, но не обязательны для baseline. Обязательны text generation и
надёжный tool/function calling.

### 12.3. Статусы подключения

- `Системный` — configured через env, credential не виден пользователю;
- `Готов` — обязательная assistant tool-chain и заявленный streaming прошли;
- `Готов с ограничениями` — текст, strict tools, возврат результата и два
  последовательных tool calls работают, но optional streaming/usage capability
  не подтверждена; провайдер можно выбирать для Assistant;
- `Нужно проверить` — конфигурация изменена или давно не проверялась;
- `Недоступен` — auth/network/model error либо обязательная strict tool-chain не
  пройдена; для Assistant выбрать нельзя;
- `Отключён`.

### 12.4. Secrets

- API key отправляется только на backend по TLS;
- хранится encrypted at rest с versioned key encryption key;
- после сохранения отображается только fingerprint/последние 4 символа;
- никогда не возвращается API response, logs, analytics или backup в открытом виде;
- удаление credential прекращает новые runs;
- активный run не переключает provider/model посередине;
- системный `LLM_API_KEY` из env не копируется в пользовательскую запись.

## 13. Privacy и prompt injection

- Материалы проекта считаются недоверенными данными, а не инструкциями модели.
- Единственное user-authored исключение — активная revision специального
  `ASSISTANT.md`, загруженная по artifact ID сервером, а не найденная поиском.
- Guide остаётся ниже system/security policy и не расширяет tools/scope.
- Текст рукописи, заметок и tool results отделяется от system/developer policy.
- Инструкции, найденные внутри проекта, не могут расширить scope или набор tools.
- Перед первым использованием конкретного внешнего провайдера UI показывает,
  какие типы данных могут быть отправлены.
- Для «Весь проект» нужен отдельный видимый scope, но не обязательная отправка
  всех данных одним запросом.
- Пользователь видит provider/model текущего run и ссылку на собственную data policy
  провайдера; «Студия автора» не обещает его retention/training условия.
- Техническая телеметрия не содержит prompt, manuscript, tool arguments/results.
- User-visible assistant history является пользовательским контентом и удаляется
  вместе с run/project/account согласно общей deletion policy.

## 14. Ошибки и восстановление

| Ситуация | Поведение |
|---|---|
| STT поручения недоступен | Текстовый ввод остаётся доступен; локальное аудио можно повторить |
| LLM недоступна | Поручение и выбранный scope сохранены; `Повторить` |
| LLM не ответила за глобальные `LLM_REQUEST_TIMEOUT=10m` до первой mutation | `provider_timeout`; правило одинаково для системного и любого пользовательского провайдера, проект не изменён, автоматического retry нет |
| LLM не ответила после staged mutations | Partial change set сохраняется для review с явной пометкой; ничего не применяется автоматически |
| Внешний provider обрывает один большой ответ через 120 секунд | Chat Completions использует SSE keepalive, а long-form пишется последовательными `document.append_chunk`; принятые чанки остаются в одном partial/reviewable proposal |
| Модель завершила long-form серию без `finalChunk` | `long_form_incomplete`; накопленный текст показывается как частичный review, а не как готовый результат |
| Tool schema не поддержана | Run останавливается до mutation; provider помечается `Ограничен` |
| Read tool failed | Показать, какой источник недоступен; разрешить уточнить/повторить |
| Write tool failed | Canonical state не меняется; change set остаётся draft |
| Base revision stale | `Проект изменился`; пересчитать preview |
| Пользователь нажал Stop | Новые calls запрещены; текущий сетевой запрос отменяется best effort, run и working turn атомарно получают `cancelled` |
| Reload | Durable run и change set открываются из Activity |
| Process/container restart во время `working` | На старте turn получает recoverable `execution_interrupted`, run снова доступен для `Повторить`; canonical state не меняется |
| Provider удалён | Existing history читается; retry требует выбрать новое подключение |
| Dictionary/memory duplicate | Показать существующую запись и merge/update choice |
| Guide превышает 24 000 символов | Не сохранять новую active revision; открыть editor на limit |

## 15. UX-копирайтинг

Использовать:

- `Ассистент читает выбранные материалы`;
- `Готовит изменения`;
- `Ничего не изменится до применения`;
- `2 документа · 4 изменения`;
- `Проект изменился после подготовки. Обновите предложение`;
- `Остановлено. Подготовленные изменения сохранены`;
- `Этот провайдер отвечает, но не поддерживает инструменты ассистента`.

Не использовать:

- `AI думает`;
- `Магически улучшить`;
- `Ассистент всё сделал`, пока change set не применён;
- `Безопасно`, если речь только о reversible operation;
- скрытые provider/model names в технической ошибке.

## 16. Analytics без содержимого

Допустимые events:

- `assistant_started`;
- `assistant_instruction_ready`;
- `assistant_first_tool_call`;
- `assistant_change_set_ready`;
- `assistant_change_set_applied`;
- `assistant_change_set_rejected`;
- `assistant_stopped`;
- `assistant_failed`;
- `assistant_input_mode_changed`;
- `llm_connection_tested`;
- `assistant_guide_revision_applied`;
- `project_memory_change_applied`.

Допустимы duration, counts, risk levels, provider/model technical IDs, latency,
token usage, tool name и error class. Запрещены instruction text, generated text,
document title, tool arguments/results, API key и snippets.

## 17. Критерии готовности реализации

- пользователь валидировал разницу между диктовкой и ассистентом;
- scope проекта/документа/выделения понятен без объяснения;
- пользователь понимает, что чтение уже выполняется, а изменения ещё не применены;
- на desktop рукопись и ordered assistant transcript видны одновременно во всех
  состояниях S-19/S-20/S-28; S-21 является modal текущего Studio, а не отдельным
  полноэкранным destination;
- selection/cursor/editor history не теряются при открытии Assistant или
  переключении Voice/Chat;
- live preview в рукописи всегда отличим от canonical text без опоры только на
  цвет и не попадает в autosave/undo history до Apply;
- понятно, включено ли руководство проекта и какая revision использована;
- dictionary/memory/guide различимы в voice preview и history;
- Voice/Chat переключаются в одном run без потери draft/history/context;
- multi-document preview читаем и не выглядит как скрытый автосейв;
- Stop, Apply, Reject и Undo различимы;
- provider setup объясняет отличие base URL, model ID и API key;
- ограниченный provider не может быть выбран для Assistant;
- MCP catalog прошёл ownership, injection и destructive-action threat review;
- для каждого mutation tool определены base version, idempotency и compensating undo;
- acceptance criteria AC-56–AC-142 сопоставлены с автоматическими или явно
  обозначенными ручными доказательствами;
- семантический protocol turn протестирован отдельно от tool loop; в
  intent-routing отсутствуют regex/verb/hyponym правила;
- exact replacement проверяет base revision, block-local Unicode anchors,
  `expectedText` и protected ranges на сервере;
- голосовой черновик восстанавливается после reconnect, а переход Voice → Chat
  требует явного выбора и ничего не отправляет автоматически.
