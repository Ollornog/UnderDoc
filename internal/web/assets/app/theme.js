// SPDX-License-Identifier: MIT
// Runs before the first paint: the design system expects a "dark" class and the page follows the system setting.
(() => {
  const scheme = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => document.documentElement.classList.toggle("dark", scheme.matches);
  apply();
  scheme.addEventListener("change", apply);
})();
