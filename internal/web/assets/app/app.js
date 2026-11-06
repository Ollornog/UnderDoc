// SPDX-License-Identifier: MIT
// The sidebar button and the search shortcuts. Inline handlers would need a weaker CSP.
const sidebar = () => document.querySelector("aside.sidebar");

document.addEventListener("click", (event) => {
  if (event.target.closest("[data-sidebar-toggle]")) {
    sidebar()?.toggle();
  }
});

// "/" must stay a normal character while someone is typing, Ctrl+K and Cmd+K work everywhere.
document.addEventListener("keydown", (event) => {
  const typing = event.target.closest("input, textarea, select, [contenteditable]");
  const slash = event.key === "/" && !typing && !event.ctrlKey && !event.metaKey && !event.altKey;
  const combination = event.key.toLowerCase() === "k" && (event.ctrlKey || event.metaKey);
  if (!slash && !combination) {
    return;
  }
  event.preventDefault();
  sidebar()?.open();
  const field = document.querySelector('input[name="q"]');
  field?.focus();
  field?.select();
});
