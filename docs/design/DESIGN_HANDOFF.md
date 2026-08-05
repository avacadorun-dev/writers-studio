# «Студия автора»: handoff для Product Design

> Версия: 0.7  
> Статус: dictation + assistant product baseline; S-38–S-41 реализованы и проверены  
> Дата: 4 августа 2026  
> Источник поведения: [WORKFLOWS.md](../specification/WORKFLOWS.md)  
> Архитектурные ограничения: [ARC42.md](../architecture/ARC42.md)  
> Assistant/MCP contract: [ASSISTANT_MODE.md](../specification/ASSISTANT_MODE.md)

## 1. Дизайн-задача

Спроектировать спокойную, профессиональную среду для длинного письма, в которой
голос является естественным способом создать текст **или сформулировать поручение**,
а AI не превращается в отдельный магический чат.

Пользователь должен в любой момент понимать:

1. записывается ли его голос;
2. где физически сохранена запись;
3. какой текст ещё может измениться;
4. какой текст уже является его редактируемой рукописью;
5. можно ли безопасно уйти с экрана;
6. как вернуться к источнику и предыдущей версии.
7. находится ли он в диктовке или ассистенте;
8. что ассистент уже прочитал, а что только предлагает изменить;
9. какой project/document/selection scope доступен текущему поручению;
10. какой provider/model выполняет run.

## 2. Что уже решено

Дизайнеру не нужно заново выбирать следующие основы:

- desktop-first PWA;
- mobile-first только для quick recording/recovery;
- полноценный Tiptap/ProseMirror editor;
- исходное аудио, source transcript и manuscript разделены;
- live partial визуально нестабилен и не сохраняется как document revision;
- после stop запускается quality pass, но конкретный STT-провайдер не виден в основном UX;
- поздний quality pass не перезаписывает ручные правки;
- локальная запись продолжается при сбое live-STT;
- части, главы, сцены, черновики и заметки находятся в одном project tree;
- версии реализуются внутри продукта, без зависимости от платного editor plugin;
- в MVP один автор, без collaborative cursors.
- диктовка и ассистент — два явных режима;
- assistant read tools работают в scope без approval;
- mutations всегда staged и применяются из multi-object preview;
- sensitive actions переходят в native flow, irreversible tools отсутствуют;
- пользовательский provider проверяется до выбора для Assistant;
- MCP и raw tool JSON не доминируют в обычном UX.

## 3. Experience principles

### 3.1. Рукопись — главный объект

Редактор занимает визуальный центр. AI, статистика, структура и аудио находятся рядом, но не превращают продукт в dashboard.

### 3.2. Запись должна ощущаться надёжной

Статус не ограничивается красной точкой. Он содержит короткий текст: «Запись идёт», «Сохранено на устройстве», «Живой текст недоступен».

### 3.3. Нестабильность должна быть честной

Partial может меняться. Это видно по стилю и label, но текст остаётся читаемым и не «дрожит» целиком.

### 3.4. Редактирование не является вторичным режимом

После диктовки пользователь получает знакомую editor surface с cursor, selection, undo, formatting, search и autosave. Review — это редактор с аудиоконтекстом, а не форма исправления.

### 3.5. Не использовать визуальный язык «магии»

Избегать:

- постоянных gradients и glow;
- sparkle-иконок на обычных механических действиях;
- человекоподобных сообщений от AI;
- геймификации количества слов в основном рабочем потоке;
- агрессивных success-анимаций.

### 3.6. Progressive disclosure

На первом уровне видны только действия текущей задачи. Форматирование, versions, dictionary и export раскрываются по контексту.

## 4. Целевые viewport

Обязательные макеты:

| Класс | Frame | Приоритет |
|---|---:|---|
| Desktop | 1440 × 1024 | Все экраны и прототип |
| Compact desktop | 1024 × 768 | Studio, recording, review |
| Tablet | 834 × 1194 | Editor и review |
| Mobile | 390 × 844 | Projects, quick record, live, pause, finalizing, recovery |

Минимальная поддерживаемая ширина desktop Studio — 1024 px. Ниже используется tablet/mobile navigation model.

## 5. Информационная архитектура

```text
Рабочее пространство
├── Проекты
│   ├── Недавние
│   ├── Все
│   ├── Архив
│   └── Корзина
├── Studio: выбранный проект
│   ├── Рукопись
│   │   └── Часть → глава → сцена
│   ├── Черновики
│   ├── Заметки
│   ├── Голосовые сессии
│   ├── Ассистент / Activity
│   ├── Словарь
│   ├── Версии
│   ├── Поиск
│   └── Экспорт
├── Recovery Center
└── Настройки
    ├── Интерфейс редактора
    ├── Диктовка и команды
    ├── AI и ассистент
    ├── Хранение
    └── Приватность
```

`Голосовые сессии` — не папка файлов в основном дереве рукописи. Это отдельное представление с destination и status.

## 6. Desktop Studio shell

### Размеры как starting point

- app header: 48–56 px;
- project tree: 240–300 px, collapsible;
- manuscript column: 680–820 px;
- context panel: 280–360 px, collapsible;
- Assistant panel: 340–440 px, docked справа, resizable и collapsible;
- manuscript line length: примерно 60–80 знаков;
- recording dock: bottom overlay/panel, не перекрывающий selection.

Это не design tokens, а ограничения композиции.

### Header

Слева направо:

- back to projects;
- project title;
- breadcrumb active node;
- save status;
- project search;
- voice entry / два ясных действия `Диктовка` и `Ассистент`;
- export;
- overflow;
- profile.

На широком desktop `Диктовка` и `Ассистент` могут быть отдельными подписанными
actions. В compact/mobile общий entry `Голос` открывает S-18. Последний режим
можно подсветить, но нельзя запускать автоматически. Во время active session/run
соответствующая action превращается в status/return action.

### Project tree

- разделы `Рукопись`, `Черновики`, `Заметки`, `Знания проекта`;
- в `Знания проекта`: `Словарь`, `Память`, optional `ASSISTANT.md`;
- вложенность частей/глав/сцен;
- node status через текст/иконку, не яркую заливку;
- contextual add;
- drag handle появляется при hover/focus;
- keyboard action `Переместить…`;
- active node хорошо различим;
- counts не показываются без пользы.

### Context panel

Переключаемые вкладки:

- `Обзор`: synopsis/status/word goal;
- `Источник`: связанное аудио и markers;
- `Версии`;
- `Статистика`.

По умолчанию panel может быть закрыта. Она не должна уменьшать editor до узкой колонки.

## 7. Manuscript editor — S-04

### 7.1. Базовый вид

Editor должен ощущаться ближе к чистому листу/рукописи, чем к CMS:

- нейтральный фон приложения и отдельная editor plane без тяжёлой рамки;
- typographic hierarchy;
- текущий document title над текстом, но внутри рабочей области;
- status и actions вне текста;
- удобные поля и вертикальный ритм;
- светлая и тёмная темы без имитации бумаги как обязательного эффекта.

