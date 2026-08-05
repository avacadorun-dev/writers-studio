# UI, стили и доступность

## UI стек

Проект использует Tailwind CSS 4 для стилей, Radix UI для доступного низкоуровневого поведения и shadcn/ui как набор проектных primitives. `shadcn/ui` добавляет исходный код компонентов в `shared/ui`, поэтому он остается адаптируемой частью приложения, а не внешним дизайн-системным runtime.

Не добавляй Ant Design, `styled-components`, CSS-in-JS или другую UI/styling библиотеку без отдельного архитектурного решения. Если в будущем появится legacy-код на этих технологиях, не расширяй его: при существенном рефакторинге переноси затронутый UI на Tailwind и shadcn/ui.

Базовый набор зависимостей для UI:

- `tailwindcss` и `@tailwindcss/vite` для Tailwind CSS 4 в Vite;
- нужные пакеты `@radix-ui/react-*` — только для добавленных primitives;
- `class-variance-authority` для variants;
- `clsx` и `tailwind-merge` для `cn` helper-а;
- `lucide-react` для иконок.

Добавляй только компоненты и Radix packages, которые используются в runtime. Не устанавливай весь каталог Radix «на будущее».

## Shared UI

`shared/ui` должен оставаться domain-agnostic.
Не размещай там компоненты, которые знают про assistants, prompts, models, auth, OpenClaw или permissions конкретной сущности.

Domain UI размещай в соответствующем slice:

- `entities/*/ui` - отображение сущности.
- `features/*/ui` - UI действия или сценария.
- `widgets/*/ui` - крупные блоки экрана.
- `pages/*` - screen composition.

Компоненты shadcn/ui храни в `shared/ui` с kebab-case именами и экспортируй через public API. Не меняй добавленный компонент без необходимости: domain-specific композицию и стили держи в соответствующем slice.

## Стили

- Глобальный entrypoint Tailwind, theme tokens и reset держи в `shared/styles`; для цветовых и радиусных tokens используй CSS custom properties, совместимые с Tailwind theme.
- Для layout, spacing, typography, responsive и state styles используй Tailwind utility classes. Сложные повторяемые patterns сначала оформляй как shared primitive или domain component, а не как глобальный utility-класс.
- Для variants компонентов используй `class-variance-authority`; классы собирай `cn` helper-ом (`clsx` + `tailwind-merge`). Не конкатенируй конфликтующие Tailwind классы вручную.
- CSS modules или обычный CSS допустимы лишь для того, что Tailwind неудобно выразить: сложных keyframes, browser-specific rules или внешней библиотеки. Стили остаются рядом с компонентом или в `shared/styles`, если они глобальны.
- Не добавляй inline `style` для нетривиальной стилизации. Динамические значения допустимы только если их нельзя представить ограниченным набором Tailwind classes или CSS variables.
- Используй `lucide-react` для иконок. Для icon-only buttons обязательно задай доступное имя через `aria-label` или visually-hidden text.

## Доступность

Минимальный стандарт для новых UI:

- Семантические HTML-элементы вместо `div` с обработчиками.
- Keyboard navigation для интерактивных элементов.
- Видимые focus states.
- `aria-*` только когда семантики HTML недостаточно.
- Текстовые labels для inputs, buttons и icon-only действий.
- Не полагайся только на цвет для передачи состояния.

## Компоненты

- Один основной компонент на файл.
- Если файл компонента становится больше 300-350 строк, раздели его на более мелкие составляющие. Разбивай по смысловым границам: самостоятельные блоки UI, повторяемые части, формы, таблицы, сложные панели.
- Не дроби файл механически только ради числа строк: локальный подкомпонент должен иметь понятную ответственность и уменьшать сложность родителя.
- Выноси большие части JSX в локальные подкомпоненты или slice UI.
- Не создавай избыточные DOM-обертки. Стремись к минимальному дереву, если дополнительный элемент не нужен для семантики, layout, accessibility или интеграции со сторонней библиотекой.
- Не смешивай data fetching, сложные преобразования и разметку в одном компоненте.
- Для форм держи schema/normalization/default values отдельно от визуального слоя, если логика растет.

## Assets

Общие изображения и svg находятся в `shared/ui/assets`.
Slice-specific assets держи внутри slice.
Новые asset файлы называй в `kebab-case`; существующие файлы с пробелами или legacy naming не копируй как шаблон.
