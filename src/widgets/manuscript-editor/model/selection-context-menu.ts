export interface SelectionContextMenuPosition {
  x: number;
  y: number;
}

const menuWidth = 268;
const menuHeight = 132;
const viewportMargin = 8;

export function placeSelectionContextMenu(
  x: number,
  y: number,
  viewportWidth: number,
  viewportHeight: number,
): SelectionContextMenuPosition {
  const maximumX = Math.max(viewportMargin, viewportWidth - menuWidth - viewportMargin);
  const maximumY = Math.max(viewportMargin, viewportHeight - menuHeight - viewportMargin);
  return {
    x: Math.min(Math.max(viewportMargin, x), maximumX),
    y: Math.min(Math.max(viewportMargin, y), maximumY),
  };
}