### 7.2. Toolbar

Первый уровень:

- style: обычный текст / заголовок;
- italic;
- bold;
- scene break;
- undo/redo;
- find;
- focus mode.

Второй уровень / overflow:

- underline, strike;
- blockquote;
- lists;
- link;
- clear formatting;
- document settings.

Toolbar может быть компактным sticky row или bubble toolbar для selection. Основные команды должны существовать и в keyboard/command palette.

### 7.3. Обязательные editor states

| State | Визуальное поведение |
|---|---|
| Empty | Placeholder «Начните писать или надиктуйте сцену» + две равноправные actions |
| Typing | Toolbar ненавязчив; save status обновляется без layout shift |
| Saving | `Сохранение…` в header |
| Saved | `Сохранено` без повторяющейся анимации |
| Local only | Cloud/device icon + «На этом устройстве» |
| Sync pending | «Нужна синхронизация» + action |
| Conflict | Persistent banner «Мы сохранили оба варианта» |
| Read only | Явная причина и доступный способ создать копию |
| Focus | Только manuscript, save status и exit |

### 7.4. Source provenance

Абзац, созданный диктовкой, не получает постоянную яркую подсветку.

При hover/focus/keyboard navigation:

- в gutter появляется небольшая audio-source action;
- accessible name: «Прослушать источник этого абзаца»;
- по нажатию открывается mini player с диапазоном;
- вручную написанный абзац не показывает пустую action.

Если word timestamps надёжны, во время playback можно мягко подсвечивать текущее слово. Без timestamps подсвечивается абзац.

### 7.5. Вставка transcript

После `Перенести в рукопись`:

- вставленный диапазон кратковременно обозначается;
- toast `Текст добавлен · Отменить`;
- cursor устанавливается в конец вставки;
- source refs не визуализируются как metadata;
- существующий текст не заменяется без отдельного preview.

## 8. Подготовка диктовки — S-05

### Desktop

Modal или dedicated compact sheet поверх Studio:

1. destination;
2. language;
3. microphone;
4. level meter;
5. optional commands toggle;
6. privacy/storage status;
7. primary `Начать запись`.

Не показывать provider/model picker. Если позже появится privacy mode, пользователь выбирает понятное свойство:

- `Лучшее качество`;
- `Локальная обработка`, если доступна;
- `Не отправлять аудио в облако`.

### Preflight states

| State | UI |
|---|---|
| Permission unknown | Объяснение перед browser prompt |
| Permission granted | Live level meter |
| Permission denied | Browser-specific steps + retry |
| No device | Device selector + reconnect |
| Offline | «Запись сохранится на устройстве» |
| Low storage | Estimated minutes + blocking/nonblocking severity |

Основная кнопка становится disabled только когда local recording нельзя гарантировать.

## 9. Live recording — S-06

### 9.1. Композиция desktop

Рекомендуемый focus workspace:

```text
┌──────────────────────────────────────────────────────┐
│ Project / Scene                       Запись  08:42  │
├──────────────────────────────────────────────────────┤
│ Previous manuscript context — read only, collapsed   │
│──────────────────────────────────────────────────────│
│ Stable live transcript                              │
│ Stable live transcript                              │
│ Current partial…                         распознаётся │
│                                                      │
├──────────────────────────────────────────────────────┤
│ Команды · «Команда»  [¶] [Диалог] [Сцена] [Метка]    │
│ ● Запись идёт                  Пауза | Завершить      │
│ Голос и текст сохраняются                            │
└──────────────────────────────────────────────────────┘
```

Previous manuscript context помогает продолжить сцену, но не редактируется в active recording. Он сворачивается.

Над recorder actions по умолчанию раскрыт компактный блок `Команды`. Он показывает
текущий prefix, короткий список примеров и подписанные quick buttons: `Новый
абзац`, `Начать диалог`, `Разделитель сцены`, `Метка`, `Отменить команду`.
Горизонтальный список можно прокручивать на узком viewport, а весь блок — свернуть.
Кнопки остаются доступны при выключенном voice-command recognition и всегда дают
тот же видимый результат/Undo, что соответствующая голосовая команда.

### 9.2. Partial и stable

Partial:

- располагается после stable text;
- имеет label/icon «распознаётся»;
- отличается начертанием/фоновой treatment и label, не только цветом;
- заменяется в одной зоне, без reflow всего текста;
- не получает cursor.

Stable:

- обычная читаемая типографика;
- сохраняет paragraph boundaries;
- может иметь ненавязчивую uncertain annotation;
- не редактируется, пока recorder фактически записывает звук.

На Pause пользователь может выбрать `Исправить текст`; тогда stable content открывается
как transcript draft. Команда `Вернись к…` сама переводит recorder в pause, прокручивает
к найденному абзацу и открывает его inline, выделяя названное предложение или слова.
Блок правки содержит `Отменить`, `Сохранить` и primary-action
`К концу и продолжить`. Последняя сохраняет правку, возвращает viewport к концу и
возобновляет запись.

### 9.3. Recorder dock

Обязательные элементы:

- recording/paused status;
- duration;
- pause/resume;
- finish;
- marker;
- storage/sync line.

Необязательные:

- waveform decoration;
- input meter после успешного preflight;
- last command chip.

Waveform не заменяет status и не должна занимать большую часть экрана.

### 9.4. Auto-scroll

- если пользователь у конца — плавно следовать за current partial;
- если он прокрутил вверх — остановить auto-scroll;
- показать compact `К текущему тексту`;
- новый partial не возвращает вниз принудительно.

## 10. Degraded recording — S-07

Это не error page. Layout остаётся recording workspace.

Изменения:

- persistent amber/neutral banner;
- основной текст: `Запись продолжается`;
- пояснение: `Живой текст временно недоступен`;
- local storage status;
- partial area заморожена с меткой «До потери соединения»;
- recorder controls остаются активными.

Не использовать:

- красный full-screen alert, если запись сохраняется;
- spinner без объяснения;
- автоматическое завершение;
- обещание восстановить пропущенный live-текст.

При reconnection banner становится success toast `Соединение восстановлено`, но gap явно закрывается только после финальной расшифровки.

## 11. Pause — S-08

При pause вся поверхность ясно перестаёт быть active recording:

- status `Запись на паузе`;
- общий session time и recorded duration;
- `Продолжить` — primary;
- `Завершить`;
- `Исправить текст`;
- `Прослушать последние 10 секунд`.

Если пользователь редактирует stable transcript на pause:

- recorder остаётся paused;
- save status относится к transcript draft;
- `Продолжить диктовку` остаётся доступным;
- `К концу и продолжить` сохраняет текущий абзац и возобновляет запись одним действием;
- live provider получает только допустимый контекст, а не весь приватный документ без необходимости.

