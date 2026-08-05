# ADR-007: Непрерывный Assistant flow внутри Studio

> Статус: принято; обязательный продуктовый и дизайн-контракт  
> Дата: 1 августа 2026  
> Владельцы: Product + Design + Engineering

## Контекст

Assistant был спроектирован как отдельный сценарий: голосовая стенограмма
получала крупный display-кегль, а готовый change set предлагал перейти в
полноэкранный review. После объединения Assistant и рукописи на одном экране эти
решения стали противоречить основной цели режима.

Автор одновременно читает рукопись, формулирует поручение, следит за работой
Assistant и принимает предложение. Резкие изменения размера текста внутри узкой
панели заставляют постоянно перестраивать внимание. Полноэкранный review скрывает
рукопись и Assistant panel, визуально выглядит как новый destination и разрывает
контекст, даже если технически состояние run можно восстановить.

Нам нужен цельный flow, похожий на editor + assistant в IDE: рукопись остаётся
основной поверхностью, Assistant — её docked-инструментом, а проверка изменения —
временным слоем принятия решения, а не отдельным рабочим местом.

## Решение

1. Docked Assistant использует фиксированную компактную типографическую шкалу:
   основной turn/transcript/chat — `14 px`, activity — `13 px`, supporting/meta —
   `10–11 px`, service labels — `9 px`, header title — `15 px`.
2. Focused voice transcript не увеличивается до display-кегля. Markdown headings
   внутри panel ограничены `14–15 px`; иерархия задаётся weight и spacing.
3. Свежий полностью выбранный change set показывает два действия:
   `Проверить изменения` и primary `Применить сразу`.
4. `Применить сразу` вызывает тот же server Apply, что и подробная проверка. После
   успеха UI обязан предложить grouped Undo; обхода staging/revision checks нет.
5. `Проверить изменения` открывает S-21 как modal поверх текущего Studio. Открытие и
   закрытие не меняют route/URL, не демонтируют editor и не теряют selection,
   cursor, scroll, autosave или undo history.
6. Modal показывает readable exact diff, change groups, исходное поручение и
   resolved target/trace. Полный ordered transcript остаётся в Assistant panel под
   modal и не дублируется отдельной полноэкранной колонкой.
7. Обычный Studio без открытого Assistant сохраняет floating manuscript. При
   открытом Assistant рукопись остаётся полноразмерной integrated-колонкой без
   внешнего margin, border и radius.
8. Если автор выбирает `Попросить исправить`, review actions заменяются формой
   уточнения. На ней одновременно видны только `Отмена` и `Отправить`; Apply и
   Reject возвращаются после отмены формы или после завершения нового turn.

## Почему это решение принято

- автор принимает решение в том же визуальном и навигационном контексте, где
  видел live-preview;
- рукопись не исчезает в момент, когда она наиболее нужна для сравнения;
- direct apply ускоряет очевидные точечные правки, не ослабляя server safety;
- modal сохраняет подробную проверку для неоднозначных и multi-object изменений;
- единая компактная шкала делает Voice и Chat частями одной панели и устраняет
  конкуренцию с рукописью.

## Последствия

- legacy standalone review route может сохраняться только как совместимый deep
  link, но не должен быть основным действием из docked Studio;
- query-параметр или navigation не используется для обычного открытия S-21;
- responsive modal получает safe inset и собственный scroll, но остаётся тем же
  state текущего run;
- acceptance и E2E обязаны проверять отсутствие URL transition, modal bounds,
  direct Apply + Undo и компактный кегль Voice transcript.

## Запрещённый откат

Следующие изменения считаются продуктовой регрессией и требуют нового ADR:

- возврат display-кегля `18–28 px` для transcript/turn внутри docked Assistant;
- единственное действие `Проверить`, ведущее на отдельную полноэкранную страницу;
- скрытие или размонтирование Studio/editor при обычной проверке change set;
- одновременный показ `Применить изменения` и `Отправить` в форме уточнения;
- direct apply без server staging/revision validation или без grouped Undo;
- возврат floating margin/radius центральной рукописи при открытой Assistant panel.

## Связанные документы

- `docs/specification/WORKFLOWS.md`: R-18, WF-23, WF-33, AC-139…AC-142;
- `docs/specification/ASSISTANT_MODE.md`: единый live workspace и approval flow;
- `docs/design/DESIGN_HANDOFF.md`: разделы 30.1, 30.3 и 30.6;
- `design.pen`: S-19, S-20 и S-21 modal state.
