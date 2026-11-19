// SPDX-License-Identifier: MIT
// The time tracking (module Time, docs/time-tracking.md) is EightPerDay's own interface below /time/, shown in a frame
// of the same origin. The workspace draws its windows anew as a whole, which would load a frame anew each time and lose
// what is typed in it, so the frames live outside the windows: one per window, laid over the place its tab keeps free
// ([data-timeframe] in the window body), and hidden, not dropped, while no tab shows it.
window.UnderDocTime = (() => {
  const frames = new Map(); // window index → frame
  let watching = false;

  // The pages of the time tracking a list offers, with the path below /time.
  const pages = [
    { path: "/", name: "List", icon: "list" },
    { path: "/weeks", name: "Every week", icon: "list" },
    { path: "/week", name: "Week by mode", icon: "clock" },
    { path: "/all", name: "All employers", icon: "grid" },
    { path: "/reports", name: "Reports and export", icon: "table" },
    { path: "/orgs", name: "Organizations", icon: "user" },
  ];
  const leave = [{ path: "/leave", name: "Leave planning", icon: "sun" }];

  function frameFor(win, src) {
    let frame = frames.get(win);
    if (!frame) {
      frame = document.createElement("iframe");
      frame.title = "Time tracking";
      frame.dataset.timeFrame = win;
      frame.style.position = "fixed";
      frame.style.border = "0";
      frame.style.zIndex = "0";
      frame.src = src;
      frame.dataset.src = src;
      document.body.appendChild(frame);
      frames.set(win, frame);
    } else if (frame.dataset.src !== src) {
      frame.src = src;
      frame.dataset.src = src;
    }
    return frame;
  }

  // place lays every frame over its place, after each drawing of the workspace and while the layout moves.
  function place() {
    const zoom = parseFloat(document.documentElement.style.zoom || "1") || 1;
    const shown = new Set();
    document.querySelectorAll("[data-timeframe]").forEach((slot) => {
      const win = slot.dataset.win;
      const area = slot.closest(".scroll") || slot;
      const r = area.getBoundingClientRect();
      const frame = frameFor(win, slot.dataset.timeframe);
      frame.style.left = r.left / zoom + "px";
      frame.style.top = r.top / zoom + "px";
      frame.style.width = r.width / zoom + "px";
      frame.style.height = r.height / zoom + "px";
      frame.hidden = r.width === 0 || r.height === 0;
      shown.add(win);
    });
    frames.forEach((frame, win) => {
      if (!shown.has(win)) frame.hidden = true;
    });
    if (shown.size && !watching) {
      watching = true;
      requestAnimationFrame(follow);
    }
  }

  // follow keeps the frames in place while a column is dragged or the window resized, as long as one is shown.
  function follow() {
    if (!document.querySelector("[data-timeframe]")) {
      watching = false;
      frames.forEach((frame) => { frame.hidden = true; });
      return;
    }
    place();
    requestAnimationFrame(follow);
  }

  return { pages, leave, place };
})();