## 12. Finalizing — S-09

### Модель

Не заставлять пользователя смотреть на blocking loader. После local flush разрешить:

- остаться;
- вернуться к проекту;
- открыть live draft;
- получить notification о готовности.

### Stepper

1. Сохраняем запись.
2. Проверяем аудио.
3. Готовим точную расшифровку.
4. Готово.

Completed step должен означать durable condition. Provider/model names доступны только в technical details.

### Failure

Главный текст:

> Аудио сохранено. Расшифровку можно повторить.

Actions:

- `Повторить`;
- `Открыть live-черновик`;
- `Прослушать запись`;
- `Скачать аудио`.

## 13. Transcript review — S-10

### Desktop layout

```text
┌──────────────────────────────────────────────────────────────┐
│ Сессия / destination / status        Сравнить  ⋯             │
├───────────────────────────────────────┬──────────────────────┤
│ Transcript editor                     │ Проверка             │
│                                       │ 3 сомнительных места │
│ [editable paragraph]                  │ markers              │
│ [editable paragraph]                  │ dictionary action    │
│                                       │ source metadata      │
├───────────────────────────────────────┴──────────────────────┤
│ waveform  02:14 / 12:48   0.75× 1× 1.25×                   │
├──────────────────────────────────────────────────────────────┤
│                           Перенести в рукопись                │
└──────────────────────────────────────────────────────────────┘
```

### Requirements

- transcript area использует тот же editor language, что S-04;
- formatting минимально, но paragraph operations, undo, search и spellcheck доступны;
- клик по абзацу меняет audio position;
- playback подсвечивает source range;
- next/previous uncertain;
- `Добавить в словарь` не запускает replace;
- player не исчезает при прокрутке;
- на mobile player sticky снизу.

### Status labels

- `Точная расшифровка` — выбранный quality-pass artifact;
- `Живой черновик`;
- `Отредактировано вами`;
- `Доступно сравнение`.

Не использовать `AI corrected`, если речь идёт о механическом STT quality pass.

## 14. Transcript diff — S-11

### Рекомендуемый паттерн

Inline diff в одной читаемой колонке лучше двух узких колонок для длинной прозы.

- current author text — base;
- additions и deletions доступны как semantic diff;
- actions на change group: `Принять`, `Отклонить`, `Аудио`;
- sticky summary: количество групп, accepted/rejected;
- `Принять всё` находится в overflow или secondary position;
- close без применения безопасен.

Для широкого desktop разрешён optional side-by-side toggle, но inline — default.

### Критическое сообщение

> Ваш текущий текст не изменится, пока вы не примете правки.

## 15. Versions — S-12

### Layout

- timeline/list слева;
- preview/diff справа;
- named versions визуально заметнее сгруппированных autosaves;
- фильтр: именованные / все;
- version item: имя, дата, word count delta, note;
- actions: compare, name, restore.

Restore dialog:

> Текущий вариант останется в истории. Восстановленная версия станет новой текущей версией.

Не использовать destructive red treatment для restore, потому что операция обратима, но показать последствия.

## 16. Search — S-13

- command-like search surface;
- tabs/filters: manuscript, drafts, notes, source transcripts;
- result: breadcrumb, snippet, match count;
- keyboard navigation;
- search query сохраняется при переходе назад;
- project-wide replace всегда открывает preview.

## 17. Export — S-14

### Layout

Слева — настройки, справа — ordered outline preview.

Поля:

- scope;
- format;
- title page;
- chapter titles;
- notes off by default.

Job state не блокирует Studio. Готовый файл появляется в notification и export history.

Provider/model, audio и source metadata не включаются в manuscript export.

## 18. Dictionary — S-15

- searchable table/list;
- canonical term;
- type;
- language;
- variants;
- active toggle;
- source shortcut из review.

Helper copy:

> Словарь помогает будущим расшифровкам. Уже написанный текст не изменится.

## 19. Recovery Center — S-16

Recovery item должен восприниматься как ценная сохранённая работа, не как browser cache.

Card:

- project/destination;
- date/time;
- recorded duration;
- `На этом устройстве` / `Частично синхронизировано`;
- `Восстановить`;
- `Прослушать`;
- `Скачать аудио`;
- delete в overflow.

Удаление local-only записи — единственный поток, где нужен сильный необратимый warning.

## 20. Settings & Privacy — S-17

S-17 всегда имеет явную кнопку `Назад` в верхнем левом углу. При входе из Studio
она возвращает к тому же проекту и открытому узлу; при входе из проекта — к
проекту, без project context — к списку проектов. История браузера не является
единственным способом выйти из настроек.

### Editor

- font family;
- size;
- line height;
- manuscript width;
- light/dark/system;
- spellcheck.

### Dictation

- default language;
- microphone;
- command toggle;
- command prefix;
- quality/privacy mode, если доступен.

### Storage

- local unsynced data;
- cloud audio retention preference в разрешённых пределах;
- download full backup;
- storage usage.

### Privacy

- куда отправляется аудио в текущем режиме;
- используется ли cloud processing;
- data retention link;
- delete account/project.

### AI и ассистент

- системный read-only provider из env;
- список пользовательских connections;
- default connection/model;
- status последней capability-проверки;
- masked key fingerprint;
- assistant instruction audio retention;
- project `ASSISTANT.md`: active revision, create/edit/history;
- consent/data destination для каждого provider;
- Activity retention/delete.

Не показывать технический список всех adapter candidates обычному пользователю.

## 21. Component inventory

| ID | Компонент | Ключевые состояния |
|---|---|---|
| C-01 | App Header | default, recording, offline |
| C-02 | Project Tree | expanded, collapsed, reorder, trash |
| C-03 | Manuscript Editor | empty, editing, local, conflict, focus |
| C-04 | Formatting Toolbar | default, selection, overflow |
| C-05 | Save Status | saving, saved, local, pending, conflict |
| C-06 | Dictation Preflight | permission/device/storage/network |
| C-07 | Recorder Dock | preparing, recording, paused, stopping |
| C-08 | Live Transcript | empty, partial, stable, scrolled-away |
| C-09 | Stream Status | online, reconnecting, degraded, recovered |
| C-10 | Audio Player | compact, full waveform, unavailable |
| C-11 | Source Action | paragraph, word-follow, no-source |
| C-12 | Uncertain Navigator | count, active, resolved, empty |
| C-13 | Diff Viewer | unchanged, insert, delete, replace, stale |
| C-14 | Version Timeline | autosave group, named, compare |
| C-15 | Background Job | queued, running, ready, retryable failure |
| C-16 | Recovery Card | local-only, partial sync, restored |
| C-17 | Toast | undoable, info, recovered |
| C-18 | Confirmation Dialog | reversible, destructive, local-only delete |
| C-19 | Voice Mode Card | dictation, assistant, unavailable |
| C-20 | Assistant Target & Context | no target, selection, document, whole-project read |
| C-21 | Assistant Live Transcript | listening, partial, stable, checkpoint, paused, editable |
| C-22 | Assistant Activity Item | reading, tool, proposing, stopped, failed |
| C-23 | Change Set Navigator | object, selected, excluded, stale, applied |
| C-24 | Provider Connection Card | system, ready, limited, unchecked, unavailable |
| C-25 | Provider Capability Result | compatible, limited, failed |
| C-26 | Assistant Guide Status | missing, active, modified, generating, over-limit |
| C-27 | Project Memory Row | fact, decision, inactive, always-relevant, conflict |
| C-28 | Assistant Input Mode Switch | voice active, chat active, disabled while working |

