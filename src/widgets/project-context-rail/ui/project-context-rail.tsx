import { Link } from "@tanstack/react-router";
import {
  AudioLines,
  BarChart3,
  History,
  Maximize2,
  MessageSquareText,
  PanelRight,
} from "lucide-react";

import { Button } from "~shared/ui";

export function ProjectContextRail({
  nodeId,
  annotationCount = 0,
  onAnnotations,
}: {
  nodeId: string;
  annotationCount?: number;
  onAnnotations?: () => void;
}) {
  return (
    <aside
      className="hidden w-14 shrink-0 flex-col items-center border-l border-[var(--border-subtle)] bg-[var(--surface-panel)] py-4 lg:flex"
      aria-label="Контекст документа"
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Обзор документа"
        onClick={() => window.dispatchEvent(new CustomEvent("writers-studio-overview"))}
      >
        <PanelRight className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="relative"
        aria-label={`Заметки на полях${annotationCount ? `: ${annotationCount}` : ""}`}
        onClick={onAnnotations}
      >
        <MessageSquareText className="size-4" aria-hidden="true" />
        {annotationCount > 0 ? (
          <span className="absolute top-0.5 right-0.5 grid min-w-4 place-items-center rounded-full bg-[var(--warning-soft)] px-1 text-[9px] leading-4 text-[var(--warning)]">
            {annotationCount > 99 ? "99+" : annotationCount}
          </span>
        ) : null}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Прослушать источник"
        onClick={() => window.dispatchEvent(new CustomEvent("writers-studio-play-source"))}
      >
        <AudioLines className="size-4" aria-hidden="true" />
      </Button>
      <Button asChild variant="ghost" size="icon">
        <Link to="/nodes/$nodeId/versions" params={{ nodeId }} aria-label="История версий">
          <History className="size-4" aria-hidden="true" />
        </Link>
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Статистика документа"
        onClick={() => window.dispatchEvent(new CustomEvent("writers-studio-stats"))}
      >
        <BarChart3 className="size-4" aria-hidden="true" />
      </Button>
      <div className="flex-1" />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="Режим фокуса"
        onClick={() => window.dispatchEvent(new CustomEvent("writers-studio-focus"))}
      >
        <Maximize2 className="size-4" aria-hidden="true" />
      </Button>
    </aside>
  );
}
