import { getViewportForBounds } from "@xyflow/react";

import type { CanvasExportBounds, CanvasExportFormat } from "~entities/canvas";

interface ExportCanvasViewportOptions {
  viewport: HTMLElement;
  bounds: CanvasExportBounds;
  width: number;
  height: number;
  backgroundColor: string;
  fileName: string;
  format: CanvasExportFormat;
}

function downloadDataUrl(dataUrl: string, fileName: string): void {
  const anchor = document.createElement("a");
  anchor.href = dataUrl;
  anchor.download = fileName;
  anchor.click();
}

export async function exportCanvasViewport({
  viewport,
  bounds,
  width,
  height,
  backgroundColor,
  fileName,
  format,
}: ExportCanvasViewportOptions): Promise<void> {
  if (format === "canvas") throw new Error("JSON Canvas export does not use the visual viewport");
  const { x, y, zoom } = getViewportForBounds(bounds, width, height, 0.01, 4, 0.08);
  const renderOptions = {
    width,
    height,
    backgroundColor,
    cacheBust: true,
    pixelRatio: 1,
    skipAutoScale: true,
    style: {
      width: `${width}px`,
      height: `${height}px`,
      transform: `translate(${x}px, ${y}px) scale(${zoom})`,
    },
  };

  const image = await import("html-to-image");
  if (format === "svg") {
    downloadDataUrl(await image.toSvg(viewport, renderOptions), fileName);
    return;
  }

  const png = await image.toPng(viewport, renderOptions);
  if (format === "png") {
    downloadDataUrl(png, fileName);
    return;
  }

  const { jsPDF } = await import("jspdf");
  const pdf = new jsPDF({
    orientation: width >= height ? "landscape" : "portrait",
    unit: "px",
    format: [width, height],
    hotfixes: ["px_scaling"],
    compress: true,
  });
  pdf.addImage(png, "PNG", 0, 0, width, height, undefined, "FAST");
  await pdf.save(fileName, { returnPromise: true });
}
