import { describe, expect, test } from "bun:test";

import { sanitizePastedHTML } from "./sanitize-pasted-html";

describe("Writer's Studio paste sanitizer", () => {
  test("keeps supported Word and Google Docs semantics without active content", () => {
    const sanitized = sanitizePastedHTML(`
      <!--[if gte mso 9]><xml>unsafe</xml><![endif]-->
      <script>alert("x")</script>
      <style>body { display: none }</style>
      <p class="MsoNormal" style="margin: 0">
        <span style="font-weight: 700">Полужирный</span>
        <span style="font-style: italic; text-decoration: underline">курсив</span>
        <a href="javascript:alert(1)" onclick="alert(1)">опасная ссылка</a>
      </p>
      <div><span>Следующий абзац</span></div>
    `);

    expect(sanitized).not.toMatch(/script|style|onclick|javascript|class=|style=/iu);
    expect(sanitized).toContain("<strong>Полужирный</strong>");
    expect(sanitized).toContain("<u><em>курсив</em></u>");
    expect(sanitized).toContain("опасная ссылка");
    expect(sanitized).not.toContain("<a");
    expect(sanitized).toContain("<p>Следующий абзац</p>");
  });

  test("preserves safe internal provenance and safe links", () => {
    const sanitized = sanitizePastedHTML(`
      <p data-block-id="block-1" data-source-refs="segment-1,segment-2" data-unknown="x">
        <a href="https://example.test" target="_blank">Источник</a>
        <a href="/projects/11111111-1111-1111-1111-111111111111/studio/22222222-2222-2222-2222-222222222222#heading">Глава</a>
      </p>
    `);

    expect(sanitized).toContain('data-block-id="block-1"');
    expect(sanitized).toContain('data-source-refs="segment-1,segment-2"');
    expect(sanitized).not.toContain("data-unknown");
    expect(sanitized).toContain('<a href="https://example.test">Источник</a>');
    expect(sanitized).toContain(
      "/projects/11111111-1111-1111-1111-111111111111/studio/22222222-2222-2222-2222-222222222222#heading",
    );
  });

  test("keeps rich editor blocks but strips unsafe image and diagram payloads", () => {
    const sanitized = sanitizePastedHTML(`
      <aside data-callout-kind="warning" onclick="alert(1)"><p>Риск</p></aside>
      <table><tbody><tr><th colspan="2">Параметр</th></tr><tr><td>A</td><td>B</td></tr></tbody></table>
      <pre><code class="language-lua">return true</code></pre>
      <figure data-image-block data-asset-id="45b17482-b4a0-4624-a68c-d2f218eb4e20">
        <img src="/api/media/45b17482-b4a0-4624-a68c-d2f218eb4e20/content" alt="Карта" width="640" onerror="alert(1)">
        <figcaption>Карта уровня</figcaption>
      </figure>
      <figure data-image-block><img src="data:image/svg+xml,bad"></figure>
      <figure data-mermaid-block><pre data-mermaid-source>flowchart TD\nA--&gt;B</pre><svg><script>alert(1)</script></svg></figure>
    `);

    expect(sanitized).toContain('data-callout-kind="warning"');
    expect(sanitized).toContain('<th colspan="2">Параметр</th>');
    expect(sanitized).toContain('class="language-lua"');
    expect(sanitized).toContain('src="/api/media/45b17482-b4a0-4624-a68c-d2f218eb4e20/content"');
    expect(sanitized).toContain("flowchart TD");
    expect(sanitized).not.toMatch(/onclick|onerror|data:image|<svg|<script/iu);
  });
});
