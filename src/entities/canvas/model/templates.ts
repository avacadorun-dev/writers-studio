import { t } from "~shared/i18n";

import type { CanvasDocument, CanvasEdge, CanvasGroupNode, CanvasNode } from "./types";

export type CanvasTemplateId =
  | "blank"
  | "brain-dump"
  | "affinity-map"
  | "mind-map"
  | "three-act"
  | "character-map"
  | "research-moodboard"
  | "story-map"
  | "character-network"
  | "chronology"
  | "quest-dependency"
  | "dialogue-tree"
  | "state-machine"
  | "gameplay-core-loop"
  | "level-flow";

export interface CanvasTemplateDefinition {
  id: CanvasTemplateId;
  title: string;
  description: string;
}

export const canvasTemplates: CanvasTemplateDefinition[] = [
  { id: "blank", title: t("Чистая доска"), description: t("Пустое пространство без подсказок") },
  { id: "brain-dump", title: "Brain dump", description: t("Быстрый захват идей и вопросов") },
  { id: "affinity-map", title: "Affinity map", description: t("Кластеры для сортировки идей") },
  { id: "mind-map", title: "Mind map", description: t("Центральная тема и направления") },
  {
    id: "three-act",
    title: t("Трёхактный сюжет"),
    description: t("Завязка, конфронтация и развязка"),
  },
  {
    id: "character-map",
    title: t("Карта персонажей"),
    description: t("Герои и отношения между ними"),
  },
  {
    id: "research-moodboard",
    title: "Research / moodboard",
    description: t("Источники, факты и визуальные референсы"),
  },
  { id: "story-map", title: "Story map", description: t("Биты сюжета от завязки до финала") },
  {
    id: "character-network",
    title: "Character network",
    description: t("Типизированная сеть персонажей и конфликтов"),
  },
  { id: "chronology", title: "Chronology", description: t("События с датами для Timeline") },
  {
    id: "quest-dependency",
    title: "Quest dependency",
    description: t("Зависимости и награды цепочки квестов"),
  },
  {
    id: "dialogue-tree",
    title: "Dialogue tree",
    description: t("Ветвящийся диалог для play-through"),
  },
  { id: "state-machine", title: "State machine", description: t("Состояния и переходы системы") },
  {
    id: "gameplay-core-loop",
    title: "Gameplay core loop",
    description: t("Механики, производство и расход ресурсов"),
  },
  { id: "level-flow", title: "Level flow", description: t("Поток уровней и развилки") },
];

