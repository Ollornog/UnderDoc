// SPDX-License-Identifier: MIT
// What is UnderDoc's own in the workspace (ADR-16). C22 does the columns, the arrows and double clicks in the lists,
// the click on the active view, the drawer and the menus (c22.js). This file swaps parts of the page in, so the list
// keeps what is open in it, and handles the menu of a row, the theme and the search keys. No inline handlers: strict CSP.
(() => {
  const workspace = () => document.querySelector(".workspace");

  const load = async (href) => {
    const response = await fetch(href, { headers: { Accept: "text/html" }, credentials: "same-origin" });
    if (!response.ok || !response.headers.get("Content-Type")?.startsWith("text/html")) {
      throw new Error(String(response.status));
    }
    return new DOMParser().parseFromString(await response.text(), "text/html");
  };
  // Markup that came in later needs the components of C22 wired up, like the page did on load.
  const wire = (element) => {
    window.C22?.init?.(element);
    window.C22?.workspace?.init?.(document);
  };
  // What the window shows stays marked in the list.
  const markOpen = () => {
    document.querySelectorAll('.workspace-nav[data-level="3"] .workspace-tree a').forEach((name) => {
      if (new URL(name.href).pathname === location.pathname) {
        name.setAttribute("aria-current", "page");
      } else {
        name.removeAttribute("aria-current");
      }
    });
  };

  // A click on a name in nav3 swaps only the window.
  const swapWindow = async (href, push) => {
    let next;
    try {
      next = await load(href);
    } catch {
      location.href = href;
      return;
    }
    const main = next.querySelector("[data-ud-main]");
    const collection = (d) => d.querySelector('.workspace-nav[data-level="3"] section > header > a')?.getAttribute("href");
    if (!main || collection(next) !== collection(document)) {
      location.href = href;
      return;
    }
    const mine = document.querySelector("[data-ud-main]");
    mine.replaceChildren(...main.childNodes);
    document.querySelector("[data-ud-tab]").replaceChildren(...next.querySelector("[data-ud-tab]").childNodes);
    document.title = next.title;
    if (push) {
      history.pushState({ ud: true }, "", href);
    }
    markOpen();
    wire(mine);
  };
  window.addEventListener("popstate", () => swapWindow(location.href, false));

  // A click in nav1 or nav2 opens something in the columns and leaves the window as it is.
  const swapColumns = async (href) => {
    let next;
    try {
      next = await load(href);
    } catch {
      location.href = href;
      return;
    }
    const parts = [1, 2, 3].map((level) => {
      const selector = `.workspace-nav[data-level="${level}"]`;
      return [document.querySelector(selector), next.querySelector(selector)];
    });
    if (parts.some(([, theirs]) => !theirs)) {
      location.href = href;
      return;
    }
    // Only the content changes: each column keeps its width.
    for (const [mine, theirs] of parts) {
      mine.replaceChildren(...theirs.childNodes);
      mine.setAttribute("aria-label", theirs.getAttribute("aria-label") ?? "");
    }
    workspace()?.removeAttribute("data-nav-hidden");
    markOpen();
    markTheme();
    wire(document.querySelector('.workspace-nav[data-level="3"]'));
  };

  // The row a right click was made on, for the menu of the row.
  let row = null;
  document.addEventListener("contextmenu", (event) => {
    row = event.target.closest?.(".workspace-tree li > div") ?? row;
  }, true);

  document.addEventListener("click", (event) => {
    const modified = event.ctrlKey || event.metaKey || event.shiftKey || event.altKey;
    const name = event.target.closest('.workspace-nav[data-level="3"] .workspace-tree li > div > a');
    if (name) {
      // Ctrl, Shift or the middle button leave it to the browser: a tab of its own. A double click is C22's: it toggles.
      if (modified) {
        return;
      }
      event.preventDefault();
      if (event.detail < 2) {
        swapWindow(name.href, true);
      }
      return;
    }
    const columns = event.target.closest("a[data-ud-columns]");
    if (columns && !modified) {
      // The active view is C22's: a second click hides nav2 and nav3.
      if (columns.closest('.workspace-nav[data-level="1"]') && columns.hasAttribute("aria-current")) {
        return;
      }
      event.preventDefault();
      // Back to the workspace shows the collection of what the window holds.
      const back = columns.hasAttribute("data-ud-workspace") && /^\/(o|f)\//.test(location.pathname);
      swapColumns(back ? location.href : columns.href);
      return;
    }
    const action = event.target.closest("[data-ud-row]");
    if (action) {
      const link = row?.querySelector(":scope > a");
      if (link && action.dataset.udRow === "open") {
        swapWindow(link.href, true);
      }
      if (link && action.dataset.udRow === "copy") {
        navigator.clipboard?.writeText(link.href);
      }
      return;
    }
    const theme = event.target.closest("[data-ud-theme]");
    if (theme) {
      window.udSetTheme(theme.dataset.udTheme);
      markTheme();
      return;
    }
    if (event.target.closest("[data-ud-reset-columns]")) {
      const ws = workspace();
      const store = ws?.dataset.workspaceStore;
      try { if (store) localStorage.removeItem(store); } catch { /* nothing kept */ }
      ws?.querySelector('.workspace-nav[data-level="1"]')?.setAttribute("data-step", "0");
      ws?.querySelector('.workspace-nav[data-level="2"]')?.setAttribute("data-step", "3");
      ws?.style.removeProperty("--workspace-nav3");
    }
  });

  const markTheme = () => document.querySelectorAll("[data-ud-theme]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.udTheme === window.udTheme())));
  markTheme();

  // Search: "/" stays a normal character while someone is typing, Ctrl+K and Cmd+K work everywhere.
  document.addEventListener("keydown", (event) => {
    const typing = event.target.closest?.("input, textarea, select, [contenteditable]");
    const slash = event.key === "/" && !typing && !event.ctrlKey && !event.metaKey && !event.altKey;
    const combination = event.key.toLowerCase() === "k" && (event.ctrlKey || event.metaKey);
    if (!slash && !combination) {
      return;
    }
    event.preventDefault();
    workspace()?.removeAttribute("data-nav-hidden");
    const field = document.querySelector('input[name="q"]');
    field?.focus();
    field?.select();
  });
})();
