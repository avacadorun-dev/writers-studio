import type { SVGProps } from "react";

export function BrandMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg" focusable="false" {...props}>
      <title>Студия автора</title>
      <path
        d="M20 1c9 0 16 6 16 15v11c0 5-7 10-16 12C11 37 4 32 4 27V16C4 7 11 1 20 1Z"
        fill="var(--brand-mark-shield)"
        stroke="var(--brand-mark-accent)"
        strokeLinejoin="round"
        strokeWidth="1.4"
      />
      <path
        d="M20 4c8 0 13 5 13 12v10c0 4-6 8-13 10-7-2-13-6-13-10V16C7 9 12 4 20 4Z"
        fill="none"
        opacity="0.78"
        stroke="var(--brand-mark-accent)"
        strokeLinejoin="round"
        strokeWidth="0.8"
      />
      <path
        d="M12.5 30c1-9 2.5-14 7.5-15 5 1 6.5 6 7.5 15Z"
        fill="var(--brand-mark-accent)"
        stroke="var(--brand-mark-ink)"
        strokeLinejoin="round"
        strokeWidth="1.1"
      />
      <ellipse
        cx="20"
        cy="14.75"
        fill="var(--brand-mark-paper)"
        rx="4"
        ry="4.25"
        stroke="var(--brand-mark-ink)"
        strokeWidth="1"
      />
      <path
        d="M10 27c4-1.2 7-1 10 1 3-2 6-2.2 10-1l-.5 7c-4.5-1.2-7.5-1-9.5 1-2-2-5-2.2-9.5-1Z"
        fill="var(--brand-mark-paper)"
        stroke="var(--brand-mark-ink)"
        strokeLinejoin="round"
        strokeWidth="1.1"
      />
      <path
        d="M20 28v7m-7.2-5.7c2.2-.6 4.2-.3 5.7.5m-5.8 1.4c2.1-.5 3.9-.3 5.5.4m9-2.3c-2.2-.6-4.2-.3-5.7.5m5.8 1.4c-2.1-.5-3.9-.3-5.5.4"
        fill="none"
        stroke="var(--brand-mark-detail)"
        strokeLinecap="round"
        strokeWidth="0.55"
      />
      <path
        d="M31.125 10.5c0-1.633-1.867-2.45-3.325-1.575-1.575.933-1.283 3.208.408 3.792 1.692.583 3.209-.642 3.034-2.392m-1.692.175c0-.817-.933-1.167-1.517-.642-.583.525-.233 1.459.525 1.517"
        fill="none"
        stroke="var(--brand-mark-detail)"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="0.73"
      />
    </svg>
  );
}
