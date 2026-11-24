---
type: Bug
title: A redraw in the background can swallow a click
status: offen
points: 5
id: B-9
milestone: M-14
created: '2026-11-24'
---
# B-9 — A redraw in the background can swallow a click

Found while fixing flaky browser tests (#193, 2026-11-24). The workspace redraws by replacing #root with innerHTML. When a redraw falls between pressing and releasing the mouse, the element under the pointer is gone and the click is lost. The tests now wait for the redraw; real users still hit it, rarely (for example right after opening the settings, while the versions load).

Fix: a redraw that keeps existing elements (patch the DOM or render into the existing nodes) instead of replacing them, or defer a redraw while a pointer is pressed. Belongs to the workspace shell (M-14). Browser test with UNDERDOC_BROWSERTEST_JITTER_MS.
