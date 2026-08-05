# Tooling, Bun, Biome и проверки

## Bun

Bun - единственный основной runtime и package manager frontend части.

Используй:

```bash
bun install
bun run dev
bun run build
bun run check
bun run type-check
bunx --bun <tool>
```

Не используй для проектовых команд:

```bash
npm install
npm run
yarn
pnpm
node <script>
```

Исключение: можно читать документацию или чужие snippets, но адаптировать команды под Bun перед применением.

## Biome

Biome - единственный formatter/linter. Конфиг находится в `biome.json`.

Не добавляй:

- ESLint config и eslint plugins.
- Prettier config и prettier plugins.
- Форматирование через IDE-specific правила, которые конфликтуют с Biome.

Если нужно поменять стиль форматирования или lint rule, меняй `biome.json` и прогоняй checks.

## Tailwind и shadcn/ui

- Для Vite подключай Tailwind CSS 4 через `@tailwindcss/vite`, а не через PostCSS, если нет отдельной технической причины.
- Храни конфигурацию shadcn/ui в `components.json`, когда проект будет инициализирован. Alias для components должен указывать на `src/shared/ui`, utilities — на `src/shared/lib`.
- Добавляй shadcn/ui компоненты Bun-совместимой командой (`bunx --bun shadcn@latest add <component>`), затем адаптируй их к public API и правилам FSD.

## TypeScript

Проект работает в strict mode. Сохраняй:

- Явные типы на публичных API, DTO, props сложных компонентов и возвращаемых значениях hooks.
- `unknown` вместо `any`, если тип пока неизвестен.
- Type guards и нормализацию данных на границе API.

Не используй `@ts-ignore`. Если подавление неизбежно, используй `@ts-expect-error` с коротким объяснением причины.

## Full Check перед коммитом

Перед каждым коммитом запускай полный frontend check:

```bash
bun run check
bun run type-check
bun run build
```

Сокращенная одной строкой форма:

```bash
bun run check && bun run type-check && bun run build
```

Если в проект добавлен тестовый скрипт, полный check должен включать его:

```bash
bun test
```

## Git hooks

`lefthook.yml` запускает `lint-staged` и type-check на pre-commit.
Не считай hook заменой полного check: hook проверяет только часть сценариев и может не покрыть production build.
