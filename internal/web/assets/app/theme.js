// SPDX-License-Identifier: MIT
// Runs before the first paint, so nothing jumps: the theme and the widths of the columns, both kept in this browser
// until accounts store them (M-28). The design system expects a "dark" class.
(() => {
  const root = document.documentElement;
  const read = (key, fallback) => {
    try { return localStorage.getItem(key) ?? fallback; } catch { return fallback; }
  };
  let theme = read("ud.theme", "system");
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

  // Nav1 and nav2 snap to four steps, nav3 moves freely. Nav1 starts at the icon, nav2 with icon and text beside it.
  window.udStages = [50, 84, 116, 172];
  let columns = {};
  try { columns = JSON.parse(read("ud.columns", "{}")) || {}; } catch { columns = {}; }
  const step = (value, fallback) => (Number.isInteger(value) && value >= 0 && value <= 3 ? value : fallback);
  const s1 = step(columns.s1, 0), s2 = step(columns.s2, 3);
  const w3 = Number.isFinite(columns.w3) ? Math.min(640, Math.max(180, columns.w3)) : 280;
  root.dataset.udS1 = s1;
  root.dataset.udS2 = s2;
  root.style.setProperty("--ud-w1", window.udStages[s1] + "px");
  root.style.setProperty("--ud-w2", window.udStages[s2] + "px");
  root.style.setProperty("--ud-w3", w3 + "px");
})();
