import type {
  CanvasDocument,
  CanvasFieldDefinition,
  CanvasRelationType,
  CanvasTypeDefinition,
  CanvasTypeSchema,
} from "./types";

const summaryField: CanvasFieldDefinition = { id: "summary", name: "Кратко", type: "markdown" };

export const builtInCanvasCardTypes: CanvasTypeDefinition[] = [
  card("idea", "Идея", "general", "Lightbulb", "3", [summaryField]),
  card("question", "Вопрос", "general", "CircleHelp", "2", [summaryField]),
  card("source", "Источник", "general", "Library", "5", [
    summaryField,
    { id: "url", name: "URL", type: "short-text" },
  ]),
  card("character", "Персонаж", "narrative", "UserRound", "6", [
    summaryField,
    { id: "goal", name: "Цель", type: "markdown" },
    { id: "conflict", name: "Конфликт", type: "markdown" },
  ]),
  card("location", "Локация", "narrative", "MapPin", "4", [summaryField]),
  card("beat", "Бит / событие", "narrative", "Clapperboard", "3", [
    summaryField,
    { id: "date", name: "Дата", type: "date" },
    { id: "duration", name: "Длительность", type: "duration" },
  ]),
  card("conflict", "Конфликт", "narrative", "Swords", "1", [summaryField]),
  card("theme", "Тема", "narrative", "Sparkles", "5", [summaryField]),
  card("mechanic", "Механика", "game-design", "Gamepad2", "5", [summaryField]),
  card("quest", "Квест", "game-design", "ScrollText", "3", [
    summaryField,
    { id: "prerequisites", name: "Предусловия", type: "multi-select" },
  ]),
  card("dialogue", "Диалог", "game-design", "MessagesSquare", "6", [
    { id: "line", name: "Реплика", type: "markdown", required: true },
    { id: "terminal", name: "Завершает диалог", type: "checkbox" },
  ]),
  card("state", "Состояние", "game-design", "Workflow", "2", [
    summaryField,
    { id: "initial", name: "Начальное", type: "checkbox" },
    { id: "terminal", name: "Конечное", type: "checkbox" },
  ]),
  card("level", "Уровень", "game-design", "Map", "4", [summaryField]),
  card("resource", "Ресурс", "game-design", "Gem", "2", [
    summaryField,
    { id: "initialAmount", name: "Стартовое количество", type: "number" },
  ]),
];

export const builtInCanvasRelationTypes: CanvasRelationType[] = [
  relation("relates", "Связано с", false, "5"),
  relation("causes", "Вызывает", true, "3"),
  relation("contradicts", "Противоречит", false, "1"),
  relation("depends_on", "Зависит от", true, "2"),
  relation("blocks", "Блокирует", true, "1"),
  relation("reveals", "Раскрывает", true, "6"),
  relation("transitions_to", "Переходит в", true, "5"),
  relation("contains", "Содержит", true, "4"),
  relation("consumes", "Потребляет", true, "2"),
  relation("produces", "Производит", true, "4"),
  relation("rewards", "Награждает", true, "3"),
];

export const defaultCanvasTypeSchema: CanvasTypeSchema = {
  revision: 0,
  cardTypes: builtInCanvasCardTypes,
  relationTypes: builtInCanvasRelationTypes,
};

export function resolveCanvasTypeSchema(schema?: CanvasTypeSchema): CanvasTypeSchema {
  if (!schema || schema.revision === 0) return defaultCanvasTypeSchema;
  return schema;
}

export interface CanvasSchemaIssue {
  kind: "node" | "edge";
  id: string;
  message: string;
}

export function validateCanvasTypedDocument(
  document: CanvasDocument,
  schema: CanvasTypeSchema,
): CanvasSchemaIssue[] {
  const cardTypes = new Map(schema.cardTypes.map((type) => [type.id, type]));
  const relationTypes = new Map(schema.relationTypes.map((type) => [type.id, type]));
  const nodeTypes = new Map<string, string>();
  const issues: CanvasSchemaIssue[] = [];
  for (const node of document.nodes) {
    const metadata = node["x-writers-studio"]?.card;
    if (!metadata) continue;
    const type = cardTypes.get(metadata.typeId);
    if (!type) {
      issues.push({ kind: "node", id: node.id, message: `Неизвестный тип ${metadata.typeId}` });
      continue;
    }
    nodeTypes.set(node.id, type.id);
    for (const field of type.fields) {
      const value = metadata.fields[field.id];
      if (field.required && (value === undefined || value === null || value === "")) {
        issues.push({ kind: "node", id: node.id, message: `Поле «${field.name}» обязательно` });
      } else if (value !== undefined && value !== null && !validFieldValue(field, value)) {
        issues.push({
          kind: "node",
          id: node.id,
          message: `Поле «${field.name}» имеет неверный формат`,
        });
      }
    }
  }
  for (const edge of document.edges) {
    const metadata = edge["x-writers-studio"]?.relation;
    if (!metadata) continue;
    const type = relationTypes.get(metadata.typeId);
    if (!type) {
      issues.push({ kind: "edge", id: edge.id, message: `Неизвестная связь ${metadata.typeId}` });
      continue;
    }
    const fromType = nodeTypes.get(edge.fromNode);
    const toType = nodeTypes.get(edge.toNode);
    if (type.fromTypes?.length && (!fromType || !type.fromTypes.includes(fromType))) {
      issues.push({ kind: "edge", id: edge.id, message: "Недопустимый исходный тип карточки" });
    }
    if (type.toTypes?.length && (!toType || !type.toTypes.includes(toType))) {
      issues.push({ kind: "edge", id: edge.id, message: "Недопустимый целевой тип карточки" });
    }
  }
  return issues;
}

function card(
  id: string,
  name: string,
  category: CanvasTypeDefinition["category"],
  icon: string,
  color: CanvasTypeDefinition["color"],
  fields: CanvasFieldDefinition[],
): CanvasTypeDefinition {
  return { id, name, category, icon, color, fields };
}

function relation(
  id: string,
  name: string,
  directed: boolean,
  color: CanvasRelationType["color"],
): CanvasRelationType {
  return { id, name, directed, color };
}

function validFieldValue(field: CanvasFieldDefinition, value: unknown): boolean {
  if (field.type === "short-text" || field.type === "markdown") return typeof value === "string";
  if (field.type === "number" || field.type === "duration") {
    return (
      typeof value === "number" &&
      Number.isFinite(value) &&
      (field.type !== "duration" || value >= 0)
    );
  }
  if (field.type === "checkbox") return typeof value === "boolean";
  if (field.type === "date") return typeof value === "string" && !Number.isNaN(Date.parse(value));
  if (field.type === "multi-select") {
    return Array.isArray(value) && value.every((item) => typeof item === "string");
  }
  if (field.type === "single-select") {
    return typeof value === "string" && (!field.options || field.options.includes(value));
  }
  return typeof value === "string" && value.trim().length > 0;
}
