export const maximumNodeTitleLength = 200;

export function normalizeNodeTitle(draft: string, current = ""): string | null {
  const title = draft.trim();
  if (!title || title === current.trim() || Array.from(title).length > maximumNodeTitleLength) {
    return null;
  }
  return title;
}
