# API, auth и env

## API слой

Общая API-инфраструктура находится в `src/shared/api`.
Доменные запросы размещай внутри slice:

- `entities/<entity>/api` для entity-level операций.
- `features/<feature>/api` только если запрос принадлежит конкретному пользовательскому сценарию.
- `widgets/<widget>/api` допустимо для временной интеграции виджета, но доменную логику лучше переносить ниже.

Не выполняй `fetch` или `axios` напрямую в JSX-компонентах. Компоненты должны использовать hooks/model слой.

## DTO и типы

- Типы внешнего API держи рядом с запросами в `api/types.ts`.
- Внутренние UI/model типы держи в `model/types.ts`.
- Добавляй adapter/mapper на границе API, когда DTO отличается от модели клиента по контракту, а не только по имени поля.
- Переводи DTO через adapter, если backend возвращает nullable/optional поля, которые в UI должны стать обязательными после default values; даты, enum/status или числовые значения требуют нормализации; вложенная структура API должна стать плоской моделью; backend отдает `snake_case`, технические поля или transport-specific формат; API объединяет несколько вариантов ответа, которые в UI должны стать discriminated union.
- Adapter должен быть чистой функцией без React, TanStack Query и UI side effects. Он принимает DTO, возвращает model type и явно задает default/fallback значения.
- Не смешивай optional поля API с обязательным UI state без нормализации.

## TanStack Query

Используй TanStack Query для server state:

- Query keys должны быть стабильными и предсказуемыми.
- Mutations должны явно инвалидировать или обновлять связанные queries.
- Не копируй server state в Zustand или local state без необходимости.

Loading/error/empty состояния обрабатывай в слое, который использует API hooks: `features`, `widgets` или `pages`. API слой должен отдавать данные, ошибки и статус так, чтобы UI мог явно собрать эти состояния.

## Auth

Auth helpers находятся в `src/shared/lib/auth.ts`, session model - в `src/entities/session`.
Для auth flows и Keycloak сначала смотри `docs/auth/`.

Не храни новые секреты в frontend коде. Любые `VITE_*` переменные публичны для клиента.

## Runtime env

Runtime env читается через `window.__ENV__` с fallback на `import.meta.env` в `src/shared/config/env.ts`.
Docker runtime template находится в `docker/env-config.template.js`.

При добавлении env:

- Не редактируй `.env` без явного запроса пользователя. Если нужна новая локальная переменная, сначала попроси пользователя добавить ее или разрешить изменение `.env`.
- Добавь тип в `Window.__ENV__`.
- Добавь экспорт в `shared/config/env.ts`.
- Обнови docker template, если значение нужно в контейнере.
- Укажи безопасный default только если он корректен для локальной разработки.