Все интерактивные состояния должны быть показаны в component variants до high-fidelity screen assembly.

## 22. Accessibility specification

### Общие требования

- WCAG 2.2 AA;
- минимум 4.5:1 для обычного текста;
- visible focus;
- 44 × 44 px touch targets на mobile;
- keyboard order соответствует визуальному;
- dialogs возвращают focus инициатору;
- status updates используют подходящий live region без чтения каждого partial целиком.

### Live transcript

Screen reader не должен перечитывать всю гипотезу при каждом delta. Рекомендуется:

- визуальный partial обновляется часто;
- доступное объявление throttled по finalized segment;
- отдельная доступная status string «Распознавание продолжается».

### Diff

Add/delete обозначаются текстом/семантикой, а не только зелёным/красным.

### Recorder

- состояние и duration доступны без waveform;
- Pause и Finish имеют разные labels и расположение;
- shortcut configurable;
- изменение online/degraded объявляется один раз.

## 23. Content style

Тон:

- спокойный;
- фактический;
- без обвинения пользователя;
- без антропоморфизации;
- сначала сохранность, затем проблема, затем действие.

Примеры:

| Не использовать | Использовать |
|---|---|
| «AI сломался» | «Живой текст временно недоступен» |
| «Ошибка загрузки» | «Запись сохранена на устройстве. Повторим синхронизацию» |
| «Вы потеряете всё!» | «Эта запись существует только на этом устройстве» |
| «Магически улучшить текст» | «Предложить механические правки» |
| «Транскрипция завершена успешно» | «Расшифровка готова» |

## 24. Visual direction

### Характер

- литературный, но не декоративно-винтажный;
- профессиональный, но не офисный;
- тёплый neutral base;
- typography-first;
- достаточно воздуха для длинного чтения;
- один спокойный accent color;
- recording red используется функционально и экономно.

### Typography

Нужны роли:

- UI sans;
- manuscript serif и sans на выбор пользователя;
- mono только для технических details, не для текста;
- цифры duration должны быть стабильной ширины.

Display-role и видимый бренд используют Google Fonts `Philosopher` Bold 700 с
кириллицей. Это начертание применяется к product name, page/dialog titles и
document title, но не к manuscript body или Assistant transcript. UI остаётся на
Literata, рукопись — на выбранной автором Sofia Sans / Inter / Source Serif 4,
технические details — IBM Plex Mono. Fallback display-цепочка: Literata → Georgia
→ serif; браузер не должен синтетически утолщать отсутствующий файл.

### Motion

- 150–250 ms для panels/status;
- recording indicator не должен агрессивно пульсировать;
- partial replacement без character-by-character typewriter animation;
- reduced motion полностью поддерживается.

## 25. Prototype flows

### P-01 — основная ценность

Frames:

1. S-02 Projects.
2. S-03 Create project.
3. S-04 Empty first scene.
4. S-05 Dictation preflight.
5. S-06 Live partial → stable.
6. S-08 Pause.
7. S-06 Resume.
8. S-09 Finalizing.
9. S-10 Transcript review.
10. Transfer preview.
11. S-04 Manuscript with inserted text.
12. Manual edit + saved state.

### P-02 — доверие при сбое

Frames:

1. S-06 Recording online.
2. S-07 Network degraded.
3. Continue speaking.
4. Connection restored.
5. Finish.
6. Finalization retryable failure.
7. Open saved audio/live draft.
8. Retry → review ready.

### P-03 — авторские правки защищены

Frames:

1. Pause/open live draft.
2. User edits sentence.
3. Quality pass arrives.
4. Banner «Ваши правки сохранены».
5. S-11 inline diff.
6. Accept one change.
7. Return to editor.
8. Undo accepted change.
9. Save named version.
10. Compare/restore.

### Mobile P-04 — capture on the go

Frames:

1. Project quick action.
2. Compact preflight.
3. Full-height recording.
4. Degraded/local-only.
5. Pause.
6. Finish/background.
7. Notification ready.
8. Basic review.

### P-05 — голосовое поручение и изменение сцены

Frames:

1. S-04 с действиями Dictation/Assistant.
2. S-18 Voice Mode Picker.
3. S-19 targeted launch из главы или global launch из project; Studio shell и
   manuscript остаются на экране, Assistant открывается справа.
4. S-19 live listening: selection в рукописи + stable/partial transcript в panel.
5. S-19 silence checkpoint `Выполнить поручение?`.
6. S-20 microphone paused + тот же transcript + reading/search/tool activity;
   manuscript остаётся редактируемым.
7. S-20 streaming result + inline manuscript preview с label `Не применено` +
   `Продолжить голосом`.
   Альтернатива рядом — `Продолжить в чате`.
8. S-21 multi-object summary и document diff.
   Для knowledge-поручения navigator одновременно показывает dictionary и memory.
9. `Попросить исправить`.
10. Обновлённый preview.
11. Apply.
12. S-22 applied operation + grouped Undo.
13. S-04 с assistant provenance в history.

### P-06 — пользовательский provider

Frames:

1. S-23 system provider selected.
2. S-24 Add provider.
3. Filled base URL/model/masked key.
4. Connection test running.
5. S-25 `Текст работает, инструменты недоступны` для limited tool calling.
6. Исправление model/endpoint.
7. S-25 `Подходит для ассистента`.
8. S-23 new default.

### Mobile P-07 — поручение на ходу

Frames:

1. Quick action with Dictation/Assistant.
2. S-19 target/context + live transcript.
3. Silence checkpoint.
4. S-20 same transcript + working + Stop.
5. Change set ready notification.
6. Mobile S-21 summary/diff.
7. Apply + Undo.

### P-08 — пополнение знаний проекта голосом

Frames:

1. S-19: `Имя Элина добавь в словарь; запомни, что она не умеет плавать`.
2. Silence checkpoint.
3. S-20: проверка duplicate candidates.
4. Inline summary `Словарь · 1 / Память · 1`.
5. S-21: два typed объекта, canonical spelling и memory statement.
6. Apply.
7. S-27: records с source/provenance.
8. Следующий S-19 turn использует spelling и находит факт через MCP.

