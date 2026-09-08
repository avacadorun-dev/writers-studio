export const maximumEditorImageBytes = 12 * 1024 * 1024;
export const supportedEditorImageTypes = ["image/png", "image/jpeg", "image/gif"] as const;

export type EditorImageValidation = { valid: true } | { valid: false; message: string };

export function validateEditorImageFile(file: Pick<File, "size" | "type">): EditorImageValidation {
  if (
    !supportedEditorImageTypes.includes(file.type as (typeof supportedEditorImageTypes)[number])
  ) {
    return { valid: false, message: "Поддерживаются только PNG, JPEG и GIF." };
  }
  if (file.size < 1) {
    return { valid: false, message: "Файл изображения пуст." };
  }
  if (file.size > maximumEditorImageBytes) {
    return { valid: false, message: "Изображение должно быть не больше 12 МиБ." };
  }
  return { valid: true };
}

export function isExternalFileDrag(dataTransfer: Pick<DataTransfer, "types">): boolean {
  return Array.from(dataTransfer.types).includes("Files");
}