export function createCanvasTemplate(
  templateId: CanvasTemplateId,
  idFactory: () => string,
  origin = { x: 0, y: 0 },
): CanvasDocument {
  const nodes: CanvasNode[] = [];
  const edges: CanvasEdge[] = [];
  const note = (x: number, y: number, text: string, color?: CanvasNode["color"]) => {
    const node: CanvasNode = {
      id: idFactory(),
      type: "text",
      x: origin.x + x,
      y: origin.y + y,
      width: 280,
      height: 180,
      text,
      color,
    };
    nodes.push(node);
    return node;
  };
  const group = (
    x: number,
    y: number,
    width: number,
    height: number,
    label: string,
    color?: CanvasNode["color"],
  ) => {
    const node: CanvasGroupNode = {
      id: idFactory(),
      type: "group",
      x: origin.x + x,
      y: origin.y + y,
      width,
      height,
      label,
      color,
    };
    nodes.unshift(node);
    return node;
  };
  const connect = (from: CanvasNode, to: CanvasNode, label?: string) => {
    edges.push({
      id: idFactory(),
      fromNode: from.id,
      fromSide: "right",
      toNode: to.id,
      toSide: "left",
      toEnd: "arrow",
      label,
    });
  };
  const typedNote = (
    x: number,
    y: number,
    text: string,
    typeId: string,
    color?: CanvasNode["color"],
    fields: Record<string, unknown> = {},
  ) => {
    const node = note(x, y, text, color);
    node["x-writers-studio"] = { card: { typeId, fields } };
    return node;
  };
  const typedConnect = (from: CanvasNode, to: CanvasNode, typeId: string, label?: string) => {
    connect(from, to, label);
    const edge = edges.at(-1);
    if (edge) edge["x-writers-studio"] = { relation: { typeId } };
  };

  if (templateId === "brain-dump") {
    group(-40, -70, 680, 540, t("Выгрузка мыслей"), "5");
    note(0, 0, t("# Идея\n\nЧто пришло в голову?"), "3");
    note(320, 0, t("# Вопрос\n\nЧто пока непонятно?"), "2");
    note(0, 220, t("# Наблюдение\n\nЧто уже известно?"), "4");
    note(320, 220, t("# Следующий шаг\n\nЧто проверить?"), "5");
  } else if (templateId === "affinity-map") {
    for (const [index, title, color] of [
      [0, t("Кластер A"), "3"],
      [1, t("Кластер B"), "4"],
      [2, t("Кластер C"), "5"],
    ] as const) {
      group(index * 360, 0, 320, 520, title, color);
      note(index * 360 + 20, 40, t("# Идея\n\nПеретащите или перепишите"), color);
      note(index * 360 + 20, 250, t("# Идея\n\nНайдите общий мотив"), color);
    }
  } else if (templateId === "mind-map") {
    const center = note(360, 180, t("# Центральная тема"), "5");
    for (const [x, y, title, color] of [
      [0, 0, t("Направление 1"), "3"],
      [720, 0, t("Направление 2"), "4"],
      [0, 400, t("Направление 3"), "2"],
      [720, 400, t("Направление 4"), "6"],
    ] as const) {
      const branch = note(x, y, t("# {0}\n\nАссоциации и детали", title), color);
      if (x < 360) connect(branch, center);
      else connect(center, branch);
    }
  } else if (templateId === "three-act") {
    for (const [index, label, prompt, color] of [
      [0, t("Акт I · Завязка"), t("Мир, герой, провоцирующее событие"), "3"],
      [1, t("Акт II · Конфронтация"), t("Препятствия, midpoint, кризис"), "2"],
      [2, t("Акт III · Развязка"), t("Кульминация и новый порядок"), "4"],
    ] as const) {
      const frame: CanvasGroupNode = {
        id: idFactory(),
        type: "group",
        x: origin.x + index * 440,
        y: origin.y,
        width: 400,
        height: 500,
        label,
        "x-writers-studio": { element: { kind: "frame", order: index + 1, label } },
      };
      nodes.unshift(frame);
      note(index * 440 + 60, 80, `# ${label}\n\n${prompt}`, color);
      note(index * 440 + 60, 290, t("# Ключевой бит\n\nЧто необратимо меняется?"), color);
    }
  } else if (templateId === "character-map") {
    const protagonist = note(360, 200, t("# Протагонист\n\nЦель, страх, потребность"), "5");
    for (const [x, y, title, relation, color] of [
      [0, 0, t("Союзник"), t("поддерживает"), "4"],
      [720, 0, t("Антагонист"), t("противостоит"), "1"],
      [0, 420, t("Наставник"), t("направляет"), "3"],
      [720, 420, t("Зеркало"), t("раскрывает"), "6"],
    ] as const) {
      const character = note(x, y, t("# {0}\n\nЖелание и конфликт", title), color);
      connect(character, protagonist, relation);
    }
  } else if (templateId === "research-moodboard") {
    group(-40, -70, 1320, 570, "Research / moodboard", "6");
    note(0, 0, t("# Вопрос исследования\n\nЧто нужно выяснить?"), "2");
    note(320, 0, t("# Источник\n\nВставьте URL или PDF"), "5");
    note(640, 0, t("# Факт\n\nЦитата и контекст"), "4");
    note(960, 0, t("# Визуальный референс\n\nПеретащите изображение"), "6");
    note(0, 230, t("# Вывод\n\nКак это влияет на произведение?"), "3");
  } else if (templateId === "story-map") {
    const beats = [
      typedNote(0, 0, t("# Завязка\n\nЧто нарушает равновесие?"), "beat", "3"),
      typedNote(340, 0, t("# Первый поворот\n\nНазад дороги нет"), "beat", "2"),
      typedNote(680, 0, t("# Midpoint\n\nМеняется понимание цели"), "beat", "5"),
      typedNote(1020, 0, t("# Кризис\n\nЦена поражения"), "beat", "1"),
      typedNote(1360, 0, t("# Финал\n\nНовый порядок"), "beat", "4"),
    ];
    beats.slice(1).forEach((beat, index) => {
      typedConnect(beats[index] as CanvasNode, beat, "causes");
    });
  } else if (templateId === "character-network") {
    const hero = typedNote(
      420,
      220,
      t("# Герой\n\nЦель и внутренняя потребность"),
      "character",
      "5",
    );
    const ally = typedNote(0, 0, t("# Союзник\n\nЧем рискует?"), "character", "4");
    const rival = typedNote(840, 0, t("# Соперник\n\nВ чём он прав?"), "character", "1");
    const mentor = typedNote(0, 440, t("# Наставник\n\nЧто скрывает?"), "character", "3");
    typedConnect(ally, hero, "relates", t("поддерживает"));
    typedConnect(rival, hero, "contradicts", t("противостоит"));
    typedConnect(mentor, hero, "reveals", t("раскрывает путь"));
  } else if (templateId === "chronology") {
    const dates = ["2027-01-01", "2027-01-05", "2027-01-12", "2027-02-01"];
    const beats = dates.map((date, index) =>
      typedNote(index * 340, 0, t("# Событие {0}\n\n{1}", index + 1, date), "beat", "5", { date }),
    );
    beats.slice(1).forEach((beat, index) => {
      typedConnect(beats[index] as CanvasNode, beat, "causes");
    });
  } else if (templateId === "quest-dependency") {
    const intro = typedNote(0, 200, t("# Сигнал бедствия\n\nПолучить координаты"), "quest", "3");
    const parts = typedNote(360, 0, t("# Найти детали\n\nПочинить передатчик"), "quest", "2");
    const ally = typedNote(360, 400, t("# Заручиться помощью\n\nУбедить механика"), "quest", "4");
    const finale = typedNote(
      760,
      200,
      t("# Запустить маяк\n\nНаграда: новый сектор"),
      "quest",
      "5",
    );
    typedConnect(intro, parts, "depends_on");
    typedConnect(intro, ally, "depends_on");
    typedConnect(parts, finale, "depends_on");
    typedConnect(ally, finale, "depends_on");
  } else if (templateId === "dialogue-tree") {
    const greeting = typedNote(0, 180, t("# Диспетчер\n\nНазовите код доступа"), "dialogue", "5", {
      line: t("Назовите код доступа"),
    });
    const honest = typedNote(380, 0, t("# Честный ответ\n\nКода у нас нет"), "dialogue", "4", {
      line: t("Кода у нас нет"),
    });
    const bluff = typedNote(380, 360, t("# Блеф\n\nКод: Орион-7"), "dialogue", "2", {
      line: t("Код: Орион-7"),
    });
    const end = typedNote(780, 180, t("# Шлюз открыт\n\nПроходите"), "dialogue", "3", {
      line: t("Проходите"),
      terminal: true,
    });
    typedConnect(greeting, honest, "transitions_to", t("Признаться"));
    typedConnect(greeting, bluff, "transitions_to", t("Солгать"));
    typedConnect(honest, end, "transitions_to", t("Попросить помощи"));
    typedConnect(bluff, end, "transitions_to", t("Подтвердить код"));
  } else if (templateId === "state-machine") {
    const idle = typedNote(0, 180, "# Idle", "state", "5", { initial: true });
    const alert = typedNote(380, 0, "# Alert", "state", "2");
    const combat = typedNote(760, 180, "# Combat", "state", "1");
    const search = typedNote(380, 360, "# Search", "state", "3");
    typedConnect(idle, alert, "transitions_to", t("шум"));
    typedConnect(alert, combat, "transitions_to", t("цель видна"));
    typedConnect(alert, search, "transitions_to", t("цель потеряна"));
    typedConnect(search, idle, "transitions_to", t("таймер истёк"));
  } else if (templateId === "gameplay-core-loop") {
    const explore = typedNote(
      0,
      180,
      t("# Исследовать\n\nНайти угрозы и возможности"),
      "mechanic",
      "5",
    );
    const fight = typedNote(380, 0, t("# Сражаться\n\nПреодолеть сопротивление"), "mechanic", "1");
    const salvage = typedNote(760, 180, t("# Собирать\n\nПолучить лом"), "mechanic", "4");
    const upgrade = typedNote(380, 360, t("# Улучшать\n\nТратить лом на модули"), "mechanic", "3");
    const scrap = typedNote(760, 430, t("# Лом"), "resource", "2", { initialAmount: 0 });
    typedConnect(explore, fight, "causes");
    typedConnect(fight, salvage, "causes");
    typedConnect(salvage, scrap, "produces");
    typedConnect(scrap, upgrade, "consumes");
    typedConnect(upgrade, explore, "causes");
  } else if (templateId === "level-flow") {
    const hub = typedNote(0, 200, t("# Хаб\n\nВыбор направления"), "level", "5");
    const stealth = typedNote(380, 0, t("# Тихий маршрут"), "level", "4");
    const combat = typedNote(380, 400, t("# Боевой маршрут"), "level", "1");
    const boss = typedNote(800, 200, t("# Финальная арена"), "level", "3");
    typedConnect(hub, stealth, "transitions_to");
    typedConnect(hub, combat, "transitions_to");
    typedConnect(stealth, boss, "transitions_to");
    typedConnect(combat, boss, "transitions_to");
  }

  return { nodes, edges, "x-writers-studio": { schemaVersion: 2 } };
}
