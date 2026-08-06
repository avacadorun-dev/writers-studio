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
  { id: "blank", title: "Чистая доска", description: "Пустое пространство без подсказок" },
  { id: "brain-dump", title: "Brain dump", description: "Быстрый захват идей и вопросов" },
  { id: "affinity-map", title: "Affinity map", description: "Кластеры для сортировки идей" },
  { id: "mind-map", title: "Mind map", description: "Центральная тема и направления" },
  { id: "three-act", title: "Трёхактный сюжет", description: "Завязка, конфронтация и развязка" },
  { id: "character-map", title: "Карта персонажей", description: "Герои и отношения между ними" },
  {
    id: "research-moodboard",
    title: "Research / moodboard",
    description: "Источники, факты и визуальные референсы",
  },
  { id: "story-map", title: "Story map", description: "Биты сюжета от завязки до финала" },
  {
    id: "character-network",
    title: "Character network",
    description: "Типизированная сеть персонажей и конфликтов",
  },
  { id: "chronology", title: "Chronology", description: "События с датами для Timeline" },
  {
    id: "quest-dependency",
    title: "Quest dependency",
    description: "Зависимости и награды цепочки квестов",
  },
  {
    id: "dialogue-tree",
    title: "Dialogue tree",
    description: "Ветвящийся диалог для play-through",
  },
  { id: "state-machine", title: "State machine", description: "Состояния и переходы системы" },
  {
    id: "gameplay-core-loop",
    title: "Gameplay core loop",
    description: "Механики, производство и расход ресурсов",
  },
  { id: "level-flow", title: "Level flow", description: "Поток уровней и развилки" },
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
    group(-40, -70, 680, 540, "Выгрузка мыслей", "5");
    note(0, 0, "# Идея\n\nЧто пришло в голову?", "3");
    note(320, 0, "# Вопрос\n\nЧто пока непонятно?", "2");
    note(0, 220, "# Наблюдение\n\nЧто уже известно?", "4");
    note(320, 220, "# Следующий шаг\n\nЧто проверить?", "5");
  } else if (templateId === "affinity-map") {
    for (const [index, title, color] of [
      [0, "Кластер A", "3"],
      [1, "Кластер B", "4"],
      [2, "Кластер C", "5"],
    ] as const) {
      group(index * 360, 0, 320, 520, title, color);
      note(index * 360 + 20, 40, "# Идея\n\nПеретащите или перепишите", color);
      note(index * 360 + 20, 250, "# Идея\n\nНайдите общий мотив", color);
    }
  } else if (templateId === "mind-map") {
    const center = note(360, 180, "# Центральная тема", "5");
    for (const [x, y, title, color] of [
      [0, 0, "Направление 1", "3"],
      [720, 0, "Направление 2", "4"],
      [0, 400, "Направление 3", "2"],
      [720, 400, "Направление 4", "6"],
    ] as const) {
      const branch = note(x, y, `# ${title}\n\nАссоциации и детали`, color);
      if (x < 360) connect(branch, center);
      else connect(center, branch);
    }
  } else if (templateId === "three-act") {
    for (const [index, label, prompt, color] of [
      [0, "Акт I · Завязка", "Мир, герой, провоцирующее событие", "3"],
      [1, "Акт II · Конфронтация", "Препятствия, midpoint, кризис", "2"],
      [2, "Акт III · Развязка", "Кульминация и новый порядок", "4"],
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
      note(index * 440 + 60, 290, "# Ключевой бит\n\nЧто необратимо меняется?", color);
    }
  } else if (templateId === "character-map") {
    const protagonist = note(360, 200, "# Протагонист\n\nЦель, страх, потребность", "5");
    for (const [x, y, title, relation, color] of [
      [0, 0, "Союзник", "поддерживает", "4"],
      [720, 0, "Антагонист", "противостоит", "1"],
      [0, 420, "Наставник", "направляет", "3"],
      [720, 420, "Зеркало", "раскрывает", "6"],
    ] as const) {
      const character = note(x, y, `# ${title}\n\nЖелание и конфликт`, color);
      connect(character, protagonist, relation);
    }
  } else if (templateId === "research-moodboard") {
    group(-40, -70, 1320, 570, "Research / moodboard", "6");
    note(0, 0, "# Вопрос исследования\n\nЧто нужно выяснить?", "2");
    note(320, 0, "# Источник\n\nВставьте URL или PDF", "5");
    note(640, 0, "# Факт\n\nЦитата и контекст", "4");
    note(960, 0, "# Визуальный референс\n\nПеретащите изображение", "6");
    note(0, 230, "# Вывод\n\nКак это влияет на произведение?", "3");
  } else if (templateId === "story-map") {
    const beats = [
      typedNote(0, 0, "# Завязка\n\nЧто нарушает равновесие?", "beat", "3"),
      typedNote(340, 0, "# Первый поворот\n\nНазад дороги нет", "beat", "2"),
      typedNote(680, 0, "# Midpoint\n\nМеняется понимание цели", "beat", "5"),
      typedNote(1020, 0, "# Кризис\n\nЦена поражения", "beat", "1"),
      typedNote(1360, 0, "# Финал\n\nНовый порядок", "beat", "4"),
    ];
    beats.slice(1).forEach((beat, index) => {
      typedConnect(beats[index] as CanvasNode, beat, "causes");
    });
  } else if (templateId === "character-network") {
    const hero = typedNote(420, 220, "# Герой\n\nЦель и внутренняя потребность", "character", "5");
    const ally = typedNote(0, 0, "# Союзник\n\nЧем рискует?", "character", "4");
    const rival = typedNote(840, 0, "# Соперник\n\nВ чём он прав?", "character", "1");
    const mentor = typedNote(0, 440, "# Наставник\n\nЧто скрывает?", "character", "3");
    typedConnect(ally, hero, "relates", "поддерживает");
    typedConnect(rival, hero, "contradicts", "противостоит");
    typedConnect(mentor, hero, "reveals", "раскрывает путь");
  } else if (templateId === "chronology") {
    const dates = ["2027-01-01", "2027-01-05", "2027-01-12", "2027-02-01"];
    const beats = dates.map((date, index) =>
      typedNote(index * 340, 0, `# Событие ${index + 1}\n\n${date}`, "beat", "5", { date }),
    );
    beats.slice(1).forEach((beat, index) => {
      typedConnect(beats[index] as CanvasNode, beat, "causes");
    });
  } else if (templateId === "quest-dependency") {
    const intro = typedNote(0, 200, "# Сигнал бедствия\n\nПолучить координаты", "quest", "3");
    const parts = typedNote(360, 0, "# Найти детали\n\nПочинить передатчик", "quest", "2");
    const ally = typedNote(360, 400, "# Заручиться помощью\n\nУбедить механика", "quest", "4");
    const finale = typedNote(760, 200, "# Запустить маяк\n\nНаграда: новый сектор", "quest", "5");
    typedConnect(intro, parts, "depends_on");
    typedConnect(intro, ally, "depends_on");
    typedConnect(parts, finale, "depends_on");
    typedConnect(ally, finale, "depends_on");
  } else if (templateId === "dialogue-tree") {
    const greeting = typedNote(0, 180, "# Диспетчер\n\nНазовите код доступа", "dialogue", "5", {
      line: "Назовите код доступа",
    });
    const honest = typedNote(380, 0, "# Честный ответ\n\nКода у нас нет", "dialogue", "4", {
      line: "Кода у нас нет",
    });
    const bluff = typedNote(380, 360, "# Блеф\n\nКод: Орион-7", "dialogue", "2", {
      line: "Код: Орион-7",
    });
    const end = typedNote(780, 180, "# Шлюз открыт\n\nПроходите", "dialogue", "3", {
      line: "Проходите",
      terminal: true,
    });
    typedConnect(greeting, honest, "transitions_to", "Признаться");
    typedConnect(greeting, bluff, "transitions_to", "Солгать");
    typedConnect(honest, end, "transitions_to", "Попросить помощи");
    typedConnect(bluff, end, "transitions_to", "Подтвердить код");
  } else if (templateId === "state-machine") {
    const idle = typedNote(0, 180, "# Idle", "state", "5", { initial: true });
    const alert = typedNote(380, 0, "# Alert", "state", "2");
    const combat = typedNote(760, 180, "# Combat", "state", "1");
    const search = typedNote(380, 360, "# Search", "state", "3");
    typedConnect(idle, alert, "transitions_to", "шум");
    typedConnect(alert, combat, "transitions_to", "цель видна");
    typedConnect(alert, search, "transitions_to", "цель потеряна");
    typedConnect(search, idle, "transitions_to", "таймер истёк");
  } else if (templateId === "gameplay-core-loop") {
    const explore = typedNote(
      0,
      180,
      "# Исследовать\n\nНайти угрозы и возможности",
      "mechanic",
      "5",
    );
    const fight = typedNote(380, 0, "# Сражаться\n\nПреодолеть сопротивление", "mechanic", "1");
    const salvage = typedNote(760, 180, "# Собирать\n\nПолучить лом", "mechanic", "4");
    const upgrade = typedNote(380, 360, "# Улучшать\n\nТратить лом на модули", "mechanic", "3");
    const scrap = typedNote(760, 430, "# Лом", "resource", "2", { initialAmount: 0 });
    typedConnect(explore, fight, "causes");
    typedConnect(fight, salvage, "causes");
    typedConnect(salvage, scrap, "produces");
    typedConnect(scrap, upgrade, "consumes");
    typedConnect(upgrade, explore, "causes");
  } else if (templateId === "level-flow") {
    const hub = typedNote(0, 200, "# Хаб\n\nВыбор направления", "level", "5");
    const stealth = typedNote(380, 0, "# Тихий маршрут", "level", "4");
    const combat = typedNote(380, 400, "# Боевой маршрут", "level", "1");
    const boss = typedNote(800, 200, "# Финальная арена", "level", "3");
    typedConnect(hub, stealth, "transitions_to");
    typedConnect(hub, combat, "transitions_to");
    typedConnect(stealth, boss, "transitions_to");
    typedConnect(combat, boss, "transitions_to");
  }

  return { nodes, edges, "x-writers-studio": { schemaVersion: 2 } };
}
