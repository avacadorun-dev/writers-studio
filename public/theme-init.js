(() => {
  const storageKey = "writers-studio-preferences";
  let theme = "system";
  let style = "quiet";
  const supportedStyles = [
    "quiet",
    "cedar-and-copper",
    "dried-rose-and-wine",
    "olive-ink",
    "fog-graphite",
    "linen-clay",
    "dusty-lavender",
    "porcelain-ink",
  ];
  try {
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? "{}");
    if (stored.theme === "light" || stored.theme === "dark") {
      theme = stored.theme;
    }
    const storedStyle =
      stored.style === "pine-copper"
        ? "cedar-and-copper"
        : stored.style === "moss-wine"
          ? "dried-rose-and-wine"
          : stored.style;
    if (supportedStyles.includes(storedStyle)) {
      style = storedStyle;
    }
  } catch {
    // Invalid local preferences are normalized after the application starts.
  }
  const systemIsDark = window.matchMedia?.("(prefers-color-scheme: dark)").matches === true;
  document.documentElement.dataset.theme =
    theme === "dark" || (theme === "system" && systemIsDark) ? "dark" : "light";
  document.documentElement.dataset.style = style;
})();
