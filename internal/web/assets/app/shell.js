// SPDX-License-Identifier: MIT
// The behavior of the workspace shell (ADR-16, ADR-17). Inline handlers would need a weaker CSP, so all of it lives here.
(() => {
  const root = document.documentElement;
  const phone = () => window.matchMedia("(max-width: 760px)").matches;

  // --- Widths of the columns: drag the right edge, nav1 and nav2 snap to four steps, nav3 moves freely.
  const saveColumns = () => {
    const value = {
      s1: Number(root.dataset.udS1), s2: Number(root.dataset.udS2),
      w3: parseInt(getComputedStyle(root).getPropertyValue("--ud-w3"), 10),
    };
    try { localStorage.setItem("ud.columns", JSON.stringify(value)); } catch { /* lasts until reload */ }
  };
  const setStep = (column, step) => {
    step = Math.min(3, Math.max(0, step));
    root.dataset["udS" + column] = step;
    root.style.setProperty("--ud-w" + column, window.udStages[step] + "px");
  };
  const setFree = (width) => root.style.setProperty("--ud-w3", Math.round(Math.min(640, Math.max(180, width))) + "px");
  const nearest = (width) => window.udStages.reduce((best, w, i) => (Math.abs(w - width) < Math.abs(window.udStages[best] - width) ? i : best), 0);

  document.addEventListener("pointerdown", (event) => {
    const grip = event.target.closest("[data-ud-grip]");
    if (!grip || event.button !== 0) {
      return;
    }
    event.preventDefault();
    const column = grip.dataset.udGrip;
    const start = event.clientX;
    const width = grip.parentElement.getBoundingClientRect().width;
    root.classList.add("ud-resizing");
    const move = (m) => {
      const wanted = width + (m.clientX - start);
      if (column === "3") {
        setFree(wanted);
      } else {
        setStep(column, nearest(wanted));
      }
    };
    const up = () => {
      removeEventListener("pointermove", move);
      removeEventListener("pointerup", up);
      root.classList.remove("ud-resizing");
      saveColumns();
    };
    addEventListener("pointermove", move);
    addEventListener("pointerup", up);
  });
  document.addEventListener("keydown", (event) => {
    const grip = event.target.closest?.("[data-ud-grip]");
    if (!grip || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) {
      return;
    }
    event.preventDefault();
    const direction = event.key === "ArrowLeft" ? -1 : 1;
    const column = grip.dataset.udGrip;
    if (column === "3") {
      setFree(grip.parentElement.getBoundingClientRect().width + 20 * direction);
    } else {
      setStep(column, Number(root.dataset["udS" + column]) + direction);
    }
    saveColumns();
  });

  // --- The list in nav3: a click opens the object, the arrow or a double click shows or hides what is in it.
  const toggle = (item) => {
    const arrow = item.querySelector(":scope > .ud-row > [data-ud-toggle]");
    const list = item.querySelector(":scope > .ud-tree");
    if (!arrow || !list) {
      return;
    }
    const open = list.hidden;
    list.hidden = !open;
    arrow.setAttribute("aria-expanded", String(open));
  };

  // A click on a name in nav3 swaps only the window, so the list keeps what is open in it.
  const swap = async (href, push) => {
    let html;
    try {
      const response = await fetch(href, { headers: { Accept: "text/html" }, credentials: "same-origin" });
      if (!response.ok || !response.headers.get("Content-Type")?.startsWith("text/html")) {
        throw new Error(String(response.status));
      }
      html = await response.text();
    } catch {
      location.href = href;
      return;
    }
    const next = new DOMParser().parseFromString(html, "text/html");
    const main = next.querySelector("[data-ud-main]");
    const collection = next.querySelector(".ud-nav3 .ud-chead")?.getAttribute("href");
    if (!main || collection !== document.querySelector(".ud-nav3 .ud-chead")?.getAttribute("href")) {
      location.href = href;
      return;
    }
    document.querySelector("[data-ud-main]").replaceChildren(...main.childNodes);
    document.querySelector("[data-ud-tab]").replaceChildren(...next.querySelector("[data-ud-tab]").childNodes);
    document.title = next.title;
    const path = new URL(href, location.href).pathname;
    document.querySelectorAll(".ud-nav3 .ud-name").forEach((name) => {
      if (new URL(name.href).pathname === path) {
        name.setAttribute("aria-current", "page");
      } else {
        name.removeAttribute("aria-current");
      }
    });
    if (push) {
      history.pushState({ ud: true }, "", href);
    }
    if (phone()) {
      root.classList.remove("ud-navopen");
    }
  };
  window.addEventListener("popstate", () => swap(location.href, false));

  // A click in nav1 or nav2 opens something in the columns and leaves the window as it is (ADR-16).
  const swapColumns = async (href) => {
    let next;
    try {
      const response = await fetch(href, { headers: { Accept: "text/html" }, credentials: "same-origin" });
      if (!response.ok || !response.headers.get("Content-Type")?.startsWith("text/html")) {
        throw new Error(String(response.status));
      }
      next = new DOMParser().parseFromString(await response.text(), "text/html");
    } catch {
      location.href = href;
      return;
    }
    const parts = [".ud-nav1", ".ud-nav2", ".ud-nav3"].map((s) => [document.querySelector(s), next.querySelector(s)]);
    if (parts.some(([, n]) => !n)) {
      location.href = href;
      return;
    }
    for (const [mine, theirs] of parts) {
      mine.replaceChildren(...theirs.childNodes);
      mine.setAttribute("aria-label", theirs.getAttribute("aria-label") ?? "");
    }
    // What the window shows stays marked in the list.
    document.querySelectorAll(".ud-nav3 .ud-name").forEach((name) => {
      if (new URL(name.href).pathname === location.pathname) {
        name.setAttribute("aria-current", "page");
      } else {
        name.removeAttribute("aria-current");
      }
    });
    root.classList.remove("ud-navhide");
    markTheme();
  };

  document.addEventListener("click", (event) => {
    const arrow = event.target.closest("[data-ud-toggle]");
    if (arrow) {
      toggle(arrow.closest("li"));
      return;
    }
    const name = event.target.closest(".ud-nav3 .ud-name");
    if (name) {
      // Ctrl, Shift or the middle button leave it to the browser: a tab of its own.
      if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
      event.preventDefault();
      if (event.detail === 2) {
        toggle(name.closest("li"));
        return;
      }
      if (event.detail < 2) {
        swap(name.href, true);
      }
      return;
    }
    // A second click on the active view hides nav2 and nav3, the next one brings them back. On a phone they open over
    // the window instead.
    const view = event.target.closest(".ud-nav1 [data-ud-view]");
    if (view) {
      event.preventDefault();
      root.classList.toggle(phone() ? "ud-navopen" : "ud-navhide");
      return;
    }
    const columns = event.target.closest("a[data-ud-columns]");
    if (columns && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) {
      event.preventDefault();
      // Back to the workspace shows the collection of what the window holds.
      const back = columns.hasAttribute("data-ud-workspace") && /^\/(o|f)\//.test(location.pathname);
      swapColumns(back ? location.href : columns.href);
      return;
    }
    const gear = event.target.closest("[data-ud-gear]");
    if (gear) {
      const drawer = document.getElementById("ud-drawer");
      if (drawer) {
        drawer.hidden = !drawer.hidden;
        document.querySelectorAll(".ud-tools [data-ud-gear]").forEach((b) => b.setAttribute("aria-expanded", String(!drawer.hidden)));
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
      try { localStorage.removeItem("ud.columns"); } catch { /* nothing kept */ }
      setStep(1, 0);
      setStep(2, 3);
      setFree(280);
      return;
    }
    // A click outside closes the contents marker.
    document.querySelectorAll("details.ud-marker[open]").forEach((d) => {
      if (!d.contains(event.target)) {
        d.open = false;
      }
    });
  });

  const markTheme = () => document.querySelectorAll("[data-ud-theme]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.udTheme === window.udTheme())));
  markTheme();

  // --- Search: "/" stays a normal character while someone is typing, Ctrl+K and Cmd+K work everywhere.
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      document.querySelectorAll("details.ud-marker[open]").forEach((d) => { d.open = false; });
      const drawer = document.getElementById("ud-drawer");
      if (drawer && !drawer.hidden) {
        drawer.hidden = true;
        document.querySelectorAll(".ud-tools [data-ud-gear]").forEach((b) => b.setAttribute("aria-expanded", "false"));
      }
      return;
    }
    const typing = event.target.closest?.("input, textarea, select, [contenteditable]");
    const slash = event.key === "/" && !typing && !event.ctrlKey && !event.metaKey && !event.altKey;
    const combination = event.key.toLowerCase() === "k" && (event.ctrlKey || event.metaKey);
    if (!slash && !combination) {
      return;
    }
    event.preventDefault();
    root.classList.remove("ud-navhide");
    const field = document.querySelector('input[name="q"]');
    field?.focus();
    field?.select();
  });
})();
