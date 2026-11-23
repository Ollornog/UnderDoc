-- SPDX-License-Identifier: MIT
-- Page kinds with writing rules (T-20, ADR-33). A page names its kind, and the kind sets its audience: structure and
-- records (overview, register, record, plan, source), agent documentation and reference (reference, agent-notes), or
-- human documentation (how-to, explanation, tutorial). The empty kind is a prose page without one, which counts as human
-- documentation. Only pages have a kind; the names are those of package pagekind.

ALTER TABLE objects ADD COLUMN page_kind text NOT NULL DEFAULT '' CHECK (page_kind IN ('', 'overview', 'register', 'record',
    'plan', 'source', 'reference', 'agent-notes', 'how-to', 'explanation', 'tutorial'));
ALTER TABLE objects ADD CONSTRAINT objects_page_kind_of_pages CHECK (page_kind = '' OR kind = 'page');
