// SPDX-License-Identifier: MIT
// Runs before the first paint, so nothing jumps: the theme, kept in this browser until accounts store it (M-28). The
// design system expects a "dark" class. The widths of the columns are C22's (workspace-boot.js).
(() => {
  const root = document.documentElement;
  let theme = "system";
  try { theme = localStorage.getItem("ud.theme") ?? "system"; } catch { /* private window */ }
  const scheme = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => root.classList.toggle("dark", theme === "dark" || (theme === "system" && scheme.matches));
  apply();
  scheme.addEventListener("change", apply);
  window.udSetTheme = (value) => {
    theme = value;
    try { localStorage.setItem("ud.theme", value); } catch { /* private window: it lasts until reload */ }
    apply();
  };
  window.udTheme = () => theme;
})();
