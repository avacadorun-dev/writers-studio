// biome-ignore-all lint/a11y/noNoninteractiveTabindex: overflow JSON regions must be keyboard-scrollable
import { Check, ChevronDown, ChevronUp, Copy } from "lucide-react";
import { useMemo, useState } from "react";

import { cn } from "~shared/lib";
import { AssistantMarkdown, sanitizeAssistantMarkdown } from "./assistant-markdown";
import { Button } from "./button";

const JSON_PREVIEW_LINES = 24;
const JSON_COLLAPSE_LENGTH = 1_800;

export interface AssistantMessageContent {
  kind: "json" | "markdown" | "plain";
  source: string;
  formatted: string;
  collapsible: boolean;
}

export function classifyAssistantMessage(value: string): AssistantMessageContent {
  const source = sanitizeAssistantMarkdown(value);
  const structured = parseStructuredJSON(source);
  if (structured !== undefined) {
    const formatted = JSON.stringify(structured, null, 2);
    return {
      kind: "json",
      source,
      formatted,
      collapsible:
        formatted.length > JSON_COLLAPSE_LENGTH ||
        formatted.split("\n").length > JSON_PREVIEW_LINES,
    };
  }
  return {
    kind: looksLikeMarkdown(source) ? "markdown" : "plain",
    source,
    formatted: source,
    collapsible: false,
  };
}

function parseStructuredJSON(value: string): unknown | undefined {
  if (!(value.startsWith("{") || value.startsWith("["))) return undefined;
  try {
    const parsed: unknown = JSON.parse(value);
    return parsed !== null && typeof parsed === "object" ? parsed : undefined;
  } catch {
    return undefined;
  }
}

function looksLikeMarkdown(value: string): boolean {
  return /(^|\n)(?:#{1,6}\s|[-*+]\s|\d+\.\s|>\s|```|~~~)|\[[^\]]+\]\([^)]+\)|[*_]{1,2}\S/u.test(
    value,
  );
}

export function AssistantMessage({
  children,
  className,
  compact = false,
}: {
  children: string;
  className?: string;
  compact?: boolean;
}) {
  const content = useMemo(() => classifyAssistantMessage(children), [children]);
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  if (!content.source) return null;
  if (content.kind === "markdown") {
    return <AssistantMarkdown className={className}>{content.source}</AssistantMarkdown>;
  }
  if (content.kind === "plain") {
    return (
      <p
        className={cn(
          "min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]",
          compact ? "leading-[1.55]" : "leading-6",
          className,
        )}
      >
        {content.source}
      </p>
    );
  }

  const preview = content.formatted.split("\n").slice(0, JSON_PREVIEW_LINES).join("\n");
  const visible = content.collapsible && !expanded ? `${preview}\n…` : content.formatted;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(content.source);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1_600);
    } catch {
      setCopied(false);
    }
  };

  return (
    <section
      aria-label="Структурированные данные"
      className={cn(
        "min-w-0 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]",
        className,
      )}
    >
      <div className="flex min-h-9 items-center justify-between gap-2 border-b border-[var(--border-subtle)] px-2.5">
        <span className="font-mono text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-tertiary)] uppercase">
          JSON
        </span>
        <Button
          type="button"
          variant="ghost"
          size="compact"
          className="min-h-7 px-2 text-[10px]"
          aria-label={copied ? "JSON скопирован" : "Скопировать исходный JSON"}
          onClick={() => void copy()}
        >
          {copied ? (
            <Check className="size-3" aria-hidden="true" />
          ) : (
            <Copy className="size-3" aria-hidden="true" />
          )}
          {copied ? "Скопировано" : "Копировать"}
        </Button>
      </div>
      <section
        tabIndex={0}
        aria-label="JSON-сообщение"
        className="max-h-80 min-w-0 overflow-auto p-3 font-mono text-[11px] leading-5 whitespace-pre [scrollbar-gutter:stable]"
      >
        <pre>
          <code>{visible}</code>
        </pre>
      </section>
      {content.collapsible ? (
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          className="flex min-h-9 w-full items-center justify-center gap-1.5 border-t border-[var(--border-subtle)] px-3 text-[11px] font-medium text-[var(--accent-ink)] hover:bg-[var(--surface-panel)] focus-visible:outline-2 focus-visible:outline-[var(--focus)]"
        >
          {expanded ? (
            <ChevronUp className="size-3.5" aria-hidden="true" />
          ) : (
            <ChevronDown className="size-3.5" aria-hidden="true" />
          )}
          {expanded ? "Свернуть" : "Показать полностью"}
        </button>
      ) : null}
    </section>
  );
}
