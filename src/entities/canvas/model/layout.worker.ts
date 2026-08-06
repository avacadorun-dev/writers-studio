/// <reference lib="webworker" />

import ELK from "elkjs/lib/elk.bundled.js";

import type { CanvasLayoutGraph } from "./layout";

const elk = new ELK();

self.addEventListener("message", (event: MessageEvent<CanvasLayoutGraph>) => {
  void elk
    .layout(event.data)
    .then((result) => self.postMessage({ result }))
    .catch((error: unknown) =>
      self.postMessage({ error: error instanceof Error ? error.message : "Ошибка раскладки" }),
    );
});
