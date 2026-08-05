import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { cn } from "~shared/lib";

const hiddenReasoning = /<(?:think|analysis)(?:\s[^>]*)?>[\s\S]*?<\/(?:think|analysis)>/giu;
const unclosedHiddenReasoning = /<(?:think|analysis)(?:\s[^>]*)?>[\s\S]*$/giu;
const orphanReasoningTags = /<\/?(?:think|analysis)(?:\s[^>]*)?>/giu;

export function sanitizeAssistantMarkdown(value: string): string {
  return value
    .replace(hiddenReasoning, "")
    .replace(unclosedHiddenReasoning, "")
    .replace(orphanReasoningTags, "")
    .replace(/\n{3,}/gu, "\n\n")
    .trim();
}

const components: Components = {
  h1: ({ children }) => <h1 className="mt-6 mb-3 text-2xl font-semibold">{children}</h1>,
  h2: ({ children }) => <h2 className="mt-5 mb-2 text-xl font-semibold">{children}</h2>,
  h3: ({ children }) => <h3 className="mt-4 mb-2 text-base font-semibold">{children}</h3>,
  p: ({ children }) => <p className="my-2 first:mt-0 last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="my-3 list-disc space-y-1 pl-6">{children}</ul>,
  ol: ({ children }) => <ol className="my-3 list-decimal space-y-1 pl-6">{children}</ol>,
  li: ({ children }) => <li className="pl-1 marker:text-[var(--accent)]">{children}</li>,
  blockquote: ({ children }) => (
    <blockquote className="my-4 border-l-2 border-[var(--accent)] bg-[var(--accent-soft)] px-4 py-2 text-[var(--ink-secondary)]">
      {children}
    </blockquote>
  ),
  a: ({ children, href }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-[var(--accent-ink)] underline decoration-[var(--accent)]/50 underline-offset-2 hover:decoration-[var(--accent)]"
    >
      {children}
    </a>
  ),
  code: ({ children, className }) => {
    const fenced = Boolean(className);
    return (
      <code
        className={cn(
          "font-mono",
          fenced
            ? className
            : "rounded bg-[var(--surface-subtle)] px-1.5 py-0.5 text-[0.92em] text-[var(--accent-ink)]",
        )}
      >
        {children}
      </code>
    );
  },
  pre: ({ children }) => (
    <pre className="my-4 overflow-x-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-4 text-[12px] leading-5">
      {children}
    </pre>
  ),
  table: ({ children }) => (
    <div className="my-4 overflow-x-auto rounded-[var(--radius-md)] border border-[var(--border-subtle)]">
      <table className="w-full min-w-[520px] border-collapse text-left text-[12px]">
        {children}
      </table>
    </div>
  ),
  thead: ({ children }) => <thead className="bg-[var(--surface-subtle)]">{children}</thead>,
  th: ({ children }) => (
    <th className="border-b border-[var(--border-subtle)] px-3 py-2 font-semibold">{children}</th>
  ),
  td: ({ children }) => (
    <td className="border-b border-[var(--border-subtle)] px-3 py-2 align-top last:border-r-0">
      {children}
    </td>
  ),
  hr: () => <hr className="my-5 border-[var(--border-subtle)]" />,
  strong: ({ children }) => (
    <strong className="font-semibold text-[var(--ink-primary)]">{children}</strong>
  ),
};

export function AssistantMarkdown({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  const safe = sanitizeAssistantMarkdown(children);
  if (!safe) return null;
  return (
    <div className={cn("assistant-markdown min-w-0 leading-6", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {safe}
      </ReactMarkdown>
    </div>
  );
}