### P-09 — Voice / Chat в одной сессии

Frames:

1. S-19 active listening с switch `Голосом`.
2. Пользователь выбирает `Чат` во время unconfirmed transcript.
3. S-29 `Перенести в чат / Вернуться к голосу / Удалить черновик`.
4. S-28 в той же docked panel с transcript в composer, но ещё не отправленным;
   manuscript/selection не исчезают.
5. Send → text turn → activity/result/change-set card.
6. Уточнение вторым text turn.
7. Switch `Голосом`.
8. S-19 next voice turn с тем же target/context/guide и общей history.

### Mobile P-10 — безопасный переход Voice → Chat

Frames:

1. Mobile Assistant live с durable unconfirmed transcript.
2. Пользователь выбирает `Чат`; запись паузится, но WebSocket draft не
   финализируется и не отправляется.
3. Отдельная карточка показывает длительность и короткий excerpt черновика.
4. Primary `Перенести в чат`; secondary `Вернуться к голосу`; отдельное
   destructive действие `Удалить`.
5. После reconnect эта же карточка восстанавливается с тем же transcript.
6. Все touch targets основных действий не меньше 44 px.

## 26. Usability test tasks

Каждый prototype test должен проверить:

1. «Надиктуйте два абзаца и поймите, сохранилась ли запись».
2. «Продолжите говорить после исчезновения live-текста».
3. «Исправьте имя персонажа и добавьте его в словарь».
4. «Перенесите текст в середину непустой сцены».
5. «Найдите аудио, из которого появился абзац».
6. «Получите позднюю точную расшифровку, но сохраните свою правку».
7. «Восстановите вчерашнюю версию, не удалив сегодняшнюю».
8. «Найдите незавершённую запись после имитации сбоя».
9. «Попросите ассистента сократить текущий абзац, не начав диктовку».
10. «Убедитесь, что сцена не изменилась до кнопки Apply».
11. «Измените поручение после первого preview и примените второй вариант».
12. «Подключите свою модель и поймите, подходит ли она для действий».
13. «Скажите поручение без глагола из подсказки и убедитесь, что Assistant понял
    желаемый результат без выбора команды».
14. «Попросите изменить второй абзац, но сохранить последнюю реплику; проверьте
    точный diff и защищённый фрагмент».
15. «Начните голосовой черновик, перейдите в чат и вернитесь к голосу, ничего не
    отправив».

Наблюдаемые вопросы:

- различает ли пользователь partial и stable;
- доверяет ли он status local/cloud save;
- понимает ли разницу между transcript и manuscript;
- находит ли manual editor без обучения;
- замечает ли, что late quality pass не применён автоматически;
- может ли закончить запись без страха потерять текст.
- различает ли пользователь чтение проекта и уже применённое изменение;
- замечает ли scope до отправки;
- понимает ли multi-document summary;
- находит ли Stop и grouped Undo;
- понимает ли статус `Ограничен` у provider.

## 27. Deliverables от Product Design

### Обязательные

- sitemap/user-flow map;
- low-fidelity wireframes S-02–S-17;
- high-fidelity desktop P-01, P-02, P-03;
- high-fidelity mobile P-04;
- component variants C-01–C-18;
- keyboard/focus annotations;
- responsive rules;
- copy deck для ошибок/status;
- prototype для usability test;
- handoff annotations, связанные с WF и AC.
- high-fidelity desktop P-05 и P-06;
- high-fidelity mobile P-07;
- variants C-19–C-25;
- MCP coverage/risk annotation для каждой assistant action.

### Можно отложить

- brand identity;
- marketing site;
- billing;
- collaborative editor;
- advanced characters/world database;
- print-layout editor.

## 28. Design QA checklist

### Recording

- [ ] На каждом frame ясно: recording / paused / stopped.
- [ ] Видно, где сохранены данные.
- [ ] Degraded state не выглядит как stop.
- [ ] Finish нельзя спутать с Pause.
- [ ] Partial и stable различимы без цвета.

### Editor

- [ ] Это полноценный manuscript editor, не textarea.
- [ ] Показаны empty, saving, saved, local-only и conflict.
- [ ] Toolbar не доминирует над текстом.
- [ ] Source audio доступен клавиатурой.
- [ ] Вставка transcript имеет Undo.

### Review

- [ ] Player остаётся доступен при чтении.
- [ ] Абзац связан с аудио.
- [ ] Uncertain navigation не превращает текст в «ёлку» предупреждений.
- [ ] Late quality pass объясняет, что правки пользователя сохранены.

### Recovery и версии

- [ ] Recovery item выглядит ценным и восстанавливаемым.
- [ ] Restore описан как новая revision.
- [ ] Необратимое удаление local-only записи имеет сильный warning.

### Accessibility

- [ ] Focus order указан.
- [ ] Status announcements не спамят partial.
- [ ] Touch targets достаточны.
- [ ] Reduced motion предусмотрен.
- [ ] Diff и statuses не полагаются на цвет.

### Assistant

- [ ] Dictation и Assistant различимы до microphone start.
- [ ] На desktop manuscript и ordered assistant transcript видны одновременно.
- [ ] Открытие/закрытие panel и Voice/Chat switch не теряют selection/cursor/scroll.
- [ ] Рукопись остаётся полноценным редактируемым editor во время listening/working.
- [ ] Inline live preview имеет буквальный `Не применено`, diff semantics и не
      выглядит как autosaved canonical text.
- [ ] Project/document/selection scope виден до отправки.
- [ ] Voice instruction можно исправить.
- [ ] Voice/Chat switch сохраняет draft, target, context и history.
- [ ] Voice draft handoff предлагает transfer/return/delete и ничего не
  отправляет автоматически.
- [ ] Chat имеет явный Send и не создаёт audio/STT artifacts.
- [ ] Working state показывает этап и Stop без fake progress.
- [ ] Read activity не выглядит как уже применённая mutation.
- [ ] Change set перечисляет все затронутые объекты.
- [ ] Exact review показывает исходный fragment, replacement, координаты и
      protected text без raw MCP/UUID в primary layer.
- [ ] Expanded review сохраняет transcript/trace рядом с выбранным diff.
- [ ] До Apply виден текст `Ничего не изменится`.
- [ ] Stale preview блокирует Apply и предлагает пересчитать.
- [ ] Applied state имеет grouped Undo.
- [ ] Provider status различим текстом, а key остаётся masked.
- [ ] Markdown сохраняет headings/lists/code/links; raw `<think>/<analysis>` и
  tool JSON не попадают в пользовательский ответ.

## 29. Открытые решения, не блокирующие дизайн

- Конкретный primary/fallback STT pipeline.
- Auth provider и точный способ входа.
- DOCX rendering engine.
- Срок cloud audio retention в тарифах.
- Наличие self-hosted/local privacy режима в самой первой alpha.

