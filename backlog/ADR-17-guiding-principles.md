---
id: ADR-17
type: Decision
title: Guiding principles for every feature
status: erledigt
tags: [process, ui, architecture]
created: 2026-11-07
---

## Context

Eight rounds of mockups (T-5) kept finding the same kind of flaw: two things that look alike reacted differently, or something
that had a place in one view had none in another. Fixing each case alone does not stop the next one. The maintainer asked for
principles that every feature is checked against before it is built.

## Options

- **Decide case by case:** quick for each feature, but the inconsistencies keep coming back.
- **A full style guide:** complete, but long and soon out of date.
- **A few principles, with ADR-16 as their worked examples:** short enough to check every feature against.

## Decision

The third option. These hold for every feature, and an exception needs a reason recorded in a decision.

1. **The same action gets the same reaction, everywhere.** A feature should be guessable from the ones the user already knows.
   - A click opens only this.
   - Ctrl with a click adds it, or takes it away again, and leaves the rest as it is. For a collection that means opening it as
     well or closing it, and for a document opening it as a tab of its own, as in a browser.
   - A middle click does what it does in a browser: it opens a tab of its own or closes a tab.
   - A click on anything with children opens the children in the list, and a second click closes them. This holds for folders,
     documents and collections alike.
   - A right click opens the menu of what is under the pointer, wherever that is.
2. **Everything is an object.** Collections, documents, folders, saved searches, values, snippets and lists are objects, and so
   are the parts of a document meant to be used elsewhere. Every object has a stable ID, opens through its own URL, can be embedded
   in other objects and can carry tags. [ADR-18](ADR-18-object-ids-and-addresses.md) settles the IDs.
3. **One source per fact, down to the part of a document.** A table or paragraph used in two places is embedded, never copied
   ([ADR-8](ADR-8-one-source-per-list.md), M-3).
4. **Phone and desktop are both first-class.** They share the objects, the URLs and the meaning of each action, but each gets a
   layout of its own instead of a squeezed copy of the other. A long press on a touch screen is the right click.
5. **Built later means greyed out, switched off means gone.** What a module will bring but is not built yet stays in sight,
   greyed out, so the shape of the whole stays visible. What an administrator switches off disappears for everyone (ADR-16).

## Consequences

- Every mockup round and every milestone talk checks its features against these principles first.
- ADR-16 records the rules of the workspace as worked examples of the first principle. Its click rule for collections is replaced
  by Ctrl with a click, and the setting for it goes.
- The private area does not fit the second principle yet. Its entries, such as to-dos and time records, may or may not be objects
  with IDs of their own. This stays open until M-24 (ADR-18).
- The phone needs mockups of its own before its layout is built (T-10).
