-- SPDX-License-Identifier: MIT
-- The text of files for the search (ADR-24): it belongs to the content, so equal content is read once, and each way that
-- read it keeps its own text, so the index can say where a text came from. Each way is switched on by the hoster, the
-- tenant and the user in turn; a queue with limits does the work on the server.

CREATE TABLE file_texts (
    blob   text NOT NULL REFERENCES blobs (sha256) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN ('browser', 'server', 'ocr', 'ai')),
    text   text NOT NULL CHECK (octet_length(text) <= 1048576),
    author text REFERENCES accounts (id),
    at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blob, source)
);

-- Work for the server: read the text of a blob one way. Account is whose upload it was, for the quota of a model.
CREATE TABLE text_jobs (
    blob         text NOT NULL REFERENCES blobs (sha256) ON DELETE CASCADE,
    way          text NOT NULL CHECK (way IN ('server', 'ocr', 'ai')),
    account      text NOT NULL REFERENCES accounts (id),
    content_type text NOT NULL,
    tries        integer NOT NULL DEFAULT 0,
    due          timestamptz NOT NULL DEFAULT now(),
    failed       text NOT NULL DEFAULT '',
    PRIMARY KEY (blob, way)
);

CREATE INDEX text_jobs_due ON text_jobs (due) WHERE tries < 3;

-- Which ways the tenant (account '') and each user switch on; NULL takes it from above.
CREATE TABLE text_settings (
    account text PRIMARY KEY,
    browser boolean,
    server  boolean,
    ai      boolean
);

-- Pages a model for OCR read, per account and month, against the quotas of the user and the tenant.
CREATE TABLE ai_ocr_usage (
    account text NOT NULL REFERENCES accounts (id),
    month   date NOT NULL,
    pages   integer NOT NULL DEFAULT 0 CHECK (pages >= 0),
    PRIMARY KEY (account, month)
);