Эти решения не меняют основные экраны и state model. Для них предусмотрены нейтральные labels и capability-based UI.

## 30. Assistant visual model

### 30.1. Docked Assistant рядом с рукописью

Assistant — это состояние Studio shell, а не отдельный полноэкранный destination.
На desktop базовая композиция повторяет IDE с assistant panel:

1. project tree — 240–280 px слева;
2. полноценный manuscript editor — центральная dominant region, минимум 680 px;
3. docked assistant panel — 340–440 px справа, collapsible и resizable;
4. review открывается крупным modal поверх Studio; editor, project tree и panel
   остаются видимыми под scrim и не демонтируются.

Voice и Chat меняют только input/feed в одной панели. Manuscript editor остаётся
mounted и сохраняет cursor, selection, scroll, formatting, undo/redo и autosave.
Панель не должна визуально конкурировать с рукописью: компактная типографика,
короткие cards и собственный scroll; длинная стенограмма доступна целиком внутри
ordered feed.

Studio имеет два консистентных presentation state. Пока Assistant закрыт,
центральный manuscript остаётся прежним плавающим полотном: видимый верхний
отступ, ограниченная ширина, тонкая рамка и внешний радиус создают спокойный
«воздух» для самостоятельного письма. Когда docked Assistant открыт, manuscript
перестаёт быть карточкой: он начинается сразу под workspace header, без внешнего
margin, border и radius. В этом состоянии header структуры, formatting toolbar и
header Assistant имеют одинаковые `56 px`, общий baseline `y` и одинаковый
нижний divider. Закрытие Assistant атомарно возвращает floating presentation;
content, selection, cursor, scroll и undo history при этом не пересоздаются.

Для global launch центр показывает последний открытый документ или project
overview. Когда Assistant разрешает target, соответствующий документ открывается
в той же центральной области без потери истории run.

### 30.2. Voice Mode Picker — S-18

Compact modal/sheet:

- видимый текущий project/document;
- две крупные cards с icon, title и one-line outcome;
- `Диктовка`: recording-red используется только в mic action;
- `Ассистент`: спокойный accent, без sparkles/avatar;
- unavailable state ведёт в provider settings;
- secondary `Отмена`.

Cards должны отвечать на вопрос «куда попадёт моя речь», а не сравнивать модели.
Глобальный launch использует label `Цель определится из поручения`; запуск из
главы/сцены показывает конкретный target.

### 30.3. Assistant Workspace — S-19

Desktop:

```text
┌──────────────┬───────────────────────────────┬────────────────┐
│ Project tree │ Manuscript editor             │ Assistant      │
│              │ selection / cursor            │ Voice | Chat   │
│              │                               │ transcript     │
│              │                               │ stable/partial │
│              │                               │ activity/result│
│              │                               ├────────────────┤
│              │                               │ state dock     │
└──────────────┴───────────────────────────────┴────────────────┘
```

Session feed визуально родственен S-06 Live Recording, но адаптирован к узкой
панели: readable transcript, literal states, compact activity cards и sticky dock.
Target/context/guide/provider остаются видимыми в panel header. `Весь проект` —
project context, а не mutation destination.

Типографика panel использует фиксированную компактную шкалу без зависимости от
viewport: header title `15 px`, основной turn/transcript и chat message `14 px`,
activity title `13 px`, supporting/meta `10–11 px`, uppercase service label `9 px`.
Focused voice turn не увеличивается до display-кегля. Markdown `h1–h3` внутри
docked feed ограничены `14–15 px`; иерархия строится weight/spacing, а не резким
скачком размера. Display-font не применяется к transcript, activity или composer.

Selection отображается одновременно в editor и panel как attached context.
Открытие Assistant не меняет ширину editor ниже 680 px; если места недостаточно,
project tree сворачивается первым, затем assistant panel переходит в overlay.

Targeted launch показывает `Пишет в: Глава 3 · Возвращение`. Global launch —
`Цель: определится из поручения`; найденные mutation targets появляются в preview.

### 30.4. Listening и instruction review

- recorder status буквальный: `Слушаю поручение`;
- live partial имеет те же honesty-patterns, что dictation;
- короткая пауза стабилизирует partial;
- длинная пауза показывает inline checkpoint `Выполнить поручение?`;
- checkpoint не запускает LLM автоматически и имеет `Продолжить говорить`,
  `Исправить текст`, `Выполнить`;
- подтверждение ставит microphone на паузу, но transcript не исчезает;
- uncertain words не отправляются автоматически;
- точные голосовые команды имеют prefix `Команда, …`;
- audio retention copy доступен через details, не как blocker.

### 30.5. Assistant Working — S-20

Не использовать indeterminate «thinking» animation как единственный feedback.

S-20 продолжает тот же panel feed: сверху остаётся read-only поручение, ниже
появляются activity items и streaming answer/proposal. Рукопись остаётся
редактируемой в центре. Dock явно говорит `Микрофон на паузе`, а подстрока
подтверждает, что ручная правка рукописи доступна.

Если proposal относится к открытому документу, editor plane показывает встроенный
live preview по мере готовности. Preview использует diff insert/delete semantics,
буквальный label `Предложение · не применено` и никогда не выглядит как обычный
selection или сохранённый текст. Ручная правка пересекающегося range переводит
preview в stale, но не блокирует editor.

Activity items:

- `Читает «Возвращение»`;
- `Ищет упоминания Элины`;
- `Готовит 3 изменения`;
- `Не удалось открыть заметку · Повторить`.

Технические tool names доступны в details. `Остановить` всегда доступно и не
выглядит destructive. Source links появляются в ответе как project breadcrumbs.
После результата primary action — `Продолжить голосом`; он создаёт следующий turn,
а не новый чат или run.

### 30.6. Change Review — S-21

S-21 — modal state текущего Studio, не отдельная страница. Он открывается поверх
смонтированной рукописи и Assistant panel, не меняет route/URL, selection, cursor,
scroll или undo history. На desktop modal имеет максимальный размер `1120 × 824`,
внешний scrim и явные close/Escape/backdrop affordances; на узких viewport он
сохраняет `16 px` safe inset и собственный scroll.

Основная колонка содержит readable exact diff. Compact aside содержит change
groups, исходное поручение, resolved target/source и краткую safety-подсказку.
Отдельный transcript-pane не дублируется: полный ordered feed остаётся в видимой
под modal Assistant panel, а в modal повторяется только контекст, необходимый для
решения.

Left/compact navigator:

- object title + type;
- action: create/replace/move/rename/dictionary;
- changed group count;
- selected/excluded/stale state.

Top summary:

> 3 объекта · 5 изменений  
> Ничего не изменится до применения.

Primary `Применить изменения`. Secondary `Попросить исправить`. Reject в overflow
или tertiary position. Raw MCP JSON скрыт в technical details.

