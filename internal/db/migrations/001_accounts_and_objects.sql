-- SPDX-License-Identifier: MIT
-- Accounts come from the sign-in gate (ADR-9). Objects are collections, folders and pages, each with an ID of ADR-18,
-- an owner and what all others may do (ADR-20). The place of an object is its parent and its position there.

CREATE TABLE accounts (
    id         text PRIMARY KEY,
    login      text NOT NULL,
    name       text NOT NULL,
    email      text NOT NULL DEFAULT '',
    groups     text[] NOT NULL DEFAULT '{}',
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE objects (
    id          text PRIMARY KEY CHECK (id ~ '^[0-9a-hjkmnp-tv-z]{10}$'),
    kind        text NOT NULL CHECK (kind IN ('collection', 'folder', 'page')),
    title       text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '',
    owner       text NOT NULL REFERENCES accounts (id),
    parent      text REFERENCES objects (id) ON DELETE RESTRICT,
    position    integer NOT NULL DEFAULT 0,
    -- What all others may do. NULL means inherited from the parent, and at the top it means edit.
    access      text CHECK (access IN ('edit', 'comment', 'read')),
    -- Only its owner sees a private object and everything below it, administrators included.
    private     boolean NOT NULL DEFAULT false,
    created     timestamptz NOT NULL DEFAULT now(),
    changed     timestamptz NOT NULL DEFAULT now(),
    CHECK (parent IS NULL OR parent <> id),
    -- Only a collection stands on its own. Everything else has a place.
    CHECK (kind = 'collection' OR parent IS NOT NULL)
);

CREATE INDEX objects_by_parent ON objects (parent, position, created);
CREATE INDEX objects_at_the_top ON objects (position, created) WHERE parent IS NULL;