В ready-card Assistant доступны два равноправно понятных пути: secondary
`Проверить изменения` открывает S-21, primary `Применить сразу` атомарно применяет весь
свежий выбранный набор. Direct apply не ослабляет safety: canonical state меняется
только server Apply, а toast немедленно предлагает `Отменить` весь operation group.

После нажатия `Попросить исправить` строка review actions заменяется полем
уточнения. В этом состоянии видны только `Отмена` и `Отправить`; primary Apply и
Reject скрыты, чтобы две взаимоисключающие развилки не конкурировали на одном
экране.

Session panel в S-21 показывает исходное поручение, answer, human-readable
activity summary и trace `поручение → selection/target → diff`. Пользователь не
должен возвращаться на другой экран, чтобы вспомнить, что именно он попросил.

Structural operations показываются человеческими sentences, например:

> Сцена «Возвращение» переместится после «Ночной поезд».

Stale state заменяет Apply на `Обновить предложение`.

### 30.7. Applied и Activity — S-22

Applied confirmation содержит:

- count объектов/revisions;
- assistant operation label;
- `Открыть изменения`;
- `Отменить все изменения`;
- provider/model в details.

Undo не должен выглядеть как restore старой версии: он создаёт compensating
revisions и может показать progress.

### 30.8. AI Providers — S-23–S-25

Provider card:

- display name;
- `Системный` или `Ваше подключение`;
- base host без path/query;
- model ID;
- capability status;
- last check;
- selected default.

S-24 использует ясную вертикальную форму. API key — password field с copy disabled
после save. Advanced headers и timeouts закрыты по умолчанию.

Connection test — stepper с реальными durable/verified steps:

1. Подключение.
2. Ответ модели.
3. Инструмент.
4. Продолжение после инструмента.

S-25 формулирует результат по возможностям, а не маркетинговой совместимости:

- `Подходит для ассистента`;
- `Текст работает, инструменты недоступны`;
- `Не удалось подключиться`.

### 30.9. Руководство ассистента — S-26

Это editor специального проектного файла, а не provider/system settings.

Desktop:

```text
┌──────────────────────────────────────────────────────────────┐
│ Проектные файлы / Руководство ассистента   revision · saved │
├───────────────────┬──────────────────────────────────────────┤
│ Секции            │ ASSISTANT.md                             │
│ Голос и тон       │ [editable structured Markdown]           │
│ Повествование     │                                          │
│ Канон             │ references: словарь · заметки · главы    │
│ Запреты/примеры   │                                          │
├───────────────────┴──────────────────────────────────────────┤
│ 8 420 / 24 000 · Используется со следующего turn · История  │
└──────────────────────────────────────────────────────────────┘
```

Empty state предлагает три равноправных варианта: `Пустой файл`, `Из шаблона`,
`Предложить по рукописи`. Последний явно помечен как preview, а не автосохранение.

В S-19/S-20 рядом с target/context показывается компактный badge
`Руководство активно · r7` или `Без руководства`. Он открывает S-26. Не показывать
весь guide в session feed: центральным объектом остаётся рукопись, а ordered
live-поток остаётся доступен в assistant panel.

Segmented switch `Голосом / Чат` всегда находится в session header, не в global
app navigation. Он меняет input mode, а не создаёт новый run.

Guide не должен визуально походить на скрытый system prompt. Показывать filename,
save status, revision/history, references и лимит размера. Generated draft
просматривается тем же diff-pattern, что другие assistant changes.

### 30.10. Память проекта — S-27

S-27 — управляемая база фактов/решений, не chat history и не «магическая память».

Desktop:

```text
┌──────────────────────────────────────────────────────────────┐
│ Память проекта             Search · Filters · Добавить факт │
├──────────────────┬───────────────────────────────────────────┤
│ Типы             │ «Элина не умеет плавать»                │
│ Все              │ character fact · active · r3             │
│ Персонажи        │ scopes · sources · tags · history        │
│ Мир              ├───────────────────────────────────────────┤
│ Сюжет/continuity │ «Ключ уже был у отца»                    │
│ Решения          │ plot promise · always relevant           │
└──────────────────┴───────────────────────────────────────────┘
```

Каждая row показывает statement как основной текст, затем kind/status/revision,
scope и source breadcrumbs. `Always relevant` имеет буквальный badge и tooltip с
объяснением prompt cost. Деактивация предпочтительнее delete.

Knowledge preview в S-21 использует разные иконки/labels:

- `Словарь · добавить имя`;
- `Память · добавить факт`;
- `Руководство · изменить правило`.

Нельзя сводить их в безымянное `Запомнить`. Duplicate state показывает существующую
запись рядом и действия `Объединить`, `Обновить`, `Оставить обе`.

### 30.11. Assistant Chat — S-28

Chat сохраняет знакомую модель: chronological feed, user/assistant messages,
multi-line sticky composer и явный Send, но живёт в той же docked panel рядом с
рукописью. Voice/Chat switch не меняет route и не remount-ит editor:

```text
┌──────────────┬───────────────────────────────┬────────────────┐
│ Project tree │ Manuscript + inline preview   │ [Voice | Chat] │
│              │                               │ USER message   │
│              │                               │ ASSISTANT      │
│              │                               │ ACTIVITY       │
│              │                               │ CHANGE SET     │
│              │                               ├────────────────┤
│              │                               │ composer · Send│
└──────────────┴───────────────────────────────┴────────────────┘
```

Разрешены спокойные message surfaces разной ширины/выравнивания, но без аватаров,
персонажей и декоративных AI-sparkles. Tool activity и change sets не маскируются
под обычный текст ответа.

Target/context/guide/provider остаются видимы. Switch во время active listening
открывает explicit S-29 draft-handoff. Во время tool work composer disabled,
Stop остаётся доступен. Completed state предлагает оба действия:
`Продолжить голосом` и `Продолжить в чате`.

Открытый document, selection, cursor, scroll и manual undo history сохраняются
при каждом переключении. Selection/project-object attachment показан и в editor,
и в composer context, но source of truth остаётся stable range anchor.

Mobile S-28 использует тот же порядок; composer sticky над keyboard safe area,
attachments открываются sheet.

Ответ Assistant рендерится как безопасный GFM Markdown с типографикой продукта:
headings, paragraphs, lists, quotes, inline/code blocks, tables и links имеют
различимую иерархию. В основной feed не выводятся raw `<think>/<analysis>`,
provider envelopes, JSON arguments, UUID или имена MCP tools. Activity рядом
объясняет, что сделано, отдельными человеческими labels.

### 30.12. Voice Draft Handoff — S-29

S-29 является состоянием того же run, а не confirmation dialog поверх
потерянного recorder:

- показывает неизменённый unconfirmed transcript или его excerpt;
- сообщает, что текст ещё не отправлен Assistant;
- primary `Перенести в чат` копирует draft в composer без Send;
- `Вернуться к голосу` продолжает тот же server draft;
- `Удалить черновик` удаляет только unconfirmed turn и визуально отделено от
  остальных действий;
- Chat composer disabled, пока пользователь не выбрал исход;
- desktop card находится в docked assistant panel рядом с неизменившейся
  рукописью; mobile — после истории и перед safe area; transcript остаётся
  видимым выше.

Exact protected-text review остаётся вариантом S-21: вместо полного документа
показываются только исходный fragment и replacement, label
`Точная замена · символы start–end` и отдельная карточка protected text. Primary
summary не содержит raw tool name или internal ID.

### 30.13. Assistant accessibility

- status live region не озвучивает каждый text/tool delta;
- finalized activity item объявляется один раз;
- Stop, Apply, Ask to revise и Undo имеют distinct labels;
- scope change объявляется до Run;
- diff semantics совпадает с S-11;
- provider capability не кодируется одним цветом;
- microphone state доступен без waveform;
- focus после Apply переходит в confirmation heading, после close возвращается
  к инициатору.
- guide section navigation имеет heading semantics; revision и limit не кодируются
  только цветом.
- memory kind/status/always-relevant имеют текстовые labels; duplicate comparison
  доступен с клавиатуры.
- input mode switch имеет `tablist/radiogroup` semantics; перенесённый transcript
  не считается отправленным до explicit Send.

### 30.14. Контекст ассистента — S-30

Studio и меню проекта ведут в отдельный hub `Контекст ассистента`. Это не скрытая
секция настроек и не единый неразличимый prompt. На первом экране находятся три
явных destination:

- `Обзор и история` — состояние guide/memory и список assistant runs;
- `ASSISTANT.md` — отдельный версионируемый проектный файл;
- `Память проекта` — типизированные факты и решения.

Карточки guide и memory показывают разные иконки, revision/count и объясняют
разницу между ними. Удаление строки assistant history сопровождается текстом
`Применённые изменения проекта сохранятся`. Provider settings доступны отдельной
кнопкой, но не смешиваются с проектным знанием.

Frame `S-30 / Assistant Context Hub` — `IeSCn`; desktop implementation использует
те же три стабильных URL, поэтому refresh/deep-link не возвращает пользователя в
общий экран.

### 30.15. Микрофон на небезопасном HTTP

Browser microphone нельзя представлять как обычную ошибку permission. Если
`window.isSecureContext=false`, preflight до запроса доступа показывает отдельное
состояние:

- буквальный статус `Нужен HTTPS или localhost`;
- объяснение browser security boundary;
- действие `Открыть localhost`, когда приложение запущено на той же машине;
- disabled `Начать запись`;
- сохранённую доступность чата и ручного редактора.

Это состояние зафиксировано в desktop `vjsHk` и mobile `UiB4H`. Для server deploy
HTTPS является release requirement: raw LAN HTTP не считается рабочим голосовым
развёртыванием.

### 30.16. Пометки рукописи и Assistant review — S-31…S-37

Создание пометки не использует persistent selection toolbar. Непустое выделение
в одном блоке открывает компактное меню только по правому клику,
`Shift+F10`/клавише меню или долгому нажатию:

- `Связать со словарём` ведёт в S-31B с поиском по реальным терминам проекта;
- `Добавить заметку` открывает S-32 с текстом и lifecycle-действиями;
- `Прикрепить к ассистенту` переносит selection как явный контекст текущего turn.

S-35 показывает все пометки проекта с поиском и фильтрами, а `Перейти` открывает
S-36 в нужном документе на точной цитате. Detached anchor обозначается отдельно
от `open/resolved`: проблема привязки не выглядит как решение пользовательской
задачи.

Frame `S-37 / Assistant Annotation Fix Review · In Studio` — `ltgSX`. Он сохраняет
рукопись смонтированной под modal review и для каждого исправления показывает:

- исходный текст заметки;
- точную старую и новую цитату без transport offsets;
- независимый checkbox `После применения отметить исправленной`;
- общий safety-текст о том, что Apply обновит document revision, anchor и audit
  history атомарно, а Undo восстановит исходные snapshots.

Карточки длинного прохода скроллятся внутри review; footer с Apply/Cancel остаётся
доступен. На mobile блоки идут последовательно, checkbox и diff не теряют state,
а канонический текст не меняется до Apply.

### 30.17. Чаты проекта и параллельная работа — S-38…S-41

Ментальная модель меняется с «один открытый ассистент» на «несколько поручений
внутри одного проекта»:

- проект остаётся общим контекстом, источником рукописи, памяти и руководства;
- каждый чат является отдельным `AssistantRun` со своим target, черновиком,
  историей turns, activity и change sets;
- `Голосом / Чат` остаются input modes выбранного чата и не создают новый run;
- переключение видимого чата не останавливает остальные runs и не демонтирует
  manuscript editor.

Вместо singleton-заголовка панель использует `Assistant Thread Header / Active`
(`SxCHY`): service/status сверху, название текущего чата как selector, отдельные
действия `Новый чат` и `Скрыть панель`. Selector открывает S-38
`Assistant Chat Switcher` (`fF5xj`). Список не является сырым журналом событий:
он группирует чаты по срочности `Нужно внимание / В работе / Недавние`, показывает
понятное название поручения, относительное время и короткий status badge.

S-39 `Assistant New Chat` (`X3XsxT`) — быстрый compose, а не мастер. Новый чат
наследует проект, текущий документ, provider/model и active guide; scope можно
изменить до отправки. Пустой compose не создаёт durable run. После явной отправки
новое поручение может работать в фоне, а автор сразу возвращается к рукописи или
другому чату.

S-40 `Parallel Proposal Conflict` (`U9Ixmc`) показывает конфликт на уровне одного
предложения, а не всей сессии. Если другой чат или ручная правка уже изменили
базовую revision, stale preview остаётся видимым, но Apply недоступен. Primary
action — `Обновить предложение`; рядом явно показаны `основа r12 → сейчас r13`.
Остальные чаты продолжают работу.

Responsive behavior:

- S-41 (`y4BlnR`) на 1024 px открывает thread switcher оверлеем шириной 360 px
  справа; рукопись остаётся mounted и видима;
- Mobile P-11 (`Q9k6d`) использует отдельное полноэкранное состояние списка, но
  сохраняет верхний путь назад и различение `Рукопись / Ассистент`;
- статусы не зависят только от цвета, touch targets не меньше 44 px, порядок
  секций и названия доступны screen reader и keyboard navigation.

S-38–S-41 реализованы в Studio и проверены на 1440×1024, 1024×768, 834×1194 и
390×844. Доказательства и найденные при browser QA регрессии (null-safe unread,
per-run mode restoration, URL restoration, plural counter) зафиксированы в
[`TRACEABILITY.md`](../implementation/TRACEABILITY.md) и
[`VERIFICATION_REPORT.md`](../implementation/VERIFICATION_REPORT.md).
